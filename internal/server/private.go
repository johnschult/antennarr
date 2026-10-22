package server

import (
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/johnschult/yagi/internal/access"
)

// Yagi serves private networks only: this machine, its own network, a VPN, Tailscale. A connection from a public
// internet address is dropped as it arrives, before a byte of HTTP is read, so a port that was forwarded by mistake
// shows scanners nothing. There is no switch to turn it off: sharing a tuner is for people you trust enough to put on
// your network, and the way to reach someone elsewhere is a VPN or Tailscale.

// localNets are the networks this machine is directly connected to. A source in one of them is on our own network
// even when its address looks public, which is how a LAN that hands out global IPv6 addresses looks.
type localNets struct {
	read func() []netip.Prefix // reads them; a test supplies its own

	mu   sync.Mutex
	at   time.Time
	nets []netip.Prefix
}

// localNetsTTL is how long what was read from the interfaces is believed: addresses change rarely, and every
// connection from a public-looking address asks.
const localNetsTTL = 30 * time.Second

func (l *localNets) contains(ip netip.Addr) bool {
	l.mu.Lock()
	if l.nets == nil || time.Since(l.at) > localNetsTTL {
		l.nets, l.at = l.read(), time.Now()
		if l.nets == nil {
			l.nets = []netip.Prefix{} // nothing to find, and not asked again for a while
		}
	}
	nets := l.nets
	l.mu.Unlock()
	for _, p := range nets {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// interfaceNets reads the networks of this machine's interfaces.
func interfaceNets() []netip.Prefix {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []netip.Prefix
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip, ok := netip.AddrFromSlice(n.IP)
		if !ok {
			continue
		}
		ones, _ := n.Mask.Size()
		ip = ip.Unmap()
		if n.IP.To4() != nil && ones >= 96 { // a mask written as 16 bytes for an IPv4 address
			ones -= 96
		}
		if p, err := ip.Prefix(ones); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// privateSource is whether a connection from a is one Yagi serves: loopback, a private or link-local address, a
// Tailscale address, or one on a network this machine is directly connected to.
func (s *Server) privateSource(a net.Addr) bool {
	ta, ok := a.(*net.TCPAddr)
	if !ok {
		return true // not an internet connection at all (a test's pipe, say)
	}
	ip, ok := netip.AddrFromSlice(ta.IP)
	if !ok {
		return false
	}
	ip = ip.Unmap().WithZone("")
	return access.PrivateAddr(ip) || s.nets.contains(ip)
}
