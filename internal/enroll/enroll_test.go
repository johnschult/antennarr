package enroll

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/access"
	"github.com/johnschult/yagi/internal/plexauth"
)

// fakePlex stands in for plex.tv: pins that a test approves, and the accounts and servers behind each token.
type fakePlex struct {
	*httptest.Server
	mu        sync.Mutex
	next      int64
	pins      map[int64]string // pin id -> token, once approved ("" until then)
	gone      map[int64]bool
	users     map[string]plexauth.User
	resources map[string][]map[string]any
	down      bool
	checks    int // how many times a pin was asked about
}

func newFakePlex(t *testing.T) *fakePlex {
	t.Helper()
	f := &fakePlex{next: 500, pins: map[int64]string{}, gone: map[int64]bool{}, users: map[string]plexauth.User{}, resources: map[string][]map[string]any{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *fakePlex) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		http.Error(w, "down", http.StatusInternalServerError)
		return
	}
	if r.Header.Get("X-Plex-Client-Identifier") == "" || r.Header.Get("X-Plex-Product") != "Yagi" {
		http.Error(w, "headers", http.StatusBadRequest)
		return
	}
	tok := r.Header.Get("X-Plex-Token")
	switch {
	case r.Method == "POST" && r.URL.Path == "/api/v2/pins":
		f.next++
		f.pins[f.next] = ""
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": f.next, "code": fmt.Sprintf("code%d-%s", f.next, strings.Repeat("x", 18)), "authToken": nil, "expiresAt": time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)})
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v2/pins/"):
		f.checks++
		id, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/v2/pins/"), 10, 64)
		token, ok := f.pins[id]
		if !ok || f.gone[id] {
			http.Error(w, "gone", http.StatusNotFound)
			return
		}
		var out any
		if token != "" {
			out = token
		}
		json.NewEncoder(w).Encode(map[string]any{"id": id, "code": "c", "authToken": out})
	case r.Method == "GET" && r.URL.Path == "/api/v2/user":
		u, ok := f.users[tok]
		if !ok {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": u.ID, "username": u.Name})
	case r.Method == "GET" && r.URL.Path == "/api/v2/resources":
		if _, ok := f.users[tok]; !ok {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(f.resources[tok])
	default:
		http.NotFound(w, r)
	}
}

// approve plays the person signing in on plex.tv: the latest pin now carries a token for this account and servers.
func (f *fakePlex) approve(token string, u plexauth.User, servers ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pins[f.next] = token
	f.users[token] = u
	f.resources[token] = servers
}

func (f *fakePlex) set(fn func()) { f.mu.Lock(); fn(); f.mu.Unlock() }

func plexServer(name, id string, owned bool, conns ...map[string]any) map[string]any {
	return map[string]any{"name": name, "clientIdentifier": id, "provides": "server", "owned": owned, "accessToken": "server-token-" + id, "connections": conns}
}

func plexConn(addr string, local, relay bool) map[string]any {
	return map[string]any{"address": addr, "port": 32400, "local": local, "relay": relay}
}

// rig is an Enroller on a real settings store, with a fake plex.tv, a clock a test can move, and a log to read.
type rig struct {
	t      *testing.T
	e      *Enroller
	h      http.Handler
	store  *access.Store
	pol    *access.Policy
	plex   *fakePlex
	path   string
	logs   *bytes.Buffer
	now    time.Time
	events []Event
	tokens map[string]string // invite name -> the token for its link
}

var t0 = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

// newRig sets up a Yagi with enrollment on (and nobody signing in with Plex, which has nothing to do with it), a viewer called Existing, and an invite for each name
// given (with a limit of 2). file false gives a Store with nowhere to save.
func newRig(t *testing.T, file bool, invites ...string) *rig {
	t.Helper()
	r := &rig{t: t, plex: newFakePlex(t), logs: &bytes.Buffer{}, now: t0, tokens: map[string]string{}}
	s := access.DefaultSettings()
	s.Plex = &access.PlexSettings{Enrollment: true}
	s.Viewers = []access.Viewer{{Name: "Existing", Addresses: []string{"198.51.100.20"}, Limit: 1}}
	for _, name := range invites {
		tok, inv := access.NewInvite(name, 2, nil, t0)
		s.Invites = append(s.Invites, inv)
		r.tokens[name] = tok
	}
	log := slog.New(slog.NewTextHandler(r.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	pol, err := access.NewPolicy(s, log)
	if err != nil {
		t.Fatal(err)
	}
	r.pol = pol
	if file {
		r.path = filepath.Join(t.TempDir(), "yagi.json")
		if err := access.SaveSettings(r.path, s); err != nil {
			t.Fatal(err)
		}
	}
	r.store = access.NewStore(r.path, pol)
	r.e = New(Config{
		Settings: pol.Settings, Store: r.store, Log: log, Now: func() time.Time { return r.now },
		Plex:     func() *plexauth.Client { return &plexauth.Client{Base: r.plex.URL, ClientID: "yagi-test-client"} },
		Enrolled: func(ev Event) { r.events = append(r.events, ev) },
		Tuners:   func() []string { return []string{"5004", "5005"} },
	})
	r.h = r.e.Handler()
	return r
}

// do makes a request as a client at remote, with the page's own origin unless told otherwise.
func (r *rig) do(method, path, remote string, form url.Values, origin ...string) (int, string, http.Header) {
	r.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	req.RemoteAddr = remote + ":5555"
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if method == "POST" {
		req.Header.Set("Origin", "http://"+req.Host)
	}
	if len(origin) > 0 {
		req.Header.Set("Origin", origin[0])
	}
	rec := httptest.NewRecorder()
	r.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String(), rec.Header()
}

const friend = "203.0.113.77" // where the person opens the page from

func (r *rig) link(name string) string { return Path + r.tokens[name] }

// signIn runs the person through the start step and returns the wait address.
func (r *rig) start(name string) (code int, body, wait string) {
	r.t.Helper()
	code, body, _ = r.do("POST", r.link(name)+"/start", friend, url.Values{})
	m := regexp.MustCompile(`href="(/invite/[^"]+/wait\?h=[^"]+)"`).FindStringSubmatch(body)
	if m != nil {
		wait = html2text(m[1])
	}
	return
}

func html2text(s string) string {
	return strings.NewReplacer("&amp;", "&", "&#34;", `"`, "&#39;", "'").Replace(s)
}

// signInAs approves the pending sign-in as this account and fetches the page that offers its servers.
func (r *rig) signInAs(name, token string, u plexauth.User, servers ...map[string]any) (string, string) {
	r.t.Helper()
	_, _, wait := r.start(name)
	if wait == "" {
		r.t.Fatal("no wait address in the start page")
	}
	r.plex.approve(token, u, servers...)
	r.now = r.now.Add(5 * time.Second)
	code, body, _ := r.do("GET", wait, friend, nil)
	if code != 200 {
		r.t.Fatalf("wait = %d %s", code, body)
	}
	return body, wait
}

func ticketAndServer(t *testing.T, body string) (ticket, server string) {
	t.Helper()
	tm := regexp.MustCompile(`name="ticket" value="([^"]+)"`).FindStringSubmatch(body)
	sm := regexp.MustCompile(`name="server" value="([^"]+)"`).FindStringSubmatch(body)
	if tm == nil || sm == nil {
		t.Fatalf("no ticket and server in the page:\n%s", body)
	}
	return tm[1], sm[1]
}

func (r *rig) settings() access.Settings { return r.pol.Settings() }

func (r *rig) viewer(name string) (access.Viewer, bool) {
	for _, v := range r.settings().Viewers {
		if strings.EqualFold(v.Name, name) {
			return v, true
		}
	}
	return access.Viewer{}, false
}

var mom = plexauth.User{ID: 7, Name: "mom"}

// home is a typical account: a server of hers with a local and a public address, a relay, and a name for each.
func home() map[string]any {
	return plexServer("Mom's Plex", "mid-mom", true,
		plexConn("192.168.1.5", true, false), plexConn("203.0.113.50", false, false), plexConn("203.0.113.51", false, true))
}

func TestWholeEnrollment(t *testing.T) {
	r := newRig(t, true, "Mom")

	code, body, hdr := r.do("GET", r.link("Mom"), friend, nil)
	if code != 200 || !strings.Contains(body, "Mom") || !strings.Contains(body, "Sign in with Plex") {
		t.Fatalf("landing = %d %s", code, body)
	}
	if csp := hdr.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "script-src") || hdr.Get("Cache-Control") != "no-store" || hdr.Get("Referrer-Policy") != "same-origin" {
		t.Errorf("headers: csp %q cache %q referrer %q", csp, hdr.Get("Cache-Control"), hdr.Get("Referrer-Policy"))
	}

	code, body, wait := r.start("Mom")
	if code != 200 || !strings.Contains(body, "https://app.plex.tv/auth") || !strings.Contains(body, `class="button plex"`) || wait == "" {
		t.Fatalf("start = %d %s", code, body)
	}
	// Nothing yet: still waiting.
	r.now = r.now.Add(5 * time.Second)
	if code, body, _ := r.do("GET", wait, friend, nil); code != 200 || !strings.Contains(body, "has not confirmed") {
		t.Fatalf("before signing in = %d %s", code, body)
	}
	// The person signs in on plex.tv: now the page offers the servers they own, with public addresses only.
	r.plex.approve("tok-secret-plex-token", mom, home(), plexServer("Shared with Mom", "mid-other", false, plexConn("198.51.100.99", false, false)),
		plexServer("Lan only", "mid-lan", true, plexConn("192.168.1.9", true, false), plexConn("100.64.0.9", false, false)))
	r.now = r.now.Add(5 * time.Second)
	code, body, _ = r.do("GET", wait, friend, nil)
	if code != 200 || !strings.Contains(body, "Mom&#39;s Plex") || !strings.Contains(body, "203.0.113.50") {
		t.Fatalf("servers = %d %s", code, body)
	}
	for _, not := range []string{"192.168.1.5", "203.0.113.51", "Shared with Mom", "198.51.100.99", "Lan only", "100.64.0.9", "server-token", "tok-secret"} {
		if strings.Contains(body, not) {
			t.Errorf("the page offers or shows %q:\n%s", not, body)
		}
	}
	ticket, server := ticketAndServer(t, body)
	if server != "mid-mom" {
		t.Errorf("server = %q", server)
	}

	code, body, _ = r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}})
	if code != 200 || !strings.Contains(body, "You are in") {
		t.Fatalf("confirm = %d %s", code, body)
	}
	v, ok := r.viewer("Mom")
	if !ok {
		t.Fatal("no viewer was made")
	}
	if len(v.Addresses) != 1 || v.Addresses[0] != "203.0.113.50" || v.Limit != 2 {
		t.Errorf("viewer = %+v", v)
	}
	if v.Plex == nil || v.Plex.AccountID != 7 || v.Plex.AccountName != "mom" || v.Plex.ServerID != "mid-mom" || v.Plex.ServerName != "Mom's Plex" {
		t.Errorf("provenance = %+v", v.Plex)
	}
	inv, _ := r.settings().InviteByID(r.settings().Invites[0].ID)
	if inv.ViewerName != "Mom" {
		t.Errorf("the invite does not say who used it: %+v", inv)
	}
	if len(r.events) != 1 || r.events[0].Viewer != "Mom" || r.events[0].Refreshed || r.events[0].Server != "Mom's Plex" {
		t.Errorf("events = %+v", r.events)
	}
	// It was saved, so a restart keeps it.
	saved, _, err := access.LoadSettings(r.path)
	if err != nil || len(saved.Viewers) != 2 || saved.Viewers[1].Name != "Mom" || !saved.Invites[0].Used() {
		t.Errorf("saved = %+v %v", saved.Viewers, err)
	}
	// The ticket is used up.
	if code, _, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}}); code != 200 {
		t.Log("a second confirmation is answered, but does nothing more") // the page says expired
	}
	if n := len(r.settings().Viewers); n != 2 {
		t.Errorf("%d viewers after a second confirmation", n)
	}

	// No secret anywhere it shouldn't be: the invite's token and the Plex token and the server's own token.
	raw, _ := os.ReadFile(r.path)
	for _, secret := range []string{r.tokens["Mom"], "tok-secret-plex-token", "server-token-mid-mom"} {
		if strings.Contains(r.logs.String(), secret) || strings.Contains(string(raw), secret) {
			t.Errorf("%q is in the log or the settings file", secret)
		}
	}
}

func TestEveryWayAnInviteCanBeBadIsTheSame404(t *testing.T) {
	r := newRig(t, true, "Mom", "Dad", "Gone")
	// Dad's was used, and then his viewer was deleted: it must not bring him back.
	_, _ = r.store.Edit(func(s *access.Settings) error {
		s.Invites[1].ViewerName = "Dad"
		return nil
	})
	// Gone's was revoked.
	_, _ = r.store.Edit(func(s *access.Settings) error { s.Invites = s.Invites[:2]; return nil })
	_, expired := access.NewInvite("Old", 1, nil, t0.Add(-8*24*time.Hour))
	_, _ = r.store.Edit(func(s *access.Settings) error { s.Invites = append(s.Invites, expired); return nil })

	want404 := func(name, path string) {
		t.Helper()
		code, body, _ := r.do("GET", path, friend, nil)
		if code != 404 || body != "not found\n" {
			t.Errorf("%s: %d %q, want the plain 404", name, code, body)
		}
	}
	other := "10.0.0." // a different address each time, so the limiter doesn't answer instead
	for i, c := range []struct{ name, path string }{
		{"a token that never existed", Path + strings.Repeat("a", 43)},
		{"the right length, wrong characters", Path + strings.Repeat("!", 43)},
		{"too short", Path + "abc"},
		{"too long", Path + strings.Repeat("a", 44)},
		{"nothing", Path},
		{"a used invite whose viewer was deleted", Path + r.tokens["Dad"]},
		{"a revoked invite", Path + r.tokens["Gone"]},
		{"the hash of a real one", Path + access.HashInviteToken(r.tokens["Mom"])[:43]},
	} {
		code, body, _ := r.do("GET", c.path, other+strconv.Itoa(i+1), nil)
		if code != 404 || body != "not found\n" {
			t.Errorf("%s: %d %q, want the plain 404", c.name, code, body)
		}
	}
	want404("an unknown step", r.link("Mom")+"/nothing")

	// Expired: a real invite, past its life.
	r.now = t0.Add(access.InviteLife)
	code, body, _ := r.do("GET", r.link("Mom"), "10.0.1.1", nil)
	if code != 404 || body != "not found\n" {
		t.Errorf("expired: %d %q", code, body)
	}
	// Enrollment off: the same 404. Whether Plex accounts may sign in to the admin pages makes no difference either way.
	r.now = t0
	_, _ = r.store.Edit(func(s *access.Settings) error { s.Plex.Enrollment = false; return nil })
	if code, body, _ := r.do("GET", r.link("Mom"), "10.0.2.1", nil); code != 404 || body != "not found\n" {
		t.Errorf("enrollment off: %d %q", code, body)
	}
	_, _ = r.store.Edit(func(s *access.Settings) error { s.Plex.Enrollment, s.Plex.Enabled = true, true; return nil })
	if code, _, _ := r.do("GET", r.link("Mom"), "10.0.2.2", nil); code != 200 {
		t.Errorf("enrollment on with Plex sign-in on too: %d", code)
	}
	_, _ = r.store.Edit(func(s *access.Settings) error { s.Plex.Enabled = false; return nil })
	if code, _, _ := r.do("GET", r.link("Mom"), "10.0.3.1", nil); code != 200 {
		t.Errorf("the good link stopped working: %d", code)
	}
}

func TestWrongLinksFromOneAddressAreLimited(t *testing.T) {
	r := newRig(t, true, "Mom")
	bad := Path + strings.Repeat("b", 43)
	for i := 0; i < maxFailures; i++ {
		if code, _, _ := r.do("GET", bad, friend, nil); code != 404 {
			t.Fatalf("try %d: %d", i+1, code)
		}
	}
	// Now even the right link is refused from that address, so a guess can't be confirmed by getting through.
	if code, _, _ := r.do("GET", r.link("Mom"), friend, nil); code != http.StatusTooManyRequests {
		t.Errorf("the sixth try = %d, want 429", code)
	}
	if code, _, _ := r.do("GET", r.link("Mom"), "203.0.113.200", nil); code != 200 {
		t.Errorf("another address = %d, want the page", code)
	}
	r.now = r.now.Add(failWindow + time.Second)
	if code, _, _ := r.do("GET", r.link("Mom"), friend, nil); code != 200 {
		t.Errorf("after the window = %d", code)
	}
}

func TestOnlyOwnedServersWithPublicAddressesAreOffered(t *testing.T) {
	r := newRig(t, true, "Mom")
	body, _ := r.signInAs("Mom", "tok", mom,
		plexServer("Shared", "mid-1", false, plexConn("198.51.100.99", false, false)),
		plexServer("Private only", "mid-2", true, plexConn("192.168.1.9", true, false), plexConn("10.1.1.1", false, false), plexConn("100.64.0.9", false, false)),
		plexServer("Relay only", "mid-3", true, plexConn("203.0.113.60", false, true)))
	if !strings.Contains(body, "no Plex Media Server of yours") || strings.Contains(body, `name="ticket"`) {
		t.Errorf("nothing to offer, and it should say so with no way to continue:\n%s", body)
	}
	if _, ok := r.viewer("Mom"); ok {
		t.Error("a viewer was made")
	}
}

func TestTheAddressesComeFromPlexNeverFromTheBrowser(t *testing.T) {
	r := newRig(t, true, "Mom")
	body, _ := r.signInAs("Mom", "tok", mom, home())
	ticket, _ := ticketAndServer(t, body)

	// An unknown server id, and a made-up address, are not taken.
	if code, _, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {"mid-unknown"}}); code != 400 {
		t.Errorf("an unknown server = %d", code)
	}
	if code, _, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {"mid-mom"}, "address": {"1.2.3.4"}, "addresses": {"5.6.7.8"}}); code != 200 {
		t.Fatalf("confirm = %d", code)
	}
	v, _ := r.viewer("Mom")
	if len(v.Addresses) != 1 || v.Addresses[0] != "203.0.113.50" {
		t.Errorf("addresses = %v, want only what Plex listed", v.Addresses)
	}
	// A server that was listed for another person's ticket is not available either.
	if code, _, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {"made-up"}, "server": {"mid-mom"}}); code != 200 {
		t.Errorf("a made-up ticket = %d", code)
	}
	if n := len(r.settings().Viewers); n != 2 {
		t.Errorf("%d viewers", n)
	}
}

func TestATicketIsForOneInviteOnly(t *testing.T) {
	r := newRig(t, true, "Mom", "Dad")
	body, _ := r.signInAs("Mom", "tok", mom, home())
	ticket, server := ticketAndServer(t, body)
	code, page, _ := r.do("POST", r.link("Dad")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}})
	if code != 200 || !strings.Contains(page, "Sign-in expired") {
		t.Errorf("Mom's ticket on Dad's invite = %d %s", code, page)
	}
	if _, ok := r.viewer("Dad"); ok {
		t.Error("Dad was enrolled with Mom's ticket")
	}
}

func TestAnAddressAnotherViewerHoldsIsRefusedPlainly(t *testing.T) {
	r := newRig(t, true, "Mom")
	_, _ = r.store.Edit(func(s *access.Settings) error {
		s.Viewers[0].Addresses = []string{"203.0.113.0/24"}
		return nil
	})
	body, _ := r.signInAs("Mom", "tok", mom, home())
	ticket, server := ticketAndServer(t, body)
	code, page, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}})
	if code != http.StatusConflict || !strings.Contains(page, "already allowed for someone else") {
		t.Fatalf("confirm = %d %s", code, page)
	}
	if strings.Contains(page, "Existing") {
		t.Error("the page names the other viewer")
	}
	if _, ok := r.viewer("Mom"); ok {
		t.Error("Mom was added anyway")
	}
	if inv := r.settings().Invites[0]; inv.Used() {
		t.Error("the invite counts as used")
	}
	if len(r.events) != 0 {
		t.Errorf("events = %+v", r.events)
	}
}

func TestANameThatIsTakenIsRefused(t *testing.T) {
	r := newRig(t, true, "Mom")
	body, _ := r.signInAs("Mom", "tok", mom, home())
	ticket, server := ticketAndServer(t, body)
	_, _ = r.store.Edit(func(s *access.Settings) error {
		s.Viewers = append(s.Viewers, access.Viewer{Name: "mom", Addresses: []string{"198.51.100.30"}, Limit: 1})
		return nil
	})
	code, page, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}})
	if code != http.StatusConflict || !strings.Contains(page, "already exists") {
		t.Errorf("confirm = %d %s", code, page)
	}
}

func TestOpeningTheLinkAgainRefreshesTheAddresses(t *testing.T) {
	r := newRig(t, true, "Mom")
	body, _ := r.signInAs("Mom", "tok1", mom, home())
	ticket, server := ticketAndServer(t, body)
	r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}})

	// The landing page now says it is an update.
	if _, page, _ := r.do("GET", r.link("Mom"), friend, nil); !strings.Contains(page, "already") || !strings.Contains(page, "Update with") {
		t.Errorf("landing for a used link:\n%s", page)
	}
	// Her address changed: a new sign-in, the same account.
	moved := plexServer("Mom's Plex", "mid-mom", true, plexConn("203.0.113.88", false, false))
	body, _ = r.signInAs("Mom", "tok2", mom, moved)
	ticket, server = ticketAndServer(t, body)
	code, page, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}})
	if code != 200 || !strings.Contains(page, "Updated") {
		t.Fatalf("refresh = %d %s", code, page)
	}
	v, _ := r.viewer("Mom")
	if len(v.Addresses) != 1 || v.Addresses[0] != "203.0.113.88" || v.Limit != 2 {
		t.Errorf("viewer after refresh = %+v", v)
	}
	if len(r.settings().Viewers) != 2 {
		t.Errorf("%d viewers: a refresh made another", len(r.settings().Viewers))
	}
	if len(r.events) != 2 || !r.events[1].Refreshed {
		t.Errorf("events = %+v", r.events)
	}

	// Someone else with the link cannot take the viewer over with their own server.
	other := plexauth.User{ID: 99, Name: "stranger"}
	body, _ = r.signInAs("Mom", "tok3", other, plexServer("Stranger's", "mid-x", true, plexConn("203.0.113.99", false, false)))
	ticket, server = ticketAndServer(t, body)
	code, page, _ = r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}})
	if code != http.StatusConflict || !strings.Contains(page, "different Plex account") {
		t.Errorf("another account = %d %s", code, page)
	}
	if v, _ := r.viewer("Mom"); v.Addresses[0] != "203.0.113.88" || v.Plex.AccountID != 7 {
		t.Errorf("another account changed the viewer: %+v", v)
	}
}

func TestARevokedInviteCannotBeConfirmedWithATicketItAlreadyHas(t *testing.T) {
	r := newRig(t, true, "Mom")
	body, _ := r.signInAs("Mom", "tok", mom, home())
	ticket, server := ticketAndServer(t, body)
	_, _ = r.store.Edit(func(s *access.Settings) error { s.Invites = nil; return nil })
	if code, _, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}}); code != 404 {
		t.Errorf("confirm after revoking = %d", code)
	}
	if _, ok := r.viewer("Mom"); ok {
		t.Error("a revoked invite enrolled someone")
	}
}

func TestAnInviteThatExpiresWhileSigningInEnrollsNobody(t *testing.T) {
	r := newRig(t, true, "Mom")
	body, _ := r.signInAs("Mom", "tok", mom, home())
	ticket, server := ticketAndServer(t, body)
	r.now = t0.Add(access.InviteLife + time.Minute)
	if code, _, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}}); code != 404 {
		t.Errorf("confirm after it expired = %d", code)
	}
}

func TestTheLimitsOfTheInviteAreKeptForDevicesThatExist(t *testing.T) {
	r := newRig(t, true)
	tok, inv := access.NewInvite("Mom", 1, map[string]int{"antenna": 3, "removed": 5}, t0)
	r.tokens["Mom"] = tok
	_, err := r.store.Edit(func(s *access.Settings) error {
		s.Devices = []access.Device{{Name: "antenna", Listen: ":5004", Tuner: "192.168.1.50"}}
		s.Invites = append(s.Invites, inv)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := r.signInAs("Mom", "tok", mom, home())
	ticket, server := ticketAndServer(t, body)
	if code, page, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}}); code != 200 {
		t.Fatalf("confirm = %d %s", code, page)
	}
	v, _ := r.viewer("Mom")
	if v.Limit != 1 || len(v.Limits) != 1 || v.Limits["antenna"] != 3 {
		t.Errorf("limits = %d %v", v.Limit, v.Limits)
	}
}

func TestFormsFromOtherSitesAreRefused(t *testing.T) {
	r := newRig(t, true, "Mom")
	if code, _, _ := r.do("POST", r.link("Mom")+"/start", friend, url.Values{}, "http://evil.example"); code != 403 {
		t.Errorf("start from another site = %d", code)
	}
	body, _ := r.signInAs("Mom", "tok", mom, home())
	ticket, server := ticketAndServer(t, body)
	if code, _, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}}, "http://evil.example"); code != 403 {
		t.Errorf("confirm from another site = %d", code)
	}
	if _, ok := r.viewer("Mom"); ok {
		t.Error("a form from another site enrolled someone")
	}
}

func TestOnlyTheRightMethodsAreAnswered(t *testing.T) {
	r := newRig(t, true, "Mom")
	for _, c := range []struct{ method, path string }{
		{"POST", r.link("Mom")}, {"GET", r.link("Mom") + "/start"}, {"GET", r.link("Mom") + "/confirm"},
		{"POST", r.link("Mom") + "/wait"}, {"PUT", r.link("Mom")}, {"DELETE", r.link("Mom") + "/start"},
	} {
		if code, _, _ := r.do(c.method, c.path, friend, nil); code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s = %d, want 405", c.method, c.path, code)
		}
	}
}

func TestPlexBeingDownIsExplainedAndTheSignInEndsCleanly(t *testing.T) {
	r := newRig(t, true, "Mom")
	r.plex.set(func() { r.plex.down = true })
	code, page, _ := r.do("POST", r.link("Mom")+"/start", friend, url.Values{})
	if code != http.StatusBadGateway || !strings.Contains(page, "couldn't reach plex.tv") {
		t.Errorf("start with plex.tv down = %d %s", code, page)
	}
	r.plex.set(func() { r.plex.down = false })
	_, _, wait := r.start("Mom")
	r.plex.set(func() { r.plex.down = true })
	r.now = r.now.Add(5 * time.Second)
	if code, _, _ := r.do("GET", wait, friend, nil); code != http.StatusBadGateway {
		t.Errorf("wait with plex.tv down = %d", code)
	}
	// And a sign-in that plex.tv has forgotten.
	r.plex.set(func() { r.plex.down = false; r.plex.gone[r.plex.next] = true })
	r.now = r.now.Add(5 * time.Second)
	if _, page, _ := r.do("GET", wait, friend, nil); !strings.Contains(page, "ran out of time") {
		t.Errorf("an expired sign-in:\n%s", page)
	}
}

func TestPollingPlexIsThrottledAndStartsAreLimited(t *testing.T) {
	r := newRig(t, true, "Mom")
	_, _, wait := r.start("Mom")
	r.now = r.now.Add(5 * time.Second)
	r.do("GET", wait, friend, nil)
	before := r.plex.checks
	for i := 0; i < 5; i++ {
		r.do("GET", wait, friend, nil) // all in the same instant
	}
	if r.plex.checks != before {
		t.Errorf("plex.tv was asked %d more times within a second", r.plex.checks-before)
	}
	for i := 0; i < startsPerMin; i++ { // one already used above
		r.do("POST", r.link("Mom")+"/start", friend, url.Values{})
	}
	if code, _, _ := r.do("POST", r.link("Mom")+"/start", friend, url.Values{}); code != http.StatusTooManyRequests {
		t.Errorf("too many starts = %d", code)
	}
	// Sign-ins in progress are capped, and a handle that is made up gets nowhere.
	if _, page, _ := r.do("GET", r.link("Mom")+"/wait?h=made-up", "203.0.113.150", nil); !strings.Contains(page, "ran out of time") {
		t.Errorf("a made-up handle:\n%s", page)
	}
}

func TestAHandleBelongsToItsOwnInvite(t *testing.T) {
	r := newRig(t, true, "Mom", "Dad")
	_, _, wait := r.start("Mom")
	other := strings.Replace(wait, r.tokens["Mom"], r.tokens["Dad"], 1)
	r.now = r.now.Add(5 * time.Second)
	if _, page, _ := r.do("GET", other, friend, nil); !strings.Contains(page, "ran out of time") {
		t.Errorf("Mom's sign-in followed on Dad's link:\n%s", page)
	}
}

func TestNothingIsSavedWhereThereIsNoSettingsFile(t *testing.T) {
	r := newRig(t, false, "Mom")
	body, _ := r.signInAs("Mom", "tok", mom, home())
	ticket, server := ticketAndServer(t, body)
	code, page, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}})
	if code != http.StatusServiceUnavailable || !strings.Contains(page, "can't save") {
		t.Errorf("confirm without a file = %d %s", code, page)
	}
	if _, ok := r.viewer("Mom"); ok {
		t.Error("a viewer exists though nothing could be saved")
	}
}

func TestNamesAreEscapedOnThePage(t *testing.T) {
	r := newRig(t, true)
	tok, inv := access.NewInvite(`Mom <script>alert(1)</script> & "co"`, 1, nil, t0)
	r.tokens["x"] = tok
	if _, err := r.store.Edit(func(s *access.Settings) error { s.Invites = append(s.Invites, inv); return nil }); err != nil {
		t.Fatal(err)
	}
	_, page, _ := r.do("GET", Path+tok, friend, nil)
	if strings.Contains(page, "<script>") || !strings.Contains(page, "&lt;script&gt;") {
		t.Errorf("the invite's name is not escaped:\n%s", page)
	}
	body, _ := func() (string, string) {
		_, _, wait := func() (int, string, string) {
			code, b, _ := r.do("POST", Path+tok+"/start", friend, url.Values{})
			m := regexp.MustCompile(`href="(/invite/[^"]+/wait\?h=[^"]+)"`).FindStringSubmatch(b)
			return code, b, html2text(m[1])
		}()
		r.plex.approve("tok", plexauth.User{ID: 8, Name: "<img src=x onerror=alert(1)>"}, plexServer("<b>Server</b>", "mid-xss", true, plexConn("203.0.113.70", false, false)))
		r.now = r.now.Add(5 * time.Second)
		_, b, _ := r.do("GET", wait, friend, nil)
		return b, wait
	}()
	if strings.Contains(body, "<img") || strings.Contains(body, "<b>Server") {
		t.Errorf("a name from Plex is not escaped:\n%s", body)
	}
}

func TestThePageIsDarkWithAPlexButtonAndALargeIcon(t *testing.T) {
	r := newRig(t, true, "Mom")
	code, body, hdr := r.do("GET", r.link("Mom"), friend, nil)
	if code != 200 {
		t.Fatal(code)
	}
	for _, want := range []string{
		`color-scheme:dark`, `<meta name="color-scheme" content="dark">`, // never a white page
		`class="mark"`, `aria-hidden="true"`, `stroke="#2dd4bf"`, // Yagi's own mark, large, hidden from a screen reader
		`<button type="submit" class="plex" aria-label="Sign in with Plex">`, `background:#e5a00d`, // Plex's own yellow-orange
		`<img src="data:image/png;base64,`, // the wordmark, inside the page
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the landing page has no %q", want)
		}
	}
	for _, light := range []string{"prefers-color-scheme", "background:#fff;", "background:#ffffff;", "background:white"} {
		if strings.Contains(body, light) {
			t.Errorf("the page has a light theme: %q", light)
		}
	}
	if false {
		t.Error("the page has a light theme")
	}
	if csp := hdr.Get("Content-Security-Policy"); !strings.Contains(csp, "img-src data:") || strings.Contains(csp, "script") || strings.Contains(csp, "http") {
		t.Errorf("csp = %q, want only its own style and the image in the page", csp)
	}
}

func TestEveryKindOfPageHasAnIconAndLoadsNothingFromOutside(t *testing.T) {
	r := newRig(t, true, "Mom")
	pages := map[string]string{}
	_, pages["landing"], _ = r.do("GET", r.link("Mom"), friend, nil)
	_, pages["waiting"], _ = r.do("POST", r.link("Mom")+"/start", friend, url.Values{})
	body, wait := r.signInAs("Mom", "tok", mom, home())
	pages["servers"] = body
	ticket, server := ticketAndServer(t, body)
	_, pages["done"], _ = r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}})
	_, pages["expired"], _ = r.do("GET", r.link("Mom")+"/wait?h=made-up", "203.0.113.150", nil)
	r.plex.set(func() { r.plex.down = true })
	_, pages["plex down"], _ = r.do("POST", r.link("Mom")+"/start", "203.0.113.151", url.Values{})
	_ = wait

	for name, p := range pages {
		if !strings.Contains(p, `<svg class="icon"`) && !strings.Contains(p, `<svg class="mark"`) || !strings.Contains(p, "<h1>") {
			t.Errorf("%s: no icon or heading:\n%s", name, p)
		}
		if strings.Contains(p, "<script") || strings.Contains(p, "<link") || strings.Contains(p, "<iframe") {
			t.Errorf("%s: loads something it shouldn't", name)
		}
		// Every address in the page is its own, except the one link out to Plex's sign-in.
		for _, m := range regexp.MustCompile(`(?:src|href|action|url)=["']?(https?://[^"' >]+)`).FindAllStringSubmatch(p, -1) {
			if !strings.HasPrefix(html2text(m[1]), "https://app.plex.tv/auth") {
				t.Errorf("%s: points outside: %s", name, m[1])
			}
		}
	}
	// The icons fit what the page says.
	for name, icon := range map[string]string{"done": "m16 9-5.5 5.5L8 12", "servers": `class="mark"`, "waiting": `class="mark"`, "landing": `class="mark"`} {
		if !strings.Contains(pages[name], icon) {
			t.Errorf("%s: wrong icon", name)
		}
	}
}

func TestTheMarkOnThePageIsTheOneInTheBrandFolder(t *testing.T) {
	svg, err := os.ReadFile("../../docs/brand/yagi-logo.svg")
	if err != nil {
		t.Skip("not run from a checkout:", err)
	}
	for _, part := range regexp.MustCompile(`<path d="[^"]+"/>|fill="#0f1b24"|stroke="#2dd4bf"|scale\(8\.5\)`).FindAllString(string(svg), -1) {
		if !strings.Contains(mark, part) {
			t.Errorf("the page's mark differs from docs/brand/yagi-logo.svg: %s", part)
		}
	}
}

func TestThePlexButtonIsOnEveryWaitingPageAndPlexSendsThePersonBack(t *testing.T) {
	r := newRig(t, true, "Mom")
	code, started, _ := r.do("POST", r.link("Mom")+"/start", friend, url.Values{})
	if code != 200 {
		t.Fatal(code)
	}
	m := regexp.MustCompile(`class="button plex" href="([^"]+)"`).FindStringSubmatch(started)
	if m == nil {
		t.Fatalf("no Plex button on the page that follows pressing sign in:\n%s", started)
	}
	auth, err := url.Parse(html2text(m[1]))
	if err != nil || auth.Host != "app.plex.tv" {
		t.Fatalf("the button goes to %s (%v)", m[1], err)
	}
	q, _ := url.ParseQuery(auth.Fragment[strings.Index(auth.Fragment, "?")+1:])
	back, _ := url.Parse(q.Get("forwardUrl"))
	wm := regexp.MustCompile(`href="(/invite/[^"]+/wait\?h=[^"]+)"`).FindStringSubmatch(started)
	if wm == nil || back.Host == "" || back.Path+"?"+back.RawQuery != html2text(wm[1]) {
		t.Errorf("Plex is sent back to %q, want this page's wait address %v", q.Get("forwardUrl"), wm)
	}
	if strings.Contains(started, `target="_blank"`) {
		t.Error("the button opens another tab, which the person then has to find their way back from")
	}

	// Nothing on the way refreshes by itself: the first version did, and it replaced the button before it was pressed.
	r.now = r.now.Add(5 * time.Second)
	_, pending, _ := r.do("GET", html2text(wm[1]), friend, nil)
	if !strings.Contains(pending, `class="button plex"`) || !strings.Contains(pending, "has not confirmed") {
		t.Errorf("the page for a sign-in not made yet has no Plex button:\n%s", pending)
	}
	r.plex.approve("tok", mom, home())
	r.now = r.now.Add(5 * time.Second)
	_, servers, _ := r.do("GET", html2text(wm[1]), friend, nil)
	_, _, _ = r.do("GET", Path+"x", friend, nil)
	for name, p := range map[string]string{"first": started, "pending": pending, "servers": servers} {
		if strings.Contains(p, "http-equiv") {
			t.Errorf("the %s page refreshes by itself", name)
		}
	}
}

func TestGoingBackOrReloadingAfterSigningInShowsTheSameServers(t *testing.T) {
	r := newRig(t, true, "Mom")
	body, wait := r.signInAs("Mom", "tok", mom, home())
	ticket, _ := ticketAndServer(t, body)
	asked := r.plex.checks
	for i := 0; i < 3; i++ {
		r.now = r.now.Add(5 * time.Second)
		code, again, _ := r.do("GET", wait, friend, nil)
		again2, _ := ticketAndServerOK(again)
		if code != 200 || again2 != ticket {
			t.Fatalf("reload %d = %d, ticket %q, want the same page with ticket %q:\n%s", i+1, code, again2, ticket, again)
		}
	}
	if r.plex.checks != asked {
		t.Errorf("plex.tv was asked again on a reload (%d more times)", r.plex.checks-asked)
	}
	// After the person has been added, the same address says it is over rather than offering the servers again.
	_, server := ticketAndServer(t, body)
	r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}})
	r.now = r.now.Add(5 * time.Second)
	if _, page, _ := r.do("GET", wait, friend, nil); strings.Contains(page, `name="ticket"`) {
		t.Errorf("the servers are offered again after confirming:\n%s", page)
	}
}

func ticketAndServerOK(body string) (string, bool) {
	m := regexp.MustCompile(`name="ticket" value="([^"]+)"`).FindStringSubmatch(body)
	if m == nil {
		return "", false
	}
	return m[1], true
}

func TestNoServerIsTheSameAnswerOnReload(t *testing.T) {
	r := newRig(t, true, "Mom")
	body, wait := r.signInAs("Mom", "tok", mom, plexServer("Private", "mid-1", true, plexConn("192.168.1.9", true, false)))
	if !strings.Contains(body, "no Plex Media Server of yours") {
		t.Fatal(body)
	}
	asked := r.plex.checks
	r.now = r.now.Add(5 * time.Second)
	if _, again, _ := r.do("GET", wait, friend, nil); !strings.Contains(again, "no Plex Media Server of yours") {
		t.Errorf("reload:\n%s", again)
	}
	if r.plex.checks != asked {
		t.Error("plex.tv was asked again")
	}
}

func TestTheLastPageSaysWhatToDoInPlexAndThatTheWindowCanBeClosed(t *testing.T) {
	r := newRig(t, true, "Mom")
	body, wait := r.signInAs("Mom", "tok", mom, home())
	ticket, server := ticketAndServer(t, body)
	code, done, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}})
	if code != 200 {
		t.Fatal(code)
	}
	for _, want := range []string{
		"You&#39;re in", "Settings &rarr; Live TV &amp; DVR &rarr; Set Up Plex DVR", "Enter its network address manually",
		"<code>example.com:5004</code> or <code>example.com:5005</code>", // where the tuners are, by the name the page was reached at
		"Scan for channels", "open your invite link again", "You can close this window",
	} {
		if !strings.Contains(done, want) {
			t.Errorf("the last page doesn't say %q:\n%s", want, done)
		}
	}
	// Opening the link again to update: nothing to do but close the window.
	_ = wait
	body, _ = r.signInAs("Mom", "tok2", mom, plexServer("Mom's Plex", "mid-mom", true, plexConn("203.0.113.88", false, false)))
	ticket, server = ticketAndServer(t, body)
	_, again, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}})
	if !strings.Contains(again, "<h1>Updated</h1>") || !strings.Contains(again, "Nothing else to do. You can close this window.") || strings.Contains(again, "Set Up Plex DVR") {
		t.Errorf("the page after an update:\n%s", again)
	}
}

func TestTheLastPageSendsThemToTheOwnerWhenItCannotSayWhereTheTunerIs(t *testing.T) {
	r := newRig(t, true, "Mom")
	r.e.cfg.Tuners = nil
	body, _ := r.signInAs("Mom", "tok", mom, home())
	ticket, server := ticketAndServer(t, body)
	_, done, _ := r.do("POST", r.link("Mom")+"/confirm", friend, url.Values{"ticket": {ticket}, "server": {server}})
	if !strings.Contains(done, "Ask the person who invited you for the tuner's address.") || strings.Contains(done, ":5004") {
		t.Errorf("without tuners to name:\n%s", done)
	}
}
