package server

import (
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/access"
	"github.com/johnschult/yagi/internal/tailnet"
)

type stubNode struct {
	mu        sync.Mutex
	closed    bool
	listeners map[string]net.Listener // by the address asked for (":5004"): a loopback listener standing in for the tailnet
}

func (n *stubNode) Listen(addr string) (net.Listener, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil, net.ErrClosed
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	if n.listeners == nil {
		n.listeners = map[string]net.Listener{}
	}
	n.listeners[addr] = ln
	return ln, nil
}

// at is where the tailnet side of a port can be reached in the test, once it has been opened.
func (n *stubNode) at(addr string) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if ln := n.listeners[addr]; ln != nil {
		return ln.Addr().String()
	}
	return ""
}

func (n *stubNode) Status() tailnet.NodeStatus {
	return tailnet.NodeStatus{State: tailnet.NeedsLogin, LoginURL: "https://login.tailscale.com/a/xyz"}
}
func (n *stubNode) Close() error {
	n.mu.Lock()
	n.closed = true
	for _, ln := range n.listeners {
		_ = ln.Close()
	}
	n.mu.Unlock()
	return nil
}
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

func tc(r *rig) tailnetControl { return tailnetControl{r.srv} }

// tailRig starts a server with the Tailscale switch on and a stub node, and waits for the node to be asked to listen.
func tailRig(t *testing.T, enabled bool) (*rig, *stubNode) {
	t.Helper()
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	devs := []access.Device{{Name: "antenna", Listen: ":5004", Tuner: a.name, StreamPort: a.streamPort}}
	cfgPath := filepath.Join(t.TempDir(), "yagi.json")
	s := access.DefaultSettings()
	s.Tailscale = &access.TailscaleSettings{Enabled: enabled, Hostname: "attic"}
	if err := access.SaveSettings(cfgPath, s); err != nil {
		t.Fatal(err)
	}
	node := &stubNode{}
	r := startConfigured(t, s, devs, "", "", func(c *Config) {
		c.ConfigPath = cfgPath
		c.TailscaleStart = func(tailnet.Config) tailnet.Node { return node }
	}, a)
	return r, node
}

func TestATunersPortIsAnsweredOnTheTailnetByTheSameHandler(t *testing.T) {
	r, node := tailRig(t, true)
	waitFor(t, "the tailnet listener", func() bool { return node.at(":5004") != "" })
	resp, err := http.Get("http://" + node.at(":5004") + "/api/state") // the admin pages are on the tuner's port
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("/api/state on the tailnet: %d", resp.StatusCode)
	}
	// and the tuner's own address still answers on the LAN side
	if code, _ := r.get(t, "antenna", "/api/state"); code != 200 {
		t.Errorf("on the LAN: %d", code)
	}
	st := tc(r).Status()
	if len(st.Serving) != 1 || st.Serving[0] != "5004" {
		t.Errorf("serving = %v, want [5004]", st.Serving)
	}
}

func TestNothingIsAnsweredOnTheTailnetWhileTheSwitchIsOff(t *testing.T) {
	r, node := tailRig(t, false)
	time.Sleep(300 * time.Millisecond)
	if got := node.at(":5004"); got != "" {
		t.Errorf("listening on the tailnet with the switch off: %s", got)
	}
	if st := tc(r).Status(); len(st.Serving) != 0 {
		t.Errorf("serving = %v", st.Serving)
	}
}

func TestSwitchingTheNodeOffStopsAnsweringOnTheTailnet(t *testing.T) {
	r, node := tailRig(t, true)
	waitFor(t, "the tailnet listener", func() bool { return node.at(":5004") != "" })
	addr := node.at(":5004")
	set := r.srv.Policy().Settings()
	set.Tailscale = &access.TailscaleSettings{Enabled: false, Hostname: "attic"}
	if err := r.srv.Policy().Update(set); err != nil {
		t.Fatal(err)
	}
	tc(r).Reconcile()
	waitFor(t, "the tailnet listener to close", func() bool {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			return false
		}
		return true
	})
	if st := tc(r).Status(); len(st.Serving) != 0 {
		t.Errorf("serving = %v after switching off", st.Serving)
	}
}

func TestAMovedPortIsAnsweredOnTheTailnetAtItsNewNumber(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	p1, p2 := freePort(t), freePort(t)
	devs := []access.Device{{Name: "antenna", Tuner: a.name, StreamPort: a.streamPort, Listen: "127.0.0.1:" + p1}}
	s := access.DefaultSettings()
	s.Tailscale = &access.TailscaleSettings{Enabled: true, Hostname: "attic"}
	node := &stubNode{}
	f := startFleetWith(t, s, devs, true, func(c *Config) { c.TailscaleStart = func(tailnet.Config) tailnet.Node { return node } }, a)
	waitFor(t, "the tailnet listener", func() bool { return node.at(":"+p1) != "" })

	if code, body := call(t, "PATCH", f.base+"/api/devices/antenna", map[string]string{"listen": "127.0.0.1:" + p2}); code != 200 {
		t.Fatalf("moving the port: %d %s", code, body)
	}
	f.srv.syncTailnet()
	waitFor(t, "the new port", func() bool { return node.at(":"+p2) != "" })
	st := tailnetControl{f.srv}.Status()
	if len(st.Serving) != 1 || st.Serving[0] != p2 {
		t.Errorf("serving = %v, want only the new port %s", st.Serving, p2)
	}
}

func TestARemovedTunerIsNoLongerAnsweredOnTheTailnet(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	b := newFakeTuner(t, "10DEF123", "cable-box")
	p1, p2 := freePort(t), freePort(t)
	devs := []access.Device{
		{Name: "antenna", Tuner: a.name, StreamPort: a.streamPort, Listen: "127.0.0.1:" + p1},
		{Name: "cable", Tuner: b.name, StreamPort: b.streamPort, Listen: "127.0.0.1:" + p2},
	}
	s := access.DefaultSettings()
	s.Tailscale = &access.TailscaleSettings{Enabled: true, Hostname: "attic"}
	node := &stubNode{}
	f := startFleetWith(t, s, devs, true, func(c *Config) { c.TailscaleStart = func(tailnet.Config) tailnet.Node { return node } }, a, b)
	waitFor(t, "both tailnet listeners", func() bool { return node.at(":"+p1) != "" && node.at(":"+p2) != "" })

	if code, body := call(t, "DELETE", f.base+"/api/devices/cable", nil); code != 204 && code != 200 {
		t.Fatalf("removing: %d %s", code, body)
	}
	f.srv.syncTailnet()
	st := tailnetControl{f.srv}.Status()
	if len(st.Serving) != 1 || st.Serving[0] != p1 {
		t.Errorf("serving = %v, want only %s", st.Serving, p1)
	}
}
