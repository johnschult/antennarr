// Package tailnet is the optional built-in Tailscale node: Yagi joins a tailnet as a machine of its own, so the
// host needs no Tailscale install and no firewall rules for it.
//
// The node itself (Tailscale's tsnet package, a large dependency) is only in builds made with -tags tsnet; see
// node_tsnet.go. Everything here, the settings, the manager that starts and stops the node, and the status the page
// shows, is in every build, so a build without it can say so plainly instead of the setting vanishing.
package tailnet

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"

	"github.com/johnschult/yagi/internal/access"
)

// State is where the node is.
type State string

const (
	// Off: not switched on.
	Off State = "off"
	// Unavailable: this build does not have it. Use the -tailscale image or build with -tags tsnet.
	Unavailable State = "unavailable"
	// NoStore: switched on, but there is nowhere to keep the node's identity (Yagi started without a settings file).
	NoStore State = "no-store"
	// Starting: connecting.
	Starting State = "starting"
	// NeedsLogin: not signed in to a tailnet yet; LoginURL says where.
	NeedsLogin State = "needs-login"
	// Running: on the tailnet.
	Running State = "running"
	// Failed: it could not start; Error says why.
	Failed State = "error"
)

// Status is what the page shows.
type Status struct {
	// Built says this build has the node at all.
	Built bool  `json:"built"`
	State State `json:"state"`
	// Hostname is the name asked for; DNSName is the full name the tailnet gave it (once running).
	Hostname string `json:"hostname"`
	DNSName  string `json:"dnsName,omitempty"`
	// LoginURL is where to sign the node in, while State is NeedsLogin.
	LoginURL  string   `json:"loginUrl,omitempty"`
	Addresses []string `json:"addresses,omitempty"`
	Error     string   `json:"error,omitempty"`
	// AuthKeyInEnv says TS_AUTHKEY is set, so signing in needs no link.
	AuthKeyInEnv bool `json:"authKeyInEnv,omitempty"`
	// Serving is the ports Yagi answers on at the node's addresses: the tuners' ports, and the admin port when it is separate.
	Serving []string `json:"serving,omitempty"`
	// LANClosed says the LAN ports are being refused right now, because tailnet-only is on and the node is connected.
	LANClosed bool `json:"lanClosed,omitempty"`
	// AdminReachable says somebody on the tailnet could open the admin pages (an admin range covers the tailnet, or a
	// Plex account is allowed), so closing the LAN does not leave only this machine able to. Always sent.
	AdminReachable bool `json:"adminReachable"`
	// HTTPS is the address the admin pages are served at over HTTPS, once they are; HTTPSError says why not, in words.
	HTTPS      string `json:"https,omitempty"`
	HTTPSError string `json:"httpsError,omitempty"`
}

// NodeStatus is what a running node knows about itself.
type NodeStatus struct {
	State     State
	DNSName   string
	LoginURL  string
	Addresses []string
	Error     string
}

// NewNode starts a node. It is nil in a build without the Tailscale node (everything but -tags tsnet): the page
// says so, and the setting does nothing.
var NewNode Starter

// Config is what a node is started with.
type Config struct {
	Hostname string
	// Dir keeps the node's identity between runs.
	Dir string
	// AuthKey signs the node in without a link. Empty to sign in with the link.
	AuthKey string
	Log     *slog.Logger
}

// Node is a running Tailscale node. It starts working as soon as it is made, in the background.
type Node interface {
	// Status is a snapshot, safe to call at any time.
	Status() NodeStatus
	// Listen accepts connections on the tailnet, at the node's own addresses (addr is ":5004"). It can be called as
	// soon as the node is made: the listener starts to answer once the node is on the tailnet.
	Listen(addr string) (net.Listener, error)
	// WhoIs says who is on the other end of a connection that came in over the node (remote is its RemoteAddr).
	WhoIs(ctx context.Context, remote string) (Who, error)
	// ListenTLS is Listen with HTTPS, using a certificate Tailscale gets for the node's name. It fails when the tailnet
	// has not turned on HTTPS certificates, and only once the node is running.
	ListenTLS(addr string) (net.Listener, error)
	// Close leaves the tailnet and stops.
	Close() error
}

// Who is the person and device the tailnet says a connection came from.
type Who struct {
	Login string // such as alice@example.com; empty for a device that belongs to a tag, not a person
	Node  string // the device's full tailnet name, such as laptop.tail1234.ts.net
}

// Starter makes a node, or nil in a build that doesn't have one.
type Starter func(Config) Node

// Manager keeps a node matching the settings: started when the switch goes on, stopped when it goes off, and
// restarted under a new name.
type Manager struct {
	settings func() access.Settings
	dir      string
	authKey  string
	log      *slog.Logger
	start    Starter

	mu   sync.Mutex
	node Node
	name string // the hostname the running node was started with
	gen  uint64 // counts nodes started and stopped, so a listener can tell it belongs to one that has gone
}

// NewManager makes a manager. dir is where the node keeps its identity ("" means there is nowhere, and the node is
// never started). start is NewNode for the real thing.
func NewManager(settings func() access.Settings, dir, authKey string, log *slog.Logger, start Starter) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{settings: settings, dir: dir, authKey: authKey, log: log, start: start}
}

// Reconcile makes the node match the settings now. It is cheap when nothing has changed.
func (m *Manager) Reconcile() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	set := m.settings()
	want := set.TailscaleEnabled() && m.dir != "" && m.start != nil
	if m.node != nil && (!want || m.name != set.TailscaleHostname()) {
		if err := m.node.Close(); err != nil {
			m.log.Warn("tailscale: stopping the node", "err", err)
		}
		m.node, m.name = nil, ""
		m.gen++
	}
	if want && m.node == nil {
		cfg := Config{Hostname: set.TailscaleHostname(), Dir: m.dir, AuthKey: m.authKey, Log: m.log}
		if n := m.start(cfg); n != nil {
			m.node, m.name = n, cfg.Hostname
			m.gen++
			m.log.Info("tailscale: starting", "hostname", cfg.Hostname)
		}
	}
}

// Status is what the page shows.
func (m *Manager) Status() Status {
	if m == nil {
		return Status{State: Unavailable, Hostname: access.DefaultTailscaleHostname}
	}
	set := m.settings()
	st := Status{Built: m.start != nil, Hostname: set.TailscaleHostname(), AuthKeyInEnv: m.authKey != ""}
	m.mu.Lock()
	node := m.node
	m.mu.Unlock()
	switch {
	case !st.Built:
		st.State = Unavailable
	case !set.TailscaleEnabled():
		st.State = Off
	case m.dir == "":
		st.State = NoStore
	case node == nil:
		st.State = Starting
	default:
		ns := node.Status()
		st.State, st.DNSName, st.LoginURL, st.Addresses, st.Error = ns.State, ns.DNSName, ns.LoginURL, ns.Addresses, ns.Error
		if st.State == "" {
			st.State = Starting
		}
	}
	return st
}

// Close stops the node.
func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.node != nil {
		_ = m.node.Close()
		m.node, m.name = nil, ""
		m.gen++
	}
}

// ErrNoNode is what Listen says when there is no node to listen on: it is off, still being made, or this build has none.
var ErrNoNode = errors.New("there is no Tailscale node running")

// Listen accepts connections on the tailnet through the running node. It also returns the generation of the node it
// belongs to; when Generation is different, that node has gone and so has the listener.
func (m *Manager) Listen(addr string) (net.Listener, uint64, error) {
	if m == nil {
		return nil, 0, ErrNoNode
	}
	m.mu.Lock()
	node, gen := m.node, m.gen
	m.mu.Unlock()
	if node == nil {
		return nil, gen, ErrNoNode
	}
	ln, err := node.Listen(addr)
	return ln, gen, err
}

// WhoIs asks the running node who is on the other end of a connection it accepted.
func (m *Manager) WhoIs(ctx context.Context, remote string) (Who, error) {
	if m == nil {
		return Who{}, ErrNoNode
	}
	m.mu.Lock()
	node := m.node
	m.mu.Unlock()
	if node == nil {
		return Who{}, ErrNoNode
	}
	return node.WhoIs(ctx, remote)
}

// CertWarmer is what a node can do to have its HTTPS certificate ready before anyone asks for it. A node that can't just
// fetches the certificate on the first visit, which takes a few seconds.
type CertWarmer interface {
	WarmCertificate(ctx context.Context, domain string) error
}

// WarmCertificate has the running node get its certificate for domain now. It does nothing, and says so with a nil
// error, for a node that can't.
func (m *Manager) WarmCertificate(ctx context.Context, domain string) error {
	if m == nil {
		return ErrNoNode
	}
	m.mu.Lock()
	node := m.node
	m.mu.Unlock()
	if node == nil {
		return ErrNoNode
	}
	if w, ok := node.(CertWarmer); ok {
		return w.WarmCertificate(ctx, domain)
	}
	return nil
}

// ListenTLS is Listen with HTTPS through the running node.
func (m *Manager) ListenTLS(addr string) (net.Listener, uint64, error) {
	if m == nil {
		return nil, 0, ErrNoNode
	}
	m.mu.Lock()
	node, gen := m.node, m.gen
	m.mu.Unlock()
	if node == nil {
		return nil, gen, ErrNoNode
	}
	ln, err := node.ListenTLS(addr)
	return ln, gen, err
}

// Generation changes whenever a node is started or stopped.
func (m *Manager) Generation() uint64 {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.gen
}
