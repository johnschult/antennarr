package api

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/access"
)

var t0 = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

// invitesOn is settings with enrollment on, and nobody signing in with Plex: the two are independent.
func invitesOn() access.Settings {
	s := johnAndMom
	s.Plex = &access.PlexSettings{Enrollment: true}
	return s
}

func TestAnInviteCanOnlyBeMadeWhileEnrollmentIsOn(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	code, v := e.call(t, "POST", "/api/invites", `{"name":"Dad","limit":1}`)
	if code != 409 || !strings.Contains(obj(t, v)["error"].(string), "turn on inviting viewers with Plex") {
		t.Errorf("with it off = %d %v", code, v)
	}
	// Plex accounts signing in to the admin pages is another thing, and doesn't help.
	if code, _ := e.call(t, "PATCH", "/api/settings", `{"plexEnabled":true}`); code != 200 {
		t.Fatal(code)
	}
	if code, _ := e.call(t, "POST", "/api/invites", `{"name":"Dad","limit":1}`); code != 409 {
		t.Errorf("with only Plex sign-in on = %d", code)
	}
	// Enrollment is enough, with sign-in off.
	e = newEnv(t, johnAndMom, true)
	if code, v := e.call(t, "PATCH", "/api/settings", `{"plexEnrollment":true}`); code != 200 || obj(t, obj(t, v)["plex"])["enrollment"] != true || obj(t, obj(t, v)["plex"])["enabled"] != false {
		t.Fatalf("turning enrollment on = %d %v", code, v)
	}
	if code, v := e.call(t, "POST", "/api/invites", `{"name":"Dad","limit":1}`); code != 201 {
		t.Errorf("with enrollment alone = %d %v", code, v)
	}
}

func TestMakingListingAndRevokingAnInvite(t *testing.T) {
	e := newEnv(t, invitesOn(), true)
	code, v := e.call(t, "POST", "/api/invites", `{"name":"  Dad  ","limit":2}`, "Host", "yagi.example:9244")
	if code != 201 {
		t.Fatalf("create = %d %v", code, v)
	}
	doc := obj(t, v)
	id, path, link := doc["id"].(string), doc["path"].(string), doc["url"].(string)
	if doc["name"] != "Dad" || doc["limit"] != float64(2) || doc["used"] != false || doc["expired"] != false || len(id) != 12 {
		t.Errorf("created = %v", doc)
	}
	if !strings.HasPrefix(path, "/invite/") || len(path) != len("/invite/")+43 || !strings.HasSuffix(link, path) || !strings.HasPrefix(link, "http://") {
		t.Errorf("path %q, url %q", path, link)
	}
	token := strings.TrimPrefix(path, "/invite/")

	// The link is shown that once: the list, the state and the settings never carry it or its hash.
	raw, _ := os.ReadFile(e.path)
	if strings.Contains(string(raw), token) {
		t.Error("the token is in the settings file")
	}
	for _, p := range []string{"/api/invites", "/api/state", "/api/settings", "/api/viewers"} {
		_, body := e.call(t, "GET", p, "")
		if s := toJSON(body); strings.Contains(s, token) || strings.Contains(s, access.HashInviteToken(token)) || strings.Contains(s, "tokenHash") {
			t.Errorf("%s shows the token or its hash: %s", p, s)
		}
	}
	code, v = e.call(t, "GET", "/api/invites", "")
	list := v.([]any)
	if code != 200 || len(list) != 1 || obj(t, list[0])["id"] != id || obj(t, list[0])["name"] != "Dad" {
		t.Errorf("list = %d %v", code, v)
	}
	_, v = e.call(t, "GET", "/api/state", "")
	if inv := obj(t, v)["invites"].([]any); len(inv) != 1 {
		t.Errorf("state invites = %v", inv)
	}

	if code, _ := e.call(t, "DELETE", "/api/invites/"+id, ""); code != 204 {
		t.Errorf("revoke = %d", code)
	}
	if code, _ := e.call(t, "DELETE", "/api/invites/"+id, ""); code != 404 {
		t.Errorf("revoke again = %d", code)
	}
	_, v = e.call(t, "GET", "/api/invites", "")
	if len(v.([]any)) != 0 {
		t.Errorf("after revoking: %v", v)
	}
	// The listing is an empty list, not null.
	if raw := toJSON(v); raw != "[]" {
		t.Errorf("no invites = %s", raw)
	}
}

func TestInviteInputIsChecked(t *testing.T) {
	s := invitesOn()
	s.Devices = []access.Device{{Name: "antenna", Listen: ":5004", Tuner: "192.168.1.50"}}
	e := newEnv(t, s, true)
	e.call(t, "POST", "/api/invites", `{"name":"Dad","limit":1}`)
	for name, c := range map[string]struct {
		body string
		code int
	}{
		"no name":               {`{"limit":1}`, 400},
		"a blank name":          {`{"name":"   ","limit":1}`, 400},
		"a control character":   {"{\"name\":\"a\\u001bb\",\"limit\":1}", 400},
		"a negative limit":      {`{"name":"X","limit":-1}`, 400},
		"a negative device":     {`{"name":"X","limit":1,"limits":{"antenna":-1}}`, 400},
		"an unknown device":     {`{"name":"X","limit":1,"limits":{"nope":1}}`, 400},
		"a viewer's name":       {`{"name":"john","limit":1}`, 409},
		"another invite's name": {`{"name":"DAD","limit":1}`, 409},
		"an unknown field":      {`{"name":"X","limit":1,"admin":true}`, 400},
	} {
		if code, v := e.call(t, "POST", "/api/invites", c.body); code != c.code {
			t.Errorf("%s: %d %v, want %d", name, code, v, c.code)
		}
	}
	if code, _ := e.call(t, "POST", "/api/invites", `{"name":"Mom2","limit":1,"limits":{"antenna":3}}`); code != 201 {
		t.Errorf("a device limit = %d", code)
	}
}

func TestOnlyAsManyInvitesAsTheLimit(t *testing.T) {
	e := newEnv(t, invitesOn(), true)
	for i := 0; i < access.MaxInvites; i++ {
		body := `{"name":"viewer` + strings.Repeat("x", i) + `","limit":1}`
		if code, v := e.call(t, "POST", "/api/invites", body); code != 201 {
			t.Fatalf("invite %d = %d %v", i, code, v)
		}
	}
	if code, _ := e.call(t, "POST", "/api/invites", `{"name":"one too many","limit":1}`); code != 409 {
		t.Errorf("one too many = %d", code)
	}
}

func TestAnEnrolledViewerKeepsWhereItCameFromWhenTheOwnerEditsIt(t *testing.T) {
	s := invitesOn()
	_, inv := access.NewInvite("Dad", 2, nil, t0)
	inv.ViewerName = "Dad"
	s.Viewers = append(s.Viewers, access.Viewer{Name: "Dad", Addresses: []string{"10.8.0.50"}, Limit: 2,
		Plex: &access.ViewerPlex{AccountID: 9, AccountName: "dad", ServerID: "mid-dad", ServerName: "Dad's"}})
	s.Invites = []access.Invite{inv}
	e := newEnv(t, s, true)

	// The owner changes the limit and renames him; he is still the viewer his invite made.
	code, v := e.call(t, "PUT", "/api/viewers/Dad", `{"name":"Father","addresses":["10.8.0.50"],"limit":3}`)
	if code != 200 {
		t.Fatalf("edit = %d %v", code, v)
	}
	plex := obj(t, obj(t, v)["plex"])
	if plex["accountId"] != float64(9) || plex["serverName"] != "Dad's" {
		t.Errorf("the provenance was lost: %v", plex)
	}
	_, v = e.call(t, "GET", "/api/invites", "")
	if got := obj(t, v.([]any)[0]); got["viewer"] != "Father" || got["used"] != true {
		t.Errorf("the invite does not follow the rename: %v", got)
	}

	// Removing the viewer takes the invite he came in by with him, so it can't bring him back.
	if code, _ := e.call(t, "DELETE", "/api/viewers/Father", ""); code != 204 {
		t.Fatal(code)
	}
	_, v = e.call(t, "GET", "/api/invites", "")
	if len(v.([]any)) != 0 {
		t.Errorf("the invite outlived its viewer: %v", v)
	}
}

func TestRemovingAViewerKeepsInvitesThatAreNotTheirs(t *testing.T) {
	s := invitesOn()
	_, unused := access.NewInvite("Nobody yet", 1, nil, t0)
	_, other := access.NewInvite("Dad", 1, nil, t0)
	other.ViewerName = "Dad"
	s.Viewers = append(s.Viewers, access.Viewer{Name: "Dad", Addresses: []string{"10.8.0.50"}, Limit: 1})
	s.Invites = []access.Invite{unused, other}
	e := newEnv(t, s, true)
	e.call(t, "DELETE", "/api/viewers/John", "")
	_, v := e.call(t, "GET", "/api/invites", "")
	if len(v.([]any)) != 2 {
		t.Errorf("an unrelated invite went with John: %v", v)
	}
}

func TestEnrolledEventsCanBeSwitchedOff(t *testing.T) {
	e := newEnv(t, invitesOn(), true)
	_, v := e.call(t, "GET", "/api/settings", "")
	if obj(t, obj(t, obj(t, v)["notifications"])["events"])["enrolled"] != true {
		t.Errorf("should be on by default: %v", v)
	}
	if code, v := e.call(t, "PATCH", "/api/settings", `{"notifyEvents":{"enrolled":false}}`); code != 200 || obj(t, obj(t, obj(t, v)["notifications"])["events"])["enrolled"] != false {
		t.Errorf("switching off = %d %v", code, v)
	}
}
