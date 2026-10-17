package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/johnschult/antennarr/internal/access"
)

func withPlex(admins ...access.PlexAccount) access.Settings {
	s := johnAndMom
	s.Plex = &access.PlexSettings{Enabled: true, Admins: admins, ClientID: "antennarr-secretclientid1234"}
	return s
}

func plexOf(t *testing.T, e *env) map[string]any {
	t.Helper()
	_, v := e.call(t, "GET", "/antennarr/api/state", "")
	return obj(t, obj(t, obj(t, v)["settings"])["plex"])
}

func TestStateReportsPlexSignInWithoutItsClientID(t *testing.T) {
	e := newEnv(t, withPlex(access.PlexAccount{ID: 7, Name: "john"}), true)
	pl := plexOf(t, e)
	admins := pl["admins"].([]any)
	if pl["enabled"] != true || len(admins) != 1 || obj(t, admins[0])["id"] != float64(7) || obj(t, admins[0])["name"] != "john" {
		t.Errorf("plex = %v", pl)
	}

	_, state := e.call(t, "GET", "/antennarr/api/state", "")
	_, settings := e.call(t, "GET", "/antennarr/api/settings", "")
	if strings.Contains(toJSON(state)+toJSON(settings), "secretclientid") {
		t.Error("the Plex client ID is in an API response")
	}
}

func TestPlexIsOffByDefaultAndListsNobody(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	pl := plexOf(t, e)
	if pl["enabled"] != false || len(pl["admins"].([]any)) != 0 || pl["admins"] == nil {
		t.Errorf("plex = %v", pl)
	}
}

func TestSwitchingPlexSignInOnAndOff(t *testing.T) {
	e := newEnv(t, johnAndMom, true)

	code, v := e.call(t, "PATCH", "/antennarr/api/settings", `{"plexEnabled":true}`)
	if code != 200 || obj(t, obj(t, v)["plex"])["enabled"] != true {
		t.Fatalf("%d %v", code, v)
	}
	if !fileSettings(t, e).PlexEnabled() {
		t.Error("not saved")
	}
	// Other settings are not disturbed, and the accounts stay when it is turned off.
	e.call(t, "PATCH", "/antennarr/api/settings", `{"sharedCap":2}`)
	if !fileSettings(t, e).PlexEnabled() {
		t.Error("an unrelated patch switched it off")
	}
	e.edit(t, func(s *access.Settings) { s.Plex.Admins = []access.PlexAccount{{ID: 7, Name: "john"}} })
	e.call(t, "PATCH", "/antennarr/api/settings", `{"plexEnabled":false}`)
	got := fileSettings(t, e)
	if got.PlexEnabled() || len(got.Plex.Admins) != 1 {
		t.Errorf("turning it off should keep the list: %+v", got.Plex)
	}
	if code, _ := e.call(t, "PATCH", "/antennarr/api/settings", `{"plexEnabled":"yes"}`); code != 400 {
		t.Errorf("a non-boolean: %d", code)
	}
}

func TestEnablingPlexKeepsAnExistingClientID(t *testing.T) {
	e := newEnv(t, withPlex(), true)
	e.call(t, "PATCH", "/antennarr/api/settings", `{"plexEnabled":false}`)
	e.call(t, "PATCH", "/antennarr/api/settings", `{"plexEnabled":true}`)
	if got := fileSettings(t, e).Plex.ClientID; got != "antennarr-secretclientid1234" {
		t.Errorf("client id = %q: switching the feature on and off must not change how plex.tv knows us", got)
	}
}

func TestRemovingAPlexAccount(t *testing.T) {
	e := newEnv(t, withPlex(access.PlexAccount{ID: 7, Name: "john"}, access.PlexAccount{ID: 9, Name: "steve"}), true)

	if code, _ := e.call(t, "DELETE", "/antennarr/api/plex/admins/7", ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	adm := fileSettings(t, e).Plex.Admins
	if len(adm) != 1 || adm[0].ID != 9 {
		t.Errorf("admins = %+v", adm)
	}
	// It takes effect on the running policy too, not only in the file.
	if len(e.pol.Settings().Plex.Admins) != 1 {
		t.Error("the live settings still list the account")
	}
	if code, v := e.call(t, "DELETE", "/antennarr/api/plex/admins/7", ""); code != 404 || !strings.Contains(errText(v), "no Plex account") {
		t.Errorf("removing it again: %d %v", code, v)
	}
	for _, bad := range []string{"abc", "-1", "0", "1.5"} {
		if code, _ := e.call(t, "DELETE", "/antennarr/api/plex/admins/"+bad, ""); code != 400 {
			t.Errorf("%q: %d", bad, code)
		}
	}
	// Removing the last one is allowed: the address-based admin access still works.
	if code, _ := e.call(t, "DELETE", "/antennarr/api/plex/admins/9", ""); code != 204 {
		t.Errorf("last account: %d", code)
	}
}

func TestRemovingAPlexAccountNeedsAFileAndAnAdmin(t *testing.T) {
	e := newEnv(t, withPlex(access.PlexAccount{ID: 7, Name: "john"}), false)
	if code, v := e.call(t, "DELETE", "/antennarr/api/plex/admins/7", ""); code != 409 || !strings.Contains(errText(v), "-config") {
		t.Errorf("no file: %d %v", code, v)
	}

	e = newEnv(t, withPlex(access.PlexAccount{ID: 7, Name: "john"}), true)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/antennarr/api/plex/admins/7", nil)
	req.RemoteAddr = "100.67.117.32:1" // a viewer
	e.pol.AdminOnly(e.api.Handler()).ServeHTTP(rec, req)
	if rec.Code != 403 || len(fileSettings(t, e).Plex.Admins) != 1 {
		t.Errorf("a viewer removed an account: %d", rec.Code)
	}
	if code, _ := e.call(t, "DELETE", "/antennarr/api/plex/admins/7", "", "Origin", "https://evil.example"); code != 403 {
		t.Errorf("cross-origin: %d", code)
	}
}

func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

// edit changes the live settings and the file together, as a settings edit would.
func (e *env) edit(t *testing.T, fn func(*access.Settings)) {
	t.Helper()
	if _, err := e.api.Store.Edit(func(s *access.Settings) error { fn(s); return nil }); err != nil {
		t.Fatal(err)
	}
}
