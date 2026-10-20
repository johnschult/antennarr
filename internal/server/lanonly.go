package server

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/johnschult/yagi/internal/tailnet"
)

// lanClosed says whether connections from the LAN are to be refused now. That is when "tailnet only" is on and the node
// is connected: while it isn't, the ports stay open, so a tailnet that is down or signed out never leaves the
// admin page unreachable. -allow-lan (YAGI_ALLOW_LAN) keeps them open whatever the setting says.
func (s *Server) lanClosed() bool {
	if s.cfg.AllowLAN || !s.policy.Settings().TailnetOnly() {
		return false
	}
	return s.tailnet.Status().State == tailnet.Running
}

// gate wraps a LAN listener so that, while the LAN is closed, only the machine itself gets through. The port is still
// bound (so a tuner keeps its port and switching the setting off is at once), but a connection from anywhere else
// is dropped as it arrives. Connections from the LAN that are already open when the LAN closes are cut too (see
// enforceLAN), so "only on the tailnet" holds from the moment it goes on.
func (s *Server) gate(ln net.Listener) net.Listener {
	return &gatedListener{Listener: ln, closed: s.lanClosed, log: s}
}

type gatedListener struct {
	net.Listener
	closed func() bool
	log    *Server
	lastAt atomic.Int64 // when a refusal was last logged (unix seconds), so a busy port isn't loud
}

func (g *gatedListener) Accept() (net.Conn, error) {
	for {
		c, err := g.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if isLoopback(c.RemoteAddr()) {
			return c, nil
		}
		if !g.closed() {
			return g.log.lan.track(c), nil // remembered, so it can be cut if the LAN closes while it is open
		}
		_ = c.Close()
		now := time.Now().Unix()
		if last := g.lastAt.Load(); now-last >= 60 && g.lastAt.CompareAndSwap(last, now) {
			g.log.log.Info("tailnet only: refused a connection from the LAN", "from", c.RemoteAddr().String(), "port", g.Addr().String())
		}
	}
}

func isLoopback(a net.Addr) bool {
	ta, ok := a.(*net.TCPAddr)
	return ok && ta.IP.IsLoopback()
}

// lanConns are the connections from outside the machine that are open, so they can be cut when the LAN closes.
type lanConns struct {
	mu sync.Mutex
	m  map[*lanConn]struct{}
}

type lanConn struct {
	net.Conn
	set  *lanConns
	once sync.Once
}

func (l *lanConns) track(c net.Conn) net.Conn {
	lc := &lanConn{Conn: c, set: l}
	l.mu.Lock()
	if l.m == nil {
		l.m = map[*lanConn]struct{}{}
	}
	l.m[lc] = struct{}{}
	l.mu.Unlock()
	return lc
}

func (c *lanConn) Close() error {
	c.once.Do(func() {
		c.set.mu.Lock()
		delete(c.set.m, c)
		c.set.mu.Unlock()
	})
	return c.Conn.Close()
}

// closeAll cuts every tracked connection and says how many there were.
func (l *lanConns) closeAll() int {
	l.mu.Lock()
	open := make([]*lanConn, 0, len(l.m))
	for c := range l.m {
		open = append(open, c)
	}
	l.mu.Unlock()
	for _, c := range open {
		_ = c.Close()
	}
	return len(open)
}

// enforceLAN cuts the connections already open from the LAN when it closes: a browser's kept-alive connection, a
// stream in progress. It acts once, at the moment the LAN goes from open to closed.
func (s *Server) enforceLAN() {
	if !s.lanClosed() {
		s.lanWasClosed.Store(false)
		return
	}
	if s.lanWasClosed.Swap(true) {
		return
	}
	if n := s.lan.closeAll(); n > 0 {
		s.log.Info("tailnet only: closed the connections already open from the LAN", "connections", n)
	}
}
