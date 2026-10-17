package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/access"
	"github.com/johnschult/antennarr/internal/live"
	"github.com/johnschult/antennarr/internal/tuner"
)

func ip(n int) *int { return &n }

// twoDevices is a settings file with an antenna box and a cable box.
func twoDevices() access.Settings {
	return access.Settings{
		OwnerReserve: 1,
		Devices: []access.Device{
			{Name: "antenna", Listen: ":5004", Tuner: "192.168.1.132", ID: "10ABCC87"},
			{Name: "cable", Listen: ":5005", Tuner: "192.168.1.133", ID: "1111AAA2", OwnerReserve: ip(0)},
		},
		Viewers: []access.Viewer{
			{Name: "John", Addresses: []string{"100.67.117.32"}, Limit: 1, Limits: map[string]int{"cable": 2}},
		},
	}
}

func newMultiEnv(t *testing.T, s access.Settings, withFile bool) *env {
	t.Helper()
	hub := live.NewHub()
	pol, err := access.NewPolicy(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	pol.SetOnChange(hub.Notify)

	e := &env{pol: pol, hub: hub}
	var devices []Device
	for _, d := range s.Devices {
		// antenna has one local stream and one of ours; cable is idle.
		status := `[{"Resource":"tuner0"},{"Resource":"tuner1"}]`
		if d.Name == "antenna" {
			status = `[{"Resource":"tuner0","VctNumber":"5.1","VctName":"WFRV","TargetIP":"192.168.1.20"},{"Resource":"tuner1","VctNumber":"2.1","VctName":"WBAY","TargetIP":"192.168.1.77"},{"Resource":"tuner2"},{"Resource":"tuner3"}]`
		}
		tun := fakeHDHRWith(t, d.ID, status)
		mon := tuner.NewMonitor(tuner.Config{
			Name: d.Name, BaseURL: tun.URL, ID: d.ID, OnChange: hub.Notify,
			LocalIPs: func() []string { return []string{"192.168.1.20"} },
		})
		pol.AddDevice(d.Name, mon, mon)
		mon.Poll(context.Background())
		devices = append(devices, Device{Name: d.Name, Listen: d.Listen, Mon: mon})
	}
	if withFile {
		e.path = filepath.Join(t.TempDir(), "antennarr.json")
		if err := access.SaveSettings(e.path, s); err != nil {
			t.Fatal(err)
		}
	}
	e.api = &API{Devices: devices, Pol: pol, Store: access.NewStore(e.path, pol), Hub: hub, KeepAlive: time.Hour}
	e.srv = httptest.NewServer(pol.AdminOnly(e.api.Handler()))
	t.Cleanup(e.srv.Close)
	return e
}

func fakeHDHRWith(t *testing.T, id, status string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/discover.json":
			w.Write([]byte(`{"FriendlyName":"Fake ` + id + `","ModelNumber":"HDFX-4K","DeviceID":"` + id + `","TunerCount":4,"FirmwareName":"hdhomerun_dvr_atsc3","FirmwareVersion":"20250815"}`))
		case "/status.json":
			w.Write([]byte(status))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func devDoc(t *testing.T, e *env, name string) map[string]any {
	t.Helper()
	_, v := e.call(t, "GET", "/antennarr/api/state", "")
	for _, d := range obj(t, v)["devices"].([]any) {
		if obj(t, d)["name"] == name {
			return obj(t, d)
		}
	}
	t.Fatalf("no device %q in state", name)
	return nil
}

func fileSettings(t *testing.T, e *env) access.Settings {
	t.Helper()
	s, _, err := access.LoadSettings(e.path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStateListsEveryDeviceWithItsOwnNumbers(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), true)
	r := httptest.NewRequest("GET", "/auto/v5.1", nil)
	r.RemoteAddr = "100.67.117.32:4000"
	if _, err := e.pol.Device("cable").Stream(r); err != nil { // John streams from the cable box only
		t.Fatal(err)
	}

	_, v := e.call(t, "GET", "/antennarr/api/state", "")
	devs := obj(t, v)["devices"].([]any)
	if len(devs) != 2 || obj(t, devs[0])["name"] != "antenna" || obj(t, devs[1])["name"] != "cable" {
		t.Fatalf("devices = %v", devs)
	}

	antenna, cable := obj(t, devs[0]), obj(t, devs[1])
	if obj(t, antenna["summary"])["inUse"] != float64(2) || len(antenna["streams"].([]any)) != 0 {
		t.Errorf("antenna = %v", antenna)
	}
	if obj(t, cable["summary"])["inUse"] != float64(0) || len(cable["streams"].([]any)) != 1 {
		t.Errorf("cable = %v", cable)
	}
	if antenna["ownerReserve"] != float64(1) || cable["ownerReserve"] != float64(0) {
		t.Errorf("effective reserves: antenna %v (want the default 1), cable %v (want its own 0)", antenna["ownerReserve"], cable["ownerReserve"])
	}
	john := obj(t, obj(t, v)["viewers"].([]any)[0])
	if by := obj(t, john["inUseByDevice"]); by["cable"] != float64(1) || by["antenna"] != nil {
		t.Errorf("inUseByDevice = %v", by)
	}
	if obj(t, john["limits"])["cable"] != float64(2) {
		t.Errorf("limits = %v", john["limits"])
	}
}

func TestViewersCarryPerDeviceLimits(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), true)

	code, v := e.call(t, "POST", "/antennarr/api/viewers", `{"name":"Mom","addresses":["10.8.0.5"],"limit":1,"limits":{"cable":3,"antenna":0}}`)
	if code != 201 || obj(t, obj(t, v)["limits"])["cable"] != float64(3) || obj(t, obj(t, v)["limits"])["antenna"] != float64(0) {
		t.Fatalf("add: %d %v", code, v)
	}
	if got := fileSettings(t, e).Viewers[1].Limits; got["cable"] != 3 || got["antenna"] != 0 {
		t.Errorf("not saved: %v", got)
	}

	code, v = e.call(t, "PUT", "/antennarr/api/viewers/Mom", `{"name":"Mom","addresses":["10.8.0.5"],"limit":2}`)
	if code != 200 || obj(t, v)["limits"] != nil {
		t.Errorf("replace without limits should clear the overrides: %d %v", code, v)
	}
	if got := fileSettings(t, e).Viewers[1].Limits; len(got) != 0 {
		t.Errorf("overrides survived: %v", got)
	}
}

func TestViewerLimitsMustNameRealDevices(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), true)
	for name, body := range map[string]string{
		"unknown device": `{"name":"X","addresses":["10.9.0.1"],"limit":1,"limits":{"satellite":1}}`,
		"negative":       `{"name":"X","addresses":["10.9.0.1"],"limit":1,"limits":{"cable":-1}}`,
	} {
		code, v := e.call(t, "POST", "/antennarr/api/viewers", body)
		if code != 400 {
			t.Errorf("%s: %d %v", name, code, v)
		}
	}
	if len(viewerNames(t, e)) != 1 {
		t.Error("a refused viewer was added")
	}
}

func TestDeviceSettingsAreSavedOnTheDevice(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), true)

	code, v := e.call(t, "PATCH", "/antennarr/api/devices/antenna", `{"ownerReserve":2,"sharedCap":1}`)
	if code != 200 || obj(t, v)["ownerReserve"] != float64(2) || obj(t, v)["sharedCap"] != float64(1) {
		t.Fatalf("%d %v", code, v)
	}
	s := fileSettings(t, e)
	if *s.Devices[0].OwnerReserve != 2 || *s.Devices[0].SharedCap != 1 {
		t.Errorf("antenna in file: %+v", s.Devices[0])
	}
	if *s.Devices[1].OwnerReserve != 0 || s.Devices[1].SharedCap != nil {
		t.Errorf("the other device changed: %+v", s.Devices[1])
	}
	if s.OwnerReserve != 1 || s.SharedCap != 0 {
		t.Errorf("a device edit changed the defaults: reserve %d cap %d", s.OwnerReserve, s.SharedCap)
	}
	if d := devDoc(t, e, "antenna"); d["ownerReserve"] != float64(2) {
		t.Errorf("state = %v", d)
	}
	if d := devDoc(t, e, "cable"); d["ownerReserve"] != float64(0) {
		t.Errorf("cable in state = %v", d)
	}
}

func TestDevicePatchChangesOnlyWhatIsSent(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), true)
	e.call(t, "PATCH", "/antennarr/api/devices/cable", `{"sharedCap":2}`)
	s := fileSettings(t, e)
	if *s.Devices[1].SharedCap != 2 || *s.Devices[1].OwnerReserve != 0 {
		t.Errorf("cable = %+v (an explicit reserve of 0 must survive)", s.Devices[1])
	}
	// Zero is a real value, not "unset".
	e.call(t, "PATCH", "/antennarr/api/devices/cable", `{"sharedCap":0}`)
	if got := fileSettings(t, e).Devices[1].SharedCap; got == nil || *got != 0 {
		t.Errorf("sharedCap after sending 0 = %v", got)
	}
}

func TestDevicePatchErrors(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), true)
	for name, c := range map[string][3]string{
		"unknown device": {"PATCH", "/antennarr/api/devices/satellite", `{"sharedCap":1}`},
		"negative":       {"PATCH", "/antennarr/api/devices/antenna", `{"ownerReserve":-1}`},
		"unknown field":  {"PATCH", "/antennarr/api/devices/antenna", `{"listen":":9999"}`}, // devices can't be rewired from the API
		"wrong type":     {"PATCH", "/antennarr/api/devices/antenna", `{"sharedCap":"two"}`},
	} {
		code, v := e.call(t, c[0], c[1], c[2])
		want := 400
		if name == "unknown device" {
			want = 404
		}
		if code != want {
			t.Errorf("%s: %d %v, want %d", name, code, v, want)
		}
	}
	if s := fileSettings(t, e); s.Devices[0].OwnerReserve != nil || s.Devices[0].Listen != ":5004" {
		t.Errorf("a refused edit changed the file: %+v", s.Devices[0])
	}
}

func TestEditsKeepTheDeviceList(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), true)
	e.call(t, "POST", "/antennarr/api/viewers", `{"name":"Mom","addresses":["10.8.0.5"],"limit":1}`)
	e.call(t, "PATCH", "/antennarr/api/settings", `{"admin":["100.64.0.0/10"]}`)
	e.call(t, "DELETE", "/antennarr/api/viewers/Mom", "")

	s := fileSettings(t, e)
	if len(s.Devices) != 2 || s.Devices[0].Name != "antenna" || s.Devices[0].Tuner != "192.168.1.132" || s.Devices[1].ID != "1111AAA2" {
		t.Errorf("viewer and settings edits lost or changed the devices: %+v", s.Devices)
	}
}

func TestDeviceFromFlagsEditsTheDefaults(t *testing.T) {
	// No devices declared: the one device came from command-line flags, so it
	// has no entry of its own to hold an override.
	e := newEnv(t, johnAndMom, true) // device "antenna" is registered with the policy but not declared
	code, v := e.call(t, "PATCH", "/antennarr/api/devices/antenna", `{"ownerReserve":0,"sharedCap":2}`)
	if code != 200 || obj(t, v)["ownerReserve"] != float64(0) || obj(t, v)["sharedCap"] != float64(2) {
		t.Fatalf("%d %v", code, v)
	}
	s := fileSettings(t, e)
	if s.OwnerReserve != 0 || s.SharedCap != 2 || len(s.Devices) != 0 {
		t.Errorf("file = reserve %d cap %d devices %v (a flags-configured setup must not gain a devices list)", s.OwnerReserve, s.SharedCap, s.Devices)
	}
}

func TestDevicePatchNeedsAFile(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), false)
	code, v := e.call(t, "PATCH", "/antennarr/api/devices/antenna", `{"sharedCap":1}`)
	if code != 409 || !strings.Contains(errText(v), "-config") {
		t.Errorf("%d %v", code, v)
	}
}

func TestOnlyAdminsCanEditDevices(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), true)
	h := e.pol.AdminOnly(e.api.Handler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/antennarr/api/devices/antenna", strings.NewReader(`{"sharedCap":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "100.67.117.32:1" // John is a viewer, not an admin
	h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Errorf("status %d", rec.Code)
	}
	if s := fileSettings(t, e); s.Devices[0].SharedCap != nil {
		t.Error("a non-admin changed a device")
	}
}

func TestEventsFollowAnyDevice(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), true)
	ch, _ := openEvents(t, e)
	if _, ok := nextMsg(t, ch, 2*time.Second); !ok {
		t.Fatal("no initial event")
	}
	r := httptest.NewRequest("GET", "/auto/v2.1", nil)
	r.RemoteAddr = "100.67.117.32:4000"
	release, err := e.pol.Device("cable").Stream(r)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	m, ok := nextMsg(t, ch, 2*time.Second)
	if !ok || streamsIn(t, m) != 1 {
		t.Errorf("a stream on the second device did not reach the event stream: %+v ok=%v", m, ok)
	}
}

// ---- naming tuners ----

func TestRenamingADevice(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), true)

	code, v := e.call(t, "PATCH", "/antennarr/api/devices/antenna", `{"label":"Antenna (Steve)"}`)
	if code != 200 || obj(t, v)["label"] != "Antenna (Steve)" || obj(t, v)["name"] != "antenna" {
		t.Fatalf("%d %v", code, v)
	}
	if got := fileSettings(t, e).Labels["antenna"]; got != "Antenna (Steve)" {
		t.Errorf("not saved: %q", got)
	}
	if d := devDoc(t, e, "antenna"); d["label"] != "Antenna (Steve)" {
		t.Errorf("state = %v", d)
	}
	if d := devDoc(t, e, "cable"); d["label"] != nil {
		t.Errorf("the other device was renamed too: %v", d["label"])
	}
	// The key is unchanged, so limits and URLs that use it keep working.
	if got := fileSettings(t, e).Viewers[0].Limits["cable"]; got != 2 {
		t.Errorf("viewer limits were disturbed: %v", got)
	}

	code, v = e.call(t, "PATCH", "/antennarr/api/devices/antenna", `{"label":""}`)
	if code != 200 || obj(t, v)["label"] != nil {
		t.Errorf("clearing: %d %v", code, v)
	}
	if _, has := fileSettings(t, e).Labels["antenna"]; has {
		t.Error("the cleared label is still in the file")
	}
}

func TestRenamingChangesOnlyWhatIsSent(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), true)
	e.call(t, "PATCH", "/antennarr/api/devices/antenna", `{"label":"Antenna"}`)
	e.call(t, "PATCH", "/antennarr/api/devices/antenna", `{"ownerReserve":2}`) // no label in this one
	s := fileSettings(t, e)
	if s.Labels["antenna"] != "Antenna" || *s.Devices[0].OwnerReserve != 2 {
		t.Errorf("labels %v, antenna %+v", s.Labels, s.Devices[0])
	}
	e.call(t, "PATCH", "/antennarr/api/devices/antenna", `{"label":"Antenna 2","sharedCap":1}`)
	s = fileSettings(t, e)
	if s.Labels["antenna"] != "Antenna 2" || *s.Devices[0].SharedCap != 1 || *s.Devices[0].OwnerReserve != 2 {
		t.Errorf("labels %v, antenna %+v", s.Labels, s.Devices[0])
	}
}

func TestBadNamesAreRefused(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), true)
	for name, body := range map[string]string{
		"too long":      `{"label":"` + strings.Repeat("x", 61) + `"}`,
		"newline":       `{"label":"two\nlines"}`,
		"padded":        `{"label":" Antenna "}`,
		"blank":         `{"label":"   "}`,
		"not a string":  `{"label":5}`,
		"alongside bad": `{"label":"ok","sharedCap":-1}`,
	} {
		if code, v := e.call(t, "PATCH", "/antennarr/api/devices/antenna", body); code != 400 {
			t.Errorf("%s: %d %v", name, code, v)
		}
	}
	if len(fileSettings(t, e).Labels) != 0 || devDoc(t, e, "antenna")["label"] != nil {
		t.Error("a refused rename changed something")
	}
	if code, _ := e.call(t, "PATCH", "/antennarr/api/devices/satellite", `{"label":"x"}`); code != 404 {
		t.Errorf("unknown device: %d", code)
	}
}

func TestADeviceFromFlagsCanBeNamedToo(t *testing.T) {
	// No devices declared in the file, so the label lives in its own section.
	e := newEnv(t, johnAndMom, true)
	code, v := e.call(t, "PATCH", "/antennarr/api/devices/antenna", `{"label":"Living room antenna"}`)
	if code != 200 || obj(t, v)["label"] != "Living room antenna" {
		t.Fatalf("%d %v", code, v)
	}
	s := fileSettings(t, e)
	if s.Labels["antenna"] != "Living room antenna" || len(s.Devices) != 0 {
		t.Errorf("labels %v devices %v", s.Labels, s.Devices)
	}
}

func TestOnlyAdminsCanRename(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), true)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/antennarr/api/devices/antenna", strings.NewReader(`{"label":"mine now"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "100.67.117.32:1"
	e.pol.AdminOnly(e.api.Handler()).ServeHTTP(rec, req)
	if rec.Code != 403 || len(fileSettings(t, e).Labels) != 0 {
		t.Errorf("status %d, labels %v", rec.Code, fileSettings(t, e).Labels)
	}
}
