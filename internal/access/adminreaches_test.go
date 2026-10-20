package access

import "testing"

func TestAdminReachesTheTailnetOnlyThroughARangeOrPlex(t *testing.T) {
	node := []string{"100.64.0.5", "fd7a:115c:a1e0::5"}
	for name, c := range map[string]struct {
		admin []string
		plex  bool
		want  bool
	}{
		"nothing listed":              {nil, false, false},
		"a LAN range":                 {[]string{"192.168.1.0/24"}, false, false},
		"the node's own address only": {[]string{"100.64.0.5"}, false, false}, // only one client could come from it
		"the tailnet range":           {[]string{"100.64.0.0/10"}, false, true},
		"an IPv6 tailnet range":       {[]string{"fd7a:115c:a1e0::/48"}, false, true},
		"everything":                  {[]string{"0.0.0.0/0"}, false, true},
		"an IPv6 single address":      {[]string{"fd7a:115c:a1e0::5/128"}, false, false},
		"a Plex account":              {nil, true, true},
	} {
		s := DefaultSettings()
		s.Admin = c.admin
		if c.plex {
			s.Plex = &PlexSettings{Enabled: true, Admins: []PlexAccount{{ID: 1, Name: "me"}}}
		}
		if got := s.AdminReaches(node); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	if DefaultSettings().AdminReaches(nil) {
		t.Error("reached with no node addresses at all")
	}
}
