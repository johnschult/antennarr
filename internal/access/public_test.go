package access

import (
	"reflect"
	"strings"
	"testing"
)

func TestPublicEntriesAreFoundInAnAddressList(t *testing.T) {
	got := PublicEntries([]string{
		"10.8.0.0/24", "192.168.1.5", " 203.0.113.9 ", "100.64.0.7", "fd7a:115c::/48", "2001:db8::/32",
		"tailscale:dad@example.com", "tailscale:laptop.tailnet.ts.net", "127.0.0.1", "not an address", "",
		"0.0.0.0/0", "100.0.0.0/8",
	})
	want := []string{"203.0.113.9", "2001:db8::/32", "0.0.0.0/0", "100.0.0.0/8"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("public entries = %v, want %v", got, want)
	}
	if PublicEntries(nil) != nil {
		t.Error("an empty list has public entries")
	}
}

func TestPublicAddressesAreWarnedAboutAndShownOnTheViewer(t *testing.T) {
	s := DefaultSettings()
	s.Admin = []string{"192.168.1.0/24", "198.51.100.0/24"}
	s.Viewers = []Viewer{
		{Name: "Mom", Addresses: []string{"10.8.0.5"}, Limit: 1},
		{Name: "Dad", Addresses: []string{"203.0.113.9", "10.8.0.6"}, Limit: 1},
	}
	var found []string
	for _, w := range s.Warnings() {
		if strings.Contains(w, "private networks only") {
			found = append(found, w)
		}
	}
	if len(found) != 2 || !strings.Contains(found[0], `"Dad"`) || !strings.Contains(found[0], "203.0.113.9") || !strings.Contains(found[1], "198.51.100.0/24") {
		t.Errorf("warnings = %v", found)
	}
	pol, err := NewPolicy(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string][]string{}
	for _, v := range pol.ViewerStatuses() {
		by[v.Name] = v.PublicAddresses
	}
	if by["Mom"] != nil || !reflect.DeepEqual(by["Dad"], []string{"203.0.113.9"}) {
		t.Errorf("public addresses by viewer = %v", by)
	}
	// It is a warning, not an error: a settings file that lists one still loads.
	if err := s.Validate(); err != nil {
		t.Errorf("a public address made the settings invalid: %v", err)
	}
}
