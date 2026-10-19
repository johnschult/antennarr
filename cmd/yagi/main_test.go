package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnschult/yagi/internal/access"
)

var defaults = deviceFlags{listen: ":5004", streamPort: "5004"}

func TestDevicesFromFlags(t *testing.T) {
	f := defaults
	f.tuner, f.id, f.advertise, f.reportID = "192.168.1.132", "10abcc87", "http://relay:5004", "10ABCD86"
	got, err := devicesFor(access.DefaultSettings(), f, map[string]bool{"tuner": true, "tuner-id": true})
	if err != nil {
		t.Fatal(err)
	}
	want := access.Device{Name: "tuner", Listen: ":5004", Tuner: "192.168.1.132", ID: "10abcc87", StreamPort: "5004", Advertise: "http://relay:5004", ReportID: "10ABCD86"}
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestNoTunerNamedMeansStartingWithNone(t *testing.T) {
	got, err := devicesFor(access.DefaultSettings(), defaults, nil)
	if err != nil || len(got) != 0 {
		t.Errorf("got %+v, %v: with nothing named, expected no devices and no error (tuners are added from the admin page)", got, err)
	}
	f := defaults
	f.id = "10ABCC87" // an id alone is enough
	if _, err := devicesFor(access.DefaultSettings(), f, nil); err != nil {
		t.Errorf("id alone: %v", err)
	}
}

func TestBadFlagValuesAreCaught(t *testing.T) {
	for name, mut := range map[string]func(*deviceFlags){
		"bad listen": func(f *deviceFlags) { f.listen = "5004" },
		"bad id":     func(f *deviceFlags) { f.id = "nope" },
		"bad port":   func(f *deviceFlags) { f.streamPort = "http" },
	} {
		f := defaults
		f.tuner = "192.168.1.132"
		mut(&f)
		if _, err := devicesFor(access.DefaultSettings(), f, nil); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestDevicesFromTheSettingsFile(t *testing.T) {
	s := access.Settings{Devices: []access.Device{
		{Name: "antenna", Listen: ":5004", Tuner: "192.168.1.132"},
		{Name: "cable", Listen: ":5005", ID: "1111AAA2"},
	}}
	got, err := devicesFor(s, defaults, map[string]bool{"poll": true}) // flags that aren't about devices are fine
	if err != nil || len(got) != 2 || got[1].Name != "cable" {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestFlagsAndFileDevicesDoNotMix(t *testing.T) {
	s := access.Settings{Devices: []access.Device{{Name: "antenna", Listen: ":5004", Tuner: "x"}}}
	_, err := devicesFor(s, defaults, map[string]bool{"tuner": true, "listen": true, "poll": true})
	if err == nil {
		t.Fatal("expected a conflict")
	}
	for _, want := range []string{"-tuner", "-listen", "settings file"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "-poll") {
		t.Errorf("-poll is not a device flag: %v", err)
	}
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// With no settings file, one is created holding the defaults, and it is the
// file edits are saved to.
func TestMissingSettingsFileIsCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "yagi.json")
	s, save, err := loadOrCreateSettings(path, false, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	if save != path {
		t.Errorf("save path = %q, want %q", save, path)
	}
	if s.OwnerReserve != access.DefaultSettings().OwnerReserve {
		t.Errorf("not the defaults: %+v", s)
	}
	got, exists, err := access.LoadSettings(path)
	if err != nil || !exists {
		t.Fatalf("the file wasn't created: exists=%v err=%v", exists, err)
	}
	if len(got.Viewers) != 0 || len(got.Devices) != 0 {
		t.Errorf("a new file should hold no viewers or devices: %+v", got)
	}
}

// An existing file is read, not overwritten.
func TestExistingSettingsFileIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "yagi.json")
	want := access.DefaultSettings()
	want.Viewers = []access.Viewer{{Name: "Mom", Addresses: []string{"10.8.0.5"}, Limit: 1}}
	if err := access.SaveSettings(path, want); err != nil {
		t.Fatal(err)
	}
	s, save, err := loadOrCreateSettings(path, true, quietLog())
	if err != nil || save != path {
		t.Fatalf("save=%q err=%v", save, err)
	}
	if len(s.Viewers) != 1 || s.Viewers[0].Name != "Mom" {
		t.Errorf("the existing file was not used: %+v", s)
	}
}

// A file that can't be created is an error when the user named it, and only a
// warning for the default path.
func TestUncreatableSettingsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "yagi.json")

	if _, _, err := loadOrCreateSettings(path, true, quietLog()); err == nil {
		t.Error("an explicit -config that can't be created should be an error")
	}
	s, save, err := loadOrCreateSettings(path, false, quietLog())
	if err != nil {
		t.Fatalf("the default path failing should only warn: %v", err)
	}
	if save != "" {
		t.Errorf("save path = %q, want empty (read-only)", save)
	}
	if s.OwnerReserve != access.DefaultSettings().OwnerReserve {
		t.Errorf("not the defaults: %+v", s)
	}
}

// A damaged file stops start-up rather than being replaced by defaults.
func TestBrokenSettingsFileIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "yagi.json")
	if err := os.WriteFile(path, []byte(`{"viewrs": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadOrCreateSettings(path, false, quietLog()); err == nil {
		t.Fatal("a file with an unknown key should fail to load")
	}
	b, _ := os.ReadFile(path)
	if string(b) != `{"viewrs": []}` {
		t.Errorf("the broken file was overwritten: %s", b)
	}
}
