// Package netinfo says which addresses this machine can be reached at, so the
// setup guide can offer the right one (its Tailscale address, its LAN address)
// instead of making you look it up. It only reads the machine's own network
// interfaces: nothing is contacted and no other program is run.
package netinfo

import (
	"net"
	"net/netip"
	"strings"
)

// Kinds of address worth offering.
const (
	Tailscale = "tailscale"
	LAN       = "lan"
)

// Address is one way to reach this machine.
type Address struct {
	IP        string `json:"ip"`
	Kind      string `json:"kind"`
	Interface string `json:"interface"`
}

var (
	// Tailscale gives every node an address in the carrier-grade NAT range.
	tailnet = netip.MustParsePrefix("100.64.0.0/10")
	private = []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
	}
	// Interfaces that exist only inside this machine (containers, virtual
	// bridges): their addresses mean nothing to anyone else.
	virtual = []string{"docker", "br-", "veth", "virbr", "cni", "flannel", "podman", "cali", "lxc", "vmnet", "vboxnet"}
)

// iface is the part of a network interface that matters here.
type iface struct {
	name  string
	up    bool
	addrs []netip.Addr
}

// Detect lists this machine's Tailscale addresses, then its LAN addresses.
// It returns nothing, rather than an error, when the interfaces can't be read:
// the guide then simply has no suggestions.
func Detect() []Address {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var found []iface
	for _, i := range ifs {
		f := iface{name: i.Name, up: i.Flags&net.FlagUp != 0 && i.Flags&net.FlagLoopback == 0}
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if addr, ok := netip.AddrFromSlice(ip); ok {
				f.addrs = append(f.addrs, addr.Unmap())
			}
		}
		found = append(found, f)
	}
	return classify(found)
}

func isVirtual(name string) bool {
	n := strings.ToLower(name)
	for _, p := range virtual {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

// classify picks the addresses worth offering, Tailscale first.
func classify(ifs []iface) []Address {
	var ts, lan []Address
	seen := map[netip.Addr]bool{}
	for _, f := range ifs {
		if !f.up {
			continue
		}
		for _, a := range f.addrs {
			if !a.Is4() || seen[a] {
				continue
			}
			switch {
			case tailnet.Contains(a):
				seen[a] = true
				ts = append(ts, Address{IP: a.String(), Kind: Tailscale, Interface: f.name})
			case !isVirtual(f.name) && isPrivate(a):
				seen[a] = true
				lan = append(lan, Address{IP: a.String(), Kind: LAN, Interface: f.name})
			}
		}
	}
	return append(ts, lan...)
}

func isPrivate(a netip.Addr) bool {
	for _, p := range private {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
