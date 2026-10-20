package access

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fromTailnet is a request that came in over the tailnet from an address, with who the node said it was.
func fromTailnet(remote string, id Identity) *http.Request {
	r := req(remote)
	return r.WithContext(WithIdentity(r.Context(), id))
}

func identitySettings() Settings {
	s := DefaultSettings()
	s.Viewers = []Viewer{
		{Name: "Alice", Addresses: []string{"tailscale:Alice@Example.com"}, Limit: 2},
		{Name: "Phone", Addresses: []string{"tailscale:phone.tail1.ts.net", "192.168.1.9"}, Limit: 1},
	}
	s.Admin = []string{"tailscale:boss@example.com"}
	return s
}

func TestAViewerIsMatchedByTailnetLoginOrDevice(t *testing.T) {
	p, _ := newPolicy(t, identitySettings(), &fakeMon{})
	for name, c := range map[string]struct {
		r    *http.Request
		want string // the viewer, or "" for refused
	}{
		"by login, from any tailnet address": {fromTailnet("100.64.9.9:1", Identity{Login: "alice@example.com", Node: "x.tail1.ts.net"}), "Alice"},
		"by device":                          {fromTailnet("100.64.9.8:1", Identity{Login: "bob@example.com", Node: "Phone.tail1.ts.net."}), "Phone"},
		"another login is refused":           {fromTailnet("100.64.9.7:1", Identity{Login: "bob@example.com", Node: "x.tail1.ts.net"}), ""},
		"a tagged device has no login":       {fromTailnet("100.64.9.6:1", Identity{Node: "tagged.tail1.ts.net"}), ""},
		"the same name from the LAN is not":  {req("192.168.1.50:1"), ""},
		"the plain address still works":      {req("192.168.1.9:1"), "Phone"},
		"a login does not match as a device": {fromTailnet("100.64.9.5:1", Identity{Login: "phone.tail1.ts.net", Node: "x.tail1.ts.net"}), ""},
	} {
		err := p.Authorize(c.r)
		if (err == nil) != (c.want != "") {
			t.Errorf("%s: Authorize = %v, want viewer %q", name, err, c.want)
		}
	}
}

func TestAHeaderCannotClaimAnIdentity(t *testing.T) {
	p, _ := newPolicy(t, identitySettings(), &fakeMon{})
	r := req("192.168.1.50:1")
	r.Header.Set("Tailscale-User-Login", "alice@example.com")
	if err := p.Authorize(r); err == nil {
		t.Error("a request header was taken for the node's answer")
	}
}

func TestAnAdminCanBeATailnetUser(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	p, _ := newPolicy(t, identitySettings(), &fakeMon{})
	h := p.AdminOnly(ok)
	for name, c := range map[string]struct {
		r    *http.Request
		want int
	}{
		"the listed user": {fromTailnet("100.64.1.1:1", Identity{Login: "boss@example.com"}), 200},
		"another user":    {fromTailnet("100.64.1.2:1", Identity{Login: "alice@example.com"}), 403},
		"no identity":     {req("100.64.1.3:1"), 403},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, c.r)
		if rec.Code != c.want {
			t.Errorf("%s: %d, want %d", name, rec.Code, c.want)
		}
	}
	if !p.HasAdmin() {
		t.Error("an admin who is a tailnet user is not counted, so the install looks unclaimed")
	}
}

func TestBadIdentitiesAreRefusedWhenTheSettingsAreChecked(t *testing.T) {
	for _, bad := range []string{"tailscale:", "tailscale:alice", "tailscale:@example.com", "tailscale:a@b@c", "tailscale:two words@example.com", "tailscale:10.0.0.0/8"} {
		s := DefaultSettings()
		s.Viewers = []Viewer{{Name: "V", Addresses: []string{bad}}}
		if err := s.Validate(); err == nil {
			t.Errorf("%q was accepted as a viewer address", bad)
		}
		s = DefaultSettings()
		s.Admin = []string{bad}
		if err := s.Validate(); err == nil {
			t.Errorf("%q was accepted as an admin address", bad)
		}
	}
	s := DefaultSettings()
	s.Viewers = []Viewer{{Name: "A", Addresses: []string{"tailscale:a@example.com"}}, {Name: "B", Addresses: []string{"tailscale:A@example.com"}}}
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "both") {
		t.Errorf("one login for two viewers: %v", err)
	}
}

func TestADeviceEntryWithTheRootDotMeansTheSame(t *testing.T) {
	s := DefaultSettings()
	s.Viewers = []Viewer{{Name: "Phone", Addresses: []string{"tailscale:Phone.tail1.ts.net."}, Limit: 1}}
	p, _ := newPolicy(t, s, &fakeMon{})
	if err := p.Authorize(fromTailnet("100.64.9.8:1", Identity{Node: "phone.tail1.ts.net."})); err != nil {
		t.Errorf("an entry with the root dot never matched: %v", err)
	}
}
