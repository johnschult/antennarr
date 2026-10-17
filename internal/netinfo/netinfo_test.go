package netinfo

import (
	"net/netip"
	"testing"
)

func ips(s ...string) []netip.Addr {
	var out []netip.Addr
	for _, v := range s {
		out = append(out, netip.MustParseAddr(v))
	}
	return out
}

func TestTailscaleComesFirstThenLAN(t *testing.T) {
	got := classify([]iface{
		{name: "en0", up: true, addrs: ips("192.168.68.72", "fe80::1")},
		{name: "utun4", up: true, addrs: ips("100.127.237.109")},
	})
	want := []Address{
		{IP: "100.127.237.109", Kind: Tailscale, Interface: "utun4"},
		{IP: "192.168.68.72", Kind: LAN, Interface: "en0"},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestOnlyTheTailnetRangeCountsAsTailscale(t *testing.T) {
	got := classify([]iface{{name: "tailscale0", up: true, addrs: ips("100.63.255.255", "100.64.0.0", "100.127.255.255", "100.128.0.0")}})
	if len(got) != 2 || got[0].IP != "100.64.0.0" || got[1].IP != "100.127.255.255" {
		t.Errorf("got %+v", got)
	}
}

func TestOnlyPrivateAddressesAreLAN(t *testing.T) {
	got := classify([]iface{{name: "eth0", up: true, addrs: ips("10.1.2.3", "172.16.0.9", "172.32.0.1", "192.168.1.5", "8.8.8.8", "169.254.1.1")}})
	var lan []string
	for _, a := range got {
		lan = append(lan, a.IP)
	}
	if len(lan) != 3 || lan[0] != "10.1.2.3" || lan[1] != "172.16.0.9" || lan[2] != "192.168.1.5" {
		t.Errorf("got %v (public, link-local and 172.32/12 must be left out)", lan)
	}
}

func TestInterfacesThatAreDownOrVirtualAreLeftOut(t *testing.T) {
	got := classify([]iface{
		{name: "eth0", up: false, addrs: ips("192.168.1.5")},
		{name: "docker0", up: true, addrs: ips("172.17.0.1")},
		{name: "br-1a2b3c", up: true, addrs: ips("172.18.0.1")},
		{name: "veth123", up: true, addrs: ips("172.19.0.1")},
		{name: "virbr0", up: true, addrs: ips("192.168.122.1")},
		{name: "eth1", up: true, addrs: ips("192.168.1.6")},
	})
	if len(got) != 1 || got[0].IP != "192.168.1.6" {
		t.Errorf("got %+v", got)
	}
}

func TestAddressesAreNotRepeated(t *testing.T) {
	got := classify([]iface{
		{name: "en0", up: true, addrs: ips("192.168.1.5")},
		{name: "en1", up: true, addrs: ips("192.168.1.5")},
	})
	if len(got) != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestTailscaleIsKeptEvenOnAnOddlyNamedInterface(t *testing.T) {
	// The address range is what identifies Tailscale, not the interface name.
	got := classify([]iface{{name: "docker-weird", up: true, addrs: ips("100.100.1.1")}})
	if len(got) != 1 || got[0].Kind != Tailscale {
		t.Errorf("got %+v", got)
	}
}

func TestNothingFoundIsAnEmptyList(t *testing.T) {
	if got := classify(nil); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestDetectRunsAgainstThisMachine(t *testing.T) {
	for _, a := range Detect() { // whatever is here, it must be well formed
		if _, err := netip.ParseAddr(a.IP); err != nil || (a.Kind != Tailscale && a.Kind != LAN) || a.Interface == "" {
			t.Errorf("bad address %+v", a)
		}
	}
}
