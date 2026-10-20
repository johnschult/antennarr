package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/johnschult/yagi/internal/access"
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

	// The admin pages over HTTPS at the node's name, when asked for: one listener, on port 443.
	https       *tailPort
	httpsErr    string
	httpsFailed uint64 // the generation the failure was last logged in
}

// httpsPort is where the admin pages are served over HTTPS on the tailnet: the usual one, so the address needs no port.
const httpsPort = "443"

// tailnetControl is what the API sees of the node: the manager's status with the ports being served, and a
// Reconcile that also brings the listeners in line, since the node starts and stops with the settings.
type tailnetControl struct{ s *Server }

func (c tailnetControl) Status() tailnet.Status {
	st := c.s.tailnet.Status()
	st.Serving = c.s.tailPorts()
	st.LANClosed = c.s.lanClosed()
	st.AdminReachable = c.s.policy.Settings().AdminReaches(st.Addresses)
	st.HTTPS, st.HTTPSError = c.s.tailHTTPS(st.DNSName)
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
		t.dropHTTPS()
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
		srv := &http.Server{Handler: s.withTailnetIdentity(h), ReadHeaderTimeout: 10 * time.Second}
		t.ports[port] = &tailPort{ln: ln, srv: srv, handler: h}
		s.log.Info("tailscale: answering on the tailnet", "port", port)
		go func() { _ = srv.Serve(ln) }()
	}
	s.syncHTTPS(gen)
}

// syncHTTPS keeps the admin pages' HTTPS listener in line with the setting and the node. The caller holds t.mu.
func (s *Server) syncHTTPS(gen uint64) {
	t := &s.tail
	want := s.policy.Settings().TailscaleHTTPS() && s.tailnet.Status().State == tailnet.Running
	if !want {
		t.dropHTTPS()
		t.httpsErr = ""
		return
	}
	if t.https != nil {
		return
	}
	ln, g, err := s.tailnet.ListenTLS(":" + httpsPort)
	if err != nil {
		t.httpsErr = err.Error()
		if !errors.Is(err, tailnet.ErrNoNode) && t.httpsFailed != g {
			t.httpsFailed = g
			s.log.Warn("tailscale: could not serve the admin pages over HTTPS (does the tailnet have HTTPS certificates turned on?)", "err", err)
		}
		return
	}
	if g != gen { // the node changed between the two calls: leave it to the next look
		_ = ln.Close()
		return
	}
	h := s.adminMux()
	srv := &http.Server{Handler: s.withTailnetIdentity(h), ReadHeaderTimeout: 10 * time.Second}
	t.https, t.httpsErr = &tailPort{ln: ln, srv: srv, handler: h}, ""
	s.log.Info("tailscale: serving the admin pages over HTTPS", "port", httpsPort)
	go func() { _ = srv.Serve(ln) }()
}

// dropHTTPS stops the HTTPS listener. The caller holds t.mu.
func (t *tailServing) dropHTTPS() {
	tp := t.https
	t.https = nil
	if tp == nil {
		return
	}
	go retire(tp.srv)
	_ = tp.ln.Close()
}

// tailHTTPS is where the admin pages are served over HTTPS now ("" if they are not), and why not, if they were asked for.
func (s *Server) tailHTTPS(dnsName string) (addr, problem string) {
	t := &s.tail
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.https != nil && dnsName != "" {
		return "https://" + dnsName + "/", ""
	}
	if s.policy.Settings().TailscaleHTTPS() {
		return "", t.httpsErr
	}
	return "", ""
}

// withTailnetIdentity asks the node who is on the other end of each request that came in over the tailnet and attaches
// the answer to the request, so a viewer or admin can be matched by login or device as well as by address. It is only
// put on the tailnet's listeners, so a request from the LAN can never carry one, and the answer is the node's, not
// anything the client sent. When the node can't say, the request goes on without an identity and is matched by its
// address as before.
func (s *Server) withTailnetIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		who, err := s.tailnet.WhoIs(ctx, r.RemoteAddr)
		cancel()
		if err == nil && (who.Login != "" || who.Node != "") {
			r = r.WithContext(access.WithIdentity(r.Context(), access.Identity{Login: who.Login, Node: who.Node}))
		}
		next.ServeHTTP(w, r)
	})
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
	t.dropHTTPS()
}

// watchTailnet looks after the listeners as the node comes up, goes away or is replaced, and as tuners are added,
// moved and removed. The node starts and stops on its own schedule, so this looks regularly instead of being told.
func (s *Server) watchTailnet(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	last := ""
	for {
		s.syncTailnet()
		// The page shows the node's state and whether the LAN is closed, which change on the node's schedule, so
		// tell the open pages when either does.
		st := tailnetControl{s}.Status()
		if key := fmt.Sprint(st.State, st.LANClosed, st.Serving, st.DNSName); key != last {
			if last != "" {
				s.hub.Notify()
			}
			last = key
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
