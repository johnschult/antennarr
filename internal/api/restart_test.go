package api

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/hdhr"
	"github.com/johnschult/yagi/internal/hdhr/hdhrtest"
	"github.com/johnschult/yagi/internal/restart"
)

const restartPath = "/api/devices/antenna/restart"

func restartEnv(t *testing.T, status string, ctrl hdhrtest.Options) (*env, *hdhrtest.Server) {
	t.Helper()
	e := newEnvWith(t, johnAndMom, true, status)
	srv, err := hdhrtest.New("127.0.0.1:0", ctrl)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e.api.Restart = restart.New(ctx, restart.Config{
		Dial:       func(ctx context.Context, _ string) (*hdhr.Conn, error) { return hdhr.Dial(ctx, srv.Addr()) },
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		GoneWithin: 2 * time.Second, BackWithin: 3 * time.Second, PollEvery: 20 * time.Millisecond,
	})
	return e, srv
}

func restartOf(t *testing.T, e *env) map[string]any {
	t.Helper()
	r, _ := devDoc(t, e, "antenna")["restart"].(map[string]any)
	return r
}

func waitRestart(t *testing.T, e *env) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if r := restartOf(t, e); r != nil && r["phase"] != "restarting" {
			return r
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("the restart did not end")
	return nil
}

func TestRestartingATunerRunsAndShowsInTheState(t *testing.T) {
	e, ctrl := restartEnv(t, idleTuner, hdhrtest.Options{RestartFor: 300 * time.Millisecond})
	if restartOf(t, e) != nil {
		t.Fatal("the state has a restart before there was one")
	}
	if code, v := e.call(t, "POST", restartPath, `{}`); code != 202 {
		t.Fatalf("%d %v", code, v)
	}
	if r := waitRestart(t, e); r["phase"] != "done" {
		t.Fatalf("restart = %v", r)
	}
	if ctrl.Restarts() != 1 {
		t.Errorf("the tuner was told to restart %d times", ctrl.Restarts())
	}
	if code, _ := e.call(t, "DELETE", restartPath, ""); code != 204 {
		t.Errorf("dismiss: %d", code)
	}
	if restartOf(t, e) != nil {
		t.Error("the result is still there after dismissing it")
	}
}

func TestSomethingInUseNeedsTheInterruptFlagToRestart(t *testing.T) {
	e, ctrl := restartEnv(t, defaultBusyStatus, hdhrtest.Options{RestartFor: 300 * time.Millisecond})
	code, v := e.call(t, "POST", restartPath, `{}`)
	if code != 409 || !strings.Contains(errText(v), "2 of the tuner's tuners are in use") || !strings.Contains(errText(v), "interrupt") {
		t.Fatalf("%d %v", code, v)
	}
	if len(ctrl.Frames()) != 0 {
		t.Errorf("a refused request sent %d frames", len(ctrl.Frames()))
	}
	if code, v := e.call(t, "POST", restartPath, `{"interrupt":true}`); code != 202 {
		t.Fatalf("%d %v", code, v)
	}
	if r := waitRestart(t, e); r["phase"] != "done" {
		t.Errorf("restart = %v", r)
	}
}

func TestATunerThatIsOfflineCannotBeRestarted(t *testing.T) {
	e, ctrl := restartEnv(t, idleTuner, hdhrtest.Options{})
	e.tun.Close()
	for i := 0; i < 4; i++ {
		e.mon.Poll(context.Background())
	}
	code, v := e.call(t, "POST", restartPath, `{"interrupt":true}`)
	if code != 409 || !strings.Contains(errText(v), "offline") {
		t.Errorf("%d %v", code, v)
	}
	if len(ctrl.Frames()) != 0 {
		t.Error("asked an offline tuner")
	}
}

func TestOnlyOneRestartAtATime(t *testing.T) {
	e, _ := restartEnv(t, idleTuner, hdhrtest.Options{RestartFor: 600 * time.Millisecond})
	if code, v := e.call(t, "POST", restartPath, `{}`); code != 202 {
		t.Fatalf("%d %v", code, v)
	}
	code, v := e.call(t, "POST", restartPath, `{}`)
	if code != 409 || !strings.Contains(errText(v), "already restarting") {
		t.Errorf("a second: %d %v", code, v)
	}
	if code, _ := e.call(t, "DELETE", restartPath, ""); code != 409 {
		t.Errorf("dismissing one that is running: %d", code)
	}
	waitRestart(t, e)
}

func TestRestartRefusals(t *testing.T) {
	e, _ := restartEnv(t, idleTuner, hdhrtest.Options{})
	if code, _ := e.call(t, "POST", "/api/devices/nope/restart", `{}`); code != 404 {
		t.Errorf("unknown tuner: %d", code)
	}
	e.api.Restart = nil
	if code, _ := e.call(t, "POST", restartPath, `{}`); code != 409 {
		t.Errorf("unavailable: %d", code)
	}
}

func TestATunerBeingUpdatedIsNotRestarted(t *testing.T) {
	e, ctrl, _ := upgradeEnv(t, idleTuner, hdhrtest.Options{RebootFor: 600 * time.Millisecond, VersionAfter: "20260326"})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e.api.Restart = restart.New(ctx, restart.Config{Dial: func(ctx context.Context, _ string) (*hdhr.Conn, error) { return hdhr.Dial(ctx, ctrl.Addr()) }})
	if code, v := e.call(t, "POST", upgradePath, `{"version":"20260326"}`); code != 202 {
		t.Fatalf("%d %v", code, v)
	}
	code, v := e.call(t, "POST", restartPath, `{}`)
	if code != 409 || !strings.Contains(errText(v), "being updated") {
		t.Errorf("%d %v", code, v)
	}
	if ctrl.Restarts() != 0 {
		t.Error("restarted a tuner in the middle of an update")
	}
	waitUpgrade(t, e)
}
