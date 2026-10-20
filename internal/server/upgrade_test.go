package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/access"
	"github.com/johnschult/yagi/internal/hdhr/hdhrtest"
	"github.com/johnschult/yagi/internal/upgrade"
)

// firmwareFiles serves what SiliconDust's download site does: one binary, under the name the tuner's
// family and the release make.
func firmwareFiles(t *testing.T, path string, size int) *httptest.Server {
	t.Helper()
	img := make([]byte, size)
	for i := range img {
		img[i] = byte(i * 7)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(img)
	}))
	t.Cleanup(srv.Close)
	return srv
}

type upgradeState struct {
	Updating bool
	Online   bool
	Phase    string
	Error    string
	Release  string
	Events   []string
	FwState  string
	FwNow    string
}

// upgradeState reads the first device from the API.
func (r *rig) upgradeState(t *testing.T) upgradeState {
	t.Helper()
	_, body := r.get(t, "antenna", "/api/state")
	var st struct {
		Devices []struct {
			Tuner struct {
				Online   bool
				Updating bool
				Events   []struct{ Text string }
			}
			Firmware struct{ State, Current string }
			Upgrade  *struct{ Phase, Error, Release string }
		}
	}
	if err := json.Unmarshal([]byte(body), &st); err != nil || len(st.Devices) == 0 {
		t.Fatalf("state: %v\n%s", err, body)
	}
	d := st.Devices[0]
	u := upgradeState{Updating: d.Tuner.Updating, Online: d.Tuner.Online, FwState: d.Firmware.State, FwNow: d.Firmware.Current}
	for _, e := range d.Tuner.Events {
		u.Events = append(u.Events, e.Text)
	}
	if d.Upgrade != nil {
		u.Phase, u.Error, u.Release = d.Upgrade.Phase, d.Upgrade.Error, d.Upgrade.Release
	}
	return u
}

func (r *rig) post(t *testing.T, path, body string) (int, string) {
	t.Helper()
	resp, err := http.Post("http://"+r.addr["antenna"]+path, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var b bytes.Buffer
	b.ReadFrom(resp.Body)
	return resp.StatusCode, b.String()
}

// The whole thing, as it happens: the tuner (its web side and its control side) is on older firmware, the
// changelog and the download site are SiliconDust's stand-ins, and the tuner really does go away for a while
// after taking the image and come back running the new version.
func TestInstallingFirmwareEndToEndWhileTheTunerRestarts(t *testing.T) {
	url, _ := changelog(t) // says 20260326 is out and applies to this tuner
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	a.setFirmware("20250815", "")
	ctrl, err := hdhrtest.New("127.0.0.1:0", hdhrtest.Options{Version: "20250815", VersionAfter: "20260326", RebootFor: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()
	files := firmwareFiles(t, "/hdhomerun_dvr_atsc3_firmware_20260326.bin", 300<<10)
	port := ctrl.Addr()[strings.LastIndex(ctrl.Addr(), ":")+1:]

	r := startConfigured(t, loopbackViewer(access.DefaultSettings()), oneDevice(a), url, "", func(c *Config) {
		c.FirmwareDownloadBase = files.URL
		c.ControlPort = port
		c.UpgradeConfig = func(u *upgrade.Config) {
			u.RestartWait, u.PollEvery, u.BackWithin = 100*time.Millisecond, 25*time.Millisecond, 5*time.Second
		}
	}, a)
	waitFor(t, "the update to be found", func() bool { return r.upgradeState(t).FwState == "update-available" })

	if code, body := r.post(t, "/api/devices/antenna/firmware/upgrade", `{"version":"20260326"}`); code != 202 {
		t.Fatalf("%d %s", code, body)
	}

	// While it restarts: the web side goes quiet too, as a real tuner's does, then it returns on the new version.
	go func() {
		for !ctrl.Upgraded() {
			time.Sleep(5 * time.Millisecond)
		}
		a.setDown(true)
		time.Sleep(250 * time.Millisecond)
		a.setFirmware("20260326", "")
		a.setDown(false)
	}()

	sawUpdating := false
	waitFor(t, "the install to finish", func() bool {
		s := r.upgradeState(t)
		if s.Updating {
			sawUpdating = true
		}
		return s.Phase == "done" || s.Phase == "failed"
	})
	s := r.upgradeState(t)
	if s.Phase != "done" || s.Error != "" {
		t.Fatalf("%+v\n%s", s, r.logs)
	}
	if !sawUpdating {
		t.Error("the page was never told the tuner was updating")
	}
	if len(ctrl.Image()) != 300<<10 {
		t.Errorf("the tuner got %d bytes", len(ctrl.Image()))
	}
	waitFor(t, "the tuner to be seen back, on the new firmware", func() bool {
		s := r.upgradeState(t)
		return s.Online && !s.Updating && s.FwState == "up-to-date" && s.FwNow == "20260326"
	})
	// A planned restart is not an outage: the timeline says what happened, and never "offline".
	s = r.upgradeState(t)
	for _, e := range s.Events {
		if strings.Contains(e, "offline") || strings.Contains(e, "moved") {
			t.Errorf("the timeline calls the restart an outage: %q", e)
		}
	}
	text := strings.Join(s.Events, "\n")
	for _, want := range []string{"Updating", "20250815 to 20260326", "is now running firmware 20260326"} {
		if !strings.Contains(text, want) {
			t.Errorf("the timeline lacks %q:\n%s", want, text)
		}
	}
	// The tuner is not offered what it now has.
	if code, body := r.post(t, "/api/devices/antenna/firmware/upgrade", `{"version":"20260326"}`); code != 400 {
		t.Errorf("installing again: %d %s", code, body)
	}
}

func TestAnInstallThatFailsReportsItAndLeavesTheTunerWatchedAsUsual(t *testing.T) {
	url, _ := changelog(t)
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	a.setFirmware("20250815", "")
	ctrl, err := hdhrtest.New("127.0.0.1:0", hdhrtest.Options{Version: "20250815", RejectUpgrade: "ERROR: not for this model"})
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()
	files := firmwareFiles(t, "/hdhomerun_dvr_atsc3_firmware_20260326.bin", 300<<10)
	port := ctrl.Addr()[strings.LastIndex(ctrl.Addr(), ":")+1:]
	r := startConfigured(t, loopbackViewer(access.DefaultSettings()), oneDevice(a), url, "", func(c *Config) {
		c.FirmwareDownloadBase = files.URL
		c.ControlPort = port
		c.UpgradeConfig = func(u *upgrade.Config) {
			u.RestartWait, u.PollEvery, u.BackWithin = 50*time.Millisecond, 20*time.Millisecond, time.Second
		}
	}, a)
	waitFor(t, "the update to be found", func() bool { return r.upgradeState(t).FwState == "update-available" })

	if code, body := r.post(t, "/api/devices/antenna/firmware/upgrade", `{"version":"20260326"}`); code != 202 {
		t.Fatalf("%d %s", code, body)
	}
	waitFor(t, "the install to end", func() bool { p := r.upgradeState(t).Phase; return p == "failed" || p == "done" })
	s := r.upgradeState(t)
	if s.Phase != "failed" || !strings.Contains(s.Error, "not for this model") || !strings.Contains(s.Error, "still running 20250815") {
		t.Fatalf("%+v", s)
	}
	if s.Updating || !s.Online {
		t.Errorf("updating=%v online=%v: after a failure the tuner is just a tuner again", s.Updating, s.Online)
	}
	// It is still offered the update, since it is still on the old firmware.
	if s.FwState != "update-available" {
		t.Errorf("firmware state %q", s.FwState)
	}
	if !strings.Contains(strings.Join(s.Events, "\n"), "failed") {
		t.Errorf("the failure is not on the timeline: %v", s.Events)
	}
}
