// Package enroll is the page a person opens from an invite link to add themselves as a viewer.
//
// The owner makes an invite and sends its link. The person signs in with Plex, picks one of the Plex Media Servers
// they own, and Yagi allows that server's public addresses, as plex.tv lists them, with the limit the invite carries.
// It is a plain server-rendered page with no script, like the claim page, so its content security policy can allow
// nothing but its own style.
//
// The Plex token is used once, to ask who signed in and which servers they have, and then thrown away: it is in no
// variable that outlives a request, no log and no file. What the person picks is a server, never an address: the
// addresses come from plex.tv, kept here between the sign-in and the confirmation under a random ticket.
//
// The link works whoever opens it, so it is guarded like a password: only a hash of it is stored, a wrong or
// expired or revoked one is the same 404 as no invite at all, and wrong tries from an address are limited.
package enroll

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/johnschult/yagi/internal/access"
	"github.com/johnschult/yagi/internal/httpx"
	"github.com/johnschult/yagi/internal/plexauth"
)

// Path is where the invite pages live: Path + token, and under it start, wait and confirm.
const Path = "/invite/"

const (
	maxFailures  = 5                // wrong links allowed from one address...
	failWindow   = 10 * time.Minute // ...within this long
	startsPerMin = 6                // sign-ins one address may begin in a minute
	minPoll      = time.Second      // the most often one sign-in asks plex.tv
	maxPending   = 50               // sign-ins in progress at once, and servers waiting to be confirmed
	pendingLife  = 30 * time.Minute // how long a sign-in may take
	ticketLife   = 15 * time.Minute // how long to pick a server after signing in
)

// tokenRe is what a link's token looks like: base64url of 32 bytes. Anything else is not worth a lookup.
var tokenRe = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// Event is someone enrolling, or bringing their addresses up to date.
type Event struct {
	Viewer    string
	Account   string // the Plex account
	Server    string // the Plex server they picked
	Addresses []string
	Refreshed bool // an invite used before, rather than for the first time
}

// Config is what an Enroller needs.
type Config struct {
	// Settings returns the settings now in force.
	Settings func() access.Settings
	// Store is the only thing that writes them.
	Store *access.Store
	// Plex returns a client for plex.tv with this Yagi's identity.
	Plex func() *plexauth.Client
	// Enrolled hears of each enrollment. It may be nil.
	Enrolled func(Event)
	// Tuners returns the ports the tuners are on, so the last page can say where to point Plex. It may be nil.
	Tuners func() []string
	Log    *slog.Logger
	Now    func() time.Time // time.Now unless a test supplies another
}

// Enroller serves the invite pages.
type Enroller struct {
	cfg Config
	log *slog.Logger

	confirmMu sync.Mutex // one confirmation at a time, so two can't both take the same ticket

	mu      sync.Mutex
	pending map[string]*signIn     // sign-ins in progress, by handle
	tickets map[string]*ticket     // servers waiting to be picked, by ticket
	fails   map[string][]time.Time // wrong links, by client address
	starts  map[string][]time.Time // sign-ins begun, by client address
}

// signIn is a Plex sign-in in progress for one invite.
type signIn struct {
	invite   string // the invite's ID
	pinID    int64
	code     string
	authURL  string // where the person goes to sign in, so the page can offer it again
	expires  time.Time
	lastPoll time.Time
	// ticket, once the person has signed in, is where their servers are kept; noServer says there were none to offer.
	// The sign-in stays known until it expires, so going back to the page or reloading it shows the same thing and
	// does not ask plex.tv again.
	ticket   string
	noServer string
}

// ticket is who signed in and the servers they own, kept until they pick one.
type ticket struct {
	invite  string
	user    plexauth.User
	servers []plexauth.Server
	expires time.Time
}

// New makes an Enroller.
func New(cfg Config) *Enroller {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Enroller{
		cfg: cfg, log: cfg.Log, pending: map[string]*signIn{}, tickets: map[string]*ticket{},
		fails: map[string][]time.Time{}, starts: map[string][]time.Time{},
	}
}

func randomID() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic("enroll: no randomness: " + err.Error()) // nothing safe to do without it
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Handler serves everything under Path. It is public by design (the person has no account here), so each step
// checks the link for itself.
func (e *Enroller) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, step, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, Path), "/")
		if !strings.HasPrefix(r.URL.Path, Path) || !tokenRe.MatchString(token) {
			notFound(w)
			return
		}
		switch {
		case step == "" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
			e.landing(w, r, token)
		case step == "start" && ok && r.Method == http.MethodPost:
			e.start(w, r, token)
		case step == "wait" && ok && (r.Method == http.MethodGet || r.Method == http.MethodHead):
			e.wait(w, r, token)
		case step == "confirm" && ok && r.Method == http.MethodPost:
			e.confirm(w, r, token)
		case step == "" || step == "start" || step == "wait" || step == "confirm":
			w.Header().Set("Allow", "GET, HEAD, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		default:
			notFound(w)
		}
	})
}

func notFound(w http.ResponseWriter) { http.Error(w, "not found", http.StatusNotFound) }

// invite finds the invite a request's link is for, or answers for it and says so. A wrong, expired or revoked link,
// and a link while enrolling is off, are all the same 404, so the page says nothing about which invites exist.
func (e *Enroller) invite(w http.ResponseWriter, r *http.Request, token string) (access.Invite, access.Settings, bool) {
	addr, ok := access.ClientAddr(r)
	if !ok {
		notFound(w)
		return access.Invite{}, access.Settings{}, false
	}
	ip := addr.String()
	now := e.cfg.Now()

	e.mu.Lock()
	recent := e.fails[ip][:0:0]
	for _, t := range e.fails[ip] {
		if now.Sub(t) < failWindow {
			recent = append(recent, t)
		}
	}
	e.fails[ip] = recent
	if len(recent) >= maxFailures {
		e.mu.Unlock()
		http.Error(w, "too many wrong tries; wait a few minutes", http.StatusTooManyRequests)
		return access.Invite{}, access.Settings{}, false
	}
	e.mu.Unlock()

	set := e.cfg.Settings()
	if !set.EnrollmentEnabled() {
		notFound(w)
		return access.Invite{}, set, false
	}
	inv, found := set.InviteForToken(token)
	if found && inv.Used() {
		found = indexViewer(set.Viewers, inv.ViewerName) >= 0 // a viewer the owner has since removed is not coming back
	}
	if !found || !inv.Live(now) {
		e.mu.Lock()
		if len(e.fails) > 1000 { // a flood from many addresses: start counting afresh
			e.fails = map[string][]time.Time{}
		}
		e.fails[ip] = append(e.fails[ip], now)
		e.mu.Unlock()
		notFound(w)
		return access.Invite{}, set, false
	}
	return inv, set, true
}

func indexViewer(vs []access.Viewer, name string) int {
	for i, v := range vs {
		if strings.EqualFold(v.Name, name) {
			return i
		}
	}
	return -1
}

func (e *Enroller) landing(w http.ResponseWriter, r *http.Request, token string) {
	inv, _, ok := e.invite(w, r, token)
	if !ok {
		return
	}
	lead := fmt.Sprintf(`You have been invited to join Yagi, a shared TV tuner, as <b>%s</b>.`, esc(inv.Name))
	button := "Sign in with"
	if inv.Used() {
		lead = fmt.Sprintf(`You joined as <b>%s</b> already. Sign in again to bring your server's address up to date.`, esc(inv.Name))
		button = "Update with"
	}
	page(w, http.StatusOK, "yagi", "Join Yagi", fmt.Sprintf(`<p>%s</p>
<p>Sign in with Plex and pick your Plex Media Server. Yagi will allow your server's internet address to use the tuner. Your Plex password and sign-in are not kept.</p>
<form method="post" action="%s">%s</form>`, lead, esc(Path+token+"/start"), plexButton(button)), "")
}

// allowStart limits how often one address may begin a sign-in.
func (e *Enroller) allowStart(ip string, now time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.starts) > 1000 {
		e.starts = map[string][]time.Time{}
	}
	recent := e.starts[ip][:0:0]
	for _, t := range e.starts[ip] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= startsPerMin {
		e.starts[ip] = recent
		return false
	}
	e.starts[ip] = append(recent, now)
	return true
}

func (e *Enroller) start(w http.ResponseWriter, r *http.Request, token string) {
	if httpx.CrossOriginForm(r) {
		http.Error(w, "cross-origin requests are not allowed", http.StatusForbidden)
		return
	}
	inv, _, ok := e.invite(w, r, token)
	if !ok {
		return
	}
	addr, _ := access.ClientAddr(r)
	now := e.cfg.Now()
	if !e.allowStart(addr.String(), now) {
		http.Error(w, "too many sign-in attempts; wait a minute", http.StatusTooManyRequests)
		return
	}
	c := e.cfg.Plex()
	pin, err := c.NewPIN(r.Context())
	if err != nil {
		e.log.Warn("an invite's Plex sign-in could not start", "err", err)
		page(w, http.StatusBadGateway, "triangle-alert", "Couldn't reach Plex", `<p>Yagi couldn't reach plex.tv to start the sign-in. Try again in a minute.</p>`, "")
		return
	}
	expires := pin.ExpiresAt
	if expires.IsZero() || expires.After(now.Add(pendingLife)) {
		expires = now.Add(pendingLife)
	}
	handle := randomID()
	wait := Path + token + "/wait?h=" + url.QueryEscape(handle)
	// Plex sends the person back to the wait page when they have signed in, so they needn't come back by hand.
	authURL := c.AuthURLReturning(pin, baseURL(r)+wait)
	e.mu.Lock()
	e.makeRoom(now)
	e.pending[handle] = &signIn{invite: inv.ID, pinID: pin.ID, code: pin.Code, authURL: authURL, expires: expires}
	e.mu.Unlock()
	e.waitingPage(w, http.StatusOK, authURL, wait, false)
}

// baseURL is the address the person reached this page at, which is the one they can be sent back to.
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// waitingPage is the page with the Plex button on it, and the way to carry on if Plex didn't send the person back.
// It never refreshes itself: it is there to be read and pressed.
func (e *Enroller) waitingPage(w http.ResponseWriter, status int, authURL, wait string, checked bool) {
	lead := `<p>Press the button to sign in with Plex. When you are done, Plex brings you back here.</p>`
	if checked {
		lead = `<p>Plex has not confirmed a sign-in yet. Press the button to sign in with Plex.</p>`
	}
	page(w, status, "yagi", "Sign in with Plex", lead+
		`<p>`+plexLink("Continue with", authURL)+`</p>`+
		`<p class="note">Already signed in, and not brought back? <a href="`+esc(wait)+`">Continue here</a>.</p>`, "")
}

// makeRoom drops what has expired, and the sign-ins and tickets that would be too many. The caller holds mu.
func (e *Enroller) makeRoom(now time.Time) {
	for k, v := range e.pending {
		if now.After(v.expires) {
			delete(e.pending, k)
		}
	}
	for k, v := range e.tickets {
		if now.After(v.expires) {
			delete(e.tickets, k)
		}
	}
	for len(e.pending) >= maxPending {
		var victim string
		for k, v := range e.pending {
			if victim == "" || v.expires.Before(e.pending[victim].expires) {
				victim = k
			}
		}
		delete(e.pending, victim)
	}
	for len(e.tickets) >= maxPending {
		var victim string
		for k, v := range e.tickets {
			if victim == "" || v.expires.Before(e.tickets[victim].expires) {
				victim = k
			}
		}
		delete(e.tickets, victim)
	}
}

func (e *Enroller) wait(w http.ResponseWriter, r *http.Request, token string) {
	inv, _, ok := e.invite(w, r, token)
	if !ok {
		return
	}
	handle := r.URL.Query().Get("h")
	now := e.cfg.Now()
	retry := Path + token
	again := `<p><a href="` + esc(retry) + `">Start again</a></p>`

	e.mu.Lock()
	p, found := e.pending[handle]
	if !found || p.invite != inv.ID || now.After(p.expires) {
		delete(e.pending, handle)
		e.mu.Unlock()
		page(w, http.StatusOK, "triangle-alert", "Sign-in expired", `<p>That sign-in ran out of time.</p>`+again, "")
		return
	}
	wait := Path + token + "/wait?h=" + url.QueryEscape(handle)
	authURL := p.authURL
	// Already signed in: the same answer again, without asking plex.tv.
	if p.noServer != "" {
		name := p.noServer
		e.mu.Unlock()
		e.noServerPage(w, name, retry)
		return
	}
	if p.ticket != "" {
		t, have := e.tickets[p.ticket]
		tid := p.ticket
		e.mu.Unlock()
		if !have || now.After(t.expires) {
			page(w, http.StatusOK, "triangle-alert", "Sign-in expired", `<p>That sign-in is finished or ran out of time.</p>`+again, "")
			return
		}
		e.chooserPage(w, r, token, inv, t, tid)
		return
	}
	if now.Sub(p.lastPoll) < minPoll {
		e.mu.Unlock()
		e.waitingPage(w, http.StatusOK, authURL, wait, true)
		return
	}
	p.lastPoll = now
	pinID, code := p.pinID, p.code
	e.mu.Unlock()

	c := e.cfg.Plex()
	tok, err := c.CheckPIN(r.Context(), pinID, code)
	switch {
	case errors.Is(err, plexauth.ErrExpired):
		e.forget(handle)
		page(w, http.StatusOK, "triangle-alert", "Sign-in expired", `<p>That sign-in ran out of time.</p>`+again, "")
		return
	case err != nil:
		e.log.Warn("an invite's Plex sign-in could not be checked", "err", err)
		page(w, http.StatusBadGateway, "triangle-alert", "Couldn't reach Plex", `<p>Yagi couldn't reach plex.tv. <a href="`+esc(wait)+`">Try again</a>.</p>`, "")
		return
	case tok == "":
		e.waitingPage(w, http.StatusOK, authURL, wait, true)
		return
	}
	// The token is used for the next two questions and goes no further than this function.
	user, servers, err := e.whoAndWhat(r.Context(), c, tok)
	if err != nil {
		e.log.Warn("an invite's Plex account could not be read", "err", err)
		page(w, http.StatusBadGateway, "triangle-alert", "Couldn't reach Plex", `<p>Yagi couldn't read your Plex account. <a href="`+esc(wait)+`">Try again</a>.</p>`, "")
		return
	}

	owned := ownedWithPublicAddresses(servers)
	if len(owned) == 0 {
		e.mu.Lock()
		p.noServer = user.Name
		e.mu.Unlock()
		e.noServerPage(w, user.Name, retry)
		return
	}
	tid := randomID()
	t := &ticket{invite: inv.ID, user: user, servers: owned, expires: now.Add(ticketLife)}
	e.mu.Lock()
	e.makeRoom(now)
	e.tickets[tid] = t
	p.ticket = tid
	e.mu.Unlock()
	e.chooserPage(w, r, token, inv, t, tid)
}

func (e *Enroller) noServerPage(w http.ResponseWriter, name, retry string) {
	msg := `<p>Signed in as <b>` + esc(name) + `</b>, but Yagi found no Plex Media Server of yours that Plex lists a public address for.</p>
<p>Make sure your server is on and that <i>Remote Access</i> is on in its settings, then <a href="` + esc(retry) + `">try again</a>. If it is, ask the person who invited you to add you by hand.</p>`
	page(w, http.StatusOK, "server-off", "No server found", msg, "")
}

// chooserPage offers the servers of the account that signed in, to pick one.
func (e *Enroller) chooserPage(w http.ResponseWriter, r *http.Request, token string, inv access.Invite, t *ticket, tid string) {
	addr, _ := access.ClientAddr(r)
	var rows strings.Builder
	for i, s := range t.servers {
		checked := ""
		if i == 0 {
			checked = " checked"
		}
		var addrs []string
		for _, a := range publicAddrs(s) {
			addrs = append(addrs, esc(a))
		}
		fmt.Fprintf(&rows, `<label class="row"><input type="radio" name="server" value="%s"%s> <b>%s</b><br><span class="addr">%s</span></label>`,
			esc(s.MachineID), checked, esc(s.Name), strings.Join(addrs, ", "))
	}
	page(w, http.StatusOK, "yagi", "Pick your server", fmt.Sprintf(`<p>Signed in as <b>%s</b>. Which Plex server should be allowed to use Yagi as <b>%s</b>?</p>
<form method="post" action="%s">
<input type="hidden" name="ticket" value="%s">
%s
<p class="note">Only the addresses listed will be allowed. This page saw you connecting from <code>%s</code>.</p>
<button type="submit">Allow this server</button>
</form>`, esc(t.user.Name), esc(inv.Name), esc(Path+token+"/confirm"), esc(tid), rows.String(), esc(addr.String())), "")
}

// whoAndWhat asks plex.tv who the token belongs to and which servers it has, within the time a request has.
func (e *Enroller) whoAndWhat(ctx context.Context, c *plexauth.Client, token string) (plexauth.User, []plexauth.Server, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	user, err := c.User(ctx, token)
	if err != nil {
		return plexauth.User{}, nil, err
	}
	servers, err := c.Resources(ctx, token)
	if err != nil {
		return plexauth.User{}, nil, err
	}
	return user, servers, nil
}

func (e *Enroller) forget(handle string) {
	e.mu.Lock()
	delete(e.pending, handle)
	e.mu.Unlock()
}

// publicAddrs is the addresses of a server worth allowing: reached over the internet, not Plex's relay or the
// server's own network, and not one that is only private.
func publicAddrs(s plexauth.Server) []string {
	var out []string
	seen := map[netip.Addr]bool{}
	for _, c := range s.Connections {
		if c.Local || !access.PublicAddr(c.Addr) || seen[c.Addr] {
			continue
		}
		seen[c.Addr] = true
		out = append(out, c.Addr.String())
	}
	sort.Strings(out)
	return out
}

// ownedWithPublicAddresses keeps the servers the account owns and has a public address for. A server that is only
// shared with the account is somebody else's, and its address says nothing about this person.
func ownedWithPublicAddresses(all []plexauth.Server) []plexauth.Server {
	var out []plexauth.Server
	for _, s := range all {
		if s.Owned && len(publicAddrs(s)) > 0 {
			out = append(out, s)
		}
	}
	return out
}

// outcome of the save: what the person is told if it didn't go through.
var (
	errGone         = errors.New("gone")
	errNameTaken    = errors.New("name taken")
	errOtherAccount = errors.New("another account")
	errAddrTaken    = errors.New("address taken")
)

func (e *Enroller) confirm(w http.ResponseWriter, r *http.Request, token string) {
	if httpx.CrossOriginForm(r) {
		http.Error(w, "cross-origin requests are not allowed", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	inv, _, ok := e.invite(w, r, token)
	if !ok {
		return
	}
	now := e.cfg.Now()
	retry := Path + token
	again := `<p><a href="` + esc(retry) + `">Start again</a></p>`

	e.confirmMu.Lock()
	defer e.confirmMu.Unlock()

	e.mu.Lock()
	t, found := e.tickets[r.PostForm.Get("ticket")]
	if !found || t.invite != inv.ID || now.After(t.expires) {
		delete(e.tickets, r.PostForm.Get("ticket"))
		e.mu.Unlock()
		page(w, http.StatusOK, "triangle-alert", "Sign-in expired", `<p>That took too long, so it was forgotten.</p>`+again, "")
		return
	}
	e.mu.Unlock()

	var srv *plexauth.Server
	for i := range t.servers {
		if t.servers[i].MachineID == r.PostForm.Get("server") {
			srv = &t.servers[i]
		}
	}
	if srv == nil {
		page(w, http.StatusBadRequest, "server", "Pick a server", `<p>Pick one of your servers.</p>`+again, "")
		return
	}
	var addrs []netip.Addr
	for _, a := range publicAddrs(*srv) {
		addrs = append(addrs, netip.MustParseAddr(a))
	}

	var ev Event
	_, err := e.cfg.Store.Edit(func(s *access.Settings) error {
		i := -1
		for j, cur := range s.Invites {
			if cur.ID == inv.ID {
				i = j
			}
		}
		if i < 0 || !s.Invites[i].Live(now) {
			return errGone
		}
		cur := s.Invites[i]
		for _, a := range addrs {
			if _, taken := s.ViewerHolding(a, cur.ViewerName); taken {
				return errAddrTaken
			}
		}
		strs := make([]string, len(addrs))
		for j, a := range addrs {
			strs[j] = a.String()
		}
		plex := &access.ViewerPlex{AccountID: t.user.ID, AccountName: t.user.Name, ServerID: srv.MachineID, ServerName: srv.Name}
		ev = Event{Account: t.user.Name, Server: srv.Name, Addresses: strs}
		if cur.Used() {
			vi := indexViewer(s.Viewers, cur.ViewerName)
			if vi < 0 {
				return errGone
			}
			if v := s.Viewers[vi]; v.Plex == nil || v.Plex.AccountID != t.user.ID {
				return errOtherAccount
			}
			s.Viewers[vi].Addresses = strs
			s.Viewers[vi].Plex = plex
			ev.Viewer, ev.Refreshed = s.Viewers[vi].Name, true
			return nil
		}
		if indexViewer(s.Viewers, cur.Name) >= 0 {
			return errNameTaken
		}
		limits := map[string]int{}
		for _, d := range s.Devices {
			if n, ok := cur.Limits[d.Name]; ok {
				limits[d.Name] = n
			}
		}
		if len(limits) == 0 {
			limits = nil
		}
		s.Viewers = append(s.Viewers, access.Viewer{Name: cur.Name, Addresses: strs, Limit: cur.Limit, Limits: limits, Plex: plex})
		s.Invites[i].ViewerName = cur.Name
		ev.Viewer = cur.Name
		return nil
	})
	if err != nil {
		var ve *access.ValidationError
		switch {
		case errors.Is(err, errGone):
			notFound(w)
		case errors.Is(err, errAddrTaken):
			page(w, http.StatusConflict, "circle-x", "Already in use", `<p>One of your server's addresses is already allowed for someone else, so it can't be added here. Ask the person who invited you.</p>`, "")
		case errors.Is(err, errNameTaken):
			page(w, http.StatusConflict, "circle-x", "Name in use", `<p>A viewer with that name already exists. Ask the person who invited you.</p>`, "")
		case errors.Is(err, errOtherAccount):
			page(w, http.StatusConflict, "circle-x", "Already used", `<p>This invite was used by a different Plex account, so it can't be used to change it. Ask the person who invited you.</p>`, "")
		case errors.Is(err, access.ErrNoFile):
			page(w, http.StatusServiceUnavailable, "triangle-alert", "Can't save", `<p>This Yagi can't save changes right now. Ask the person who invited you.</p>`, "")
		case errors.As(err, &ve):
			page(w, http.StatusConflict, "circle-x", "Can't add", `<p>Yagi couldn't add you with these addresses. Ask the person who invited you.</p>`, "")
		default:
			e.log.Error("could not save an enrollment", "err", err)
			page(w, http.StatusInternalServerError, "triangle-alert", "Can't save", `<p>Something went wrong saving that. Try again, or ask the person who invited you.</p>`, "")
		}
		return
	}
	e.mu.Lock()
	delete(e.tickets, r.PostForm.Get("ticket"))
	e.mu.Unlock()

	if ev.Refreshed {
		e.log.Info("a viewer brought their addresses up to date with Plex", "viewer", ev.Viewer, "account", ev.Account, "server", ev.Server, "addresses", len(ev.Addresses))
	} else {
		e.log.Info("a viewer enrolled with Plex", "viewer", ev.Viewer, "account", ev.Account, "server", ev.Server, "addresses", len(ev.Addresses))
	}
	if e.cfg.Enrolled != nil {
		e.cfg.Enrolled(ev)
	}
	done := `<p>You are in, as <b>` + esc(ev.Viewer) + `</b>. Your server <b>` + esc(ev.Server) + `</b> can use the tuner now.</p>` +
		e.nextSteps(r) +
		`<p class="note">If your internet address changes and it stops working, open your invite link again to update it.</p>
<p>That is everything here. You can close this window.</p>`
	title := "You're in"
	if ev.Refreshed {
		title = "Updated"
		done = `<p><b>` + esc(ev.Server) + `</b> is allowed from its current address.</p>
<p>Nothing else to do. You can close this window.</p>`
	}
	page(w, http.StatusOK, "circle-check", title, done, "")
}

// nextSteps says what to do in Plex now: where the tuner is, as this page was reached, since the machine is the same.
// Where Yagi can't say, it sends the person to whoever invited them.
func (e *Enroller) nextSteps(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = strings.Trim(r.Host, "[]")
	}
	var addrs []string
	if e.cfg.Tuners != nil {
		for _, port := range e.cfg.Tuners() {
			addrs = append(addrs, "<code>"+esc(net.JoinHostPort(host, port))+"</code>")
		}
	}
	where := `Ask the person who invited you for the tuner's address.`
	if len(addrs) > 0 && host != "" {
		where = `Enter ` + strings.Join(addrs, " or ") + `.`
	}
	return `<p><b>Now, in Plex:</b> (Live TV needs Plex Pass)</p>
<ol class="steps"><li>Open <i>Settings &rarr; Live TV &amp; DVR &rarr; Set Up Plex DVR</i>.</li>
<li>Click <i>Don't see your HDHomeRun device? Enter its network address manually</i>. ` + where + `</li>
<li>Scan for channels and match the guide.</li></ol>`
}

func esc(s string) string { return html.EscapeString(s) }
