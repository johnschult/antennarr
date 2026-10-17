package main

import (
	"strings"
	"testing"

	"github.com/johnschult/antennarr/internal/access"
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

func TestFlagsNeedATunerOrAnID(t *testing.T) {
	_, err := devicesFor(access.DefaultSettings(), defaults, nil)
	if err == nil || !strings.Contains(err.Error(), "-tuner") || !strings.Contains(err.Error(), "-config") {
		t.Errorf("err = %v", err)
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
