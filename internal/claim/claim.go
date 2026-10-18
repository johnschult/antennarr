// Package claim is the first-run step that makes someone the admin.
//
// A new install has no viewers and no admin, so nobody can use the tuner and
// only this machine can open the admin page. To let the owner in from another
// computer without opening anything up, Antennarr makes a one-time token when
// it starts and writes a link containing it to its log. Whoever can read the
// log (so, the owner) opens the link, confirms, and the address they came from
// is saved under "admin".
//
// The token lives in memory only, works once, and a new one is made at each
// start while nobody has claimed yet.
package claim

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/johnschult/antennarr/internal/access"
)

// Path is where the claim page lives.
const Path = "/antennarr/claim"

const (
	maxFailures = 5                // wrong tries allowed from one address...
	failWindow  = 10 * time.Minute // ...within this long
)

// Config is what a Claimer needs.
type Config struct {
	Policy *access.Policy
	Store  *access.Store
	Log    *slog.Logger
	Now    func() time.Time // time.Now unless a test supplies another
}

// Claimer holds the one-time token and serves the claim page.
type Claimer struct {
	cfg Config
	log *slog.Logger

	mu    sync.Mutex
	token string                 // "" once used, or if there was never anything to claim
	fails map[string][]time.Time // wrong tries, by client address
}

// New makes the token if there is something to claim: nobody is an admin yet
// and the settings can be saved (an admin address has to be written
// somewhere).
func New(cfg Config) *Claimer {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	c := &Claimer{cfg: cfg, log: cfg.Log, fails: map[string][]time.Time{}}
	if !cfg.Policy.HasAdmin() && cfg.Store.Editable() {
		c.token = newToken()
	}
	return c
}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("claim: no randomness: " + err.Error()) // nothing safe to do without it
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Token is the token to put in the link, or "" if there is nothing to claim.
func (c *Claimer) Token() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token
}

// Link is the path and query of the claim link for token.
func Link(token string) string {
	return Path + "?token=" + url.QueryEscape(token)
}

// Handler serves GET (a page asking for confirmation, which changes nothing, so
// a link previewer that fetches it can't use the token up) and POST (the
// confirmation).
func (c *Claimer) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			c.page(w, r)
		case http.MethodPost:
			c.confirm(w, r)
		default:
			w.Header().Set("Allow", "GET, HEAD, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

// check says whether this request may claim, and why not if it may not. A
// refusal for a wrong or missing token, or for there being nothing to claim,
// is the same 404, so the page doesn't say whether a claim is open.
func (c *Claimer) check(r *http.Request, token string) (ip string, status int, msg string) {
	addr, ok := access.ClientAddr(r)
	if !ok {
		return "", http.StatusNotFound, "not found"
	}
	ip = addr.String()
	// Only from the owner's own networks: the setup link isn't for the internet.
	if !access.PrivateAddr(addr) {
		return ip, http.StatusNotFound, "not found"
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.cfg.Now()
	recent := c.fails[ip][:0:0]
	for _, t := range c.fails[ip] {
		if now.Sub(t) < failWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= maxFailures {
		c.fails[ip] = recent
		return ip, http.StatusTooManyRequests, "too many wrong tries; wait a few minutes"
	}
	if c.token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(c.token)) != 1 {
		c.fails[ip] = append(recent, now)
		return ip, http.StatusNotFound, "not found"
	}
	delete(c.fails, ip)
	return ip, 0, ""
}

func (c *Claimer) page(w http.ResponseWriter, r *http.Request) {
	ip, status, msg := c.check(r, r.URL.Query().Get("token"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if addr, _ := access.ClientAddr(r); addr.IsLoopback() {
		http.Error(w, "This machine is already an admin. Open the link from the device you want to use as the admin.", http.StatusBadRequest)
		return
	}
	setPageHeaders(w)
	fmt.Fprintf(w, `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Set up Antennarr</title>
<style>body{font:16px system-ui,sans-serif;max-width:32rem;margin:3rem auto;padding:0 1rem;line-height:1.5}button{font:inherit;padding:.6rem 1rem}code{background:#8882;padding:.1rem .3rem;border-radius:.25rem}</style>
<h1>Set up Antennarr</h1>
<p>Make this device an admin? Its address, <code>%s</code>, will be saved, and anything that connects from it will be able to change Antennarr's settings and viewers.</p>
<form method="post" action="%s">
<input type="hidden" name="token" value="%s">
<button type="submit">Make this device an admin</button>
</form>
<p>If you didn't ask for this, close the page. Nothing has been changed.</p>
</html>`, html.EscapeString(ip), Path, html.EscapeString(r.URL.Query().Get("token")))
}

func (c *Claimer) confirm(w http.ResponseWriter, r *http.Request) {
	// A form on another site can't know the token, but check where it came from
	// anyway, as the admin API does.
	if origin := r.Header.Get("Origin"); origin != "" {
		if u, err := url.Parse(origin); err != nil || u.Host != r.Host {
			http.Error(w, "cross-origin requests are not allowed", http.StatusForbidden)
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ip, status, msg := c.check(r, r.PostForm.Get("token"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	addr, _ := access.ClientAddr(r)
	if addr.IsLoopback() {
		http.Error(w, "This machine is already an admin. Open the link from the device you want to use as the admin.", http.StatusBadRequest)
		return
	}

	// Hold the lock across the save so two confirmations can't both win.
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_, err := c.cfg.Store.Edit(func(s *access.Settings) error {
		for _, a := range s.Admin {
			if a == ip {
				return nil
			}
		}
		s.Admin = append(s.Admin, ip)
		return nil
	})
	if err != nil {
		if errors.Is(err, access.ErrNoFile) {
			http.Error(w, "settings can't be saved, so nothing was changed", http.StatusServiceUnavailable)
			return
		}
		c.log.Error("claim: couldn't save the admin address", "err", err)
		http.Error(w, "couldn't save the settings; see the Antennarr log", http.StatusInternalServerError)
		return
	}
	c.token = ""
	c.log.Info("claimed: this address is now an admin", "address", ip)
	http.Redirect(w, r, "/antennarr/", http.StatusSeeOther)
}

func setPageHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer") // the token is in the address
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
}

// URLs returns the claim link for each address the machine can be reached at.
func URLs(token, port string, hosts []string) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, "http://"+net.JoinHostPort(h, port)+Link(token))
	}
	return out
}
