package plexauth

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/johnschult/antennarr/internal/access"
	"github.com/johnschult/antennarr/internal/httpx"
)

// CookieName is the session cookie. It is only sent to the admin pages.
const CookieName = "antennarr_session"

// CookiePath keeps the cookie off the tuner endpoints that media servers use.
const CookiePath = "/antennarr/"

// Config for Auth.
type Config struct {
	Base, AuthBase string
	HTTP           *http.Client
	// Settings returns the current settings.
	Settings func() access.Settings
	// Edit saves a change to the settings (adding an admin, keeping the client
	// ID). It may be nil, or fail with access.ErrNoFile, when there is no
	// settings file; Antennarr then works but forgets on restart.
	Edit func(fn func(*access.Settings) error) (access.Settings, error)
	// IsAdmin says whether a request may administer Antennarr, by address or
	// by an existing sign-in. Adding a Plex account needs it.
	IsAdmin func(*http.Request) bool
	Now     func() time.Time
	Log     *slog.Logger

	// SessionsFile, if set, is where sign-ins are kept so they survive a
	// restart. Empty means they are forgotten when Antennarr stops.
	SessionsFile string

	SessionIdle     time.Duration // default 12h
	SessionMax      time.Duration // default 7 days
	MaxPending      int           // sign-ins in progress at once; default 50
	StartsPerMinute int           // per address; default 6
}

// Auth is "Sign in with Plex".
type Auth struct {
	cfg  Config
	log  *slog.Logger
	sess *sessions

	mu       sync.Mutex
	clientID string
	pins     map[int64]*pending
	starts   map[string][]time.Time
}

type pending struct {
	code     string
	add      bool // adding an account to the allowlist, rather than signing in
	expires  time.Time
	lastPoll time.Time
}

func New(cfg Config) *Auth {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.SessionIdle == 0 {
		cfg.SessionIdle = 12 * time.Hour
	}
	if cfg.SessionMax == 0 {
		cfg.SessionMax = 7 * 24 * time.Hour
	}
	if cfg.MaxPending == 0 {
		cfg.MaxPending = 50
	}
	if cfg.StartsPerMinute == 0 {
		cfg.StartsPerMinute = 6
	}
	if cfg.IsAdmin == nil {
		cfg.IsAdmin = func(*http.Request) bool { return false }
	}
	return &Auth{
		cfg: cfg, log: cfg.Log, sess: newSessions(cfg.SessionIdle, cfg.SessionMax, cfg.Now, cfg.SessionsFile, cfg.Log),
		pins: map[int64]*pending{}, starts: map[string][]time.Time{},
	}
}

func (a *Auth) enabled() bool { return a.cfg.Settings().PlexEnabled() }

// Admin says who is signed in as an admin: a good session whose account is
// still on the allowlist, with Plex sign-in still switched on. Removing an
// account, or turning the feature off, therefore takes effect on the very
// next request.
func (a *Auth) Admin(r *http.Request) (access.PlexAccount, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return access.PlexAccount{}, false
	}
	user, ok := a.sess.get(c.Value)
	if !ok {
		return access.PlexAccount{}, false
	}
	set := a.cfg.Settings()
	if !set.PlexEnabled() {
		return access.PlexAccount{}, false
	}
	acct, ok := set.PlexAdmin(user.ID)
	return acct, ok
}

// IsSignedInAdmin is Admin as a yes/no, for the policy.
func (a *Auth) IsSignedInAdmin(r *http.Request) bool { _, ok := a.Admin(r); return ok }

// clientIDFor returns this Antennarr's identity for plex.tv, making and saving
// one the first time.
func (a *Auth) clientIDFor() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.clientID != "" {
		return a.clientID
	}
	if set := a.cfg.Settings(); set.Plex != nil && set.Plex.ClientID != "" {
		a.clientID = set.Plex.ClientID
		return a.clientID
	}
	id := "antennarr-" + randomID(18)
	if a.cfg.Edit != nil {
		_, err := a.cfg.Edit(func(s *access.Settings) error {
			if s.Plex == nil {
				s.Plex = &access.PlexSettings{}
			}
			if s.Plex.ClientID == "" {
				s.Plex.ClientID = id
			} else {
				id = s.Plex.ClientID
			}
			return nil
		})
		if err != nil && !errors.Is(err, access.ErrNoFile) {
			a.log.Warn("could not save the Plex client ID; using one for this run only", "err", err)
		}
	}
	a.clientID = id
	return id
}

func (a *Auth) client() *Client {
	return &Client{Base: a.cfg.Base, AuthBase: a.cfg.AuthBase, HTTP: a.cfg.HTTP, ClientID: a.clientIDFor()}
}

// Handler serves /antennarr/auth/*. It is public by design (you can't sign in
// to reach the sign-in), so each endpoint does its own checks.
func (a *Auth) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /antennarr/auth/status", a.status)
	mux.HandleFunc("POST /antennarr/auth/plex/start", func(w http.ResponseWriter, r *http.Request) { a.start(w, r, false) })
	mux.HandleFunc("POST /antennarr/auth/plex/poll", func(w http.ResponseWriter, r *http.Request) { a.poll(w, r, false) })
	mux.HandleFunc("POST /antennarr/auth/plex/add/start", func(w http.ResponseWriter, r *http.Request) { a.start(w, r, true) })
	mux.HandleFunc("POST /antennarr/auth/plex/add/poll", func(w http.ResponseWriter, r *http.Request) { a.poll(w, r, true) })
	mux.HandleFunc("POST /antennarr/auth/logout", a.logout)
	mux.HandleFunc("/antennarr/auth/", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteErr(w, http.StatusNotFound, "no such endpoint")
	})
	return httpx.GuardWrites(mux)
}

type accountDoc struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name"`
}

func (a *Auth) status(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{"plexEnabled": a.enabled(), "signedIn": nil, "admin": a.cfg.IsAdmin(r)}
	if acct, ok := a.Admin(r); ok {
		resp["signedIn"] = accountDoc{ID: acct.ID, Name: acct.Name}
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func clientHost(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

// allowStart limits how often one address may begin a sign-in.
func (a *Auth) allowStart(host string, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.starts) > 1000 { // a flood from many addresses: start counting afresh
		a.starts = map[string][]time.Time{}
	}
	recent := a.starts[host][:0:0]
	for _, t := range a.starts[host] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= a.cfg.StartsPerMinute {
		a.starts[host] = recent
		return false
	}
	a.starts[host] = append(recent, now)
	return true
}

// remember stores a sign-in in progress, making room if there are too many.
func (a *Auth) remember(id int64, p *pending, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, v := range a.pins {
		if now.After(v.expires) {
			delete(a.pins, k)
		}
	}
	for len(a.pins) >= a.cfg.MaxPending { // drop the one closest to expiring
		var victim int64
		first := true
		for k, v := range a.pins {
			if first || v.expires.Before(a.pins[victim].expires) {
				victim, first = k, false
			}
		}
		delete(a.pins, victim)
	}
	a.pins[id] = p
}

func (a *Auth) start(w http.ResponseWriter, r *http.Request, add bool) {
	if !a.enabled() {
		httpx.WriteErr(w, http.StatusNotFound, "signing in with Plex is turned off")
		return
	}
	if add && !a.cfg.IsAdmin(r) {
		httpx.WriteErr(w, http.StatusForbidden, "only an admin can add a Plex account")
		return
	}
	now := a.cfg.Now()
	if !a.allowStart(clientHost(r), now) {
		httpx.WriteErr(w, http.StatusTooManyRequests, "too many sign-in attempts; wait a minute")
		return
	}
	c := a.client()
	pin, err := c.NewPIN(r.Context())
	if err != nil {
		a.log.Warn("plex sign-in could not start", "err", err)
		httpx.WriteErr(w, http.StatusBadGateway, "couldn't start a sign-in with plex.tv: "+err.Error())
		return
	}
	expires := pin.ExpiresAt
	if expires.IsZero() || expires.After(now.Add(30*time.Minute)) {
		expires = now.Add(30 * time.Minute)
	}
	a.remember(pin.ID, &pending{code: pin.Code, add: add, expires: expires}, now)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"pinId": pin.ID, "authUrl": c.AuthURL(pin), "expiresAt": expires})
}

type pollReq struct {
	PinID int64 `json:"pinId"`
}

// minPoll stops a browser asking plex.tv more often than is polite.
const minPoll = time.Second

func (a *Auth) poll(w http.ResponseWriter, r *http.Request, add bool) {
	var in pollReq
	if !httpx.Decode(w, r, &in) {
		return
	}
	if !a.enabled() {
		httpx.WriteErr(w, http.StatusNotFound, "signing in with Plex is turned off")
		return
	}
	if add && !a.cfg.IsAdmin(r) {
		httpx.WriteErr(w, http.StatusForbidden, "only an admin can add a Plex account")
		return
	}
	now := a.cfg.Now()

	a.mu.Lock()
	p, ok := a.pins[in.PinID]
	if !ok || p.add != add || now.After(p.expires) {
		delete(a.pins, in.PinID)
		a.mu.Unlock()
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "expired"})
		return
	}
	if now.Sub(p.lastPoll) < minPoll {
		a.mu.Unlock()
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "pending"})
		return
	}
	p.lastPoll = now
	code := p.code
	a.mu.Unlock()

	c := a.client()
	token, err := c.CheckPIN(r.Context(), in.PinID, code)
	switch {
	case errors.Is(err, ErrExpired):
		a.forget(in.PinID)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "expired"})
		return
	case err != nil:
		httpx.WriteErr(w, http.StatusBadGateway, "couldn't ask plex.tv: "+err.Error())
		return
	case token == "":
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "pending"})
		return
	}

	user, err := c.User(r.Context(), token)
	if err != nil {
		httpx.WriteErr(w, http.StatusBadGateway, "couldn't ask plex.tv who signed in: "+err.Error())
		return
	}
	a.forget(in.PinID) // a sign-in can be completed once

	if add {
		a.addAccount(w, user)
		return
	}
	a.signIn(w, r, user)
}

func (a *Auth) forget(id int64) {
	a.mu.Lock()
	delete(a.pins, id)
	a.mu.Unlock()
}

func (a *Auth) signIn(w http.ResponseWriter, r *http.Request, user User) {
	acct, ok := a.cfg.Settings().PlexAdmin(user.ID)
	if !ok {
		a.log.Warn("a Plex account that is not allowed tried to sign in", "name", user.Name, "id", user.ID)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "denied", "user": accountDoc{Name: user.Name}})
		return
	}
	id := a.sess.create(acct)
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: id, Path: CookiePath, HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		MaxAge: int(a.cfg.SessionMax.Seconds()),
	})
	a.log.Info("signed in with Plex", "name", acct.Name, "id", acct.ID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "user": accountDoc{ID: acct.ID, Name: acct.Name}})
}

func (a *Auth) addAccount(w http.ResponseWriter, user User) {
	if a.cfg.Edit == nil {
		httpx.WriteErr(w, http.StatusConflict, access.ErrNoFile.Error())
		return
	}
	acct := access.PlexAccount{ID: user.ID, Name: user.Name}
	_, err := a.cfg.Edit(func(s *access.Settings) error {
		if s.Plex == nil {
			s.Plex = &access.PlexSettings{}
		}
		for i, existing := range s.Plex.Admins {
			if existing.ID == acct.ID {
				s.Plex.Admins[i].Name = acct.Name // refresh the name they have now
				return nil
			}
		}
		s.Plex.Admins = append(s.Plex.Admins, acct)
		return nil
	})
	if err != nil {
		var ve *access.ValidationError
		switch {
		case errors.Is(err, access.ErrNoFile):
			httpx.WriteErr(w, http.StatusConflict, err.Error())
		case errors.As(err, &ve):
			httpx.WriteErr(w, http.StatusBadRequest, ve.Error())
		default:
			a.log.Error("could not save the Plex account", "err", err)
			httpx.WriteErr(w, http.StatusInternalServerError, "could not save the settings")
		}
		return
	}
	a.log.Info("a Plex account was allowed to sign in", "name", acct.Name, "id", acct.ID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "account": accountDoc{ID: acct.ID, Name: acct.Name}})
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		a.sess.delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: CookiePath, MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"signedIn": nil})
}

// compile-time check that the context package is used by callers' signatures.
var _ = context.Background
