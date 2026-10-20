package server

import (
	"net"
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
// is dropped as it arrives. Connections already open are not cut: a stream running when the switch goes on ends
// when it ends.
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
		if !g.closed() || isLoopback(c.RemoteAddr()) {
			return c, nil
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
