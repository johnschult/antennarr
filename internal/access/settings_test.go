package access

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	good := Settings{
		OwnerReserve: 1, SharedCap: 2, Admin: []string{"100.64.0.0/10", "::1"},
		Viewers: []Viewer{
			{Name: "John", Addresses: []string{"100.67.117.32"}, Limit: 2},
			{Name: "Mom", Addresses: []string{"10.8.0.0/24", "fd00::/8"}, Limit: 0},
		},
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("good settings rejected: %v", err)
	}
	if err := (Settings{}).Validate(); err != nil {
		t.Errorf("empty settings rejected: %v", err)
	}

	v := func(name string, limit int, addrs ...string) Viewer {
		return Viewer{Name: name, Addresses: addrs, Limit: limit}
	}
	bad := map[string]Settings{
		"empty name":           {Viewers: []Viewer{v("  ", 1, "1.2.3.4")}},
		"duplicate name":       {Viewers: []Viewer{v("John", 1, "1.2.3.4"), v("john", 1, "1.2.3.5")}},
		"no addresses":         {Viewers: []Viewer{v("John", 1)}},
		"bad ip":               {Viewers: []Viewer{v("John", 1, "300.1.1.1")}},
		"bad cidr":             {Viewers: []Viewer{v("John", 1, "10.0.0.0/33")}},
		"hostname":             {Viewers: []Viewer{v("John", 1, "tower.local")}},
		"negative limit":       {Viewers: []Viewer{v("John", -1, "1.2.3.4")}},
		"negative cap":         {SharedCap: -1},
		"negative reserve":     {OwnerReserve: -1},
		"bad admin":            {Admin: []string{"nope"}},
		"same address twice":   {Viewers: []Viewer{v("A", 1, "1.2.3.4"), v("B", 1, "1.2.3.4")}},
		"same address, mapped": {Viewers: []Viewer{v("A", 1, "192.168.1.5"), v("B", 1, "::ffff:192.168.1.5")}},
	}
	for name, s := range bad {
		if err := s.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestParsePrefixMasksHostBits(t *testing.T) {
	p, err := parsePrefix("10.1.2.3/8")
	if err != nil || p.String() != "10.0.0.0/8" {
		t.Errorf("got %v, %v", p, err)
	}
	p, err = parsePrefix("::ffff:10.1.2.3/104") // IPv4-mapped /8
	if err != nil || p.String() != "10.0.0.0/8" {
		t.Errorf("mapped prefix: got %v, %v", p, err)
	}
}

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	s, exists, err := LoadSettings(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
	if s.OwnerReserve != 1 || len(s.Viewers) != 0 {
		t.Errorf("defaults = %+v", s)
	}
}

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "antennarr.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadRejectsBadFiles(t *testing.T) {
	cases := map[string]string{
		"malformed":      `{"viewers": [`,
		"unknown field":  `{"viewrs": []}`, // a typo must not silently open access
		"wrong type":     `{"ownerReserve": "one"}`,
		"trailing data":  `{"ownerReserve": 1} {"ownerReserve": 2}`,
		"invalid values": `{"viewers":[{"name":"John","addresses":["bogus"],"limit":1}]}`,
	}
	for name, content := range cases {
		if _, exists, err := LoadSettings(write(t, content)); err == nil || !exists {
			t.Errorf("%s: err=%v exists=%v", name, err, exists)
		}
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "antennarr.json")
	in := Settings{
		OwnerReserve: 1, SharedCap: 2, Admin: []string{"100.64.0.0/10"},
		Viewers: []Viewer{{Name: "John", Addresses: []string{"100.67.117.32"}, Limit: 2}},
	}
	if err := SaveSettings(path, in); err != nil {
		t.Fatal(err)
	}
	out, exists, err := LoadSettings(path)
	if err != nil || !exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
	if out.SharedCap != 2 || len(out.Viewers) != 1 || out.Viewers[0].Name != "John" || out.Viewers[0].Limit != 2 || out.Admin[0] != "100.64.0.0/10" {
		t.Errorf("round trip changed settings: %+v", out)
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
}

func TestSaveWritesEmptyListsNotNull(t *testing.T) {
	path := filepath.Join(t.TempDir(), "antennarr.json")
	if err := SaveSettings(path, DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "null") {
		t.Errorf("file contains null: %s", b)
	}
}

func TestSaveInvalidKeepsOldFile(t *testing.T) {
	path := write(t, `{"ownerReserve": 3}`)
	err := SaveSettings(path, Settings{Viewers: []Viewer{{Name: "x"}}})
	if err == nil {
		t.Fatal("saved invalid settings")
	}
	s, _, _ := LoadSettings(path)
	if s.OwnerReserve != 3 {
		t.Errorf("old file was clobbered: %+v", s)
	}
}

func TestSaveToMissingDirFails(t *testing.T) {
	if err := SaveSettings(filepath.Join(t.TempDir(), "no", "such", "dir", "a.json"), DefaultSettings()); err == nil {
		t.Error("expected an error")
	}
}

func TestWarnsWhenAViewerIsInsideAnAdminRange(t *testing.T) {
	s := Settings{
		Admin:   []string{"100.64.0.0/10"},
		Viewers: []Viewer{{Name: "Mom", Addresses: []string{"100.64.0.7"}, Limit: 1}},
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("an overlap must not stop the settings loading: %v", err)
	}
	w := s.Warnings()
	if len(w) != 1 || !strings.Contains(w[0], "Mom") || !strings.Contains(w[0], "100.64.0.0/10") {
		t.Errorf("warnings = %q", w)
	}

	s.Admin = []string{"100.64.0.1/32"}
	if w := s.Warnings(); len(w) != 0 {
		t.Errorf("a narrow admin range still warned: %q", w)
	}
	s.Admin = nil
	if w := s.Warnings(); len(w) != 0 {
		t.Errorf("no admin range warned: %q", w)
	}
}
