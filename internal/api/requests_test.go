package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/johnschult/yagi/internal/access"
)

// ask makes the tuner's policy see a request from someone, as a media server asking for the tuner would.
func (e *env) ask(remote string, id *access.Identity) {
	r := httptest.NewRequest(http.MethodGet, "/discover.json", nil)
	r.RemoteAddr = remote
	if id != nil {
		r = r.WithContext(access.WithIdentity(r.Context(), *id))
	}
	_ = e.pol.Device("antenna").Authorize(r)
}

func TestSomeoneWhoAsksAppearsUnderRequests(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	e.ask("100.64.0.9:4000", nil)
	e.ask("100.64.0.9:4001", nil)
	e.ask("100.64.0.30:4000", &access.Identity{Login: "pal@example.com", Node: "den.tail1.ts.net"})

	code, v := e.call(t, "GET", "/api/requests", "")
	list := v.([]any)
	if code != 200 || len(list) != 2 {
		t.Fatalf("requests = %d %v", code, v)
	}
	first := obj(t, list[0]) // the latest first: the one with a login
	if first["label"] != "pal@example.com" || first["login"] != "pal@example.com" || first["node"] != "den.tail1.ts.net" || first["count"] != float64(1) || first["device"] != "antenna" || len(first["id"].(string)) != 12 {
		t.Errorf("first = %v", first)
	}
	second := obj(t, list[1])
	if second["label"] != "100.64.0.9" || second["address"] != "100.64.0.9" || second["count"] != float64(2) || second["login"] != nil {
		t.Errorf("second = %v", second)
	}
	_, state := e.call(t, "GET", "/api/state", "")
	if got := obj(t, state)["requests"].([]any); len(got) != 2 {
		t.Errorf("state requests = %v", got)
	}
}

func TestNoRequestsIsAnEmptyListNotNull(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	_, v := e.call(t, "GET", "/api/requests", "")
	if toJSON(v) != "[]" {
		t.Errorf("= %s", toJSON(v))
	}
	_, state := e.call(t, "GET", "/api/state", "")
	if toJSON(obj(t, state)["requests"]) != "[]" {
		t.Errorf("state requests = %s", toJSON(obj(t, state)["requests"]))
	}
}

func idOf(t *testing.T, e *env, label string) string {
	t.Helper()
	_, v := e.call(t, "GET", "/api/requests", "")
	for _, x := range v.([]any) {
		if m := obj(t, x); m["label"] == label {
			return m["id"].(string)
		}
	}
	t.Fatalf("no request for %s in %v", label, v)
	return ""
}

func TestAllowingARequestMakesThemAViewerByWhoTheyAre(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	e.ask("100.64.0.9:4000", nil)
	e.ask("100.64.0.30:4000", &access.Identity{Login: "pal@example.com"})
	e.ask("100.64.0.31:4000", &access.Identity{Node: "kiosk.tail1.ts.net"})

	code, v := e.call(t, "POST", "/api/requests/"+idOf(t, e, "100.64.0.9")+"/allow", `{"name":"Dad","limit":2}`)
	if code != 201 || obj(t, v)["name"] != "Dad" || toJSON(obj(t, v)["addresses"]) != `["100.64.0.9"]` || obj(t, v)["limit"] != float64(2) {
		t.Errorf("by address = %d %v", code, v)
	}
	code, v = e.call(t, "POST", "/api/requests/"+idOf(t, e, "pal@example.com")+"/allow", `{"name":"Pal","limit":1}`)
	if code != 201 || toJSON(obj(t, v)["addresses"]) != `["tailscale:pal@example.com"]` {
		t.Errorf("by login = %d %v", code, v)
	}
	code, v = e.call(t, "POST", "/api/requests/"+idOf(t, e, "kiosk.tail1.ts.net")+"/allow", `{"name":"Kiosk","limit":1}`)
	if code != 201 || toJSON(obj(t, v)["addresses"]) != `["tailscale:kiosk.tail1.ts.net"]` {
		t.Errorf("by device = %d %v", code, v)
	}
	// All three are viewers, and so no longer requests.
	_, list := e.call(t, "GET", "/api/requests", "")
	if len(list.([]any)) != 0 {
		t.Errorf("still listed: %v", list)
	}
	saved, _, err := access.LoadSettings(e.path)
	if err != nil || len(saved.Viewers) != 5 {
		t.Errorf("saved viewers = %d (%v)", len(saved.Viewers), err)
	}
}

func TestAllowingNeedsANameAndAFreeOne(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	e.ask("100.64.0.9:4000", nil)
	id := idOf(t, e, "100.64.0.9")
	for name, c := range map[string]struct {
		body string
		code int
	}{
		"no name":       {`{"limit":1}`, 400},
		"a blank name":  {`{"name":"  ","limit":1}`, 400},
		"a taken name":  {`{"name":"mom","limit":1}`, 409},
		"a bad limit":   {`{"name":"X","limit":-1}`, 400},
		"unknown field": {`{"name":"X","limit":1,"addresses":["8.8.8.8"]}`, 400},
	} {
		if code, v := e.call(t, "POST", "/api/requests/"+id+"/allow", c.body); code != c.code {
			t.Errorf("%s: %d %v, want %d", name, code, v, c.code)
		}
	}
	if code, _ := e.call(t, "POST", "/api/requests/000000000000/allow", `{"name":"X","limit":1}`); code != 404 {
		t.Errorf("an unknown request = %d", code)
	}
	// Still there to try again.
	if _, list := e.call(t, "GET", "/api/requests", ""); len(list.([]any)) != 1 {
		t.Errorf("a failed allow removed the request: %v", list)
	}
}

func TestTheOwnerCannotPickTheAddressOfARequest(t *testing.T) {
	// "addresses" in the body is an unknown field above; and the address always comes from what Yagi saw.
	e := newEnv(t, johnAndMom, true)
	e.ask("100.64.0.9:4000", nil)
	e.call(t, "POST", "/api/requests/"+idOf(t, e, "100.64.0.9")+"/allow", `{"name":"Dad","limit":1}`)
	_, v := e.call(t, "GET", "/api/viewers", "")
	for _, x := range v.([]any) {
		if m := obj(t, x); m["name"] == "Dad" && toJSON(m["addresses"]) != `["100.64.0.9"]` {
			t.Errorf("addresses = %v", m["addresses"])
		}
	}
}

func TestDismissingARequestKeepsThemOffTheList(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	e.ask("100.64.0.9:4000", nil)
	id := idOf(t, e, "100.64.0.9")
	if code, _ := e.call(t, "DELETE", "/api/requests/"+id, ""); code != 204 {
		t.Errorf("dismiss = %d", code)
	}
	if code, _ := e.call(t, "DELETE", "/api/requests/"+id, ""); code != 404 {
		t.Errorf("dismiss again = %d", code)
	}
	e.ask("100.64.0.9:4001", nil)
	if _, list := e.call(t, "GET", "/api/requests", ""); len(list.([]any)) != 0 {
		t.Errorf("they came back: %v", list)
	}
}

func TestRequestEventsAreOnAndCanBeSwitchedOff(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	_, v := e.call(t, "GET", "/api/settings", "")
	if obj(t, obj(t, obj(t, v)["notifications"])["events"])["request"] != true {
		t.Errorf("not on by default: %v", v)
	}
	if code, v := e.call(t, "PATCH", "/api/settings", `{"notifyEvents":{"request":false}}`); code != 200 || obj(t, obj(t, obj(t, v)["notifications"])["events"])["request"] != false {
		t.Errorf("switching off = %d %v", code, v)
	}
}
