// Package access decides who may use the tuner and how much of it they may use.
package access

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
)

// Viewer is someone you share the tuner with.
type Viewer struct {
	Name string `json:"name"`
	// Addresses the viewer's media server connects from: single IPs or CIDRs.
	Addresses []string `json:"addresses"`
	// Limit is how many tuners they may use at once. 0 pauses the viewer.
	Limit int `json:"limit"`
}

// Settings is everything that decides access. With no viewers the tuner is
// open to anyone who can reach it, bounded only by SharedCap and OwnerReserve.
type Settings struct {
	Viewers []Viewer `json:"viewers"`
	// SharedCap is the most tuners all viewers together may use. 0 = no cap.
	SharedCap int `json:"sharedCap"`
	// OwnerReserve tuners are always kept free for your own devices.
	OwnerReserve int `json:"ownerReserve"`
	// Admin lists extra addresses (IPs or CIDRs) allowed to see /antennarr/*.
	// Loopback is always allowed once any viewer is configured.
	Admin []string `json:"admin"`
}

// DefaultSettings is open access with one tuner held back for the owner.
func DefaultSettings() Settings {
	return Settings{OwnerReserve: 1}
}

// parsed is Settings with the addresses already parsed.
type parsed struct {
	Settings
	viewers []parsedViewer
	admin   []netip.Prefix
}

type parsedViewer struct {
	Viewer
	prefixes []netip.Prefix
}

func parsePrefix(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("bad CIDR %q", s)
		}
		return netip.PrefixFrom(p.Addr().Unmap(), unmapBits(p)).Masked(), nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("bad address %q", s)
	}
	a = a.Unmap()
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// unmapBits keeps the prefix length meaningful when an IPv4-mapped IPv6
// prefix (::ffff:a.b.c.d/120) is unmapped to plain IPv4.
func unmapBits(p netip.Prefix) int {
	if p.Addr().Is4In6() {
		return p.Bits() - 96
	}
	return p.Bits()
}

// parse checks the settings and returns them with addresses parsed.
func (s Settings) parse() (parsed, error) {
	out := parsed{Settings: s}
	if s.SharedCap < 0 {
		return out, errors.New("sharedCap cannot be negative")
	}
	if s.OwnerReserve < 0 {
		return out, errors.New("ownerReserve cannot be negative")
	}
	for _, a := range s.Admin {
		p, err := parsePrefix(a)
		if err != nil {
			return out, fmt.Errorf("admin: %w", err)
		}
		out.admin = append(out.admin, p)
	}

	names := map[string]bool{}
	owner := map[netip.Prefix]string{}
	for _, v := range s.Viewers {
		name := strings.TrimSpace(v.Name)
		if name == "" {
			return out, errors.New("every viewer needs a name")
		}
		key := strings.ToLower(name)
		if names[key] {
			return out, fmt.Errorf("viewer %q is listed twice", name)
		}
		names[key] = true
		if v.Limit < 0 {
			return out, fmt.Errorf("viewer %q: limit cannot be negative", name)
		}
		if len(v.Addresses) == 0 {
			return out, fmt.Errorf("viewer %q has no addresses", name)
		}
		pv := parsedViewer{Viewer: v}
		pv.Name = name
		for _, a := range v.Addresses {
			p, err := parsePrefix(a)
			if err != nil {
				return out, fmt.Errorf("viewer %q: %w", name, err)
			}
			if other, dup := owner[p]; dup {
				return out, fmt.Errorf("address %s is listed for both %q and %q", p, other, name)
			}
			owner[p] = name
			pv.prefixes = append(pv.prefixes, p)
		}
		out.viewers = append(out.viewers, pv)
	}
	return out, nil
}

// Validate reports the first problem with the settings, if any.
func (s Settings) Validate() error {
	_, err := s.parse()
	return err
}

// LoadSettings reads a settings file. A missing file is not an error: it
// returns the defaults and exists=false.
func LoadSettings(path string) (s Settings, exists bool, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultSettings(), false, nil
	}
	if err != nil {
		return s, false, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields() // a typo'd key should fail loudly, not silently open access
	if err := dec.Decode(&s); err != nil {
		return s, true, fmt.Errorf("%s: %w", path, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return s, true, fmt.Errorf("%s: unexpected data after the settings object", path)
	}
	if err := s.Validate(); err != nil {
		return s, true, fmt.Errorf("%s: %w", path, err)
	}
	return s, true, nil
}

// SaveSettings writes the settings atomically (temp file, then rename).
func SaveSettings(path string, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.Viewers == nil { // write [] rather than null
		s.Viewers = []Viewer{}
	}
	if s.Admin == nil {
		s.Admin = []string{}
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".antennarr-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
