package server

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/access"
	"github.com/johnschult/yagi/internal/tailnet"
)

type stubNode struct {
	mu     sync.Mutex
	closed bool
}

func (n *stubNode) Status() tailnet.NodeStatus {
	return tailnet.NodeStatus{State: tailnet.NeedsLogin, LoginURL: "https://login.tailscale.com/a/xyz"}
}
func (n *stubNode) Close() error { n.mu.Lock(); n.closed = true; n.mu.Unlock(); return nil }
func (n *stubNode) isClosed() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.closed
}

func TestTheTailscaleNodeStartsWithTheServerAndStopsWithIt(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	devs := []access.Device{{Name: "antenna", Listen: ":5004", Tuner: a.name, StreamPort: a.streamPort}}
	cfgPath := filepath.Join(t.TempDir(), "yagi.json")
	s := access.DefaultSettings()
	s.Tailscale = &access.TailscaleSettings{Enabled: true, Hostname: "attic"}
	if err := access.SaveSettings(cfgPath, s); err != nil {
		t.Fatal(err)
	}

	var started tailnet.Config
	node := &stubNode{}
	r := startConfigured(t, s, devs, "", "", func(c *Config) {
		c.ConfigPath = cfgPath
		c.TailscaleAuthKey = "tskey-auth-123"
		c.TailscaleStart = func(tc tailnet.Config) tailnet.Node { started = tc; return node }
	}, a)

	if started.Hostname != "attic" || started.AuthKey != "tskey-auth-123" || started.Dir != filepath.Join(filepath.Dir(cfgPath), "tailscale") {
		t.Errorf("node config = %+v", started)
	}
	code, body := r.get(t, "antenna", "/api/state")
	if code != 200 {
		t.Fatalf("state: %d", code)
	}
	var st struct {
		Tailscale struct {
			Built, AuthKeyInEnv bool
			State, LoginURL     string
		}
		Settings struct{ Tailscale struct{ Enabled bool } }
	}
	if err := json.Unmarshal([]byte(body), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Settings.Tailscale.Enabled || st.Tailscale.State != "needs-login" || st.Tailscale.LoginURL == "" || !st.Tailscale.AuthKeyInEnv {
		t.Errorf("state = %s", body)
	}

	r.cancel()
	deadline := time.Now().Add(5 * time.Second)
	for !node.isClosed() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !node.isClosed() {
		t.Error("the node was left running after the server stopped")
	}
}
