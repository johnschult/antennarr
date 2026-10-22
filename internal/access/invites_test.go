package access

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

func TestAnInviteKeepsOnlyAHashOfItsToken(t *testing.T) {
	token, inv := NewInvite("  Mom  ", 2, map[string]int{}, t0)
	if len(token) < 40 {
		t.Errorf("token %q is too short to guess-proof", token)
	}
	if inv.Name != "Mom" || inv.Limit != 2 || inv.Limits != nil {
		t.Errorf("invite = %+v", inv)
	}
	if inv.TokenHash != HashInviteToken(token) || strings.Contains(inv.TokenHash, token) {
		t.Errorf("the hash %q is not a hash of the token", inv.TokenHash)
	}
	if !inv.Created.Equal(t0) || !inv.Expires.Equal(t0.Add(InviteLife)) {
		t.Errorf("created %v, expires %v", inv.Created, inv.Expires)
	}
	_, other := NewInvite("Mom", 2, nil, t0)
	if other.ID == inv.ID || other.TokenHash == inv.TokenHash {
		t.Error("two invites share an ID or a token")
	}

	// Nothing of the token reaches a settings file.
	s := DefaultSettings()
	s.Invites = []Invite{inv}
	path := filepath.Join(t.TempDir(), "yagi.json")
	if err := SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), token) {
		t.Error("the token itself was written to the settings file")
	}
	back, _, err := LoadSettings(path)
	if err != nil || len(back.Invites) != 1 || back.Invites[0].TokenHash != inv.TokenHash {
		t.Errorf("round trip: %+v %v", back.Invites, err)
	}
}

func TestAnInviteWorksUntilItExpiresOrIsUsed(t *testing.T) {
	_, inv := NewInvite("Mom", 1, nil, t0)
	if !inv.Live(t0) || !inv.Live(t0.Add(InviteLife-time.Second)) {
		t.Error("an unused invite should work for its whole life")
	}
	if inv.Live(t0.Add(InviteLife)) {
		t.Error("an unused invite should stop working when it expires")
	}
	inv.ViewerName = "Mom"
	if !inv.Used() || !inv.Live(t0.Add(365*24*time.Hour)) {
		t.Error("a used invite stays good until the owner revokes it: that is how an address is refreshed")
	}
}

func TestFindingAnInviteByTheTokenInTheLink(t *testing.T) {
	tok1, a := NewInvite("Mom", 1, nil, t0)
	tok2, b := NewInvite("Dad", 1, nil, t0)
	s := Settings{Invites: []Invite{a, b}}
	if got, ok := s.InviteForToken(tok2); !ok || got.ID != b.ID {
		t.Errorf("second token: %+v %v", got, ok)
	}
	if got, ok := s.InviteForToken(tok1); !ok || got.ID != a.ID {
		t.Errorf("first token: %+v %v", got, ok)
	}
	for _, wrong := range []string{"", "nope", tok1 + "x", tok1[:len(tok1)-1], a.TokenHash} { // the hash is not a token
		if _, ok := s.InviteForToken(wrong); ok {
			t.Errorf("%q was accepted", wrong)
		}
	}
	if got, ok := s.InviteByID(a.ID); !ok || got.Name != "Mom" {
		t.Errorf("by ID: %+v %v", got, ok)
	}
	if _, ok := s.InviteByID("000000000000"); ok {
		t.Error("an unknown ID was found")
	}
}

func TestInvitesAreValidated(t *testing.T) {
	_, good := NewInvite("Mom", 1, nil, t0)
	_, other := NewInvite("Dad", 1, nil, t0)
	with := func(mut func(*Invite)) error {
		inv := good
		mut(&inv)
		s := DefaultSettings()
		s.Invites = []Invite{inv}
		return s.Validate()
	}
	if err := with(func(*Invite) {}); err != nil {
		t.Fatalf("a good invite: %v", err)
	}
	for name, mut := range map[string]func(*Invite){
		"no id":              func(i *Invite) { i.ID = "" },
		"an id that isn't":   func(i *Invite) { i.ID = "NOT-HEX-AT-ALL" },
		"no hash":            func(i *Invite) { i.TokenHash = "" },
		"the token as hash":  func(i *Invite) { i.TokenHash = "short" },
		"no name":            func(i *Invite) { i.Name = "  " },
		"a name with a ctrl": func(i *Invite) { i.Name = "Mom\x1b[31m" },
		"a negative limit":   func(i *Invite) { i.Limit = -1 },
		"a negative device":  func(i *Invite) { i.Limits = map[string]int{"antenna": -1} },
		"no expiry":          func(i *Invite) { i.Expires = time.Time{} },
	} {
		if err := with(mut); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A used invite needs no expiry.
	if err := with(func(i *Invite) { i.Expires = time.Time{}; i.ViewerName = "Mom" }); err != nil {
		t.Errorf("a used invite: %v", err)
	}

	s := DefaultSettings()
	s.Invites = []Invite{good, good}
	if err := s.Validate(); err == nil {
		t.Error("the same invite twice was accepted")
	}
	dupName := other
	dupName.Name = "MOM"
	s.Invites = []Invite{good, dupName}
	if err := s.Validate(); err == nil {
		t.Error("two invites for one name were accepted")
	}
	sameToken := other
	sameToken.TokenHash = good.TokenHash
	s.Invites = []Invite{good, sameToken}
	if err := s.Validate(); err == nil {
		t.Error("two invites with one token were accepted")
	}
	s.Invites = nil
	for i := 0; i <= MaxInvites; i++ {
		_, inv := NewInvite("viewer"+string(rune('a'+i%26))+strings.Repeat("x", i/26), 1, nil, t0)
		s.Invites = append(s.Invites, inv)
	}
	if err := s.Validate(); err == nil {
		t.Errorf("%d invites were accepted", len(s.Invites))
	}
}

func TestAnEnrolledViewerIsValidatedAndCopied(t *testing.T) {
	s := DefaultSettings()
	s.Viewers = []Viewer{{Name: "Mom", Addresses: []string{"203.0.113.9"}, Limit: 1, Plex: &ViewerPlex{AccountID: 7, AccountName: "mom", ServerID: "abc123", ServerName: "Mom's"}}}
	if err := s.Validate(); err != nil {
		t.Fatalf("an enrolled viewer: %v", err)
	}
	c := s.Clone()
	c.Viewers[0].Plex.ServerName = "changed"
	if s.Viewers[0].Plex.ServerName != "Mom's" {
		t.Error("the copy shares the Plex details with the original")
	}
	for name, mut := range map[string]func(*ViewerPlex){
		"no account":         func(p *ViewerPlex) { p.AccountID = 0 },
		"a bad server id":    func(p *ViewerPlex) { p.ServerID = "has space" },
		"no server id":       func(p *ViewerPlex) { p.ServerID = "" },
		"a control in names": func(p *ViewerPlex) { p.ServerName = "x\ny" },
	} {
		bad := s.Clone()
		mut(bad.Viewers[0].Plex)
		if err := bad.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestEnrollmentNeedsPlexSignInToo(t *testing.T) {
	var s Settings
	if s.EnrollmentEnabled() {
		t.Error("on by default")
	}
	s.Plex = &PlexSettings{Enrollment: true}
	if s.EnrollmentEnabled() {
		t.Error("on while Sign in with Plex is off")
	}
	s.Plex.Enabled = true
	if !s.EnrollmentEnabled() {
		t.Error("off though both are on")
	}
	s.Plex.Enrollment = false
	if s.EnrollmentEnabled() {
		t.Error("on though its own switch is off")
	}
}

func TestInvitesAreCopiedWithTheSettings(t *testing.T) {
	_, inv := NewInvite("Mom", 1, map[string]int{"antenna": 2}, t0)
	s := Settings{Invites: []Invite{inv}}
	c := s.Clone()
	c.Invites[0].Limits["antenna"] = 9
	c.Invites[0].Name = "changed"
	if s.Invites[0].Limits["antenna"] != 2 || s.Invites[0].Name != "Mom" {
		t.Error("the copy shares an invite with the original")
	}
}

func TestEnrolledEventsAreOnUnlessSwitchedOff(t *testing.T) {
	var s Settings
	if !s.NotifyEnabled(NotifyEnrolled) {
		t.Error("enrolled events should be on by default")
	}
	off := false
	s.Notifications = &Notifications{Events: NotifyEvents{Enrolled: &off}}
	if s.NotifyEnabled(NotifyEnrolled) {
		t.Error("switched off but still on")
	}
	if c := s.Clone(); c.NotifyEnabled(NotifyEnrolled) {
		t.Error("the copy lost the switch")
	}
}

func TestOnlyPublicAddressesAreWorthAllowingForAServer(t *testing.T) {
	for in, want := range map[string]bool{
		"203.0.113.50": true, "2001:db8::5": true, "::ffff:198.51.100.9": true,
		"192.168.1.5": false, "10.0.0.1": false, "172.16.0.9": false, "127.0.0.1": false, "::1": false,
		"169.254.1.1": false, "100.64.0.7": false, "100.127.237.109": false, // a tailnet
		"0.0.0.0": false, "224.0.0.1": false, "fe80::1": false, "fd00::1": false,
	} {
		if got := PublicAddr(netip.MustParseAddr(in)); got != want {
			t.Errorf("PublicAddr(%s) = %v, want %v", in, got, want)
		}
	}
	if PublicAddr(netip.Addr{}) {
		t.Error("the zero address is public")
	}
}

func TestWhichViewerAlreadyHoldsAnAddress(t *testing.T) {
	s := DefaultSettings()
	s.Viewers = []Viewer{
		{Name: "Mom", Addresses: []string{"203.0.113.9"}, Limit: 1},
		{Name: "Office", Addresses: []string{"198.51.100.0/24", "tailscale:dad@example.com"}, Limit: 1},
	}
	for in, want := range map[string]string{"203.0.113.9": "Mom", "198.51.100.77": "Office", "203.0.113.10": "", "2001:db8::1": ""} {
		got, ok := s.ViewerHolding(netip.MustParseAddr(in), "")
		if got != want || ok != (want != "") {
			t.Errorf("%s: %q %v, want %q", in, got, ok, want)
		}
	}
	if _, ok := s.ViewerHolding(netip.MustParseAddr("203.0.113.9"), "mom"); ok {
		t.Error("a viewer's own address counted against them")
	}
}
