// Package discovery finds HDHomeRun tuners on the network, either with the
// native UDP discovery protocol or by probing HTTP.
package discovery

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"net"
	"net/url"
	"strings"
	"time"
)

// Port is the HDHomeRun control/discovery port (UDP and TCP).
const Port = "65001"

// Device is one tuner found on the network.
type Device struct {
	ID         string // 8 hex digits, upper case
	BaseURL    string // e.g. http://192.168.1.132
	TunerCount int
}

const (
	typeDiscoverReq   = 0x0002
	typeDiscoverReply = 0x0003

	tagDeviceType = 0x01
	tagDeviceID   = 0x02
	tagTunerCount = 0x10
	tagBaseURL    = 0x2A

	deviceTypeTuner = 0x00000001
	wildcard        = 0xFFFFFFFF
)

// frame wraps payload in an HDHomeRun packet: type, length, payload, CRC32
// (IEEE, little-endian) over everything before it.
func frame(typ uint16, payload []byte) []byte {
	b := make([]byte, 4, 4+len(payload)+4)
	binary.BigEndian.PutUint16(b[0:], typ)
	binary.BigEndian.PutUint16(b[2:], uint16(len(payload)))
	b = append(b, payload...)
	return binary.LittleEndian.AppendUint32(b, crc32.ChecksumIEEE(b))
}

func discoverRequest() []byte {
	var p []byte
	for _, tag := range []byte{tagDeviceType, tagDeviceID} {
		p = append(p, tag, 4)
		p = binary.BigEndian.AppendUint32(p, wildcard)
	}
	return frame(typeDiscoverReq, p)
}

// parseReply decodes a discover reply. ok is false for replies that are valid
// but not from a tuner (e.g. a DVR storage engine answering the same broadcast).
func parseReply(b []byte) (d Device, ok bool, err error) {
	if len(b) < 8 {
		return d, false, errors.New("short packet")
	}
	typ := binary.BigEndian.Uint16(b[0:])
	n := int(binary.BigEndian.Uint16(b[2:]))
	if len(b) < 4+n+4 {
		return d, false, errors.New("truncated packet")
	}
	if got, want := binary.LittleEndian.Uint32(b[4+n:]), crc32.ChecksumIEEE(b[:4+n]); got != want {
		return d, false, errors.New("bad CRC")
	}
	if typ != typeDiscoverReply {
		return d, false, fmt.Errorf("unexpected packet type 0x%04x", typ)
	}

	var devType uint32
	p := b[4 : 4+n]
	for len(p) >= 2 {
		tag, l := p[0], int(p[1])
		p = p[2:]
		if l&0x80 != 0 { // two-byte length
			if len(p) < 1 {
				return d, false, errors.New("bad tag length")
			}
			l = l&0x7F | int(p[0])<<7
			p = p[1:]
		}
		if l > len(p) {
			return d, false, errors.New("tag overruns packet")
		}
		v := p[:l]
		p = p[l:]
		switch {
		case tag == tagDeviceType && l == 4:
			devType = binary.BigEndian.Uint32(v)
		case tag == tagDeviceID && l == 4:
			d.ID = fmt.Sprintf("%08X", binary.BigEndian.Uint32(v))
		case tag == tagTunerCount && l == 1:
			d.TunerCount = int(v[0])
		case tag == tagBaseURL:
			d.BaseURL = string(v)
		}
	}
	if d.ID == "" {
		return d, false, errors.New("reply has no device id")
	}
	return d, devType == deviceTypeTuner, nil
}

// NormalizeBase trims a tuner's self-reported base URL to one canonical form
// (no trailing slash, no redundant default port) so the same tuner always
// compares equal to itself. Tuners report e.g. "http://192.168.1.132:80".
func NormalizeBase(s string) string {
	s = strings.TrimRight(s, "/")
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return s
	}
	if (u.Scheme == "http" && u.Port() == "80") || (u.Scheme == "https" && u.Port() == "443") {
		u.Host = u.Hostname()
		if strings.Contains(u.Host, ":") { // IPv6 literal needs its brackets back
			u.Host = "[" + u.Host + "]"
		}
	}
	return u.String()
}

// Query sends a discovery request to each target (host:port) and collects
// tuner replies for up to wait. The request is sent twice, since UDP is lossy.
func Query(ctx context.Context, wait time.Duration, targets []string) ([]Device, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var addrs []*net.UDPAddr
	for _, t := range targets {
		a, err := net.ResolveUDPAddr("udp4", t)
		if err != nil {
			return nil, fmt.Errorf("target %q: %w", t, err)
		}
		addrs = append(addrs, a)
	}

	req := discoverRequest()
	send := func() {
		for _, a := range addrs {
			_, _ = conn.WriteTo(req, a) // unreachable networks just don't answer
		}
	}
	send()

	deadline := time.Now().Add(wait)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	resend := time.Now().Add(wait / 2)
	resent := false

	seen := map[string]bool{}
	var found []Device
	buf := make([]byte, 2048)
	for {
		if ctx.Err() != nil {
			return found, ctx.Err()
		}
		next := deadline
		if !resent && resend.Before(next) {
			next = resend
		}
		_ = conn.SetReadDeadline(next)
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			var ne net.Error
			if !errors.As(err, &ne) || !ne.Timeout() {
				return found, err
			}
			if time.Now().After(deadline.Add(-time.Millisecond)) {
				return found, nil
			}
			if !resent {
				resent = true
				send()
			}
			continue
		}
		d, isTuner, err := parseReply(buf[:n])
		if err != nil || !isTuner || seen[d.ID] {
			continue
		}
		seen[d.ID] = true
		if d.BaseURL == "" {
			if ua, ok := from.(*net.UDPAddr); ok {
				d.BaseURL = "http://" + ua.IP.String()
			}
		}
		d.BaseURL = NormalizeBase(d.BaseURL)
		found = append(found, d)
	}
}

// Discover broadcasts on every active IPv4 interface (and the limited
// broadcast address), plus unicasts to any extra hosts, and returns the tuners
// that answered.
func Discover(ctx context.Context, wait time.Duration, extraHosts ...string) ([]Device, error) {
	targets := []string{"255.255.255.255:" + Port}
	targets = append(targets, directedBroadcasts()...)
	for _, h := range extraHosts {
		if !strings.Contains(h, ":") {
			h += ":" + Port
		}
		targets = append(targets, h)
	}
	return Query(ctx, wait, dedupe(targets))
}

func directedBroadcasts() []string {
	var out []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 || ifc.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipn.IP.To4()
			if ip4 == nil || len(ipn.Mask) != net.IPv4len {
				continue
			}
			bc := make(net.IP, 4)
			for i := range bc {
				bc[i] = ip4[i] | ^ipn.Mask[i]
			}
			out = append(out, net.JoinHostPort(bc.String(), Port))
		}
	}
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
