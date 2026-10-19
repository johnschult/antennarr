package plexauth

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/access"
)

// ---- harness ----

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

type logBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuf) Write(p []byte) (int, error) { l.mu.Lock(); defer l.mu.Unlock(); return l.b.Write(p) }
func (l *logBuf) String() string              { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

// rig is Yagi's side of the flow: an Auth, a settings store it edits, a
// fake plex.tv, and a browser (with a cookie jar) talking to it.
type rig struct {
	t      *testing.T
	auth   *Auth
	tv     *fakePlexTV
	clock  *clock
	logs   *logBuf
	srv    *httptest.Server
	client *http.Client

	mu       sync.Mutex
	settings access.Settings
	admin    bool // what IsAdmin answers for the next requests
	editErr  error
	edits    int
}

func newRig(t *testing.T, mut ...func(*Config)) *rig {
	t.Helper()
	r := &rig{t: t, tv: newFakePlexTV(t), clock: &clock{t: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}, logs: &logBuf{}}
	r.settings = access.Settings{Plex: &access.PlexSettings{Enabled: true, Admins: []access.PlexAccount{{ID: 7, Name: "john"}}}}
	cfg := Config{
		Base:     r.tv.URL,
		Settings: r.get,
		Edit:     r.edit,
		IsAdmin:  func(*http.Request) bool { r.mu.Lock(); defer r.mu.Unlock(); return r.admin },
		Now:      r.clock.now,
		Log:      slog.New(slog.NewTextHandler(r.logs, nil)),
	}
	for _, m := range mut {
		m(&cfg)
	}
	r.auth = New(cfg)
	r.srv = httptest.NewServer(r.auth.Handler())
	t.Cleanup(r.srv.Close)
	jar, _ := cookiejar.New(nil)
	r.client = &http.Client{Jar: jar}
	return r
}

func (r *rig) get() access.Settings { r.mu.Lock(); defer r.mu.Unlock(); return r.settings.Clone() }

func (r *rig) edit(fn func(*access.Settings) error) (access.Settings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.editErr != nil {
		return r.settings, r.editErr
	}
	cur := r.settings.Clone()
	if err := fn(&cur); err != nil {
		return r.settings, err
	}
	if err := cur.Validate(); err != nil {
		return r.settings, &access.ValidationError{Err: err}
	}
	r.settings = cur
	r.edits++
	return cur, nil
}

func (r *rig) set(f func(*access.Settings)) { r.mu.Lock(); f(&r.settings); r.mu.Unlock() }
func (r *rig) setAdmin(b bool)              { r.mu.Lock(); r.admin = b; r.mu.Unlock() }

// post sends a JSON POST and decodes the JSON reply.
func (r *rig) post(path, body string, hdr ...string) (int, map[string]any, *http.Response) {
	r.t.Helper()
	req, _ := http.NewRequest("POST", r.srv.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	return r.do(req)
}

func (r *rig) getJSON(path string) (int, map[string]any, *http.Response) {
	r.t.Helper()
	req, _ := http.NewRequest("GET", r.srv.URL+path, nil)
	return r.do(req)
}

func (r *rig) do(req *http.Request) (int, map[string]any, *http.Response) {
	r.t.Helper()
	resp, err := r.client.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			r.t.Fatalf("%s %s: not JSON: %s", req.Method, req.URL.Path, raw)
		}
	}
	return resp.StatusCode, m, resp
}

// begin starts a login and returns the PIN id.
func (r *rig) begin(prefix string) int64 {
	r.t.Helper()
	code, m, _ := r.post("/yagi/auth/plex"+prefix+"/start", `{}`)
	if code != 200 {
		r.t.Fatalf("start: %d %v", code, m)
	}
	return int64(m["pinId"].(float64))
}

func (r *rig) poll(prefix string, pin int64) (int, map[string]any, *http.Response) {
	r.t.Helper()
	body, _ := json.Marshal(map[string]int64{"pinId": pin})
	return r.post("/yagi/auth/plex"+prefix+"/poll", string(body))
}

// pollOK polls until the status leaves "pending" (moving the clock past the
// minimum gap between polls each time).
func (r *rig) pollDone(prefix string, pin int64) (int, map[string]any, *http.Response) {
	r.t.Helper()
	for i := 0; i < 5; i++ {
		r.clock.add(2 * time.Second)
		code, m, resp := r.poll(prefix, pin)
		if m["status"] != "pending" {
			return code, m, resp
		}
	}
	r.t.Fatal("still pending")
	return 0, nil, nil
}

func (r *rig) status() map[string]any {
	r.t.Helper()
	_, m, _ := r.getJSON("/yagi/auth/status")
	return m
}

// signedIn runs a whole successful login as john.
func (r *rig) signedIn() {
	r.t.Helper()
	pin := r.begin("")
	r.tv.signIn(pin, "SECRET-TOKEN-1", fakeUser{ID: 7, Username: "john"})
	if code, m, _ := r.pollDone("", pin); code != 200 || m["status"] != "ok" {
		r.t.Fatalf("login: %d %v", code, m)
	}
}

func (r *rig) isAdmin() bool {
	req := httptest.NewRequest("GET", "/x", nil)
	for _, c := range r.client.Jar.Cookies(mustURL(r.srv.URL + "/yagi/")) {
		req.AddCookie(c)
	}
	return r.auth.IsSignedInAdmin(req)
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
