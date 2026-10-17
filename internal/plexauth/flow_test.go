package plexauth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/access"
)

const secretToken = "SECRET-TOKEN-1"

func TestItDoesNothingUntilTurnedOn(t *testing.T) {
	r := newRig(t)
	r.set(func(s *access.Settings) { s.Plex = nil }) // never configured

	if st := r.status(); st["plexEnabled"] != false || st["signedIn"] != nil {
		t.Errorf("status = %v", st)
	}
	for _, path := range []string{"/antennarr/auth/plex/start", "/antennarr/auth/plex/poll", "/antennarr/auth/plex/add/start", "/antennarr/auth/plex/add/poll"} {
		if code, m, _ := r.post(path, `{"pinId":1}`); code != 404 || !strings.Contains(m["error"].(string), "turned off") {
			t.Errorf("%s: %d %v", path, code, m)
		}
	}
	r.set(func(s *access.Settings) {
		s.Plex = &access.PlexSettings{Enabled: false, Admins: []access.PlexAccount{{ID: 7, Name: "john"}}}
	})
	if code, _, _ := r.post("/antennarr/auth/plex/start", `{}`); code != 404 {
		t.Errorf("configured but off: %d", code)
	}
	if n := len(r.tv.calls); n != 0 {
		t.Errorf("plex.tv was contacted %d times while the feature was off: %v", n, r.tv.calls)
	}
}

func TestSigningInWithAnAllowedAccount(t *testing.T) {
	r := newRig(t)

	code, m, _ := r.post("/antennarr/auth/plex/start", `{}`)
	if code != 200 {
		t.Fatalf("start: %d %v", code, m)
	}
	pin := int64(m["pinId"].(float64))
	link := m["authUrl"].(string)
	if !strings.HasPrefix(link, "https://app.plex.tv/auth#?") || !strings.Contains(link, "code1001-") {
		t.Errorf("authUrl = %s", link)
	}

	// Nobody has approved it yet.
	r.clock.add(2 * time.Second)
	if code, m, _ := r.poll("", pin); code != 200 || m["status"] != "pending" {
		t.Errorf("poll before approval: %d %v", code, m)
	}
	if r.isAdmin() {
		t.Fatal("admin before signing in")
	}

	r.tv.signIn(pin, secretToken, fakeUser{ID: 7, Username: "john"})
	code, m, resp := r.pollDone("", pin)
	if code != 200 || m["status"] != "ok" {
		t.Fatalf("poll after approval: %d %v", code, m)
	}
	if u := m["user"].(map[string]any); u["name"] != "john" || u["id"] != float64(7) {
		t.Errorf("user = %v", u)
	}

	// The cookie is locked down and only goes to the admin pages.
	var c *http.Cookie
	for _, x := range resp.Cookies() {
		if x.Name == CookieName {
			c = x
		}
	}
	if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/antennarr/" || c.Secure || c.MaxAge != 7*24*3600 || len(c.Value) < 40 {
		t.Fatalf("cookie = %+v", c)
	}
	if !r.isAdmin() {
		t.Error("not an admin after signing in")
	}
	st := r.status()
	if si, _ := st["signedIn"].(map[string]any); si == nil || si["name"] != "john" {
		t.Errorf("status = %v", st)
	}
}

func TestThePlexTokenIsNeverKeptOrRevealed(t *testing.T) {
	r := newRig(t)
	pin := r.begin("")
	r.tv.signIn(pin, secretToken, fakeUser{ID: 7, Username: "john"})

	var everything strings.Builder
	for i := 0; i < 3; i++ {
		r.clock.add(2 * time.Second)
		_, m, resp := r.poll("", pin)
		everything.WriteString(toString(m))
		everything.WriteString(resp.Header.Get("Set-Cookie"))
		for _, c := range resp.Cookies() {
			everything.WriteString(c.Name + "=" + c.Value)
		}
	}
	everything.WriteString(toString(r.status()))
	everything.WriteString(r.logs.String())
	if strings.Contains(everything.String(), secretToken) {
		t.Errorf("the token appeared in a response, a cookie or the logs:\n%s", everything.String())
	}
	if strings.Contains(everything.String(), "secret@example.com") {
		t.Error("the user's email was kept or sent on")
	}
	// Nor is it held in memory for later: the session knows only who signed in.
	for _, ss := range r.auth.sess.m {
		if strings.Contains(ss.user.Name+toString(ss.user), secretToken) {
			t.Error("the session holds the token")
		}
	}
}

func TestAnAccountThatIsNotAllowedIsDenied(t *testing.T) {
	r := newRig(t)
	pin := r.begin("")
	r.tv.signIn(pin, "other-token", fakeUser{ID: 99, Username: "stranger"})

	code, m, resp := r.pollDone("", pin)
	if code != 200 || m["status"] != "denied" {
		t.Fatalf("%d %v", code, m)
	}
	u := m["user"].(map[string]any)
	if u["name"] != "stranger" || u["id"] != nil {
		t.Errorf("a denied account should be named but not identified further: %v", u)
	}
	if len(resp.Cookies()) != 0 || r.isAdmin() || r.auth.sess.len() != 0 {
		t.Error("a denied account got a session")
	}
	if !strings.Contains(r.logs.String(), "not allowed") {
		t.Errorf("the attempt should be logged: %s", r.logs)
	}
}

func TestAllowedByAccountIDNotByName(t *testing.T) {
	r := newRig(t)
	pin := r.begin("")
	// Someone whose Plex username is "john" but is a different account.
	r.tv.signIn(pin, "impostor-token", fakeUser{ID: 8, Username: "john"})
	if _, m, _ := r.pollDone("", pin); m["status"] != "denied" {
		t.Errorf("a namesake was let in: %v", m)
	}
}

func TestExpiredAndUnknownSignIns(t *testing.T) {
	r := newRig(t)

	pin := r.begin("")
	r.tv.expire(pin) // plex.tv has forgotten it
	r.clock.add(2 * time.Second)
	if _, m, _ := r.poll("", pin); m["status"] != "expired" {
		t.Errorf("plex.tv forgot it: %v", m)
	}

	pin = r.begin("")
	r.clock.add(31 * time.Minute) // too long ago, by our own clock
	before := r.tv.count("GET /api/v2/pins/" + itoa(pin))
	if _, m, _ := r.poll("", pin); m["status"] != "expired" {
		t.Errorf("timed out: %v", m)
	}
	if r.tv.count("GET /api/v2/pins/"+itoa(pin)) != before {
		t.Error("asked plex.tv about a sign-in we already knew had run out")
	}

	if _, m, _ := r.poll("", 987654); m["status"] != "expired" {
		t.Errorf("a PIN we never started: %v", m)
	}
}

func TestASignInCanOnlyBeCompletedOnce(t *testing.T) {
	r := newRig(t)
	pin := r.begin("")
	r.tv.signIn(pin, secretToken, fakeUser{ID: 7, Username: "john"})
	if _, m, _ := r.pollDone("", pin); m["status"] != "ok" {
		t.Fatalf("%v", m)
	}
	r.clock.add(2 * time.Second)
	if _, m, _ := r.poll("", pin); m["status"] != "expired" {
		t.Errorf("the same PIN worked twice: %v", m)
	}
	if r.auth.sess.len() != 1 {
		t.Errorf("%d sessions", r.auth.sess.len())
	}
}

func TestPollingIsThrottled(t *testing.T) {
	r := newRig(t)
	pin := r.begin("")
	path := "GET /api/v2/pins/" + itoa(pin)

	r.clock.add(2 * time.Second)
	r.poll("", pin)
	r.poll("", pin) // straight away: answered without bothering plex.tv
	r.poll("", pin)
	if n := r.tv.count(path); n != 1 {
		t.Errorf("%d requests to plex.tv for three quick polls, want 1", n)
	}
	r.clock.add(1100 * time.Millisecond)
	r.poll("", pin)
	if n := r.tv.count(path); n != 2 {
		t.Errorf("after the gap: %d requests, want 2", n)
	}
}

func TestRemovingAnAccountSignsItOutAtOnce(t *testing.T) {
	r := newRig(t)
	r.signedIn()
	if !r.isAdmin() {
		t.Fatal("setup")
	}

	r.set(func(s *access.Settings) { s.Plex.Admins = nil })
	if r.isAdmin() || r.status()["signedIn"] != nil {
		t.Error("an account that was removed is still signed in")
	}
}

func TestTurningItOffSignsEveryoneOut(t *testing.T) {
	r := newRig(t)
	r.signedIn()
	r.set(func(s *access.Settings) { s.Plex.Enabled = false })
	if r.isAdmin() {
		t.Error("still admin with Plex sign-in switched off")
	}
	r.set(func(s *access.Settings) { s.Plex.Enabled = true })
	if !r.isAdmin() {
		t.Error("switching it back on should restore a still-valid session")
	}
}

func TestTheNameShownIsTheCurrentOne(t *testing.T) {
	r := newRig(t)
	r.signedIn()
	r.set(func(s *access.Settings) { s.Plex.Admins[0].Name = "john (renamed)" })
	if si := r.status()["signedIn"].(map[string]any); si["name"] != "john (renamed)" {
		t.Errorf("%v", si)
	}
}

func TestSessionsTimeOut(t *testing.T) {
	r := newRig(t)
	r.signedIn()

	// Visited every 11 hours, so never idle for long, but only good for 7 days in all.
	hours := 0
	for r.isAdmin() && hours < 24*10 {
		r.clock.add(11 * time.Hour)
		hours += 11
	}
	if hours < 24*7 {
		t.Errorf("signed out after only %d hours; a busy session should last the full 7 days", hours)
	}
	if hours > 24*7+11 {
		t.Errorf("a session outlived the 7-day maximum: still good after %d hours", hours)
	}

	r2 := newRig(t)
	r2.signedIn()
	r2.clock.add(13 * time.Hour) // idle too long
	if r2.isAdmin() {
		t.Error("an idle session did not expire")
	}
}

func TestLoggingOut(t *testing.T) {
	r := newRig(t)
	r.signedIn()
	code, _, resp := r.post("/antennarr/auth/logout", "")
	if code != 200 {
		t.Fatalf("%d", code)
	}
	var cleared bool
	for _, c := range resp.Cookies() {
		cleared = cleared || (c.Name == CookieName && c.MaxAge < 0)
	}
	if !cleared || r.isAdmin() || r.auth.sess.len() != 0 {
		t.Errorf("cleared=%v admin=%v sessions=%d", cleared, r.isAdmin(), r.auth.sess.len())
	}
	if code, _, _ := r.post("/antennarr/auth/logout", ""); code != 200 { // harmless when already signed out
		t.Errorf("second logout: %d", code)
	}
}

func TestSessionsAreIndependent(t *testing.T) {
	a, b := newRig(t), newRig(t)
	a.signedIn()
	if b.isAdmin() {
		t.Error("another instance's browser is signed in")
	}
	// A cookie that was never issued means nothing.
	req := requestWithCookie("forged-session-id")
	if a.auth.IsSignedInAdmin(req) {
		t.Error("a forged cookie worked")
	}
}

func TestSecureCookieBehindAnHTTPSProxy(t *testing.T) {
	r := newRig(t)
	pin := r.begin("")
	r.tv.signIn(pin, secretToken, fakeUser{ID: 7, Username: "john"})
	r.clock.add(2 * time.Second)
	body := `{"pinId":` + itoa(pin) + `}`
	_, _, resp := r.post("/antennarr/auth/plex/poll", body, "X-Forwarded-Proto", "https")
	var c *http.Cookie
	for _, x := range resp.Cookies() {
		if x.Name == CookieName {
			c = x
		}
	}
	if c == nil || !c.Secure {
		t.Errorf("cookie = %+v, want Secure when the request came over https", c)
	}
}

// ---- adding an account ----

func TestAnAdminCanAddAnAccount(t *testing.T) {
	r := newRig(t)
	r.set(func(s *access.Settings) { s.Plex.Admins = nil })

	if code, m, _ := r.post("/antennarr/auth/plex/add/start", `{}`); code != 403 {
		t.Errorf("a non-admin could start adding an account: %d %v", code, m)
	}

	r.setAdmin(true) // from an admin address
	pin := r.begin("/add")
	r.tv.signIn(pin, secretToken, fakeUser{ID: 42, Username: "steve"})
	code, m, resp := r.pollDone("/add", pin)
	if code != 200 || m["status"] != "ok" || m["account"].(map[string]any)["name"] != "steve" {
		t.Fatalf("%d %v", code, m)
	}
	if len(resp.Cookies()) != 0 {
		t.Error("adding an account must not sign anyone in")
	}
	if acct, ok := r.get().PlexAdmin(42); !ok || acct.Name != "steve" {
		t.Errorf("not saved: %+v", r.get().Plex)
	}
	if strings.Contains(toString(m), secretToken) || strings.Contains(r.logs.String(), secretToken) {
		t.Error("the token leaked")
	}
}

func TestAddingAnAccountTwiceUpdatesItsName(t *testing.T) {
	r := newRig(t)
	r.setAdmin(true)
	for _, name := range []string{"john", "john-renamed"} {
		pin := r.begin("/add")
		r.tv.signIn(pin, "tok-"+name, fakeUser{ID: 7, Username: name})
		r.pollDone("/add", pin)
	}
	adm := r.get().Plex.Admins
	if len(adm) != 1 || adm[0].ID != 7 || adm[0].Name != "john-renamed" {
		t.Errorf("admins = %+v", adm)
	}
}

func TestAddAndLoginSignInsAreNotInterchangeable(t *testing.T) {
	r := newRig(t)
	r.setAdmin(true)

	loginPin := r.begin("")
	r.tv.signIn(loginPin, "t1", fakeUser{ID: 55, Username: "x"})
	r.clock.add(2 * time.Second)
	if _, m, _ := r.poll("/add", loginPin); m["status"] != "expired" {
		t.Errorf("a login PIN worked as an add PIN: %v", m)
	}

	addPin := r.begin("/add")
	r.tv.signIn(addPin, "t2", fakeUser{ID: 7, Username: "john"})
	r.clock.add(2 * time.Second)
	if _, m, _ := r.poll("", addPin); m["status"] != "expired" {
		t.Errorf("an add PIN worked as a login: %v", m)
	}
	if r.isAdmin() {
		t.Error("someone signed in through an add PIN")
	}
}

func TestAddingNeedsAnAdminAtEveryStep(t *testing.T) {
	r := newRig(t)
	r.setAdmin(true)
	pin := r.begin("/add")
	r.tv.signIn(pin, "t", fakeUser{ID: 50, Username: "sneaky"})
	r.setAdmin(false) // the admin walks away; someone else finishes the request
	r.clock.add(2 * time.Second)
	if code, _, _ := r.poll("/add", pin); code != 403 {
		t.Errorf("polling an add without being admin: %d", code)
	}
	if _, ok := r.get().PlexAdmin(50); ok {
		t.Error("an account was added without an admin")
	}
}

func TestAddingWithoutASettingsFileSaysSo(t *testing.T) {
	r := newRig(t, func(c *Config) { c.Edit = nil })
	r.setAdmin(true)
	pin := r.begin("/add")
	r.tv.signIn(pin, "t", fakeUser{ID: 42, Username: "steve"})
	code, m, _ := r.pollDone("/add", pin)
	if code != 409 {
		t.Errorf("%d %v", code, m)
	}

	r2 := newRig(t)
	r2.editErr = access.ErrNoFile
	r2.setAdmin(true)
	pin = r2.begin("/add")
	r2.tv.signIn(pin, "t", fakeUser{ID: 42, Username: "steve"})
	if code, m, _ := r2.pollDone("/add", pin); code != 409 || !strings.Contains(m["error"].(string), "-config") {
		t.Errorf("%d %v", code, m)
	}
}

func TestAddingBeyondTheLimitIsRefused(t *testing.T) {
	r := newRig(t)
	r.setAdmin(true)
	r.set(func(s *access.Settings) {
		s.Plex.Admins = nil
		for i := int64(1); i <= 50; i++ {
			s.Plex.Admins = append(s.Plex.Admins, access.PlexAccount{ID: i, Name: "a"})
		}
	})
	pin := r.begin("/add")
	r.tv.signIn(pin, "t", fakeUser{ID: 999, Username: "one-too-many"})
	if code, m, _ := r.pollDone("/add", pin); code != 400 || !strings.Contains(m["error"].(string), "at most") {
		t.Errorf("%d %v", code, m)
	}
}

// ---- plex.tv trouble, limits and the client ID ----

func TestPlexTVBeingDownIsReportedNotHidden(t *testing.T) {
	r := newRig(t)
	pin := r.begin("")

	r.tv.setFail(true)
	r.clock.add(2 * time.Second)
	if code, m, _ := r.poll("", pin); code != 502 || !strings.Contains(m["error"].(string), "plex.tv") {
		t.Errorf("poll: %d %v", code, m)
	}
	if code, m, _ := r.post("/antennarr/auth/plex/start", `{}`); code != 502 {
		t.Errorf("start: %d %v", code, m)
	}

	// The sign-in is still there to be finished once plex.tv recovers.
	r.tv.setFail(false)
	r.tv.signIn(pin, secretToken, fakeUser{ID: 7, Username: "john"})
	if _, m, _ := r.pollDone("", pin); m["status"] != "ok" {
		t.Errorf("after recovery: %v", m)
	}
}

func TestAFailureToIdentifyTheUserKeepsTheSignInOpen(t *testing.T) {
	r := newRig(t)
	pin := r.begin("")
	r.tv.signIn(pin, secretToken, fakeUser{ID: 7, Username: "john"})
	delete(r.tv.users, secretToken) // plex.tv will refuse the token
	r.clock.add(2 * time.Second)
	if code, _, _ := r.poll("", pin); code != 502 {
		t.Errorf("%d", code)
	}
	r.tv.addUser(secretToken, fakeUser{ID: 7, Username: "john"})
	if _, m, _ := r.pollDone("", pin); m["status"] != "ok" {
		t.Errorf("%v", m)
	}
}

func TestStartingSignInsIsRateLimited(t *testing.T) {
	r := newRig(t)
	for i := 0; i < 6; i++ {
		if code, _, _ := r.post("/antennarr/auth/plex/start", `{}`); code != 200 {
			t.Fatalf("start %d: %d", i+1, code)
		}
	}
	if code, m, _ := r.post("/antennarr/auth/plex/start", `{}`); code != 429 {
		t.Errorf("7th start in a minute: %d %v", code, m)
	}
	if n := r.tv.count("POST /api/v2/pins"); n != 6 {
		t.Errorf("a refused start still asked plex.tv: %d calls", n)
	}
	r.clock.add(61 * time.Second)
	if code, _, _ := r.post("/antennarr/auth/plex/start", `{}`); code != 200 {
		t.Errorf("after a minute: %d", code)
	}
}

func TestEachAddressHasItsOwnStartAllowance(t *testing.T) {
	r := newRig(t)
	now := r.clock.now()
	for i := 0; i < 6; i++ {
		if !r.auth.allowStart("10.0.0.1", now) {
			t.Fatal("refused too early")
		}
	}
	if r.auth.allowStart("10.0.0.1", now) || !r.auth.allowStart("10.0.0.2", now) {
		t.Error("the allowance is not per address")
	}
}

func TestSignInsInProgressAreCapped(t *testing.T) {
	r := newRig(t, func(c *Config) { c.MaxPending = 3; c.StartsPerMinute = 100 })
	var pins []int64
	for i := 0; i < 5; i++ {
		pins = append(pins, r.begin(""))
		r.clock.add(time.Second) // each one expires a little after the one before
	}
	r.auth.mu.Lock()
	n := len(r.auth.pins)
	r.auth.mu.Unlock()
	if n != 3 {
		t.Errorf("%d pending sign-ins, want the cap of 3", n)
	}
	r.clock.add(2 * time.Second)
	if _, m, _ := r.poll("", pins[0]); m["status"] != "expired" {
		t.Errorf("the oldest should have been dropped: %v", m)
	}
	if _, m, _ := r.poll("", pins[4]); m["status"] != "pending" {
		t.Errorf("the newest should still be there: %v", m)
	}
}

func TestExpiredSignInsAreClearedOut(t *testing.T) {
	r := newRig(t, func(c *Config) { c.StartsPerMinute = 100 })
	for i := 0; i < 5; i++ {
		r.begin("")
	}
	r.clock.add(31 * time.Minute)
	r.begin("")
	r.auth.mu.Lock()
	n := len(r.auth.pins)
	r.auth.mu.Unlock()
	if n != 1 {
		t.Errorf("%d pending after the old ones expired, want 1", n)
	}
}

func TestTheClientIDIsMadeOnceSavedAndKeptPrivate(t *testing.T) {
	r := newRig(t)
	if r.get().Plex.ClientID != "" {
		t.Fatal("setup")
	}
	r.begin("")
	id := r.get().Plex.ClientID
	if !regexp.MustCompile(`^antennarr-[A-Za-z0-9_-]{24}$`).MatchString(id) {
		t.Errorf("client id = %q", id)
	}
	if seen := r.tv.ids(); len(seen) != 1 || !seen[id] {
		t.Errorf("plex.tv saw %v, want only %q", seen, id)
	}
	r.begin("")
	if r.get().Plex.ClientID != id || r.edits != 1 {
		t.Errorf("a second start changed it (edits=%d)", r.edits)
	}

	// A restart reads the saved one back rather than making another.
	again := New(Config{Base: r.tv.URL, Settings: r.get, Edit: r.edit, Now: r.clock.now})
	if got := again.clientIDFor(); got != id {
		t.Errorf("after a restart: %q", got)
	}

	// The sign-in link has to carry it, because plex.tv's own page needs it.
	// Nothing else is allowed to.
	_, m, _ := r.post("/antennarr/auth/plex/start", `{}`)
	link := m["authUrl"].(string)
	if !strings.Contains(link, id) {
		t.Error("the sign-in link should carry the client ID to plex.tv's page")
	}
	delete(m, "authUrl")
	pin := int64(m["pinId"].(float64))
	r.tv.signIn(pin, secretToken, fakeUser{ID: 7, Username: "john"})
	_, poll, _ := r.pollDone("", pin)
	_, st, _ := r.getJSON("/antennarr/auth/status")
	if all := toString(m) + toString(poll) + toString(st); strings.Contains(all, id) {
		t.Errorf("the client ID appeared somewhere other than the sign-in link: %s", all)
	}
}

func TestWithoutASettingsFileTheClientIDLastsForTheRun(t *testing.T) {
	r := newRig(t, func(c *Config) { c.Edit = nil })
	a := r.auth.clientIDFor()
	if a == "" || a != r.auth.clientIDFor() {
		t.Errorf("client id not stable within a run: %q", a)
	}
	r2 := newRig(t)
	r2.editErr = access.ErrNoFile
	id := r2.auth.clientIDFor()
	if id == "" || id != r2.auth.clientIDFor() {
		t.Error("ErrNoFile should be tolerated")
	}
	r3 := newRig(t)
	r3.editErr = errors.New("disk full")
	if id := r3.auth.clientIDFor(); id == "" {
		t.Error("a save failure should not stop sign-in working")
	}
	if !strings.Contains(r3.logs.String(), "could not save the Plex client ID") {
		t.Errorf("the failure should be logged: %s", r3.logs)
	}
}

// ---- the HTTP surface ----

func TestWritesAreGuardedAgainstOtherSites(t *testing.T) {
	r := newRig(t)
	for name, c := range map[string]struct {
		hdr  []string
		body string
		want int
	}{
		"cross-origin": {[]string{"Origin", "https://evil.example"}, `{}`, 403},
		"text/plain":   {[]string{"Content-Type", "text/plain"}, `{}`, 415},
		"same-origin":  {[]string{"Origin", "SELF"}, `{}`, 200},
	} {
		hdr := c.hdr
		for i, h := range hdr {
			if h == "SELF" {
				hdr[i] = r.srv.URL
			}
		}
		req, _ := http.NewRequest("POST", r.srv.URL+"/antennarr/auth/plex/start", strings.NewReader(c.body))
		req.Header.Set("Content-Type", "application/json")
		for i := 0; i+1 < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		if code, _, _ := r.do(req); code != c.want {
			t.Errorf("%s: %d, want %d", name, code, c.want)
		}
	}
	if code, _, _ := r.post("/antennarr/auth/logout", "", "Origin", "https://evil.example"); code != 403 {
		t.Errorf("cross-origin logout: %d", code)
	}
}

func TestBadRequestsAndUnknownPaths(t *testing.T) {
	r := newRig(t)
	for name, body := range map[string]string{"not json": `nope`, "unknown field": `{"pinId":1,"x":2}`, "wrong type": `{"pinId":"one"}`} {
		if code, _, _ := r.post("/antennarr/auth/plex/poll", body); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, m, _ := r.getJSON("/antennarr/auth/nope"); code != 404 || m["error"] == nil {
		t.Errorf("unknown path: %d %v", code, m)
	}
	if code, _, _ := r.getJSON("/antennarr/auth/plex/start"); code == 200 {
		t.Error("start answered a GET")
	}
}

func TestStatusSaysWhetherYouAreAnAdminByAddress(t *testing.T) {
	r := newRig(t)
	if r.status()["admin"] != false {
		t.Error("admin without being one")
	}
	r.setAdmin(true)
	if st := r.status(); st["admin"] != true || st["signedIn"] != nil {
		t.Errorf("%v", st)
	}
}

func TestSimultaneousPollsMakeOneSession(t *testing.T) {
	r := newRig(t, func(c *Config) { c.StartsPerMinute = 100 })
	pin := r.begin("")
	r.tv.signIn(pin, secretToken, fakeUser{ID: 7, Username: "john"})
	r.clock.add(2 * time.Second)

	var wg sync.WaitGroup
	var oks atomic.Int32
	body := `{"pinId":` + itoa(pin) + `}`
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("POST", r.srv.URL+"/antennarr/auth/plex/poll", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			for _, c := range resp.Cookies() {
				if c.Name == CookieName {
					oks.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	if oks.Load() > 1 || r.auth.sess.len() > 1 {
		t.Errorf("%d cookies issued, %d sessions: one sign-in must give one session", oks.Load(), r.auth.sess.len())
	}
}

// ---- sessions ----

func TestTheSessionStoreIsBounded(t *testing.T) {
	c := &clock{t: time.Unix(1_000_000, 0)}
	s := newSessions(time.Hour, 24*time.Hour, c.now, "", nil)
	first := s.create(access.PlexAccount{ID: 1})
	c.add(time.Second)
	for i := 0; i < maxSessions; i++ {
		s.create(access.PlexAccount{ID: int64(i + 2)})
		c.add(time.Second)
	}
	if s.len() > maxSessions {
		t.Errorf("%d sessions", s.len())
	}
	if _, ok := s.get(first); ok {
		t.Error("the longest-idle session should have been the one dropped")
	}
}

func TestSessionIDsAreLongAndUnique(t *testing.T) {
	s := newSessions(time.Hour, time.Hour, time.Now, "", nil)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id := s.create(access.PlexAccount{ID: 1})
		if len(id) < 40 || seen[id] {
			t.Fatalf("id %q (len %d, repeated=%v)", id, len(id), seen[id])
		}
		seen[id] = true
	}
	if _, ok := s.get(""); ok {
		t.Error("an empty id matched")
	}
}

func TestActivityKeepsASessionAliveButNotForever(t *testing.T) {
	c := &clock{t: time.Unix(1_000_000, 0)}
	s := newSessions(time.Hour, 3*time.Hour, c.now, "", nil) // idle limit 1h, absolute limit 3h
	id := s.create(access.PlexAccount{ID: 1})
	for i := 1; i <= 3; i++ {
		c.add(50 * time.Minute) // never idle for a full hour
		if i < 3 {
			if _, ok := s.get(id); !ok {
				t.Fatalf("expired after %d visits (%d minutes)", i, i*50)
			}
		}
	}
	// That was 150 minutes of visits at 50-minute gaps; one more takes it past 3 hours.
	c.add(50 * time.Minute)
	if _, ok := s.get(id); ok {
		t.Error("the absolute maximum was not enforced for an active session")
	}

	id = s.create(access.PlexAccount{ID: 2})
	c.add(61 * time.Minute) // idle for more than the idle limit, well inside the absolute one
	if _, ok := s.get(id); ok {
		t.Error("the idle limit was not enforced")
	}
}

// ---- small helpers ----

func itoa(n int64) string   { return strconv.FormatInt(n, 10) }
func toString(v any) string { b, _ := json.Marshal(v); return string(b) }

// requestWithCookie is a request carrying a session cookie with the given value.
func requestWithCookie(value string) *http.Request {
	req := httptest.NewRequest("GET", "/antennarr/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: value})
	return req
}
