package api

import (
	"sync"
	"testing"

	"github.com/johnschult/yagi/internal/access"
	"github.com/johnschult/yagi/internal/tailnet"
)

type fakeTailnet struct {
	mu         sync.Mutex
	reconciled int
	status     tailnet.Status
}

func (f *fakeTailnet) Reconcile() { f.mu.Lock(); f.reconciled++; f.mu.Unlock() }
func (f *fakeTailnet) Status() tailnet.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func tsDoc(t *testing.T, e *env) (settings, status map[string]any) {
	t.Helper()
	code, v := e.call(t, "GET", "/api/state", "")
	if code != 200 {
		t.Fatalf("state: %d", code)
	}
	st := obj(t, v)
	return obj(t, obj(t, st["settings"])["tailscale"]), obj(t, st["tailscale"])
}

func TestTailscaleSettingsAndStatusInTheState(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	ft := &fakeTailnet{status: tailnet.Status{Built: true, State: tailnet.Off, Hostname: "yagi"}}
	e.api.Tailnet = ft

	set, status := tsDoc(t, e)
	if set["enabled"] != false || set["hostname"] != "yagi" || status["state"] != "off" || status["built"] != true {
		t.Fatalf("defaults: settings %v, status %v", set, status)
	}

	if code, v := e.call(t, "PATCH", "/api/settings", `{"tailscaleEnabled":true,"tailscaleHostname":" Attic "}`); code != 200 {
		t.Fatalf("patch: %d %v", code, v)
	}
	if ft.reconciled != 1 {
		t.Errorf("reconciled %d times, want once after the change", ft.reconciled)
	}
	set, _ = tsDoc(t, e)
	if set["enabled"] != true || set["hostname"] != "attic" {
		t.Errorf("settings = %v, want on and attic", set)
	}
	if got := e.pol.Settings(); got.Tailscale == nil || got.Tailscale.Hostname != "attic" {
		t.Errorf("policy settings = %+v", got.Tailscale)
	}

	// Naming it the default stores nothing; an unrelated change does not touch the node.
	if code, _ := e.call(t, "PATCH", "/api/settings", `{"tailscaleHostname":"yagi"}`); code != 200 {
		t.Fatal("patch to the default name failed")
	}
	if got := e.pol.Settings(); got.Tailscale.Hostname != "" {
		t.Errorf("default hostname was written: %q", got.Tailscale.Hostname)
	}
	before := ft.reconciled
	e.call(t, "PATCH", "/api/settings", `{"sharedCap":2}`)
	if ft.reconciled != before {
		t.Error("an unrelated change nudged the Tailscale node")
	}
}

func TestTailscaleRefusesABadHostname(t *testing.T) {
	e := newEnv(t, access.Settings{}, true)
	e.api.Tailnet = &fakeTailnet{}
	for _, bad := range []string{"my yagi", "-x", "a.b", ""} {
		body := `{"tailscaleHostname":"` + bad + `"}`
		code, _ := e.call(t, "PATCH", "/api/settings", body)
		if bad == "" {
			if code != 200 { // blank means the default
				t.Errorf("blank hostname: %d, want 200 (the default)", code)
			}
			continue
		}
		if code != 400 {
			t.Errorf("%q: %d, want 400", bad, code)
		}
	}
}

func TestWithoutANodeTheStateSaysUnavailable(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	_, status := tsDoc(t, e)
	if status["state"] != "unavailable" || status["built"] != false {
		t.Errorf("status = %v, want unavailable and not built", status)
	}
}

func TestTailnetOnlyCanOnlyBeSwitchedOnWhileTheNodeIsConnected(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	ft := &fakeTailnet{status: tailnet.Status{Built: true, State: tailnet.NeedsLogin, Hostname: "yagi"}}
	e.api.Tailnet = ft
	if code, _ := e.call(t, "PATCH", "/api/settings", `{"tailscaleEnabled":true}`); code != 200 {
		t.Fatal("turning the node on failed")
	}
	if code, v := e.call(t, "PATCH", "/api/settings", `{"tailnetOnly":true}`); code != 409 {
		t.Fatalf("with the node not connected: %d %v, want 409", code, v)
	}
	if e.pol.Settings().TailnetOnly() {
		t.Fatal("tailnet only was saved although it was refused")
	}

	ft.mu.Lock()
	ft.status.State = tailnet.Running
	ft.mu.Unlock()
	if code, v := e.call(t, "PATCH", "/api/settings", `{"tailnetOnly":true}`); code != 200 {
		t.Fatalf("with the node connected: %d %v", code, v)
	}
	set, _ := tsDoc(t, e)
	if set["tailnetOnly"] != true || !e.pol.Settings().TailnetOnly() {
		t.Errorf("settings = %v, want tailnet only on", set)
	}

	// Switching it off is always allowed, connected or not.
	ft.mu.Lock()
	ft.status.State = tailnet.Failed
	ft.mu.Unlock()
	if code, _ := e.call(t, "PATCH", "/api/settings", `{"tailnetOnly":false}`); code != 200 {
		t.Error("could not switch tailnet only off while the node was down")
	}
	if e.pol.Settings().TailnetOnly() {
		t.Error("still on")
	}
}

func TestHTTPSIsASeparateOptInSetting(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	ft := &fakeTailnet{status: tailnet.Status{Built: true, State: tailnet.Off, Hostname: "yagi"}}
	e.api.Tailnet = ft
	if set, _ := tsDoc(t, e); set["https"] != false {
		t.Fatalf("off by default, got %v", set)
	}
	if code, v := e.call(t, "PATCH", "/api/settings", `{"tailscaleEnabled":true,"tailscaleHttps":true}`); code != 200 {
		t.Fatalf("patch: %d %v", code, v)
	}
	set, _ := tsDoc(t, e)
	if set["https"] != true || !e.pol.Settings().TailscaleHTTPS() {
		t.Errorf("settings = %v, want HTTPS on", set)
	}
	before := ft.reconciled
	e.call(t, "PATCH", "/api/settings", `{"tailscaleHttps":false}`)
	if ft.reconciled == before || e.pol.Settings().TailscaleHTTPS() {
		t.Error("switching HTTPS off did not reach the node, or did not save")
	}
}
