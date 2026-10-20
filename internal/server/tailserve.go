package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/johnschult/yagi/internal/api"
	"github.com/johnschult/yagi/internal/tailnet"
)

// tailPort is one port Yagi answers on at the Tailscale node's addresses, besides the LAN.
type tailPort struct {
	ln      net.Listener
	srv     *http.Server
	handler http.Handler
}

// tailServing keeps the tailnet side of every port Yagi serves: a listener on the node for each tuner's port (the admin
// pages are on the same port, or on the separate admin port), answered by the very handler that port has on the LAN.
// So a viewer on the tailnet adds the tuner at <node name>:<port> like any other HDHomeRun, and the same limits,
// allowlist and logging apply to it.
type tailServing struct {
	mu     sync.Mutex
	gen    uint64
	ports  map[string]*tailPort
	failed map[string]uint64 // port -> the generation it was last reported failing in, so a failure is said once
}

// tailnetControl is what the API sees of the node: the manager's status with the ports being served, and a
// Reconcile that also brings the listeners in line, since the node starts and stops with the settings.
type tailnetControl struct{ s *Server }

func (c tailnetControl) Status() tailnet.Status {
	st := c.s.tailnet.Status()
	st.Serving = c.s.tailPorts()
	st.LANClosed = c.s.lanClosed()
	return st
}

func (c tailnetControl) Reconcile() {
	c.s.tailnet.Reconcile()
	c.s.syncTailnet()
}

var _ api.TailnetControl = tailnetControl{}

// tailWanted is every port that should be answered on the tailnet, with its handler: each tuner's, and the admin port's
// when it has one of its own (or there are no tuners).
func (s *Server) tailWanted() map[string]http.Handler {
	want := map[string]http.Handler{}
	for _, d := range s.deviceList() {
		if d.handler != nil {
			want[portOfListen(d.cfg.Listen)] = d.handler
		}
	}
	s.fleetMu.Lock()
	if s.bare != nil && s.bare.srv != nil {
		if _, port, err := net.SplitHostPort(s.bare.ln.Addr().String()); err == nil {
			want[port] = s.bare.srv.Handler
		}
	}
	s.fleetMu.Unlock()
	return want
}

func portOfListen(listen string) string {
	if _, p, err := net.SplitHostPort(listen); err == nil {
		return p
	}
	return listen
}

// syncTailnet makes the tailnet listeners match the node and the ports: one for each port wanted while a node is
// running, none when it is off, and a new set when the node was replaced. It is cheap when nothing has changed.
func (s *Server) syncTailnet() {
	want := s.tailWanted() // before the lock: it takes the fleet's
	t := &s.tail
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ports == nil {
		t.ports = map[string]*tailPort{}
		t.failed = map[string]uint64{}
	}
	gen := s.tailnet.Generation()
	if gen != t.gen {
		// The node these listeners were made on has gone, and they have gone with it.
		for port := range t.ports {
			t.drop(port)
		}
		t.gen = gen
	}
	for port, tp := range t.ports {
		if h, ok := want[port]; !ok || h != tp.handler {
			t.drop(port)
		}
	}
	for port, h := range want {
		if _, ok := t.ports[port]; ok {
			continue
		}
		ln, g, err := s.tailnet.Listen(":" + port)
		if err != nil {
			if !errors.Is(err, tailnet.ErrNoNode) && t.failed[port] != g {
				t.failed[port] = g
				s.log.Warn("tailscale: could not answer on the tailnet", "port", port, "err", err)
			}
			continue
		}
		if g != gen { // the node changed between the two calls: leave it to the next look
			_ = ln.Close()
			continue
		}
		srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
		t.ports[port] = &tailPort{ln: ln, srv: srv, handler: h}
		s.log.Info("tailscale: answering on the tailnet", "port", port)
		go func() { _ = srv.Serve(ln) }()
	}
}

// drop stops answering on a port. The caller holds t.mu.
func (t *tailServing) drop(port string) {
	tp := t.ports[port]
	delete(t.ports, port)
	if tp == nil {
		return
	}
	go retire(tp.srv) // closes its listener; a stream already running is given a moment
	_ = tp.ln.Close()
}

// tailPorts is the ports being answered on the tailnet now, in order.
func (s *Server) tailPorts() []string {
	t := &s.tail
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.ports))
	for p := range t.ports {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// closeTailnetServing stops every tailnet listener, when the server is stopping.
func (s *Server) closeTailnetServing() {
	t := &s.tail
	t.mu.Lock()
	defer t.mu.Unlock()
	for port := range t.ports {
		t.drop(port)
	}
}

// watchTailnet looks after the listeners as the node comes up, goes away or is replaced, and as tuners are added,
// moved and removed. The node starts and stops on its own schedule, so this looks regularly instead of being told.
func (s *Server) watchTailnet(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		s.syncTailnet()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
