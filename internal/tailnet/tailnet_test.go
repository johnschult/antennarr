package tailnet

import (
	"sync"
	"testing"

	"github.com/johnschult/yagi/internal/access"
)

type fakeNode struct {
	cfg    Config
	mu     sync.Mutex
	status NodeStatus
	closed bool
}

func (f *fakeNode) Status() NodeStatus { f.mu.Lock(); defer f.mu.Unlock(); return f.status }
func (f *fakeNode) Close() error       { f.mu.Lock(); f.closed = true; f.mu.Unlock(); return nil }
func (f *fakeNode) set(s NodeStatus)   { f.mu.Lock(); f.status = s; f.mu.Unlock() }

type rig struct {
	set   access.Settings
	nodes []*fakeNode
	m     *Manager
}

func newRig(t *testing.T, dir, key string, built bool) *rig {
	t.Helper()
	r := &rig{set: access.DefaultSettings()}
	var start Starter
	if built {
		start = func(c Config) Node {
			n := &fakeNode{cfg: c, status: NodeStatus{State: Starting}}
			r.nodes = append(r.nodes, n)
			return n
		}
	}
	r.m = NewManager(func() access.Settings { return r.set }, dir, key, nil, start)
	return r
}

func (r *rig) enable(host string) {
	r.set.Tailscale = &access.TailscaleSettings{Enabled: true, Hostname: host}
}

func TestOffByDefaultAndNothingStarts(t *testing.T) {
	r := newRig(t, "/config/tailscale", "", true)
	r.m.Reconcile()
	if len(r.nodes) != 0 {
		t.Fatalf("started %d nodes with the switch off", len(r.nodes))
	}
	if st := r.m.Status(); st.State != Off || !st.Built || st.Hostname != "yagi" {
		t.Errorf("status = %+v, want off, built, named yagi", st)
	}
}

func TestSwitchingOnStartsOneNodeAndReportsItsStatus(t *testing.T) {
	r := newRig(t, "/config/tailscale", "tskey-abc", true)
	r.enable("")
	r.m.Reconcile()
	r.m.Reconcile() // nothing changed: nothing more starts
	if len(r.nodes) != 1 {
		t.Fatalf("started %d nodes, want 1", len(r.nodes))
	}
	if c := r.nodes[0].cfg; c.Hostname != "yagi" || c.Dir != "/config/tailscale" || c.AuthKey != "tskey-abc" {
		t.Errorf("node config = %+v", c)
	}
	if st := r.m.Status(); st.State != Starting || !st.AuthKeyInEnv {
		t.Errorf("status = %+v, want starting with the key noted", st)
	}
	r.nodes[0].set(NodeStatus{State: NeedsLogin, LoginURL: "https://login.tailscale.com/a/123"})
	if st := r.m.Status(); st.State != NeedsLogin || st.LoginURL != "https://login.tailscale.com/a/123" {
		t.Errorf("status = %+v, want the login link", st)
	}
	r.nodes[0].set(NodeStatus{State: Running, DNSName: "yagi.tail1234.ts.net", Addresses: []string{"100.64.0.5"}})
	if st := r.m.Status(); st.State != Running || st.DNSName != "yagi.tail1234.ts.net" || len(st.Addresses) != 1 || st.LoginURL != "" {
		t.Errorf("status = %+v, want running with its name and address", st)
	}
}

func TestANewHostnameRestartsTheNodeAndSwitchingOffStopsIt(t *testing.T) {
	r := newRig(t, "/config/tailscale", "", true)
	r.enable("")
	r.m.Reconcile()
	r.enable("attic")
	r.m.Reconcile()
	if len(r.nodes) != 2 || !r.nodes[0].closed || r.nodes[1].cfg.Hostname != "attic" {
		t.Fatalf("nodes = %d, first closed = %v: want the old one stopped and a new one named attic", len(r.nodes), r.nodes[0].closed)
	}
	r.set.Tailscale.Enabled = false
	r.m.Reconcile()
	if !r.nodes[1].closed {
		t.Error("the node is still running after the switch went off")
	}
	if st := r.m.Status(); st.State != Off {
		t.Errorf("status = %+v, want off", st)
	}
}

func TestWithNowhereToKeepItsIdentityItDoesNotStart(t *testing.T) {
	r := newRig(t, "", "", true)
	r.enable("")
	r.m.Reconcile()
	if len(r.nodes) != 0 {
		t.Error("started with no directory to keep the node's identity in")
	}
	if st := r.m.Status(); st.State != NoStore {
		t.Errorf("state = %q, want %q", st.State, NoStore)
	}
}

func TestABuildWithoutItSaysSoAndDoesNothing(t *testing.T) {
	r := newRig(t, "/config/tailscale", "", false)
	r.enable("")
	r.m.Reconcile()
	if st := r.m.Status(); st.Built || st.State != Unavailable {
		t.Errorf("status = %+v, want unavailable and not built", st)
	}
	var none *Manager
	none.Reconcile() // a server with no manager at all is fine
	if st := none.Status(); st.State != Unavailable {
		t.Errorf("nil manager status = %+v", st)
	}
	none.Close()
}

func TestCloseStopsTheNode(t *testing.T) {
	r := newRig(t, "/config/tailscale", "", true)
	r.enable("")
	r.m.Reconcile()
	r.m.Close()
	if !r.nodes[0].closed {
		t.Error("Close left the node running")
	}
}
