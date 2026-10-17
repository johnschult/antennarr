// Package access decides who may use the tuners and how much of them they may use.
package access

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Device is one HDHomeRun that Antennarr fronts. Each device looks like its
// own HDHomeRun to media servers, on its own port.
type Device struct {
	// Name identifies the device in settings, limits and the UI. Letters,
	// digits, '.', '_' and '-' only, since it appears in URLs.
	Name string `json:"name"`
	// Listen is where media servers reach this device, e.g. ":5004".
	Listen string `json:"listen"`
	// Tuner is the address to look for the tuner at first (host or URL).
	Tuner string `json:"tuner,omitempty"`
	// ID is the DeviceID to follow if the tuner's address changes. If empty it
	// is learned from the tuner at Tuner.
	ID string `json:"id,omitempty"`
	// StreamPort is the tuner's streaming port; default 5004.
	StreamPort string `json:"streamPort,omitempty"`
	// Advertise overrides the address handed to media servers.
	Advertise string `json:"advertise,omitempty"`
	// ReportID is a DeviceID to show media servers instead of the tuner's own,
	// for when a media server already knows the real tuner.
	ReportID string `json:"reportId,omitempty"`
	// OwnerReserve and SharedCap override the top-level values for this device.
	OwnerReserve *int `json:"ownerReserve,omitempty"`
	SharedCap    *int `json:"sharedCap,omitempty"`
}

// Viewer is someone you share the tuners with.
type Viewer struct {
	Name string `json:"name"`
	// Addresses the viewer's media server connects from: single IPs or CIDRs.
	Addresses []string `json:"addresses"`
	// Limit is how many tuners they may use at once on a device. 0 pauses them.
	Limit int `json:"limit"`
	// Limits overrides Limit for particular devices, by device name.
	Limits map[string]int `json:"limits,omitempty"`
}

// limitFor is the viewer's limit on the named device.
func (v Viewer) limitFor(device string) int {
	if n, ok := v.Limits[device]; ok {
		return n
	}
	return v.Limit
}

// Settings is everything that decides access. With no viewers the tuners are
// open to anyone who can reach them, bounded only by SharedCap and OwnerReserve.
type Settings struct {
	// Devices to front. If empty, Antennarr is started with one device from
	// its command-line flags.
	Devices []Device `json:"devices,omitempty"`
	Viewers []Viewer `json:"viewers"`
	// SharedCap is the most tuners all viewers together may use on a device.
	// 0 = no cap. Devices can override it.
	SharedCap int `json:"sharedCap"`
	// OwnerReserve tuners are always kept free for your own devices. Devices
	// can override it.
	OwnerReserve int `json:"ownerReserve"`
	// Admin lists extra addresses (IPs or CIDRs) allowed to see /antennarr/*.
	// Loopback is always allowed once any viewer is configured.
	Admin []string `json:"admin"`
}

// DefaultSettings is open access with one tuner held back for the owner.
func DefaultSettings() Settings {
	return Settings{OwnerReserve: 1}
}

// clone copies the settings deeply, so callers can't change ours through the
// slices, maps and pointers inside.
func (s Settings) clone() Settings {
	out := s
	out.Devices = make([]Device, len(s.Devices))
	for i, d := range s.Devices {
		out.Devices[i] = d
		out.Devices[i].OwnerReserve = cloneInt(d.OwnerReserve)
		out.Devices[i].SharedCap = cloneInt(d.SharedCap)
	}
	if len(s.Devices) == 0 {
		out.Devices = nil
	}
	out.Viewers = make([]Viewer, len(s.Viewers))
	for i, v := range s.Viewers {
		out.Viewers[i] = v
		out.Viewers[i].Addresses = append([]string(nil), v.Addresses...)
		if v.Limits != nil {
			out.Viewers[i].Limits = make(map[string]int, len(v.Limits))
			for k, n := range v.Limits {
				out.Viewers[i].Limits[k] = n
			}
		}
	}
	out.Admin = append([]string(nil), s.Admin...)
	return out
}

func cloneInt(p *int) *int {
	if p == nil {
		return nil
	}
	n := *p
	return &n
}

// parsed is Settings with the addresses already parsed.
type parsed struct {
	Settings
	viewers []parsedViewer
	admin   []netip.Prefix
	devices map[string]Device
}

type parsedViewer struct {
	Viewer
	prefixes []netip.Prefix
}

// reserveFor and capFor give the values in force for a device: its own
// override if it has one, else the top-level default.
func (p *parsed) reserveFor(device string) int {
	if d, ok := p.devices[device]; ok && d.OwnerReserve != nil {
		return *d.OwnerReserve
	}
	return p.OwnerReserve
}

func (p *parsed) capFor(device string) int {
	if d, ok := p.devices[device]; ok && d.SharedCap != nil {
		return *d.SharedCap
	}
	return p.SharedCap
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

var (
	deviceName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,40}$`)
	deviceID   = regexp.MustCompile(`^[0-9A-Fa-f]{8}$`)
)

func validPort(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n >= 1 && n <= 65535
}

// Validate reports the first problem with the device's own settings.
func (d Device) Validate() error {
	if !deviceName.MatchString(d.Name) {
		return fmt.Errorf("device name %q: use 1-40 letters, digits, '.', '_' or '-'", d.Name)
	}
	_, port, err := net.SplitHostPort(d.Listen)
	if err != nil || !validPort(port) {
		return fmt.Errorf("device %q: listen %q must look like \":5004\" or \"127.0.0.1:5004\"", d.Name, d.Listen)
	}
	if strings.TrimSpace(d.Tuner) == "" && d.ID == "" {
		return fmt.Errorf("device %q needs a tuner address, an id, or both", d.Name)
	}
	if d.ID != "" && !deviceID.MatchString(d.ID) {
		return fmt.Errorf("device %q: id %q is not 8 hex digits", d.Name, d.ID)
	}
	if d.ReportID != "" && !deviceID.MatchString(d.ReportID) {
		return fmt.Errorf("device %q: reportId %q is not 8 hex digits", d.Name, d.ReportID)
	}
	if d.StreamPort != "" && !validPort(d.StreamPort) {
		return fmt.Errorf("device %q: bad streamPort %q", d.Name, d.StreamPort)
	}
	if d.OwnerReserve != nil && *d.OwnerReserve < 0 {
		return fmt.Errorf("device %q: ownerReserve cannot be negative", d.Name)
	}
	if d.SharedCap != nil && *d.SharedCap < 0 {
		return fmt.Errorf("device %q: sharedCap cannot be negative", d.Name)
	}
	return nil
}

// parse checks the settings and returns them with addresses parsed.
func (s Settings) parse() (parsed, error) {
	out := parsed{Settings: s, devices: map[string]Device{}}
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

	ports := map[string]string{}
	lowered := map[string]bool{}
	for _, d := range s.Devices {
		if err := d.Validate(); err != nil {
			return out, err
		}
		if lowered[strings.ToLower(d.Name)] { // "Antenna" and "antenna" would be confusing
			return out, fmt.Errorf("device %q is listed twice", d.Name)
		}
		lowered[strings.ToLower(d.Name)] = true
		out.devices[d.Name] = d
		_, port, _ := net.SplitHostPort(d.Listen)
		if other, taken := ports[port]; taken {
			return out, fmt.Errorf("devices %q and %q both listen on port %s", other, d.Name, port)
		}
		ports[port] = d.Name
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
		for dev, n := range v.Limits {
			if len(s.Devices) == 0 {
				return out, fmt.Errorf("viewer %q: limits for devices need devices declared in the settings file", name)
			}
			if _, ok := out.devices[dev]; !ok {
				return out, fmt.Errorf("viewer %q: limit for unknown device %q", name, dev)
			}
			if n < 0 {
				return out, fmt.Errorf("viewer %q: limit for %q cannot be negative", name, dev)
			}
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
