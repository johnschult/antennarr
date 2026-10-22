package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/access"
)

const (
	owner   = "203.0.113.9:4000"   // an admin by address
	friendA = "198.51.100.77:5000" // a stranger, until the invite is used
)

// withInvite is settings for a Yagi that lets people enroll: Plex sign-in and enrollment on, an owner who is an admin by
// address, one viewer, and an invite for "Dad". It returns the invite's token too.
func withInvite() (access.Settings, string) {
	token, inv := access.NewInvite("Dad", 2, nil, time.Now())
	s := access.Settings{
		Admin:   []string{"203.0.113.9"},
		Viewers: []access.Viewer{{Name: "John", Addresses: []string{"100.67.117.32"}, Limit: 1}},
		Plex:    &access.PlexSettings{Enabled: true, Enrollment: true, Admins: []access.PlexAccount{{ID: 1, Name: "owner"}}},
		Invites: []access.Invite{inv},
	}
	return s, token
}

// post sends a form to the invite pages the way a browser on the same site would.
func (st *site) form(path, from string, form url.Values) (int, string) {
	st.t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://"+req.Host)
	req.RemoteAddr = from
	rec := httptest.NewRecorder()
	st.srv.adminMux().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// tunerRefuses is whether the tuner's own port turns this address away as not allowed.
func (st *site) tunerRefuses(from string) bool {
	st.t.Helper()
	req := httptest.NewRequest("GET", "/discover.json", nil)
	req.RemoteAddr = from
	rec := httptest.NewRecorder()
	st.srv.devices[0].handler.ServeHTTP(rec, req)
	return rec.Code == http.StatusForbidden
}

func TestAnInvitedPersonBecomesAViewerWhoCanThenUseTheTuner(t *testing.T) {
	s, token := withInvite()
	st := newSite(t, s, true)
	st.tv.mu.Lock()
	st.tv.servers["tok-dad"] = `[{"name":"Dad's Plex","clientIdentifier":"mid-dad","provides":"server","owned":true,
		"connections":[{"address":"192.168.1.9","port":32400,"local":true,"relay":false},{"address":"198.51.100.77","port":32400,"local":false,"relay":false}]}]`
	st.tv.mu.Unlock()

	if !st.tunerRefuses(friendA) {
		t.Fatal("the stranger could use the tuner before enrolling")
	}
	// It is public: the page opens from an address that is nobody's yet, which the claim page would not.
	if code, _, _ := st.do("GET", "/invite/"+token, "", friendA); code != 200 {
		t.Fatalf("the invite page = %d", code)
	}
	code, body := st.form("/invite/"+token+"/start", friendA, url.Values{})
	m := regexp.MustCompile(`href="(/invite/[^"]+/wait\?h=[^"]+)"`).FindStringSubmatch(body)
	if code != 200 || m == nil {
		t.Fatalf("start = %d %s", code, body)
	}
	wait := strings.ReplaceAll(m[1], "&amp;", "&")

	st.tv.approve(st.tv.next, "tok-dad", 42, "dad")
	time.Sleep(1100 * time.Millisecond) // a sign-in is not asked about more than once a second
	code, page, _ := st.do("GET", wait, "", friendA)
	tm := regexp.MustCompile(`name="ticket" value="([^"]+)"`).FindStringSubmatch(page)
	if code != 200 || tm == nil || !strings.Contains(page, "198.51.100.77") || strings.Contains(page, "192.168.1.9") {
		t.Fatalf("servers = %d %s", code, page)
	}
	code, page = st.form("/invite/"+token+"/confirm", friendA, url.Values{"ticket": {tm[1]}, "server": {"mid-dad"}})
	if code != 200 || !strings.Contains(page, "You are in") {
		t.Fatalf("confirm = %d %s", code, page)
	}

	if st.tunerRefuses(friendA) {
		t.Error("the tuner still turns him away after he enrolled")
	}
	if !st.tunerRefuses("198.51.100.78:1") {
		t.Error("an address next to his was let in")
	}
	// The owner sees who he is, from the admin page.
	code, state, _ := st.do("GET", "/api/state", "", owner)
	if code != 200 || !strings.Contains(state, `"accountName":"dad"`) || !strings.Contains(state, `"serverName":"Dad's Plex"`) || !strings.Contains(state, `"used":true`) {
		t.Errorf("state = %d %s", code, state)
	}
	if strings.Contains(state, token) || strings.Contains(state, "tokenHash") {
		t.Errorf("state shows the invite's token: %s", state)
	}
	// It is on the tuner's timeline.
	found := false
	for _, ev := range st.srv.devices[0].mon.Snapshot().Events {
		found = found || strings.Contains(ev.Text, "Dad joined as a viewer")
	}
	if !found {
		t.Error("the tuner's timeline doesn't say Dad joined")
	}
}

func TestAnInvitePinCannotSignAnyoneIn(t *testing.T) {
	s, token := withInvite()
	st := newSite(t, s, true)
	_, body := st.form("/invite/"+token+"/start", friendA, url.Values{})
	if !strings.Contains(body, "app.plex.tv") {
		t.Fatalf("start: %s", body)
	}
	// Whoever holds that pin, even an account that is allowed to sign in as an admin, gets nothing from the sign-in
	// endpoints: they have never heard of it.
	pin := st.tv.next
	st.tv.approve(pin, "tok-owner", 1, "owner")
	time.Sleep(1100 * time.Millisecond)
	for _, path := range []string{"/auth/plex/poll", "/auth/plex/add/poll"} {
		code, resp, _ := st.do("POST", path, `{"pinId":`+strconv.FormatInt(pin, 10)+`}`, friendA)
		if code == 200 && strings.Contains(resp, `"ok"`) {
			t.Errorf("%s accepted an invite's sign-in: %s", path, resp)
		}
	}
	if code, _, _ := st.do("GET", "/api/state", "", friendA); code != 403 {
		t.Errorf("the stranger can open the admin API: %d", code)
	}
}

func TestOnlyAnAdminMakesInvites(t *testing.T) {
	s, _ := withInvite()
	st := newSite(t, s, true)
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/invites", `{"name":"X","limit":1}`}, {"GET", "/api/invites", ""}, {"DELETE", "/api/invites/aaaaaaaaaaaa", ""},
	} {
		if code, _, _ := st.do(c.method, c.path, c.body, friendA); code != 403 {
			t.Errorf("%s %s from a stranger = %d", c.method, c.path, code)
		}
	}
	if code, _, _ := st.do("POST", "/api/invites", `{"name":"Mom","limit":1}`, owner); code != 201 {
		t.Errorf("the owner = %d", code)
	}
}

func TestTheInvitePagesNeedOnlyEnrollmentAndAreOffWithoutIt(t *testing.T) {
	s, token := withInvite()
	s.Plex.Enabled = false // nobody signs in to the admin pages with Plex: invites don't care
	st := newSite(t, s, true)
	if code, _, _ := st.do("GET", "/invite/"+token, "", friendA); code != 200 {
		t.Errorf("with Plex sign-in off and enrollment on = %d, want the page", code)
	}

	s, token = withInvite()
	s.Plex.Enrollment = false
	st = newSite(t, s, true)
	if code, _, _ := st.do("GET", "/invite/"+token, "", friendA); code != 404 {
		t.Errorf("with enrollment off = %d, want 404", code)
	}
}
