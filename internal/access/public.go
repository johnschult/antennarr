package access

import (
	"fmt"
	"net/netip"
	"strings"
)

// Yagi serves private networks only, so an address on the public internet in a viewer's or an admin's list can never
// connect. PublicEntries finds those, so the page can say so and the API can refuse to add one.

// PublicEntries are the entries of an address list that name public internet addresses: not loopback, private,
// link-local or Tailscale, and not a Tailscale login or device. A range counts when it starts at a public address.
// An entry that can't be read is left out: the settings say what is wrong with it.
func PublicEntries(entries []string) []string {
	var out []string
	for _, e := range entries {
		if _, ok, _ := parseIdentity(e); ok {
			continue
		}
		p, err := parsePrefix(e)
		if err != nil {
			continue
		}
		if !privateAddr(p.Addr().Unmap()) {
			out = append(out, strings.TrimSpace(e))
		}
	}
	return out
}

// publicEntry is whether a parsed prefix starts at a public internet address.
func publicEntry(p netip.Prefix) bool { return !privateAddr(p.Addr().Unmap()) }

func (p parsed) publicWarnings() []string {
	var out []string
	for _, v := range p.viewers {
		for _, pre := range v.prefixes {
			if publicEntry(pre) {
				out = append(out, fmt.Sprintf("viewer %q lists %s, a public internet address. Yagi serves private networks only, so they can't connect: use Tailscale or a VPN, and list the address Yagi sees from there", v.Name, pre))
			}
		}
	}
	for _, pre := range p.admin {
		if publicEntry(pre) {
			out = append(out, fmt.Sprintf("the admin list has %s, a public internet address. Yagi serves private networks only, so it can't open the admin page", pre))
		}
	}
	return out
}
