package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/johnschult/yagi/internal/access"
)

const (
	owner   = "203.0.113.9:4000"   // an admin by address
	friendA = "198.51.100.77:5000" // a stranger
)

// askTuner is someone's media server asking the tuner's own port, from remote, as the tailnet identity if given.
func (st *site) askTuner(remote string, id *access.Identity) int {
	st.t.Helper()
	req := httptest.NewRequest("GET", "/discover.json", nil)
	req.RemoteAddr = remote
	if id != nil {
		req = req.WithContext(access.WithIdentity(context.Background(), *id))
	}
	rec := httptest.NewRecorder()
	st.srv.devices[0].handler.ServeHTTP(rec, req)
	return rec.Code
}

func TestAFriendWhoTriesToConnectShowsUpToBeAllowedAndThenGetsIn(t *testing.T) {
	st := newSite(t, access.Settings{
		Admin:   []string{"203.0.113.9"},
		Viewers: []access.Viewer{{Name: "John", Addresses: []string{"100.67.117.32"}, Limit: 1}},
	}, true)

	if code := st.askTuner("100.64.0.30:5000", &access.Identity{Login: "pal@example.com", Node: "den.tail1.ts.net"}); code != http.StatusForbidden {
		t.Fatalf("a stranger got %d, want 403", code)
	}
	code, body, _ := st.do("GET", "/api/requests", "", owner)
	var list []struct{ ID, Label, Login string }
	if err := json.Unmarshal([]byte(body), &list); err != nil || code != 200 || len(list) != 1 || list[0].Label != "pal@example.com" {
		t.Fatalf("requests = %d %s", code, body)
	}
	if code, _, _ := st.do("GET", "/api/requests", "", friendA); code != 403 {
		t.Errorf("the stranger could read the requests: %d", code)
	}
	if code, _, _ := st.do("POST", "/api/requests/"+list[0].ID+"/allow", `{"name":"Pal","limit":1}`, friendA); code != 403 {
		t.Errorf("the stranger could allow themselves: %d", code)
	}

	if code, body, _ := st.do("POST", "/api/requests/"+list[0].ID+"/allow", `{"name":"Pal","limit":1}`, owner); code != 201 {
		t.Fatalf("allow = %d %s", code, body)
	}
	// By who they are: the same login from another device gets in, and the same device from a stranger's address too.
	if code := st.askTuner("100.64.0.31:5000", &access.Identity{Login: "pal@example.com", Node: "phone.tail1.ts.net"}); code == http.StatusForbidden {
		t.Errorf("allowed by login, but their phone was refused")
	}
	if code := st.askTuner("100.64.0.99:5000", &access.Identity{Login: "someone-else@example.com"}); code != http.StatusForbidden {
		t.Errorf("another login got %d", code)
	}
	if _, body, _ := st.do("GET", "/api/requests", "", owner); strings.Contains(body, "pal@example.com") {
		t.Errorf("still listed after being allowed: %s", body)
	}
}
