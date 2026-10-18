package claim

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/access"
)

type rig struct {
	pol  *access.Policy
	st   *access.Store
	c    *Claimer
	path string
	now  time.Time
}

func newRig(t *testing.T, s access.Settings, withFile bool) *rig {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	pol, err := access.NewPolicy(s, log)
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{pol: pol, now: time.Date(2026, 10, 18, 5, 0, 0, 0, time.UTC)}
	if withFile {
		r.path = filepath.Join(t.TempDir(), "antennarr.json")
		if err := access.SaveSettings(r.path, s); err != nil {
			t.Fatal(err)
		}
	}
	r.st = access.NewStore(r.path, pol)
	r.c = New(Config{Policy: pol, Store: r.st, Log: log, Now: func() time.Time { return r.now }})
	return r
}

func (r *rig) get(remote, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, Link(token), nil)
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	r.c.Handler().ServeHTTP(rec, req)
	return rec
}

func (r *rig) post(remote, token string, hdr map[string]string) *httptest.ResponseRecorder {
	form := url.Values{"token": {token}}
	req := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(form.Encode()))
	req.RemoteAddr = remote
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.c.Handler().ServeHTTP(rec, req)
	return rec
}

func (r *rig) admins() []string { return r.pol.Settings().Admin }

func TestClaimSavesTheAddressAsAdminAndBurnsTheToken(t *testing.T) {
	r := newRig(t, access.DefaultSettings(), true)
	tok := r.c.Token()
	if len(tok) < 40 {
		t.Fatalf("token %q is too short to be safe", tok)
	}

	rec := r.post("192.168.1.20:5555", tok, nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/antennarr/" {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Location"))
	}
	if got := r.admins(); len(got) != 1 || got[0] != "192.168.1.20" {
		t.Errorf("admin = %v", got)
	}
	// It reached the file, so it survives a restart.
	saved, _, err := access.LoadSettings(r.path)
	if err != nil || len(saved.Admin) != 1 || saved.Admin[0] != "192.168.1.20" {
		t.Errorf("saved admin = %v, %v", saved.Admin, err)
	}
	if !r.pol.HasAdmin() {
		t.Error("the install should now count as claimed")
	}

	// One use only.
	if r.c.Token() != "" {
		t.Error("the token survived being used")
	}
	if rec := r.post("192.168.1.21:1", tok, nil); rec.Code != http.StatusNotFound {
		t.Errorf("a used token gave %d, want 404", rec.Code)
	}
	if got := r.admins(); len(got) != 1 {
		t.Errorf("a second claim changed the admins: %v", got)
	}
}

// A page that only shows the confirmation must not use the token up, or a link
// previewer fetching it would.
func TestOpeningTheLinkChangesNothing(t *testing.T) {
	r := newRig(t, access.DefaultSettings(), true)
	tok := r.c.Token()
	rec := r.get("192.168.1.20:5555", tok)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "192.168.1.20") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Referrer-Policy"), "same-origin") ||
		!strings.Contains(rec.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Errorf("the page needs a same-origin referrer policy (the token is in the address) and a strict CSP: %v", rec.Header())
	}
	if len(r.admins()) != 0 || r.c.Token() != tok {
		t.Error("opening the page changed something")
	}
	if rec := r.post("192.168.1.20:5555", tok, nil); rec.Code != http.StatusSeeOther {
		t.Errorf("confirming after opening: %d", rec.Code)
	}
}

func TestWrongOrMissingTokenIsA404(t *testing.T) {
	r := newRig(t, access.DefaultSettings(), true)
	for _, tok := range []string{"", "nope", r.c.Token() + "x", r.c.Token()[1:]} {
		if rec := r.get("192.168.1.20:1", tok); rec.Code != http.StatusNotFound {
			t.Errorf("GET with %q: %d, want 404", tok, rec.Code)
		}
	}
	if len(r.admins()) != 0 {
		t.Error("a wrong token made someone an admin")
	}
}

func TestTooManyWrongTriesLocksOutThatAddressOnly(t *testing.T) {
	r := newRig(t, access.DefaultSettings(), true)
	tok := r.c.Token()
	for i := 0; i < maxFailures; i++ {
		r.post("192.168.1.20:1", "wrong", nil)
	}
	if rec := r.post("192.168.1.20:1", tok, nil); rec.Code != http.StatusTooManyRequests {
		t.Errorf("the right token after %d wrong ones from the same address: %d, want 429", maxFailures, rec.Code)
	}
	if len(r.admins()) != 0 {
		t.Error("a locked-out address claimed")
	}

	// Another address isn't affected, and the lock-out ends.
	r.now = r.now.Add(failWindow + time.Second)
	if rec := r.post("192.168.1.20:1", tok, nil); rec.Code != http.StatusSeeOther {
		t.Errorf("after the window: %d, want 303", rec.Code)
	}
}

func TestOnlyPrivateAddressesCanClaim(t *testing.T) {
	r := newRig(t, access.DefaultSettings(), true)
	tok := r.c.Token()
	for _, remote := range []string{"203.0.113.9:1", "8.8.8.8:1", "[2001:db8::1]:1", "not-an-address"} {
		if rec := r.post(remote, tok, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", remote, rec.Code)
		}
		if rec := r.get(remote, tok); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404", remote, rec.Code)
		}
	}
	if len(r.admins()) != 0 || r.c.Token() != tok {
		t.Error("a public address affected the claim")
	}
	// Tailscale's range counts as private.
	if rec := r.post("100.64.0.7:1", tok, nil); rec.Code != http.StatusSeeOther {
		t.Errorf("a Tailscale address: %d, want 303", rec.Code)
	}
}

func TestLoopbackIsToldItIsAlreadyAdmin(t *testing.T) {
	r := newRig(t, access.DefaultSettings(), true)
	tok := r.c.Token()
	if rec := r.get("127.0.0.1:1", tok); rec.Code != http.StatusBadRequest {
		t.Errorf("GET from loopback: %d, want 400", rec.Code)
	}
	if rec := r.post("127.0.0.1:1", tok, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("POST from loopback: %d, want 400", rec.Code)
	}
	if r.c.Token() != tok || len(r.admins()) != 0 {
		t.Error("loopback used up the claim")
	}
}

func TestCrossOriginPostIsRefused(t *testing.T) {
	r := newRig(t, access.DefaultSettings(), true)
	rec := r.post("192.168.1.20:1", r.c.Token(), map[string]string{"Origin": "http://evil.example"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("%d, want 403", rec.Code)
	}
	rec = r.post("192.168.1.20:1", r.c.Token(), map[string]string{"Origin": "http://example.com"}) // httptest's Host
	if rec.Code != http.StatusSeeOther {
		t.Errorf("same-origin: %d, want 303", rec.Code)
	}
}

func TestNothingToClaim(t *testing.T) {
	t.Run("an admin already exists", func(t *testing.T) {
		s := access.DefaultSettings()
		s.Admin = []string{"100.64.0.1"}
		r := newRig(t, s, true)
		if r.c.Token() != "" {
			t.Error("a claimed install made a token")
		}
		if rec := r.post("192.168.1.20:1", "anything", nil); rec.Code != http.StatusNotFound {
			t.Errorf("%d, want 404", rec.Code)
		}
	})
	t.Run("there is nowhere to save the admin", func(t *testing.T) {
		r := newRig(t, access.DefaultSettings(), false)
		if r.c.Token() != "" {
			t.Error("a read-only install made a token")
		}
	})
}

func TestOtherMethods(t *testing.T) {
	r := newRig(t, access.DefaultSettings(), true)
	req := httptest.NewRequest(http.MethodDelete, Path, nil)
	req.RemoteAddr = "192.168.1.20:1"
	rec := httptest.NewRecorder()
	r.c.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("%d, want 405", rec.Code)
	}
}

func TestTokensAreDifferentEachTime(t *testing.T) {
	a := newRig(t, access.DefaultSettings(), true).c.Token()
	b := newRig(t, access.DefaultSettings(), true).c.Token()
	if a == "" || a == b {
		t.Errorf("tokens %q and %q", a, b)
	}
}

func TestURLs(t *testing.T) {
	got := URLs("abc_-", "5004", []string{"100.64.0.5", "192.168.1.5", "fd7a:115c:a1e0::5"})
	want := []string{
		"http://100.64.0.5:5004/antennarr/claim?token=abc_-",
		"http://192.168.1.5:5004/antennarr/claim?token=abc_-",
		"http://[fd7a:115c:a1e0::5]:5004/antennarr/claim?token=abc_-",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: %q, want %q", i, got[i], want[i])
		}
	}
}

// A browser sends "Origin: null" when a form is posted from a page whose
// referrer policy is no-referrer, which the setup page used to have. That made
// the real button fail with "cross-origin requests are not allowed".
func TestTheSetupPageCanBePostedFromTheBrowser(t *testing.T) {
	r := newRig(t, access.DefaultSettings(), true)
	page := r.get("192.168.1.20:1", r.c.Token())
	if p := page.Header().Get("Referrer-Policy"); p == "no-referrer" || p == "" {
		t.Errorf("the page's Referrer-Policy is %q: browsers then send Origin: null on its form", p)
	}

	// A null origin counts only when the browser says the request is same-origin.
	rec := r.post("192.168.1.20:1", r.c.Token(), map[string]string{"Origin": "null"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("Origin null with no Sec-Fetch-Site: %d, want 403", rec.Code)
	}
	rec = r.post("192.168.1.20:1", r.c.Token(), map[string]string{"Origin": "null", "Sec-Fetch-Site": "cross-site"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("Origin null, cross-site: %d, want 403", rec.Code)
	}
	rec = r.post("192.168.1.20:1", r.c.Token(), map[string]string{"Origin": "null", "Sec-Fetch-Site": "same-origin"})
	if rec.Code != http.StatusSeeOther {
		t.Errorf("Origin null, same-origin: %d, want 303", rec.Code)
	}
}
