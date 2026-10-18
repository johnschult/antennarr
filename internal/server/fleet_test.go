package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/access"
)

// fleet is a server started for the add/remove tests, serving the admin pages
// on a real port (not a pre-bound listener) so that handing the port from the
// admin-only listener to a tuner is exercised for real.
type fleet struct {
	srv  *Server
	base string // http://127.0.0.1:port
	path string // the settings file
	logs *lockedBuf
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, p, _ := net.SplitHostPort(ln.Addr().String())
	return p
}

// startFleet runs a server from s, with devices already running.
func startFleet(t *testing.T, s access.Settings, devices []access.Device, withFile bool, tuners ...*fakeTuner) *fleet {
	t.Helper()
	f := &fleet{logs: &lockedBuf{}}
	off := false
	s.FirmwareCheck, s.Guide = &off, &off
	s.Devices = devices
	port := freePort(t)
	cfg := Config{
		Settings: s, Devices: devices, Poll: 50 * time.Millisecond, Locate: locateFrom(tuners...),
		Log: slog.New(slog.NewTextHandler(f.logs, nil)), AdminListen: "127.0.0.1:" + port,
	}
	if withFile {
		f.path = filepath.Join(t.TempDir(), "antennarr.json")
		if err := access.SaveSettings(f.path, s); err != nil {
			t.Fatal(err)
		}
		cfg.ConfigPath = f.path
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	f.srv = srv
	go srv.Run(ctx)
	f.base = "http://127.0.0.1:" + port
	if len(devices) > 0 {
		f.base = "http://" + devices[0].Listen
	}
	f.waitUp(t)
	return f
}

func (f *fleet) waitUp(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(f.base + "/antennarr/api/state"); err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the admin pages never came up at %s\n%s", f.base, f.logs.String())
}

func call(t *testing.T, method, url string, body any) (int, []byte) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

type stateDoc struct {
	Devices []struct {
		Name   string `json:"name"`
		Listen string `json:"listen"`
		Tuner  struct {
			Online bool   `json:"online"`
			ID     string `json:"id"`
		} `json:"tuner"`
	} `json:"devices"`
}

func (f *fleet) state(t *testing.T) stateDoc {
	t.Helper()
	code, b := call(t, "GET", f.base+"/antennarr/api/state", nil)
	if code != 200 {
		t.Fatalf("state: %d %s", code, b)
	}
	var st stateDoc
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func (f *fleet) saved(t *testing.T) access.Settings {
	t.Helper()
	s, _, err := access.LoadSettings(f.path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStartsWithNoTunersAndServesTheAdminPages(t *testing.T) {
	f := startFleet(t, access.DefaultSettings(), nil, true)
	if n := len(f.state(t).Devices); n != 0 {
		t.Errorf("devices = %d, want none", n)
	}
	// The bare address goes to the admin page, as it does on a tuner's port.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(f.base + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("GET / = %d, want a redirect to the admin page", resp.StatusCode)
	}
}

func TestAddingATunerTakesOverTheAdminPort(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	f := startFleet(t, access.DefaultSettings(), nil, true, a)

	code, body := call(t, "POST", f.base+"/antennarr/api/devices", map[string]string{"tuner": "antenna-box"})
	if code != http.StatusCreated {
		t.Fatalf("add = %d %s\n%s", code, body, f.logs.String())
	}
	var added struct {
		Name   string `json:"name"`
		Listen string `json:"listen"`
	}
	json.Unmarshal(body, &added)
	if added.Name != "tuner" {
		t.Errorf("name = %q, want tuner", added.Name)
	}
	if !strings.HasSuffix(f.base, added.Listen) {
		t.Errorf("listen = %q, want the port the admin page is on (%s)", added.Listen, f.base)
	}

	// The same address now serves the tuner and still the admin pages.
	f.waitUp(t)
	st := f.state(t)
	if len(st.Devices) != 1 || st.Devices[0].Name != "tuner" {
		t.Fatalf("state devices = %+v", st.Devices)
	}
	set := f.saved(t)
	if len(set.Devices) != 1 || !strings.EqualFold(set.Devices[0].ID, "10ABCC87") {
		t.Errorf("saved devices = %+v, want one with the DeviceID learned from the tuner", set.Devices)
	}
	if code, _ := call(t, "GET", f.base+"/discover.json", nil); code != http.StatusForbidden && code != http.StatusOK {
		t.Errorf("discover.json = %d: the tuner side isn't answering on the admin port", code)
	}
}

func TestAddingASecondTunerGetsItsOwnNameAndPort(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	b := newFakeTuner(t, "10DEF123", "cable-box")
	f := startFleet(t, access.DefaultSettings(), nil, true, a, b)
	if code, body := call(t, "POST", f.base+"/antennarr/api/devices", map[string]string{"tuner": "antenna-box"}); code != 201 {
		t.Fatalf("first add = %d %s", code, body)
	}
	f.waitUp(t)
	code, body := call(t, "POST", f.base+"/antennarr/api/devices", map[string]any{"tuner": "cable-box", "id": "10DEF123", "listen": "127.0.0.1:" + freePort(t)})
	if code != 201 {
		t.Fatalf("second add = %d %s\n%s", code, body, f.logs.String())
	}
	var added struct{ Name string }
	json.Unmarshal(body, &added)
	if added.Name != "tuner-10def123" {
		t.Errorf("second name = %q, want tuner-10def123", added.Name)
	}
	if n := len(f.saved(t).Devices); n != 2 {
		t.Errorf("saved %d devices, want 2", n)
	}
}

func TestAddingIsRefusedWhenItCannotWork(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	f := startFleet(t, access.DefaultSettings(), nil, true, a)
	url := f.base + "/antennarr/api/devices"

	for name, tc := range map[string]struct {
		body map[string]string
		code int
	}{
		"nothing to find":    {map[string]string{}, 400},
		"bad name":           {map[string]string{"tuner": "antenna-box", "name": "no spaces"}, 400},
		"nothing there":      {map[string]string{"tuner": "nowhere"}, 400}, // the locator doesn't know it
		"unknown field":      {map[string]string{"tuner": "antenna-box", "bogus": "x"}, 400},
		"bad listen address": {map[string]string{"tuner": "antenna-box", "listen": "bad"}, 400},
	} {
		if code, body := call(t, "POST", url, tc.body); code != tc.code {
			t.Errorf("%s: %d %s, want %d", name, code, body, tc.code)
		}
	}
	if n := len(f.state(t).Devices); n != 0 {
		t.Errorf("a refused add left %d devices behind", n)
	}
	if n := len(f.saved(t).Devices); n != 0 {
		t.Errorf("a refused add saved %d devices", n)
	}

	// The same tuner twice.
	if code, body := call(t, "POST", url, map[string]string{"tuner": "antenna-box"}); code != 201 {
		t.Fatalf("add = %d %s", code, body)
	}
	f.waitUp(t)
	if code, _ := call(t, "POST", f.base+"/antennarr/api/devices", map[string]string{"tuner": "antenna-box", "listen": "127.0.0.1:" + freePort(t)}); code != http.StatusConflict {
		t.Errorf("adding the same tuner twice = %d, want 409", code)
	}
}

func TestAddingNeedsASettingsFile(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	f := startFleet(t, access.DefaultSettings(), nil, false, a)
	if code, _ := call(t, "POST", f.base+"/antennarr/api/devices", map[string]string{"tuner": "antenna-box"}); code != http.StatusConflict {
		t.Errorf("add without a settings file = %d, want 409", code)
	}
}

func TestTunersFromFlagsCannotBeAddedToOrRemoved(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	dev := access.Device{Name: "antenna", Listen: "127.0.0.1:" + freePort(t), Tuner: "antenna-box", StreamPort: a.streamPort}
	// Devices given to the server but not in the settings: what -tuner does.
	f := &fleet{logs: &lockedBuf{}, path: filepath.Join(t.TempDir(), "antennarr.json")}
	s := access.DefaultSettings()
	off := false
	s.FirmwareCheck, s.Guide = &off, &off
	if err := access.SaveSettings(f.path, s); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := New(ctx, Config{Settings: s, Devices: []access.Device{dev}, ConfigPath: f.path, Poll: 50 * time.Millisecond,
		Locate: locateFrom(a), Log: slog.New(slog.NewTextHandler(f.logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Run(ctx)
	f.base = "http://" + dev.Listen
	f.waitUp(t)
	if code, _ := call(t, "DELETE", f.base+"/antennarr/api/devices/antenna", nil); code != http.StatusConflict {
		t.Errorf("removing a flag tuner = %d, want 409", code)
	}
	if code, _ := call(t, "POST", f.base+"/antennarr/api/devices", map[string]string{"tuner": "antenna-box", "name": "again", "listen": "127.0.0.1:" + freePort(t)}); code != http.StatusConflict {
		t.Errorf("adding next to a flag tuner = %d, want 409", code)
	}
}

func TestRemovingATunerCutsItsStreamsAndTidiesTheSettings(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	b := newFakeTuner(t, "10DEF123", "cable-box")
	devs := []access.Device{
		{Name: "antenna", Listen: "127.0.0.1:" + freePort(t), Tuner: "antenna-box", ID: "10ABCC87", StreamPort: a.streamPort},
		{Name: "cable", Listen: "127.0.0.1:" + freePort(t), Tuner: "cable-box", ID: "10DEF123", StreamPort: b.streamPort},
	}
	s := access.DefaultSettings()
	s.Viewers = []access.Viewer{{Name: "test", Addresses: []string{"127.0.0.1"}, Limit: 2, Limits: map[string]int{"antenna": 1, "cable": 2}}}
	s.Labels = map[string]string{"antenna": "Antenna", "cable": "Cable"}
	f := startFleet(t, s, devs, true, a, b)
	waitOnline(t, f, 2)

	// Someone is watching the antenna.
	resp, err := http.Get("http://" + devs[0].Listen + "/auto/v5.1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 8)
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatalf("stream didn't start: %v", err)
	}

	// Remove it through the page on its own port: the reply must still arrive.
	if code, body := call(t, "DELETE", "http://"+devs[0].Listen+"/antennarr/api/devices/antenna", nil); code != http.StatusNoContent {
		t.Fatalf("remove = %d %s\n%s", code, body, f.logs.String())
	}
	done := make(chan struct{})
	go func() { io.Copy(io.Discard, resp.Body); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream on the removed tuner kept running")
	}

	set := f.saved(t)
	if len(set.Devices) != 1 || set.Devices[0].Name != "cable" {
		t.Errorf("saved devices = %+v", set.Devices)
	}
	if _, ok := set.Labels["antenna"]; ok {
		t.Error("the removed tuner's label was kept")
	}
	if _, ok := set.Viewers[0].Limits["antenna"]; ok {
		t.Error("the removed tuner's viewer limit was kept")
	}
	if set.Viewers[0].Limits["cable"] != 2 {
		t.Errorf("the other tuner's limit changed: %+v", set.Viewers[0].Limits)
	}

	// The remaining one still works, and the removed one's port is free.
	f.base = "http://" + devs[1].Listen
	if n := len(f.state(t).Devices); n != 1 {
		t.Errorf("state shows %d devices, want 1", n)
	}
	if ln, err := net.Listen("tcp", devs[0].Listen); err != nil {
		t.Errorf("the removed tuner's port is still held: %v", err)
	} else {
		ln.Close()
	}
	if code, _ := call(t, "DELETE", f.base+"/antennarr/api/devices/antenna", nil); code != http.StatusNotFound {
		t.Errorf("removing it again = %d, want 404", code)
	}
}

func TestRemovingTheLastTunerBringsTheAdminPageBack(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	f := startFleet(t, access.DefaultSettings(), nil, true, a)
	if code, body := call(t, "POST", f.base+"/antennarr/api/devices", map[string]string{"tuner": "antenna-box"}); code != 201 {
		t.Fatalf("add = %d %s", code, body)
	}
	f.waitUp(t)
	// The tuner is on the port the admin page was on. Take it away again.
	if code, body := call(t, "DELETE", f.base+"/antennarr/api/devices/tuner", nil); code != 204 {
		t.Fatalf("remove = %d %s\n%s", code, body, f.logs.String())
	}
	f.waitUp(t)
	if n := len(f.state(t).Devices); n != 0 {
		t.Errorf("devices = %d, want none", n)
	}
	if n := len(f.saved(t).Devices); n != 0 {
		t.Errorf("saved devices = %d, want none", n)
	}
	// And the settings that remain are valid, so a restart works.
	if _, err := os.Stat(f.path); err != nil {
		t.Fatal(err)
	}
}

// waitOnline waits for n tuners to be online as the page at f.base sees them.
func waitOnline(t *testing.T, f *fleet, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		online := 0
		for _, d := range f.state(t).Devices {
			if d.Tuner.Online {
				online++
			}
		}
		if online == n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%d tuners never came online\n%s", n, f.logs.String())
}
