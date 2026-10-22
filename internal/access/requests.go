package access

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"time"
)

// A request is someone, who is not a viewer yet, asking to use a tuner. Yagi serves private networks only, so anyone
// who gets this far is on the owner's network, a VPN or the tailnet: a friend who has just added Yagi in their media
// server. The owner sees them, and presses Allow. The list is in memory and short: it is a place to pick people from,
// not a record.

const (
	maxRequests  = 50  // the most that are kept; the one asked about longest ago goes first
	maxDismissed = 200 // the most dismissed that are remembered (until Yagi restarts)
)

// Request is one person or device that asked to use a tuner and isn't a viewer.
type Request struct {
	// ID names the request in the API. It is made from who it is, so it is the same each time they ask.
	ID string `json:"id"`
	// Address is the address the requests came from.
	Address string `json:"address"`
	// Login and Node are who the tailnet says it is, when the request came over Yagi's own Tailscale node.
	Login string `json:"login,omitempty"`
	Node  string `json:"node,omitempty"`
	// Device is the tuner they asked for, the last time.
	Device string    `json:"device"`
	First  time.Time `json:"first"`
	Last   time.Time `json:"last"`
	// Count is how many times they have asked.
	Count int `json:"count"`

	key string
}

// Label is what to call the requester: their login if the tailnet knows it, else their device, else their address.
func (r Request) Label() string {
	switch {
	case r.Login != "":
		return r.Login
	case r.Node != "":
		return r.Node
	}
	return r.Address
}

// Entry is what to put in a viewer's addresses so that this requester, and only they, are let in: their Tailscale
// login or device when the tailnet named one, else their address.
func (r Request) Entry() string {
	switch {
	case r.Login != "":
		return IdentityPrefix + r.Login
	case r.Node != "":
		return IdentityPrefix + r.Node
	}
	return r.Address
}

func requestKey(ip netip.Addr, id Identity, has bool) string {
	switch {
	case has && id.Login != "":
		return "login:" + id.Login
	case has && id.Node != "":
		return "node:" + id.Node
	}
	return "ip:" + ip.String()
}

func requestID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:6])
}

// noteRequest remembers that a request from someone who isn't a viewer came in. isNew is whether this is the first
// time Yagi has seen them (and they haven't been dismissed), the moment to tell the owner.
func (p *Policy) noteRequest(device string, r *http.Request) (req Request, isNew bool) {
	ip, ok := clientIP(r)
	if !ok {
		return Request{}, false
	}
	id, has := IdentityOf(r)
	key := requestKey(ip, id, has)
	now := p.now()

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dismissed[key] {
		return Request{}, false
	}
	if p.requests == nil {
		p.requests = map[string]*Request{}
	}
	cur, seen := p.requests[key]
	if !seen {
		cur = &Request{ID: requestID(key), Address: ip.String(), First: now, key: key}
		if has {
			cur.Login, cur.Node = id.Login, id.Node
		}
		if len(p.requests) >= maxRequests {
			var oldest string
			for k, v := range p.requests {
				if oldest == "" || v.Last.Before(p.requests[oldest].Last) {
					oldest = k
				}
			}
			delete(p.requests, oldest)
		}
		p.requests[key] = cur
	}
	cur.Last, cur.Device, cur.Address = now, device, ip.String()
	cur.Count++
	return *cur, !seen
}

// matches reports whether a request now belongs to a viewer, so it needs no allowing. p.mu must be held.
func (p *Policy) requestMatches(r *Request) bool {
	for _, v := range p.cfg.viewers {
		for _, e := range v.idents {
			if (r.Login != "" && Identity{Login: r.Login}.matches(e)) || (r.Node != "" && Identity{Node: r.Node}.matches(e)) {
				return true
			}
		}
	}
	if ip, err := netip.ParseAddr(r.Address); err == nil && r.Login == "" && r.Node == "" {
		for _, v := range p.cfg.viewers {
			for _, pre := range v.prefixes {
				if pre.Contains(ip) {
					return true
				}
			}
		}
	}
	return false
}

// Requests lists who has asked to use a tuner and isn't a viewer, the latest first. Someone who has been made a
// viewer since, or dismissed, is not in it.
func (p *Policy) Requests() []Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Request, 0, len(p.requests))
	for _, r := range p.requests {
		if !p.requestMatches(r) {
			out = append(out, *r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Last.After(out[j].Last) })
	return out
}

// RequestByID finds a request by the ID the API gave it.
func (p *Policy) RequestByID(id string) (Request, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.requests {
		if r.ID == id && !p.requestMatches(r) {
			return *r, true
		}
	}
	return Request{}, false
}

// DismissRequest drops a request and keeps quiet about that requester until Yagi restarts. It says whether there was one.
func (p *Policy) DismissRequest(id string) bool {
	p.mu.Lock()
	var found bool
	for k, r := range p.requests {
		if r.ID == id {
			delete(p.requests, k)
			if p.dismissed == nil {
				p.dismissed = map[string]bool{}
			}
			if len(p.dismissed) >= maxDismissed {
				p.dismissed = map[string]bool{}
			}
			p.dismissed[k] = true
			found = true
			break
		}
	}
	p.mu.Unlock()
	if found {
		p.changed()
	}
	return found
}

// requestWho is how an event names a requester.
func requestWho(r Request) string { return strings.TrimSpace(r.Label()) }
