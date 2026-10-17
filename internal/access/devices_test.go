package access

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func intp(n int) *int { return &n }

func dev(name, listen string) Device {
	return Device{Name: name, Listen: listen, Tuner: "192.168.1.132"}
}

// ---- settings ----

func TestDeviceValidation(t *testing.T) {
	good := Settings{Devices: []Device{
		{Name: "antenna", Listen: ":5004", Tuner: "192.168.1.132", ID: "10ABCC87", StreamPort: "5004", ReportID: "10abcd86", OwnerReserve: intp(0), SharedCap: intp(2)},
		{Name: "cable.box-2", Listen: "127.0.0.1:5005", ID: "1111AAA2"}, // an id alone is enough: it will be searched for
	}}
	if err := good.Validate(); err != nil {
		t.Fatalf("good devices rejected: %v", err)
	}

	d := func(mut func(*Device)) Settings {
		x := dev("a", ":5004")
		mut(&x)
		return Settings{Devices: []Device{x}}
	}
	bad := map[string]Settings{
		"empty name":           d(func(x *Device) { x.Name = "" }),
		"space in name":        d(func(x *Device) { x.Name = "my antenna" }),
		"slash in name":        d(func(x *Device) { x.Name = "a/b" }),
		"long name":            d(func(x *Device) { x.Name = strings.Repeat("a", 41) }),
		"no listen":            d(func(x *Device) { x.Listen = "" }),
		"listen without port":  d(func(x *Device) { x.Listen = "localhost" }),
		"bare port":            d(func(x *Device) { x.Listen = "5004" }),
		"port zero":            d(func(x *Device) { x.Listen = ":0" }),
		"port too big":         d(func(x *Device) { x.Listen = ":70000" }),
		"no tuner or id":       d(func(x *Device) { x.Tuner = "" }),
		"blank tuner":          d(func(x *Device) { x.Tuner = "   " }),
		"bad id":               d(func(x *Device) { x.ID = "XYZ" }),
		"short id":             d(func(x *Device) { x.ID = "10ABCC8" }),
		"bad reportId":         d(func(x *Device) { x.ReportID = "nothex!!" }),
		"bad streamPort":       d(func(x *Device) { x.StreamPort = "http" }),
		"negative reserve":     d(func(x *Device) { x.OwnerReserve = intp(-1) }),
		"negative cap":         d(func(x *Device) { x.SharedCap = intp(-1) }),
		"duplicate name":       {Devices: []Device{dev("a", ":5004"), dev("a", ":5005")}},
		"name differs by case": {Devices: []Device{dev("Antenna", ":5004"), dev("antenna", ":5005")}},
		"same port":            {Devices: []Device{dev("a", ":5004"), dev("b", "127.0.0.1:5004")}},
	}
	for name, s := range bad {
		if err := s.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestViewerLimitsPerDevice(t *testing.T) {
	devs := []Device{dev("antenna", ":5004"), dev("cable", ":5005")}
	v := func(limits map[string]int) Settings {
		return Settings{Devices: devs, Viewers: []Viewer{{Name: "John", Addresses: []string{"1.2.3.4"}, Limit: 1, Limits: limits}}}
	}
	if err := v(map[string]int{"cable": 3, "antenna": 0}).Validate(); err != nil {
		t.Errorf("valid per-device limits rejected: %v", err)
	}
	for name, s := range map[string]Settings{
		"unknown device":      v(map[string]int{"satellite": 1}),
		"negative":            v(map[string]int{"cable": -1}),
		"no devices declared": {Viewers: []Viewer{{Name: "John", Addresses: []string{"1.2.3.4"}, Limits: map[string]int{"x": 1}}}},
	} {
		if err := s.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestLimitForFallsBackToTheDefault(t *testing.T) {
	v := Viewer{Limit: 2, Limits: map[string]int{"cable": 0, "antenna": 4}}
	for dev, want := range map[string]int{"antenna": 4, "cable": 0, "other": 2} { // 0 is a real override (paused), not "unset"
		if got := v.limitFor(dev); got != want {
			t.Errorf("limitFor(%q) = %d, want %d", dev, got, want)
		}
	}
	if got := (Viewer{Limit: 3}).limitFor("x"); got != 3 {
		t.Errorf("no overrides: %d", got)
	}
}

func TestDeviceOverridesBeatDefaults(t *testing.T) {
	s := Settings{OwnerReserve: 1, SharedCap: 3, Devices: []Device{
		{Name: "a", Listen: ":1", Tuner: "x"},                                            // no overrides
		{Name: "b", Listen: ":2", Tuner: "x", OwnerReserve: intp(0), SharedCap: intp(0)}, // explicit zeros
		{Name: "c", Listen: ":3", Tuner: "x", OwnerReserve: intp(2), SharedCap: intp(5)},
	}}
	p, err := s.parse()
	if err != nil {
		t.Fatal(err)
	}
	type rc struct{ reserve, cap int }
	want := map[string]rc{"a": {1, 3}, "b": {0, 0}, "c": {2, 5}, "undeclared": {1, 3}}
	for name, w := range want {
		if got := (rc{p.reserveFor(name), p.capFor(name)}); got != w {
			t.Errorf("%s: %+v, want %+v (an explicit 0 must override, not fall back)", name, got, w)
		}
	}
}

func TestCloneIsDeep(t *testing.T) {
	orig := Settings{
		Devices: []Device{{Name: "a", Listen: ":1", Tuner: "x", OwnerReserve: intp(1)}},
		Viewers: []Viewer{{Name: "J", Addresses: []string{"1.2.3.4"}, Limit: 1, Limits: map[string]int{"a": 2}}},
		Admin:   []string{"10.0.0.0/8"},
	}
	c := orig.clone()
	*c.Devices[0].OwnerReserve = 99
	c.Devices[0].Name = "changed"
	c.Viewers[0].Limits["a"] = 99
	c.Viewers[0].Addresses[0] = "9.9.9.9"
	c.Admin[0] = "changed"

	if *orig.Devices[0].OwnerReserve != 1 || orig.Devices[0].Name != "a" || orig.Viewers[0].Limits["a"] != 2 ||
		orig.Viewers[0].Addresses[0] != "1.2.3.4" || orig.Admin[0] != "10.0.0.0/8" {
		t.Errorf("clone shares state with the original: %+v", orig)
	}
	if got := (Settings{}).clone(); got.Devices != nil || got.Viewers == nil {
		t.Errorf("empty clone: %+v", got)
	}
}

func TestDevicesSurviveSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "antennarr.json")
	in := Settings{
		OwnerReserve: 1,
		Devices: []Device{
			{Name: "antenna", Listen: ":5004", Tuner: "192.168.1.132", ID: "10ABCC87", ReportID: "10ABCD86", OwnerReserve: intp(0), SharedCap: intp(2)},
			{Name: "cable", Listen: ":5005", ID: "1111AAA2"},
		},
		Viewers: []Viewer{{Name: "John", Addresses: []string{"1.2.3.4"}, Limit: 1, Limits: map[string]int{"cable": 3}}},
	}
	if err := SaveSettings(path, in); err != nil {
		t.Fatal(err)
	}
	out, _, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	a, c := out.Devices[0], out.Devices[1]
	if len(out.Devices) != 2 || a.Name != "antenna" || a.ReportID != "10ABCD86" || *a.OwnerReserve != 0 || *a.SharedCap != 2 {
		t.Errorf("antenna = %+v", a)
	}
	if c.OwnerReserve != nil || c.SharedCap != nil || c.Tuner != "" {
		t.Errorf("unset fields came back set: %+v", c)
	}
	if out.Viewers[0].Limits["cable"] != 3 {
		t.Errorf("viewer limits lost: %+v", out.Viewers[0])
	}
}

func TestSettingsWithoutDevicesStayWithoutThem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.json")
	if err := SaveSettings(path, DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "devices") {
		t.Errorf("flag-configured setups must not gain a devices section: %s", b)
	}
}

// ---- policy across devices ----

func newMulti(t *testing.T, s Settings, mons map[string]*fakeMon) (*Policy, *fakeNotes) {
	t.Helper()
	n := &fakeNotes{}
	p, err := NewPolicy(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, m := range mons {
		p.AddDevice(name, m, n)
	}
	return p, n
}

func twoDevices() (Settings, map[string]*fakeMon) {
	a, b := &fakeMon{}, &fakeMon{}
	a.set(true, 4)
	b.set(true, 2)
	s := Settings{
		Devices: []Device{dev("antenna", ":5004"), dev("cable", ":5005")},
		Viewers: []Viewer{{Name: "John", Addresses: []string{"100.67.117.32"}, Limit: 1, Limits: map[string]int{"cable": 2}}},
	}
	return s, map[string]*fakeMon{"antenna": a, "cable": b}
}

func TestLimitsAreCountedPerDevice(t *testing.T) {
	s, mons := twoDevices()
	p, _ := newMulti(t, s, mons)
	john := req("100.67.117.32:1")
	antenna, cable := p.Device("antenna"), p.Device("cable")

	if _, err := antenna.Stream(john); err != nil {
		t.Fatal(err)
	}
	// John is at his limit on the antenna, but that doesn't touch the cable box.
	if _, err := antenna.Stream(john); denied(t, err).Reason != ReasonViewerLimit {
		t.Error("second antenna stream should be refused")
	}
	if _, err := cable.Stream(john); err != nil {
		t.Errorf("first cable stream: %v", err)
	}
	if _, err := cable.Stream(john); err != nil { // his cable limit is 2, from the override
		t.Errorf("second cable stream (override of 2): %v", err)
	}
	if _, err := cable.Stream(john); denied(t, err).Reason != ReasonViewerLimit {
		t.Error("third cable stream should be refused")
	}
}

func TestSharedCapAndReserveArePerDevice(t *testing.T) {
	s, mons := twoDevices()
	s.Viewers[0].Limit = 9
	s.Viewers[0].Limits = nil
	s.Devices[0].SharedCap = intp(1)    // antenna: one viewer stream at most
	s.Devices[1].OwnerReserve = intp(2) // cable has 2 tuners and keeps both for the owner
	s.Devices[1].SharedCap = nil
	p, _ := newMulti(t, s, mons)
	john := req("100.67.117.32:1")

	if _, err := p.Device("antenna").Stream(john); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Device("antenna").Stream(john); denied(t, err).Reason != ReasonSharedCap {
		t.Error("antenna's cap of 1 not applied")
	}
	if _, err := p.Device("cable").Stream(john); denied(t, err).Reason != ReasonOwnerReserve {
		t.Error("cable's reserve of 2 not applied")
	}
}

func TestOwnersLocalUseOnOneDeviceDoesNotAffectAnother(t *testing.T) {
	s, mons := twoDevices()
	s.Viewers[0].Limit = 9
	s.Viewers[0].Limits = nil
	s.OwnerReserve = 1
	mons["antenna"].set(true, 4, local, local, local) // the owner's TVs hold 3 of 4 antenna tuners
	p, _ := newMulti(t, s, mons)
	john := req("100.67.117.32:1")

	if _, err := p.Device("antenna").Stream(john); denied(t, err).Reason != ReasonOwnerReserve {
		t.Error("antenna has nothing left for viewers")
	}
	if _, err := p.Device("cable").Stream(john); err != nil {
		t.Errorf("cable is untouched by antenna use: %v", err)
	}
}

func TestAdvertisedTunerCountIsPerDevice(t *testing.T) {
	s, mons := twoDevices()
	s.OwnerReserve = 1
	s.Devices[1].OwnerReserve = intp(0)
	p, _ := newMulti(t, s, mons)
	john := req("100.67.117.32:1")
	cases := []struct {
		device string
		real   int
		want   int
	}{
		{"antenna", 4, 1}, // limit 1 beats 4-1
		{"cable", 2, 2},   // override limit 2, reserve 0
	}
	for _, c := range cases {
		if got := p.Device(c.device).TunerCount(john, c.real); got != c.want {
			t.Errorf("%s: TunerCount = %d, want %d", c.device, got, c.want)
		}
	}
}

func TestOneViewerIsOnePersonAcrossDevices(t *testing.T) {
	s, mons := twoDevices()
	p, _ := newMulti(t, s, mons)
	now := time.Unix(3_000_000, 0)
	p.now = func() time.Time { return now }

	if err := p.Device("cable").Authorize(req("100.67.117.32:1")); err != nil {
		t.Fatal(err)
	}
	// Contact with any device counts as the viewer having been seen.
	if v := p.ViewerStatuses()[0]; v.LastSeen == nil || !v.LastSeen.Equal(now) {
		t.Errorf("lastSeen = %v", v.LastSeen)
	}
	// And a stranger is a stranger on every device.
	for _, d := range []string{"antenna", "cable"} {
		if err := p.Device(d).Authorize(req("203.0.113.9:1")); err == nil {
			t.Errorf("%s let a stranger in", d)
		}
	}
}

func TestStreamsKnowTheirDevice(t *testing.T) {
	s, mons := twoDevices()
	p, _ := newMulti(t, s, mons)
	john := req("100.67.117.32:1")
	p.Device("antenna").Stream(john)
	p.Device("cable").Stream(john)

	act := p.Active()
	if len(act) != 2 || act[0].Device != "antenna" || act[1].Device != "cable" {
		t.Fatalf("active = %+v", act)
	}
	v := p.ViewerStatuses()[0]
	if v.InUse != 2 || v.InUseByDevice["antenna"] != 1 || v.InUseByDevice["cable"] != 1 {
		t.Errorf("viewer use = %d %v", v.InUse, v.InUseByDevice)
	}
}

func TestRefusalNotesNameTheDeviceOnlyWhenThereAreSeveral(t *testing.T) {
	s, mons := twoDevices()
	p, notes := newMulti(t, s, mons)
	john := req("100.67.117.32:1")
	p.Device("antenna").Stream(john)
	p.Device("antenna").Stream(john) // refused
	p.Device("antenna").Stream(john) // refused again: same note, rate limited

	if notes.count() != 1 || notes.notes[0] != "warn: John was refused a tuner on antenna: all 1 of your tuners are in use" {
		t.Fatalf("notes = %q", notes.notes)
	}

	// The same viewer being refused on a different device is a different story.
	for i := 0; i < 3; i++ {
		p.Device("cable").Stream(john)
	}
	if notes.count() != 2 || !strings.Contains(notes.notes[1], "on cable") {
		t.Errorf("notes = %q", notes.notes)
	}
}

func TestUnknownDeviceViewRefusesAsOffline(t *testing.T) {
	s, mons := twoDevices()
	p, _ := newMulti(t, s, mons)
	_, err := p.Device("ghost").Stream(req("100.67.117.32:1"))
	if d := denied(t, err); d.Reason != ReasonOffline {
		t.Errorf("%+v", d)
	}
}

func TestOneDevicesOutageDoesNotStopTheOther(t *testing.T) {
	s, mons := twoDevices()
	mons["antenna"].set(false, 4)
	p, _ := newMulti(t, s, mons)
	john := req("100.67.117.32:1")
	if _, err := p.Device("antenna").Stream(john); denied(t, err).Reason != ReasonOffline {
		t.Error("offline antenna accepted a stream")
	}
	if _, err := p.Device("cable").Stream(john); err != nil {
		t.Errorf("cable refused because antenna is down: %v", err)
	}
}

func TestLimitsForADevice(t *testing.T) {
	s, mons := twoDevices()
	s.OwnerReserve = 1
	s.SharedCap = 3
	s.Devices[0].OwnerReserve = intp(0)
	p, _ := newMulti(t, s, mons)
	if r, c := p.Limits("antenna"); r != 0 || c != 3 {
		t.Errorf("antenna = %d, %d", r, c)
	}
	if r, c := p.Limits("cable"); r != 1 || c != 3 {
		t.Errorf("cable = %d, %d", r, c)
	}
	// Settings edits apply live.
	s.Devices[1].SharedCap = intp(1)
	if err := p.Update(s); err != nil {
		t.Fatal(err)
	}
	if _, c := p.Limits("cable"); c != 1 {
		t.Errorf("after update: cap = %d", c)
	}
}
