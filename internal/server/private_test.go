package server

import (
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/access"
)

func tcp(ip string) net.Addr { return &net.TCPAddr{IP: net.ParseIP(ip), Port: 40000} }

func serverWithNets(nets ...string) *Server {
	var ps []netip.Prefix
	for _, n := range nets {
		ps = append(ps, netip.MustParsePrefix(n))
	}
	return &Server{nets: localNets{read: func() []netip.Prefix { return ps }}}
}

func TestOnlyPrivateSourcesAreServed(t *testing.T) {
	s := serverWithNets("192.168.1.5/24", "2001:db8:1::10/64")
	for name, c := range map[string]struct {
		ip   string
		want bool
	}{
		"loopback":                       {"127.0.0.1", true},
		"loopback v6":                    {"::1", true},
		"10/8":                           {"10.1.2.3", true},
		"172.16/12":                      {"172.20.0.9", true},
		"192.168/16":                     {"192.168.50.2", true},
		"link-local v4":                  {"169.254.3.4", true},
		"link-local v6":                  {"fe80::1", true},
		"unique-local v6":                {"fd12:3456::9", true},
		"a tailnet address":              {"100.64.0.7", true},
		"the top of the tailnet range":   {"100.127.255.254", true},
		"the tailnet's own v6":           {"fd7a:115c:a1e0::5", true},
		"v4-mapped private":              {"::ffff:10.0.0.1", true},
		"a public v4 address":            {"203.0.113.9", false},
		"another":                        {"8.8.8.8", false},
		"just outside the tailnet range": {"100.128.0.1", false},
		"just outside 172.16/12":         {"172.32.0.1", false},
		"a public v6 address":            {"2001:db8::1", false},
		"a public v6 address elsewhere":  {"2a00:1450::5", false},
		"v4-mapped public":               {"::ffff:8.8.8.8", false},
		"on-link public v6 (our LAN)":    {"2001:db8:1::99", true},
		"the next v6 network over":       {"2001:db8:2::99", false},
	} {
		if got := s.privateSource(tcp(c.ip)); got != c.want {
			t.Errorf("%s (%s): %v, want %v", name, c.ip, got, c.want)
		}
	}
	// An address that isn't a TCP one at all (a test's pipe) is not an internet connection.
	if !s.privateSource(&net.UnixAddr{Name: "x", Net: "unix"}) {
		t.Error("a unix socket was refused")
	}
}

func TestLocalNetworksAreReadOnceInAWhile(t *testing.T) {
	reads := 0
	s := &Server{nets: localNets{read: func() []netip.Prefix { reads++; return []netip.Prefix{netip.MustParsePrefix("2001:db8:1::/64")} }}}
	for i := 0; i < 50; i++ {
		s.privateSource(tcp("2001:db8:1::5"))
		s.privateSource(tcp("203.0.113.9"))
	}
	if reads != 1 {
		t.Errorf("the interfaces were read %d times for 100 connections", reads)
	}
	// And when there is nothing to find, that is not asked about again at once either.
	reads = 0
	s = &Server{nets: localNets{read: func() []netip.Prefix { reads++; return nil }}}
	for i := 0; i < 10; i++ {
		s.privateSource(tcp("203.0.113.9"))
	}
	if reads != 1 {
		t.Errorf("with no networks, the interfaces were read %d times", reads)
	}
	s.nets.at = time.Now().Add(-2 * localNetsTTL)
	s.privateSource(tcp("203.0.113.9"))
	if reads != 2 {
		t.Errorf("after the time is up they were read %d times, want 2", reads)
	}
}

func TestTheRealInterfacesAreReadWithoutTrouble(t *testing.T) {
	nets := interfaceNets()
	loop := false
	for _, n := range nets {
		if !n.IsValid() || n.Bits() < 0 {
			t.Errorf("a bad network: %v", n)
		}
		loop = loop || n.Contains(netip.MustParseAddr("127.0.0.1"))
	}
	if !loop {
		t.Errorf("the loopback network is missing from %v", nets)
	}
}

// sourceListener hands out connections that claim to come from the given addresses.
type sourceListener struct {
	net.Listener
	from chan string
}

type sourceConn struct {
	net.Conn
	from string
	shut chan struct{}
}

func (c *sourceConn) RemoteAddr() net.Addr { return tcp(c.from) }
func (c *sourceConn) Close() error         { close(c.shut); return nil }

func (l *sourceListener) Accept() (net.Conn, error) {
	from, ok := <-l.from
	if !ok {
		return nil, net.ErrClosed
	}
	a, _ := net.Pipe()
	return &sourceConn{Conn: a, from: from, shut: make(chan struct{})}, nil
}

func TestThePortDropsPublicConnectionsAndLogsOnceAMinute(t *testing.T) {
	logs := &lockedBuf{}
	srv := serverWithNets("192.168.1.5/24")
	srv.log = slog.New(slog.NewTextHandler(logs, nil))
	srv.cfg.AllowLAN = true // so the tailnet-only check, which isn't what is tested here, is out of the way
	from := make(chan string, 10)
	inner := &sourceListener{Listener: ephemeral(t), from: from}
	g := srv.gate(inner)

	for _, ip := range []string{"203.0.113.9", "198.51.100.4", "8.8.8.8"} {
		from <- ip
	}
	from <- "192.168.1.77" // one of ours, after the three public ones
	c, err := g.Accept()
	if err != nil {
		t.Fatal(err)
	}
	if got := c.RemoteAddr().String(); !strings.HasPrefix(got, "192.168.1.77") {
		t.Errorf("the connection that got through is from %s, want the private one", got)
	}
	if n := strings.Count(logs.String(), "refused a connection from a public internet address"); n != 1 {
		t.Errorf("%d log lines for three refusals in a second, want one:\n%s", n, logs.String())
	}
	if !strings.Contains(logs.String(), "private networks only") {
		t.Errorf("the log doesn't say why:\n%s", logs.String())
	}
}

func TestAViewerWithAPublicAddressIsWarnedAboutAtStart(t *testing.T) {
	s := loopbackViewer(access.DefaultSettings())
	s.Viewers = append(s.Viewers, access.Viewer{Name: "Dad", Addresses: []string{"203.0.113.9"}, Limit: 1})
	r := start(t, s, nil)
	if n := strings.Count(r.logs.String(), "a public internet address. Yagi serves private networks only"); n != 1 {
		t.Errorf("%d warnings, want one:\n%s", n, r.logs.String())
	}
	if !strings.Contains(r.logs.String(), `viewer \"Dad\" lists 203.0.113.9/32, a public internet address`) {
		t.Errorf("no warning in the log:\n%s", r.logs.String())
	}
}
