package api

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/firmware"
	"github.com/johnschult/yagi/internal/hdhr"
	"github.com/johnschult/yagi/internal/hdhr/hdhrtest"
	"github.com/johnschult/yagi/internal/upgrade"
)

const upgradePath = "/yagi/api/devices/antenna/firmware/upgrade"

// idleTuner is a status.json with nothing in use.
const idleTuner = `[{"Resource":"tuner0"},{"Resource":"tuner1"},{"Resource":"tuner2"},{"Resource":"tuner3"}]`

// fwDownload hands out an image and records what was asked for.
type fwDownload struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (d *fwDownload) Fetch(_ context.Context, family, release string) (firmware.Image, error) {
	d.mu.Lock()
	d.calls = append(d.calls, family+"/"+release)
	d.mu.Unlock()
	if d.err != nil {
		return firmware.Image{}, d.err
	}
	img := make([]byte, 4096)
	for i := range img {
		img[i] = byte(i)
	}
	return firmware.Image{Family: family, Release: release, Data: img, SHA256: "deadbeef"}, nil
}

func (d *fwDownload) count() int { d.mu.Lock(); defer d.mu.Unlock(); return len(d.calls) }

// upgradeEnv is an environment whose tuner has newer firmware on offer (the changelog says 20260326 and it
// runs 20250815), a firmware checker that has read it, and an install talking to a fake tuner of its own.
func upgradeEnv(t *testing.T, tunerStatus string, ctrl hdhrtest.Options) (*env, *hdhrtest.Server, *fwDownload) {
	t.Helper()
	e := newEnvWith(t, johnAndMom, true, tunerStatus)
	c := withChecker(e, changelogServer(t, 200).URL, nil)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv, err := hdhrtest.New("127.0.0.1:0", ctrl)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	dl := &fwDownload{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e.api.Upgrade = upgrade.New(ctx, upgrade.Config{
		Download:    dl,
		Dial:        func(ctx context.Context, _ string) (*hdhr.Conn, error) { return hdhr.Dial(ctx, srv.Addr()) },
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RestartWait: 30 * time.Millisecond, BackWithin: 2 * time.Second, PollEvery: 20 * time.Millisecond,
	})
	return e, srv, dl
}

func upgradeOf(t *testing.T, e *env) map[string]any {
	t.Helper()
	d := devDoc(t, e, "antenna")
	u, _ := d["upgrade"].(map[string]any)
	return u
}

// waitUpgrade waits for an install to end and returns how.
func waitUpgrade(t *testing.T, e *env) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if u := upgradeOf(t, e); u != nil && u["phase"] != "downloading" && u["phase"] != "sending" && u["phase"] != "restarting" && u["phase"] != "verifying" {
			return u
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("the install did not end")
	return nil
}

func TestInstallingTheLatestFirmwareRunsAndShowsInTheState(t *testing.T) {
	e, ctrl, dl := upgradeEnv(t, idleTuner, hdhrtest.Options{RebootFor: 80 * time.Millisecond, VersionAfter: "20260326"})
	if upgradeOf(t, e) != nil {
		t.Fatal("the state has an upgrade before there was one")
	}

	code, v := e.call(t, "POST", upgradePath, `{"version":"20260326"}`)
	if code != 202 {
		t.Fatalf("%d %v", code, v)
	}
	if r := obj(t, v); r["release"] != "20260326" || r["from"] != "20250815" {
		t.Errorf("reply = %v", r)
	}
	u := waitUpgrade(t, e)
	if u["phase"] != "done" || u["release"] != "20260326" || u["sha256"] != "deadbeef" {
		t.Fatalf("upgrade = %v", u)
	}
	if len(ctrl.Image()) != 4096 {
		t.Errorf("the tuner got %d bytes", len(ctrl.Image()))
	}
	if dl.calls[0] != "hdhomerun_dvr_atsc3/20260326" {
		t.Errorf("downloaded %v", dl.calls)
	}
	if e.mon.Snapshot().Updating {
		t.Error("the monitor is still marked updating")
	}
}

func TestSomethingInUseNeedsTheInterruptFlag(t *testing.T) {
	e, ctrl, dl := upgradeEnv(t, defaultBusyStatus, hdhrtest.Options{RebootFor: 80 * time.Millisecond, VersionAfter: "20260326"})
	code, v := e.call(t, "POST", upgradePath, `{"version":"20260326"}`)
	if code != 409 || !strings.Contains(errText(v), "2 of the tuner's tuners are in use") || !strings.Contains(errText(v), "interrupt") {
		t.Fatalf("%d %v", code, v)
	}
	if dl.count() != 0 || len(ctrl.Frames()) != 0 {
		t.Errorf("a refused request went on to download %d times and send %d frames", dl.count(), len(ctrl.Frames()))
	}
	// told, and confirmed: it goes ahead
	if code, v := e.call(t, "POST", upgradePath, `{"version":"20260326","interrupt":true}`); code != 202 {
		t.Fatalf("%d %v", code, v)
	}
	if u := waitUpgrade(t, e); u["phase"] != "done" {
		t.Errorf("upgrade = %v", u)
	}
}

// defaultBusyStatus is what newEnv's tuner has: two tuners in use.
const defaultBusyStatus = `[
	{"Resource":"tuner0","VctNumber":"5.1","VctName":"WFRV","TargetIP":"192.168.1.20"},
	{"Resource":"tuner1","VctNumber":"2.1","VctName":"WBAY","TargetIP":"192.168.1.77"},
	{"Resource":"tuner2"},{"Resource":"tuner3"}
]`

func TestAStaleOrWrongVersionIsRefusedBeforeAnythingIsDone(t *testing.T) {
	e, ctrl, dl := upgradeEnv(t, idleTuner, hdhrtest.Options{})
	for name, body := range map[string]string{
		"an older release":   `{"version":"20250815"}`,
		"a later one":        `{"version":"20990101"}`,
		"none":               `{}`,
		"not a version":      `{"version":"../../x"}`,
		"the right one, but": `{"version":" 20260326"}`,
	} {
		code, v := e.call(t, "POST", upgradePath, body)
		if code != 409 || !strings.Contains(errText(v), "look again before updating") {
			t.Errorf("%s: %d %v", name, code, v)
		}
	}
	if dl.count() != 0 || len(ctrl.Frames()) != 0 || upgradeOf(t, e) != nil {
		t.Errorf("downloads %d, frames %d, upgrade %v: nothing should have started", dl.count(), len(ctrl.Frames()), upgradeOf(t, e))
	}
}

func TestNothingIsInstalledUnlessTheChangelogSaysThereIsSomethingNewer(t *testing.T) {
	// A checker that has not read the changelog yet.
	e := newEnvWith(t, johnAndMom, true, idleTuner)
	withChecker(e, changelogServer(t, 200).URL, nil)
	dl := &fwDownload{}
	e.api.Upgrade = upgrade.New(context.Background(), upgrade.Config{Download: dl})
	code, v := e.call(t, "POST", upgradePath, `{"version":"20260326"}`)
	if code != 400 || !strings.Contains(errText(v), "no newer firmware") {
		t.Errorf("%d %v", code, v)
	}
	if dl.count() != 0 {
		t.Error("it downloaded something")
	}
}

func TestInstallingIsNotAvailableWithoutTheFirmwareCheck(t *testing.T) {
	e := newEnvWith(t, johnAndMom, true, idleTuner)
	if code, v := e.call(t, "POST", upgradePath, `{"version":"20260326"}`); code != 409 || !strings.Contains(errText(v), "not available") {
		t.Errorf("no manager at all: %d %v", code, v)
	}
	e.api.Upgrade = upgrade.New(context.Background(), upgrade.Config{})
	if code, v := e.call(t, "POST", upgradePath, `{"version":"20260326"}`); code != 409 || !strings.Contains(errText(v), "not available") {
		t.Errorf("no checker: %d %v", code, v)
	}

	// The check switched off in the settings: nothing is installed on its word.
	e2, _, dl := upgradeEnv(t, idleTuner, hdhrtest.Options{})
	if code, v := e2.call(t, "PATCH", "/yagi/api/settings", `{"firmwareCheck":false}`); code != 200 {
		t.Fatalf("%d %v", code, v)
	}
	code, v := e2.call(t, "POST", upgradePath, `{"version":"20260326"}`)
	if code != 409 || !strings.Contains(errText(v), "Check for new tuner firmware") {
		t.Errorf("check off: %d %v", code, v)
	}
	if dl.count() != 0 {
		t.Error("it downloaded something")
	}
}

func TestATunerThatIsOfflineCannotBeUpdated(t *testing.T) {
	e, ctrl, dl := upgradeEnv(t, idleTuner, hdhrtest.Options{})
	e.tun.Close() // its web side stops answering
	for i := 0; i < 4; i++ {
		e.mon.Poll(context.Background())
	}
	if e.mon.Snapshot().Online {
		t.Fatal("the monitor still thinks it is online")
	}
	code, v := e.call(t, "POST", upgradePath, `{"version":"20260326","interrupt":true}`)
	if code != 409 || !strings.Contains(errText(v), "offline") {
		t.Errorf("%d %v", code, v)
	}
	if dl.count() != 0 || len(ctrl.Frames()) != 0 {
		t.Errorf("%d downloads, %d frames for a tuner that is offline", dl.count(), len(ctrl.Frames()))
	}
}

func TestOnlyOneInstallAtATime(t *testing.T) {
	e, _, dl := upgradeEnv(t, idleTuner, hdhrtest.Options{RebootFor: 600 * time.Millisecond, VersionAfter: "20260326"})
	if code, v := e.call(t, "POST", upgradePath, `{"version":"20260326"}`); code != 202 {
		t.Fatalf("%d %v", code, v)
	}
	code, v := e.call(t, "POST", upgradePath, `{"version":"20260326"}`)
	if code != 409 || !strings.Contains(errText(v), "already under way") {
		t.Errorf("a second: %d %v", code, v)
	}
	if code, v := e.call(t, "DELETE", upgradePath, ""); code != 409 || !strings.Contains(errText(v), "still running") {
		t.Errorf("dismissing a running one: %d %v", code, v)
	}
	waitUpgrade(t, e)
	if dl.count() != 1 {
		t.Errorf("%d downloads", dl.count())
	}
}

func TestAFailedInstallIsShownWithItsReasonAndCanBeDismissed(t *testing.T) {
	e, _, _ := upgradeEnv(t, idleTuner, hdhrtest.Options{RejectUpgrade: "ERROR: invalid firmware"})
	if code, v := e.call(t, "POST", upgradePath, `{"version":"20260326"}`); code != 202 {
		t.Fatalf("%d %v", code, v)
	}
	u := waitUpgrade(t, e)
	if u["phase"] != "failed" || !strings.Contains(u["error"].(string), "invalid firmware") {
		t.Fatalf("upgrade = %v", u)
	}
	if code, _ := e.call(t, "DELETE", upgradePath, ""); code != 204 {
		t.Errorf("dismiss: %d", code)
	}
	if upgradeOf(t, e) != nil {
		t.Error("a dismissed install is still shown")
	}
	// and the person can try again
	if code, _ := e.call(t, "POST", upgradePath, `{"version":"20260326"}`); code != 202 {
		t.Errorf("trying again: %d", code)
	}
	waitUpgrade(t, e)
}

func TestTheRouteRefusesWhatIsNotAskedFor(t *testing.T) {
	e, ctrl, dl := upgradeEnv(t, idleTuner, hdhrtest.Options{})
	if code, _ := e.call(t, "GET", upgradePath, ""); code != 405 {
		t.Errorf("GET: %d", code)
	}
	if code, v := e.call(t, "POST", "/yagi/api/devices/nope/firmware/upgrade", `{"version":"20260326"}`); code != 404 {
		t.Errorf("an unknown tuner: %d %v", code, v)
	}
	if code, v := e.call(t, "DELETE", "/yagi/api/devices/nope/firmware/upgrade", ""); code != 404 {
		t.Errorf("dismissing on an unknown tuner: %d %v", code, v)
	}
	if code, _ := e.call(t, "POST", upgradePath, `{"version":"20260326","surprise":1}`); code != 400 {
		t.Errorf("an unknown field: %d", code)
	}
	if code, _ := e.call(t, "POST", upgradePath, `{"version":"20260326"}`, "Origin", "https://evil.example"); code != 403 {
		t.Errorf("a cross-site request: %d, so a web page you visit could flash your tuner", code)
	}
	if code, _ := e.call(t, "POST", upgradePath, `not json`); code != 400 {
		t.Errorf("not JSON: %d", code)
	}
	if dl.count() != 0 || len(ctrl.Frames()) != 0 {
		t.Errorf("a refused request started something: %d downloads, %d frames", dl.count(), len(ctrl.Frames()))
	}
}
