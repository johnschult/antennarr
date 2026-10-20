package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/access"
	"github.com/johnschult/yagi/internal/discovery"
)

// ---- fakes ----

// fakeTuner is an HDHomeRun: an API server and a separate streaming server.
type fakeTuner struct {
	mu          sync.Mutex
	fw, upgrade string // firmware version it reports, and a newer one it says exists
	id, name    string
	channel     string // if set, tuner0 reports someone watching this channel
	down        bool   // if set, its API answers 503, as a tuner that is unplugged would
	api         *httptest.Server
	stream      *httptest.Server
	streamPort  string
}

func newFakeTuner(t *testing.T, id, name string) *fakeTuner {
	t.Helper()
	f := &fakeTuner{id: id, name: name, fw: "20260326"}
	f.stream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(5 * time.Millisecond):
				w.Write([]byte("TSPACKET"))
				w.(http.Flusher).Flush()
			}
		}
	}))
	u, _ := url.Parse(f.stream.URL)
	f.streamPort = u.Port()
	f.api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		down := f.down
		f.mu.Unlock()
		if down {
			http.Error(w, "unplugged", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/discover.json":
			f.mu.Lock()
			fw, upgrade := f.fw, f.upgrade
			f.mu.Unlock()
			extra := ""
			if upgrade != "" {
				extra = fmt.Sprintf(`,"UpgradeAvailable":%q`, upgrade)
			}
			fmt.Fprintf(w, `{"DeviceAuth":"S3CR3T-DEVICE-AUTH","FriendlyName":%q,"ModelNumber":"HDFX-4K","FirmwareName":"hdhomerun_dvr_atsc3","FirmwareVersion":%q,"DeviceID":%q,"TunerCount":4,"BaseURL":"http://x","LineupURL":"http://x/lineup.json"%s}`, name, fw, id, extra)
		case "/lineup.json":
			fmt.Fprintf(w, `[{"GuideNumber":"5.1","GuideName":"WFRV","URL":"http://%s/auto/v5.1"}]`, "127.0.0.1:"+f.streamPort)
		case "/status.json":
			f.mu.Lock()
			ch := f.channel
			f.mu.Unlock()
			first := `{"Resource":"tuner0"}`
			if ch != "" {
				first = fmt.Sprintf(`{"Resource":"tuner0","VctNumber":%q,"VctName":"WXIA","TargetIP":"192.168.1.77","NetworkRate":9000000}`, ch)
			}
			io.WriteString(w, `[`+first+`,{"Resource":"tuner1"},{"Resource":"tuner2"},{"Resource":"tuner3"}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() { f.api.Close(); f.stream.Close() })
	return f
}

// setDown makes the tuner stop answering, or answer again.
func (f *fakeTuner) setDown(down bool) {
	f.mu.Lock()
	f.down = down
	f.mu.Unlock()
}

func (f *fakeTuner) setFirmware(version, upgrade string) {
	f.mu.Lock()
	f.fw, f.upgrade = version, upgrade
	f.mu.Unlock()
}

// locate maps a device's configured address to the fake tuner behind it.
func locateFrom(tuners ...*fakeTuner) Locator {
	return func(_ context.Context, addr, id string) (string, error) {
		for _, f := range tuners {
			if addr == f.name || strings.EqualFold(id, f.id) {
				return f.api.URL, nil
			}
		}
		return "", fmt.Errorf("no tuner %q/%q", addr, id)
	}
}

// noFind is the finder every test server uses: a fake tuner that goes quiet must not send the monitor looking
// for a real one on this network (the fakes have the DeviceID of a real FLEX, so one could be found).
func noFind(context.Context, string, string) (discovery.Device, error) {
	return discovery.Device{}, errors.New("not found: tests never look")
}

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

type rig struct {
	srv    *Server
	addr   map[string]string // device name -> host:port
	logs   *lockedBuf
	cancel context.CancelFunc
	done   chan error
}

func ephemeral(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

// loopbackViewer adds the one viewer the tests connect as (127.0.0.1): with no
// viewers a server serves nobody.
func loopbackViewer(s access.Settings) access.Settings {
	s.Viewers = []access.Viewer{{Name: "test", Addresses: []string{"127.0.0.1"}, Limit: 8}}
	return s
}

// start builds a server with the given devices on ephemeral ports and runs it.
func start(t *testing.T, s access.Settings, devices []access.Device, tuners ...*fakeTuner) *rig {
	t.Helper()
	return startWith(t, s, devices, "", tuners...)
}

// startWith is start, reading the firmware changelog from fwURL.
func startWith(t *testing.T, s access.Settings, devices []access.Device, fwURL string, tuners ...*fakeTuner) *rig {
	t.Helper()
	return startFull(t, s, devices, fwURL, "", tuners...)
}

// startFull is startWith, also reading the program guide from guideURL (and
// logos from the same server). With no guideURL the guide is off, so a test
// can never reach SiliconDust.
func startFull(t *testing.T, s access.Settings, devices []access.Device, fwURL, guideURL string, tuners ...*fakeTuner) *rig {
	t.Helper()
	return startConfigured(t, s, devices, fwURL, guideURL, nil, tuners...)
}

// startConfigured is startFull, letting a test change the server's Config first.
func startConfigured(t *testing.T, s access.Settings, devices []access.Device, fwURL, guideURL string, tweak func(*Config), tuners ...*fakeTuner) *rig {
	t.Helper()
	r := &rig{addr: map[string]string{}, logs: &lockedBuf{}, done: make(chan error, 1)}
	lns := map[string]net.Listener{}
	for _, d := range devices {
		ln := ephemeral(t)
		lns[d.Name] = ln
		r.addr[d.Name] = ln.Addr().String()
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	t.Cleanup(cancel)

	if s.FirmwareCheck == nil && fwURL == "" { // never reach the real website from a test
		off := false
		s.FirmwareCheck = &off
	}
	if s.Guide == nil && guideURL == "" {
		off := false
		s.Guide = &off
	}
	cfg := Config{GuideURL: guideURL + "/api/xmltv", GuideIconBase: guideURL,
		Settings: s, Devices: devices, Poll: 50 * time.Millisecond, Locate: locateFrom(tuners...),
		Log: slog.New(slog.NewTextHandler(r.logs, nil)), Listeners: lns, FirmwareURL: fwURL,
	}
	cfg.Find = noFind
	if tweak != nil {
		tweak(&cfg)
	}
	srv, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	r.srv = srv
	go func() { r.done <- srv.Run(ctx) }()
	r.waitOnline(t, len(devices))
	return r
}

func (r *rig) get(t *testing.T, device, path string) (int, string) {
	t.Helper()
	resp, err := http.Get("http://" + r.addr[device] + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (r *rig) waitOnline(t *testing.T, devices int) {
	t.Helper()
	var first string
	for _, a := range r.addr {
		first = a
		break
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + first + "/api/state")
		if err == nil {
			var st struct {
				Devices []struct{ Tuner struct{ Online bool } }
			}
			json.NewDecoder(resp.Body).Decode(&st)
			resp.Body.Close()
			online := 0
			for _, d := range st.Devices {
				if d.Tuner.Online {
					online++
				}
			}
			if online == devices {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("devices did not come online:\n%s", r.logs)
}

var twoDevs = func(a, c *fakeTuner) []access.Device {
	return []access.Device{
		{Name: "antenna", Listen: ":5004", Tuner: a.name, StreamPort: a.streamPort},
		{Name: "cable", Listen: ":5005", Tuner: c.name, StreamPort: c.streamPort},
	}
}

// ---- tests ----

func TestEachDeviceServesItsOwnTuner(t *testing.T) {
	a, c := newFakeTuner(t, "10ABCC87", "antenna-box"), newFakeTuner(t, "1111AAA2", "cable-box")
	r := start(t, loopbackViewer(access.DefaultSettings()), twoDevs(a, c), a, c)

	for dev, want := range map[string]string{"antenna": "antenna-box", "cable": "cable-box"} {
		code, body := r.get(t, dev, "/discover.json")
		var d map[string]any
		if err := json.Unmarshal([]byte(body), &d); err != nil || code != 200 {
			t.Fatalf("%s: %d %s", dev, code, body)
		}
		if d["FriendlyName"] != want || d["BaseURL"] != "http://"+r.addr[dev] {
			t.Errorf("%s: %v (want %s, advertised as its own listener)", dev, d, want)
		}

		_, lineup := r.get(t, dev, "/lineup.json")
		if !strings.Contains(lineup, "http://"+r.addr[dev]+"/auto/v5.1") {
			t.Errorf("%s: lineup not rewritten to this device's address: %s", dev, lineup)
		}

		resp, err := http.Get("http://" + r.addr[dev] + "/auto/v5.1")
		if err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 16)
		if _, err := io.ReadFull(resp.Body, buf); err != nil || !strings.HasPrefix(string(buf), "TSPACKET") {
			t.Errorf("%s: stream = %q (%v)", dev, buf, err)
		}
		resp.Body.Close()
	}
}

func TestDevicesAreRatedSeparately(t *testing.T) {
	a, c := newFakeTuner(t, "10ABCC87", "antenna-box"), newFakeTuner(t, "1111AAA2", "cable-box")
	s := access.Settings{
		OwnerReserve: 0,
		Devices:      twoDevs(a, c),
		Viewers:      []access.Viewer{{Name: "me", Addresses: []string{"127.0.0.1"}, Limit: 1, Limits: map[string]int{"cable": 2}}},
	}
	r := start(t, s, s.Devices, a, c)

	open := func(dev string) *http.Response {
		resp, err := http.Get("http://" + r.addr[dev] + "/auto/v5.1")
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	var keep []*http.Response
	defer func() {
		for _, x := range keep {
			x.Body.Close()
		}
	}()

	// One viewer, one limit of 1 on the antenna box, 2 on the cable box.
	ok1 := open("antenna")
	keep = append(keep, ok1)
	if ok1.StatusCode != 200 {
		t.Fatalf("first antenna stream: %d", ok1.StatusCode)
	}
	refused := open("antenna")
	refused.Body.Close()
	if refused.StatusCode != 503 || refused.Header.Get("X-Yagi-Reason") != "viewer-limit" {
		t.Errorf("second antenna stream: %d %q", refused.StatusCode, refused.Header.Get("X-Yagi-Reason"))
	}
	for i := 1; i <= 2; i++ {
		x := open("cable")
		keep = append(keep, x)
		if x.StatusCode != 200 {
			t.Errorf("cable stream %d: %d (the antenna limit must not affect it)", i, x.StatusCode)
		}
	}
	third := open("cable")
	third.Body.Close()
	if third.StatusCode != 503 {
		t.Errorf("third cable stream: %d", third.StatusCode)
	}

	// Each device is told its own number.
	for dev, want := range map[string]float64{"antenna": 1, "cable": 2} {
		_, body := r.get(t, dev, "/discover.json")
		var d map[string]any
		json.Unmarshal([]byte(body), &d)
		if d["TunerCount"] != want {
			t.Errorf("%s TunerCount = %v, want %v", dev, d["TunerCount"], want)
		}
	}
}

func TestAdminPagesAreOnEveryPort(t *testing.T) {
	a, c := newFakeTuner(t, "10ABCC87", "antenna-box"), newFakeTuner(t, "1111AAA2", "cable-box")
	r := start(t, access.DefaultSettings(), twoDevs(a, c), a, c)

	for dev := range r.addr {
		code, body := r.get(t, dev, "/api/state")
		var st struct{ Devices []struct{ Name string } }
		json.Unmarshal([]byte(body), &st)
		if code != 200 || len(st.Devices) != 2 || st.Devices[0].Name != "antenna" || st.Devices[1].Name != "cable" {
			t.Errorf("%s: %d %s", dev, code, body)
		}
		if code, _ := r.get(t, dev, "/streams"); code != 200 {
			t.Errorf("%s: /streams %d", dev, code)
		}
		if code, _ := r.get(t, dev, "/"); code != 200 && code != 503 { // 503: the UI isn't built in this checkout
			t.Errorf("%s: UI %d", dev, code)
		}
	}
}

func TestBareAddressGoesToTheAdminPage(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	devs := []access.Device{{Name: "antenna", Listen: ":5004", Tuner: a.name, StreamPort: a.streamPort}}
	r := start(t, access.DefaultSettings(), devs, a)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get("http://" + r.addr["antenna"] + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 503 { // 503: the UI isn't built in this checkout
		t.Errorf("GET / = %d, want the admin page", resp.StatusCode)
	}
}

func TestStrangersAreRefusedOnEveryDevice(t *testing.T) {
	a, c := newFakeTuner(t, "10ABCC87", "antenna-box"), newFakeTuner(t, "1111AAA2", "cable-box")
	s := access.Settings{Devices: twoDevs(a, c), Viewers: []access.Viewer{{Name: "someone", Addresses: []string{"203.0.113.9"}, Limit: 1}}}
	r := start(t, s, s.Devices, a, c)
	// Our test client is 127.0.0.1: loopback is an admin but not a viewer.
	for dev := range r.addr {
		if code, _ := r.get(t, dev, "/discover.json"); code != 403 {
			t.Errorf("%s: discover.json from a non-viewer: %d", dev, code)
		}
		if code, _ := r.get(t, dev, "/api/state"); code != 200 {
			t.Errorf("%s: the admin should still get in: %d", dev, code)
		}
	}
}

func TestRunStopsWhenAskedAndReleasesThePorts(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	devs := []access.Device{{Name: "antenna", Listen: ":5004", Tuner: a.name, StreamPort: a.streamPort}}
	r := start(t, access.DefaultSettings(), devs, a)

	r.cancel()
	select {
	case err := <-r.done:
		if err != nil {
			t.Errorf("Run returned %v on a normal stop", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
	if _, err := http.Get("http://" + r.addr["antenna"] + "/discover.json"); err == nil {
		t.Error("still serving after shutdown")
	}
}

func TestAPortInUseFailsEverythingCleanly(t *testing.T) {
	a, c := newFakeTuner(t, "10ABCC87", "antenna-box"), newFakeTuner(t, "1111AAA2", "cable-box")
	taken := ephemeral(t) // someone else already has this port
	defer taken.Close()
	first := ephemeral(t)

	devs := []access.Device{
		{Name: "antenna", Listen: "127.0.0.1:1", Tuner: a.name, StreamPort: a.streamPort},
		{Name: "cable", Listen: taken.Addr().String(), Tuner: c.name, StreamPort: c.streamPort},
	}
	srv, err := New(context.Background(), Config{
		Find: noFind, Settings: access.DefaultSettings(), Devices: devs, Locate: locateFrom(a, c),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Listeners: map[string]net.Listener{"antenna": first},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = srv.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), `device "cable"`) {
		t.Fatalf("err = %v, want a failure naming the device whose port is taken", err)
	}
	// The device that did bind must not be left running.
	if conn, err := net.DialTimeout("tcp", first.Addr().String(), 200*time.Millisecond); err == nil {
		conn.Close()
		t.Error("the first device was left listening after the second failed to start")
	}
}

func TestNewRefusesBadConfiguration(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	good := access.Device{Name: "antenna", Listen: ":5004", Tuner: a.name, StreamPort: a.streamPort}

	for name, cfg := range map[string]Config{
		"duplicate ports": {Settings: access.DefaultSettings(), Devices: []access.Device{good, {Name: "b", Listen: ":5004", Tuner: a.name}}, Log: log, Locate: locateFrom(a)},
		"bad device":      {Settings: access.DefaultSettings(), Devices: []access.Device{{Name: "bad name", Listen: ":5004", Tuner: "x"}}, Log: log},
		"bad settings":    {Settings: access.Settings{OwnerReserve: -1}, Devices: []access.Device{good}, Log: log, Locate: locateFrom(a)},
	} {
		if _, err := New(context.Background(), cfg); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	_, err := New(context.Background(), Config{
		Settings: access.DefaultSettings(), Log: log, Locate: locateFrom(a),
		Devices: []access.Device{good, {Name: "ghost", Listen: ":5005", ID: "FFFFFFFF"}}, // this tuner can't be found
	})
	if err == nil || !strings.Contains(err.Error(), `device "ghost"`) {
		t.Errorf("err = %v, want it to name the device that couldn't be found", err)
	}
}

func TestReloadAppliesLimitsAtOnce(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	s := access.Settings{
		OwnerReserve: 0,
		Viewers:      []access.Viewer{{Name: "me", Addresses: []string{"127.0.0.1"}, Limit: 1}},
	}
	devs := []access.Device{{Name: "antenna", Listen: ":5004", Tuner: a.name, StreamPort: a.streamPort}}
	r := start(t, s, devs, a)

	count := func() float64 {
		_, body := r.get(t, "antenna", "/discover.json")
		var d map[string]any
		json.Unmarshal([]byte(body), &d)
		return d["TunerCount"].(float64)
	}
	if count() != 1 {
		t.Fatalf("TunerCount = %v", count())
	}
	s.Viewers[0].Limit = 3
	if err := r.srv.Reload(s); err != nil {
		t.Fatal(err)
	}
	if count() != 3 {
		t.Errorf("after reload TunerCount = %v, want 3", count())
	}
	if strings.Contains(r.logs.String(), "restart Yagi") {
		t.Error("a limits-only reload asked for a restart")
	}
}

func TestReloadKeepsOldSettingsWhenTheNewOnesAreBad(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	s := access.Settings{Viewers: []access.Viewer{{Name: "me", Addresses: []string{"127.0.0.1"}, Limit: 2}}}
	devs := []access.Device{{Name: "antenna", Listen: ":5004", Tuner: a.name, StreamPort: a.streamPort}}
	r := start(t, s, devs, a)

	bad := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(bad, []byte(`{"viewers":[{"name":"x","addresses":["nope"],"limit":1}]}`), 0o600)
	if err := r.srv.ReloadFile(bad); err == nil {
		t.Fatal("a bad file was accepted")
	}
	if got := r.srv.Policy().Settings(); len(got.Viewers) != 1 || got.Viewers[0].Name != "me" || got.Viewers[0].Limit != 2 {
		t.Errorf("a bad reload changed the running settings: %+v", got)
	}

	good := filepath.Join(t.TempDir(), "good.json")
	os.WriteFile(good, []byte(`{"viewers":[{"name":"me","addresses":["127.0.0.1"],"limit":3}]}`), 0o600)
	if err := r.srv.ReloadFile(good); err != nil {
		t.Fatal(err)
	}
	if got := r.srv.Policy().Settings(); got.Viewers[0].Limit != 3 {
		t.Errorf("a good file was not applied: %+v", got)
	}
}

func TestReloadWarnsWhenDevicesChange(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	devs := []access.Device{{Name: "antenna", Listen: ":5004", Tuner: a.name, StreamPort: a.streamPort}}
	r := start(t, access.Settings{Devices: devs, OwnerReserve: 1}, devs, a)

	// A per-device limit is not a restart-worthy change...
	changed := access.Settings{Devices: append([]access.Device(nil), devs...), OwnerReserve: 1}
	two := 2
	changed.Devices[0].SharedCap = &two
	if err := r.srv.Reload(changed); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.logs.String(), "restart Yagi") {
		t.Fatal("changing a device's cap asked for a restart")
	}

	// ...but moving a device to another port is.
	changed.Devices[0].Listen = ":6000"
	if err := r.srv.Reload(changed); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.logs.String(), "restart Yagi") {
		t.Errorf("no restart notice after the device moved ports:\n%s", r.logs)
	}
}

func TestRestartKeys(t *testing.T) {
	d := access.Device{Name: "a", Listen: ":1", Tuner: "x", ID: "10abcc87", ReportID: "10abcd86"}
	e := d
	n := 5
	e.OwnerReserve, e.SharedCap = &n, &n
	if !equal(restartKeys([]access.Device{d}), restartKeys([]access.Device{e})) {
		t.Error("limit overrides are not restart-worthy")
	}
	f := d
	f.ID = "10ABCC87" // same id, different case
	if !equal(restartKeys([]access.Device{d}), restartKeys([]access.Device{f})) {
		t.Error("ids differing only by case are the same")
	}
	for name, mut := range map[string]func(*access.Device){
		"name": func(x *access.Device) { x.Name = "b" }, "listen": func(x *access.Device) { x.Listen = ":2" },
		"tuner": func(x *access.Device) { x.Tuner = "y" }, "stream": func(x *access.Device) { x.StreamPort = "9" },
		"advertise": func(x *access.Device) { x.Advertise = "http://z" }, "report": func(x *access.Device) { x.ReportID = "FFFFFFFF" },
	} {
		g := d
		mut(&g)
		if equal(restartKeys([]access.Device{d}), restartKeys([]access.Device{g})) {
			t.Errorf("a changed %s should need a restart", name)
		}
	}
	// Order doesn't matter.
	x, y := access.Device{Name: "x", Listen: ":1", Tuner: "t"}, access.Device{Name: "y", Listen: ":2", Tuner: "t"}
	if !equal(restartKeys([]access.Device{x, y}), restartKeys([]access.Device{y, x})) {
		t.Error("device order should not matter")
	}
	if equal([]string{"a"}, []string{"a", "b"}) || equal([]string{"a"}, []string{"b"}) {
		t.Error("equal() is wrong")
	}
}

func TestDefaultLocate(t *testing.T) {
	ctx := context.Background()
	for in, want := range map[string]string{
		"192.168.1.132": "http://192.168.1.132", "host:80": "http://host:80", "https://tuner.internal": "https://tuner.internal",
	} {
		if got, err := DefaultLocate(ctx, in, ""); err != nil || got != want {
			t.Errorf("DefaultLocate(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := DefaultLocate(ctx, "http://", ""); err == nil {
		t.Error("a bad address was accepted")
	}
	if _, err := DefaultLocate(ctx, "", ""); err == nil {
		t.Error("with neither an address nor an id there is nothing to look for")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := DefaultLocate(canceled, "", "10ABCC87"); err == nil {
		t.Error("expected a not-found error when the search can't find the tuner")
	}
}

func TestParseTuner(t *testing.T) {
	good := map[string]string{
		"192.168.1.132": "http://192.168.1.132", "192.168.1.132:80": "http://192.168.1.132:80",
		"http://192.168.1.132": "http://192.168.1.132", "hdhr.example.ts.net": "http://hdhr.example.ts.net",
		"https://tuner.internal": "https://tuner.internal", "  192.168.1.132  ": "http://192.168.1.132",
	}
	for in, want := range good {
		u, err := ParseTuner(in)
		if err != nil || u.String() != want {
			t.Errorf("ParseTuner(%q) = %v, %v; want %q", in, u, err, want)
		}
	}
	for _, in := range []string{"", "http://", "://x", "   "} {
		if _, err := ParseTuner(in); err == nil {
			t.Errorf("ParseTuner(%q) succeeded", in)
		}
	}
}

func TestRenamingATunerNeverChangesWhatMediaServersSee(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	s := loopbackViewer(access.Settings{})
	r := start(t, s, oneDevice(a), a)

	friendly := func() string {
		_, body := r.get(t, "antenna", "/discover.json")
		var d map[string]any
		json.Unmarshal([]byte(body), &d)
		return d["FriendlyName"].(string)
	}
	label := func() string {
		_, body := r.get(t, "antenna", "/api/state")
		var st struct{ Devices []struct{ Label string } }
		json.Unmarshal([]byte(body), &st)
		return st.Devices[0].Label
	}
	if got := friendly(); got != "antenna-box" {
		t.Fatalf("before naming it: %q", got)
	}

	// The name shows on the page at once, and media servers still see the tuner's own.
	s.Labels = map[string]string{"antenna": "Living room antenna"}
	if err := r.srv.Reload(s); err != nil {
		t.Fatal(err)
	}
	if got := label(); got != "Living room antenna" {
		t.Errorf("the page should have the name: %q", got)
	}
	if got := friendly(); got != "antenna-box" {
		t.Errorf("a media server was told %q after the rename; it should keep the tuner's own name", got)
	}

	s.Labels = nil
	if err := r.srv.Reload(s); err != nil {
		t.Fatal(err)
	}
	if got := friendly(); got != "antenna-box" {
		t.Errorf("after clearing the name: %q", got)
	}
}
