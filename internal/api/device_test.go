package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/johnschult/antennarr/internal/hdhr"
	"github.com/johnschult/antennarr/internal/hdhr/hdhrtest"
	"github.com/johnschult/antennarr/internal/tunerinfo"
)

const infoPath = "/antennarr/api/devices/antenna/info"

func infoEnv(t *testing.T) (*env, *hdhrtest.Server) {
	t.Helper()
	e := newEnvWith(t, johnAndMom, true, idleTuner)
	ctrl, err := hdhrtest.New("127.0.0.1:0", hdhrtest.Options{Version: "20250815"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctrl.Close)
	e.api.Device = &tunerinfo.Reader{Dial: func(ctx context.Context, host string) (*hdhr.Conn, error) { return hdhr.Dial(ctx, ctrl.Addr()) }}
	return e, ctrl
}

func TestDeviceInfoReadsTheTuner(t *testing.T) {
	e, _ := infoEnv(t)
	code, v := e.call(t, "GET", infoPath, "")
	if code != 200 {
		t.Fatalf("%d %v", code, v)
	}
	m := v.(map[string]any)
	if m["hardwareModel"] != "HDFX-4K" || m["version"] != "20250815" || m["model"] != "hdhomerun_dvr_atsc3" {
		t.Errorf("got %v", m)
	}
	// The fake tuner has no System Menu page: what it couldn't read is said, not hidden.
	if p, _ := m["problems"].([]any); len(p) != 1 || !strings.Contains(p[0].(string), "System Menu") {
		t.Errorf("problems = %v", m["problems"])
	}
}

func TestDeviceInfoNeverWrites(t *testing.T) {
	e, ctrl := infoEnv(t)
	e.call(t, "GET", infoPath, "")
	for _, f := range ctrl.Frames() {
		if f.Type != hdhr.TypeGetSetReq {
			t.Fatalf("frame type %#x", f.Type)
		}
		tlvs, _ := hdhr.ParseTLVs(f.Payload)
		for _, tl := range tlvs {
			if tl.Tag == hdhr.TagGetSetValue {
				t.Fatal("a read carried a value")
			}
		}
	}
}

func TestDeviceInfoRefusals(t *testing.T) {
	e, ctrl := infoEnv(t)
	if code, _ := e.call(t, "GET", "/antennarr/api/devices/nope/info", ""); code != 404 {
		t.Errorf("unknown tuner: %d", code)
	}
	e.api.Device = nil
	if code, _ := e.call(t, "GET", infoPath, ""); code != 409 {
		t.Errorf("unavailable: %d", code)
	}
	e.api.Device = &tunerinfo.Reader{Dial: func(ctx context.Context, host string) (*hdhr.Conn, error) { return hdhr.Dial(ctx, ctrl.Addr()) }}
	e.tun.Close()
	for i := 0; i < 4; i++ {
		e.mon.Poll(context.Background())
	}
	ctrl.Forget()
	code, v := e.call(t, "GET", infoPath, "")
	if code != 409 || !strings.Contains(errText(v), "offline") {
		t.Errorf("offline: %d %v", code, v)
	}
	if len(ctrl.Frames()) != 0 {
		t.Error("asked an offline tuner")
	}
}

func TestDeviceInfoIsForAdminsOnly(t *testing.T) {
	e, ctrl := infoEnv(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", infoPath, nil)
	req.RemoteAddr = "100.67.117.32:1" // John is a viewer, not an admin
	e.pol.AdminOnly(e.api.Handler()).ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Errorf("status %d", rec.Code)
	}
	if len(ctrl.Frames()) != 0 {
		t.Error("the tuner was asked on a viewer's behalf")
	}
}
