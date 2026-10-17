package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/access"
)

// plexTV is a small stand-in for plex.tv: PINs that a test can approve.
type plexTV struct {
	*httptest.Server
	mu    sync.Mutex
	next  int64
	codes map[int64]string
	token map[int64]string
	users map[string]struct {
		id   int64
		name string
	}
}

func newPlexTV(t *testing.T) *plexTV {
	t.Helper()
	p := &plexTV{next: 500, codes: map[int64]string{}, token: map[int64]string{}, users: map[string]struct {
		id   int64
		name string
	}{}}
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v2/pins":
			p.next++
			p.codes[p.next] = "code-" + strconv.FormatInt(p.next, 10)
			fmt.Fprintf(w, `{"id":%d,"code":%q,"authToken":null,"expiresAt":%q}`, p.next, p.codes[p.next], time.Now().Add(30*time.Minute).UTC().Format(time.RFC3339))
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v2/pins/"):
			id, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/v2/pins/"), 10, 64)
			if _, ok := p.codes[id]; !ok {
				http.NotFound(w, r)
				return
			}
			if tok := p.token[id]; tok != "" {
				fmt.Fprintf(w, `{"id":%d,"authToken":%q}`, id, tok)
			} else {
				fmt.Fprintf(w, `{"id":%d,"authToken":null}`, id)
			}
		case r.Method == "GET" && r.URL.Path == "/api/v2/user":
			u, ok := p.users[r.Header.Get("X-Plex-Token")]
			if !ok {
				http.Error(w, "no", http.StatusUnauthorized)
				return
			}
			fmt.Fprintf(w, `{"id":%d,"username":%q}`, u.id, u.name)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.Close)
	return p
}

func (p *plexTV) approve(pin int64, token string, id int64, name string) {
	p.mu.Lock()
	p.token[pin] = token
	p.users[token] = struct {
		id   int64
		name string
	}{id, name}
	p.mu.Unlock()
}

// site is Antennarr's handler, called directly so requests can come from any address.
type site struct {
	t        *testing.T
	srv      *Server
	tv       *plexTV
	cookie   *http.Cookie
	path     string // the settings file, if any
	remote   string
	lastBody string
}

func newSite(t *testing.T, s access.Settings, withFile bool) *site {
	t.Helper()
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	st := &site{t: t, tv: newPlexTV(t), remote: "203.0.113.9:4000"}
	off := false
	s.FirmwareCheck = &off
	if withFile {
		st.path = filepath.Join(t.TempDir(), "antennarr.json")
		if err := access.SaveSettings(st.path, s); err != nil {
			t.Fatal(err)
		}
	}
	srv, err := New(context.Background(), Config{
		Settings: s, Devices: oneDevice(a), ConfigPath: st.path, Locate: locateFrom(a),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), PlexURL: st.tv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	st.srv = srv
	return st
}

func (st *site) do(method, path, body string, from string) (int, string, *http.Response) {
	st.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.RemoteAddr = from
	if st.cookie != nil && from == st.remote {
		req.AddCookie(st.cookie)
	}
	rec := httptest.NewRecorder()
	st.srv.devices[0].handler.ServeHTTP(rec, req)
	resp := rec.Result()
	for _, c := range resp.Cookies() {
		if c.Name == "antennarr_session" && from == st.remote {
			if c.MaxAge < 0 {
				st.cookie = nil
			} else {
				st.cookie = c
			}
		}
	}
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp
}

func (st *site) here(method, path, body string) (int, string) {
	st.t.Helper()
	c, b, _ := st.do(method, path, body, st.remote)
	return c, b
}

// signIn runs a whole login as the given Plex account and returns the final status.
func (st *site) signIn(token string, id int64, name string) string {
	st.t.Helper()
	code, body := st.here("POST", "/antennarr/auth/plex/start", `{}`)
	if code != 200 {
		st.t.Fatalf("start: %d %s", code, body)
	}
	var m struct{ PinID int64 }
	json.Unmarshal([]byte(strings.Replace(body, "pinId", "PinID", 1)), &m)
	st.tv.approve(m.PinID, token, id, name)
	// polls are throttled to one a second; wait it out
	for i := 0; i < 5; i++ {
		time.Sleep(1100 * time.Millisecond)
		code, body = st.here("POST", "/antennarr/auth/plex/poll", fmt.Sprintf(`{"pinId":%d}`, m.PinID))
		if !strings.Contains(body, "pending") {
			return body
		}
	}
	st.t.Fatal("still pending")
	return ""
}

var enforced = access.Settings{
	Viewers: []access.Viewer{{Name: "john", Addresses: []string{"100.67.117.32"}, Limit: 1}},
	Plex:    &access.PlexSettings{Enabled: true, Admins: []access.PlexAccount{{ID: 7, Name: "john"}}},
}

func TestSigningInWithPlexOpensTheAdminPagesFromAnyAddress(t *testing.T) {
	st := newSite(t, enforced, true)

	// A stranger's address: refused, but told that Plex sign-in is the way in.
	if code, _ := st.here("GET", "/antennarr/api/state", ""); code != 403 {
		t.Fatalf("before signing in: %d", code)
	}
	if _, body := st.here("GET", "/antennarr/auth/status", ""); !strings.Contains(body, `"plexEnabled":true`) || !strings.Contains(body, `"admin":false`) {
		t.Errorf("status = %s", body)
	}

	if body := st.signIn("tok-1", 7, "john"); !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("sign-in: %s", body)
	}
	for _, path := range []string{"/antennarr/api/state", "/antennarr/streams", "/antennarr/"} {
		if code, _ := st.here("GET", path, ""); code != 200 && code != 503 { // 503: the UI isn't built in this checkout
			t.Errorf("%s after signing in: %d", path, code)
		}
	}
	if code, _, resp := st.do("GET", "/", "", st.remote); code != 302 || resp.Header.Get("Location") != "/antennarr/" {
		t.Errorf("the bare address should go to the admin page: %d %q", code, resp.Header.Get("Location"))
	}
	if _, body := st.here("GET", "/antennarr/auth/status", ""); !strings.Contains(body, `"admin":true`) || !strings.Contains(body, `"name":"john"`) {
		t.Errorf("status after = %s", body)
	}

	// It is the admin pages that open, not the tuners: a Plex login is not a viewer.
	if code, _ := st.here("GET", "/discover.json", ""); code != 403 {
		t.Errorf("a signed-in admin can use the tuner as a viewer: %d", code)
	}
	// And the cookie is only sent to /antennarr/, so the tuner endpoints never see it.
	if st.cookie.Path != "/antennarr/" {
		t.Errorf("cookie path = %q", st.cookie.Path)
	}
}

func TestAnAccountThatIsNotAllowedGetsNowhere(t *testing.T) {
	st := newSite(t, enforced, true)
	if body := st.signIn("tok-x", 99, "stranger"); !strings.Contains(body, `"status":"denied"`) {
		t.Fatalf("%s", body)
	}
	if code, _ := st.here("GET", "/antennarr/api/state", ""); code != 403 {
		t.Errorf("a denied account reached the admin pages: %d", code)
	}
}

func TestRemovingTheAccountLocksItOutOnTheNextRequest(t *testing.T) {
	st := newSite(t, access.Settings{
		Viewers: enforced.Viewers,
		Plex:    &access.PlexSettings{Enabled: true, Admins: []access.PlexAccount{{ID: 7, Name: "john"}, {ID: 9, Name: "steve"}}},
	}, true)
	st.signIn("tok-1", 7, "john")
	if code, _ := st.here("GET", "/antennarr/api/state", ""); code != 200 {
		t.Fatal("setup")
	}

	// An admin signed in with Plex can remove accounts, including their own...
	if code, body := st.here("DELETE", "/antennarr/api/plex/admins/7", ""); code != 204 {
		t.Fatalf("removing: %d %s", code, body)
	}
	// ...and the very next request is refused.
	if code, _ := st.here("GET", "/antennarr/api/state", ""); code != 403 {
		t.Errorf("still in after being removed: %d", code)
	}
}

func TestSwitchingPlexSignInOffLocksItOut(t *testing.T) {
	st := newSite(t, enforced, true)
	st.signIn("tok-1", 7, "john")
	if code, body := st.here("PATCH", "/antennarr/api/settings", `{"plexEnabled":false}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if code, _ := st.here("GET", "/antennarr/api/state", ""); code != 403 {
		t.Errorf("signed in with Plex while it is off: %d", code)
	}
	if code, _ := st.here("POST", "/antennarr/auth/plex/start", `{}`); code != 404 {
		t.Errorf("starting a sign-in while off: %d", code)
	}
}

func TestAnAddressAdminCanAddThePlexAccountThatThenSignsIn(t *testing.T) {
	st := newSite(t, access.Settings{Viewers: enforced.Viewers, Plex: &access.PlexSettings{Enabled: true}}, true)
	loopback := "127.0.0.1:5000" // an admin by address

	code, body, _ := st.do("POST", "/antennarr/auth/plex/add/start", `{}`, loopback)
	if code != 200 {
		t.Fatalf("add/start: %d %s", code, body)
	}
	var m struct{ PinID int64 }
	json.Unmarshal([]byte(strings.Replace(body, "pinId", "PinID", 1)), &m)
	st.tv.approve(m.PinID, "tok-steve", 42, "steve")
	time.Sleep(1100 * time.Millisecond)
	code, body, _ = st.do("POST", "/antennarr/auth/plex/add/poll", fmt.Sprintf(`{"pinId":%d}`, m.PinID), loopback)
	if code != 200 || !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("add/poll: %d %s", code, body)
	}

	// The account is in the settings file, so it survives a restart...
	got, _, err := access.LoadSettings(st.path)
	if err != nil || len(got.Plex.Admins) != 1 || got.Plex.Admins[0].ID != 42 {
		t.Fatalf("file: %+v %v", got.Plex, err)
	}
	if got.Plex.ClientID == "" {
		t.Error("the client ID should have been saved with it")
	}
	// ...and now steve can sign in from somewhere else.
	if body := st.signIn("tok-steve-2", 42, "steve"); !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("steve could not sign in: %s", body)
	}
	if code, _ := st.here("GET", "/antennarr/api/state", ""); code != 200 {
		t.Errorf("steve after signing in: %d", code)
	}
}

func TestAStrangerCannotAddAnAccount(t *testing.T) {
	st := newSite(t, access.Settings{Viewers: enforced.Viewers, Plex: &access.PlexSettings{Enabled: true}}, true)
	if code, _ := st.here("POST", "/antennarr/auth/plex/add/start", `{}`); code != 403 {
		t.Errorf("a stranger started adding an account: %d", code)
	}
}

func TestPlexSignInDoesNothingWhenTheSettingIsOff(t *testing.T) {
	st := newSite(t, access.Settings{Viewers: enforced.Viewers}, true) // never configured
	if _, body := st.here("GET", "/antennarr/auth/status", ""); !strings.Contains(body, `"plexEnabled":false`) {
		t.Errorf("status = %s", body)
	}
	if code, _ := st.here("GET", "/antennarr/api/state", ""); code != 403 {
		t.Errorf("open without a reason: %d", code)
	}
}

func TestConfigFileSurvivesPlexEdits(t *testing.T) {
	st := newSite(t, enforced, true)
	st.signIn("tok-1", 7, "john")
	st.here("PATCH", "/antennarr/api/settings", `{"sharedCap":2}`)
	raw, _ := os.ReadFile(st.path)
	if !strings.Contains(string(raw), `"clientId"`) || !strings.Contains(string(raw), `"admins"`) || !strings.Contains(string(raw), `"sharedCap": 2`) {
		t.Errorf("file lost something:\n%s", raw)
	}
}
