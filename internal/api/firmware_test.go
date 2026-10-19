package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/firmware"
)

func changelogServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile("../firmware/testdata/changelog.html")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != 200 {
			http.Error(w, "nope", status)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// withChecker gives the environment a firmware checker reading from url.
func withChecker(e *env, url string, enabled func() bool) *firmware.Checker {
	c := firmware.NewChecker(firmware.Config{URL: url, Enabled: enabled, MinGap: 30 * time.Second})
	e.api.Firmware = c
	return c
}

func fwOf(t *testing.T, e *env) map[string]any {
	t.Helper()
	d := devDoc(t, e, "antenna")
	return obj(t, d["firmware"])
}

func TestStateCarriesFirmwareStatus(t *testing.T) {
	e := newEnv(t, johnAndMom, true) // its tuner reports firmware 20250815
	if fw := fwOf(t, e); fw["state"] != "unknown" || !strings.Contains(fw["error"].(string), "not available") || fw["current"] != "20250815" {
		t.Errorf("with no checker: %v", fw)
	}

	c := withChecker(e, changelogServer(t, 200).URL, nil)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	fw := fwOf(t, e)
	if fw["state"] != "update-available" || fw["latest"] != "20260326" || fw["modelRecognized"] != true || fw["checkedAt"] == nil {
		t.Fatalf("firmware = %v", fw)
	}
	updates := fw["updates"].([]any)
	if len(updates) != 2 || obj(t, updates[0])["version"] != "20260326" || obj(t, updates[0])["certain"] != true {
		t.Errorf("updates = %v", updates)
	}
	if notes := obj(t, updates[0])["notes"].([]any); len(notes) != 1 || obj(t, notes[0])["scope"] != "ATSC3 models" {
		t.Errorf("notes = %v", notes)
	}
}

func TestSettingsReportTheFirmwareSwitch(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	_, v := e.call(t, "GET", "/yagi/api/settings", "")
	if obj(t, v)["firmwareCheck"] != true {
		t.Errorf("default = %v, want on", obj(t, v)["firmwareCheck"])
	}

	code, v := e.call(t, "PATCH", "/yagi/api/settings", `{"firmwareCheck":false}`)
	if code != 200 || obj(t, v)["firmwareCheck"] != false {
		t.Fatalf("%d %v", code, v)
	}
	if fileSettings(t, e).FirmwareCheckEnabled() {
		t.Error("the choice was not saved")
	}
	// Other settings are left alone by a patch that doesn't mention it.
	e.call(t, "PATCH", "/yagi/api/settings", `{"sharedCap":2}`)
	if fileSettings(t, e).FirmwareCheckEnabled() {
		t.Error("an unrelated patch switched the check back on")
	}
	if code, _ := e.call(t, "PATCH", "/yagi/api/settings", `{"firmwareCheck":"yes"}`); code != 400 {
		t.Errorf("a non-boolean: %d", code)
	}
	if code, v := e.call(t, "PATCH", "/yagi/api/settings", `{"firmwareCheck":true}`); code != 200 || obj(t, v)["firmwareCheck"] != true {
		t.Errorf("switching back on: %d %v", code, v)
	}
}

func TestSwitchingTheCheckOnKicksTheChecker(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	hits := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits <- struct{}{}
		body, _ := os.ReadFile("../firmware/testdata/changelog.html")
		w.Write(body)
	}))
	defer srv.Close()
	c := firmware.NewChecker(firmware.Config{URL: srv.URL, Interval: time.Hour, MinGap: time.Nanosecond,
		Enabled: func() bool { return e.pol.Settings().FirmwareCheckEnabled() }})
	e.api.Firmware = c
	e.call(t, "PATCH", "/yagi/api/settings", `{"firmwareCheck":false}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)
	time.Sleep(100 * time.Millisecond)
	select {
	case <-hits:
		t.Fatal("looked while off")
	default:
	}

	e.call(t, "PATCH", "/yagi/api/settings", `{"firmwareCheck":true}`)
	select {
	case <-hits:
	case <-time.After(3 * time.Second):
		t.Error("switching the check on did not make it look")
	}
}

func TestCheckNowReadsTheChangelog(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	c := withChecker(e, changelogServer(t, 200).URL, nil)

	code, v := e.call(t, "POST", "/yagi/api/firmware/check", "")
	if code != 200 || obj(t, v)["checkedAt"] == nil || c.CheckedAt().IsZero() {
		t.Fatalf("%d %v", code, v)
	}
	if fw := fwOf(t, e); fw["state"] != "update-available" {
		t.Errorf("status after checking: %v", fw)
	}

	// A second press straight away is told to wait, not sent to SiliconDust again.
	code, v = e.call(t, "POST", "/yagi/api/firmware/check", "")
	if code != 429 || !strings.Contains(errText(v), "a moment ago") {
		t.Errorf("%d %v", code, v)
	}
}

func TestCheckNowExplainsFailures(t *testing.T) {
	e := newEnv(t, johnAndMom, true)

	// No checker at all.
	if code, v := e.call(t, "POST", "/yagi/api/firmware/check", ""); code != 409 || !strings.Contains(errText(v), "not available") {
		t.Errorf("no checker: %d %v", code, v)
	}

	// The check is switched off.
	off := false
	e.call(t, "PATCH", "/yagi/api/settings", `{"firmwareCheck":false}`)
	withChecker(e, changelogServer(t, 200).URL, func() bool { return off })
	if code, v := e.call(t, "POST", "/yagi/api/firmware/check", ""); code != 409 || !strings.Contains(errText(v), "turned off") {
		t.Errorf("off: %d %v", code, v)
	}

	// SiliconDust's site is having a bad day.
	e.call(t, "PATCH", "/yagi/api/settings", `{"firmwareCheck":true}`)
	withChecker(e, changelogServer(t, 503).URL, nil)
	code, v := e.call(t, "POST", "/yagi/api/firmware/check", "")
	if code != 502 || !strings.Contains(errText(v), "503") {
		t.Errorf("site down: %d %v", code, v)
	}
	if fw := fwOf(t, e); fw["state"] != "unknown" || !strings.Contains(fw["error"].(string), "503") {
		t.Errorf("the status should carry the reason: %v", fw)
	}
}

func TestCheckNowIsAdminOnlyAndGuarded(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	c := withChecker(e, changelogServer(t, 200).URL, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/yagi/api/firmware/check", nil)
	req.RemoteAddr = "100.67.117.32:1" // a viewer, not an admin
	e.pol.AdminOnly(e.api.Handler()).ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Errorf("viewer: %d", rec.Code)
	}
	if code, _ := e.call(t, "POST", "/yagi/api/firmware/check", "", "Origin", "https://evil.example"); code != 403 {
		t.Errorf("cross-origin: %d", code)
	}
	if !c.CheckedAt().IsZero() {
		t.Error("a refused request still reached SiliconDust")
	}
	if code, _ := e.call(t, "GET", "/yagi/api/firmware/check", ""); code != 405 {
		t.Errorf("GET: %d", code)
	}
}

func TestEachDeviceGetsItsOwnFirmwareStatus(t *testing.T) {
	e := newMultiEnv(t, twoDevices(), true)
	c := withChecker(e, changelogServer(t, 200).URL, nil)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"antenna", "cable"} {
		fw := obj(t, devDoc(t, e, name)["firmware"])
		if fw["state"] != "update-available" || fw["current"] != "20250815" {
			t.Errorf("%s: %v", name, fw)
		}
	}
}
