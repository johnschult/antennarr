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
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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

// PlexAccount is a Plex account allowed to sign in as an admin. It is
// identified by its account ID, which never changes; the name is only for
// showing in the UI.
type PlexAccount struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// PlexSettings is the optional "Sign in with Plex" for the admin pages.
type PlexSettings struct {
	// Enabled turns it on. Nothing contacts plex.tv while it is off.
	Enabled bool `json:"enabled"`
	// Admins are the Plex accounts that may sign in.
	Admins []PlexAccount `json:"admins"`
	// ClientID identifies this Antennarr to plex.tv. It is generated once and
	// kept; Plex lists the sign-ins under it in the account's authorized devices.
	ClientID string `json:"clientId,omitempty"`
}

// Settings is everything that decides access. With no viewers nobody can use
// the tuners; each viewer gets what SharedCap, OwnerReserve and their own limit
// leave.
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
	// Labels are display names for devices, by device name. The name itself is
	// a stable key (used in limits and URLs); the label is what people see, and
	// what media servers are told the tuner is called.
	Labels map[string]string `json:"labels,omitempty"`
	// Plex configures signing in to the admin pages with a Plex account.
	Plex *PlexSettings `json:"plex,omitempty"`
	// FirmwareCheck turns on looking at SiliconDust's firmware changelog (one
	// request to their website every 12 hours). Unset means on.
	FirmwareCheck *bool `json:"firmwareCheck,omitempty"`
	// Guide turns on reading SiliconDust's program guide, so tiles can show
	// the channel's logo and what is on. It sends the tuner's DeviceAuth to
	// SiliconDust. Unset means on.
	Guide *bool `json:"guide,omitempty"`
	// LogLevel is how much Antennarr logs and shows on the Logs page: debug, info, warn or error. Unset means info.
	LogLevel string `json:"logLevel,omitempty"`
	// History turns on keeping who watched what (for 30 days, beside the settings file), for the Activity
	// page. Unset means on. Nothing leaves this machine either way.
	History *bool `json:"history,omitempty"`
	// Notifications says where to send word of firmware, tuner and viewer events.
	// Nothing is sent, or contacted, while it has no channels.
	Notifications *Notifications `json:"notifications,omitempty"`
}

// PlexEnabled reports whether signing in with Plex is switched on (off by default).
func (s Settings) PlexEnabled() bool { return s.Plex != nil && s.Plex.Enabled }

// PlexAdmin finds an allowed Plex account by ID.
func (s Settings) PlexAdmin(id int64) (PlexAccount, bool) {
	if s.Plex != nil {
		for _, a := range s.Plex.Admins {
			if a.ID == id {
				return a, true
			}
		}
	}
	return PlexAccount{}, false
}

// FirmwareCheckEnabled reports whether the firmware check is on (the default).
func (s Settings) FirmwareCheckEnabled() bool { return s.FirmwareCheck == nil || *s.FirmwareCheck }

// HistoryEnabled reports whether the activity history is being kept (the default).
func (s Settings) HistoryEnabled() bool { return s.History == nil || *s.History }

// LogLevels are the levels LogLevel may be, quietest last.
var LogLevels = []string{"debug", "info", "warn", "error"}

// LogLevelName is the log level in use: what was chosen, or info.
func (s Settings) LogLevelName() string {
	if s.LogLevel == "" {
		return "info"
	}
	return s.LogLevel
}

// GuideEnabled reports whether the program guide is on (the default).
func (s Settings) GuideEnabled() bool { return s.Guide == nil || *s.Guide }

// DefaultSettings is open access with one tuner held back for the owner.
func DefaultSettings() Settings {
	return Settings{OwnerReserve: 1}
}

// Clone copies the settings deeply, so callers can't change ours through the
// slices, maps and pointers inside.
func (s Settings) Clone() Settings {
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
	if s.Labels != nil {
		out.Labels = make(map[string]string, len(s.Labels))
		for k, v := range s.Labels {
			out.Labels[k] = v
		}
	}
	if s.FirmwareCheck != nil {
		b := *s.FirmwareCheck
		out.FirmwareCheck = &b
	}
	if s.Guide != nil {
		b := *s.Guide
		out.Guide = &b
	}
	if s.History != nil {
		b := *s.History
		out.History = &b
	}
	if s.Plex != nil {
		pl := *s.Plex
		pl.Admins = append([]PlexAccount(nil), s.Plex.Admins...)
		out.Plex = &pl
	}
	out.Notifications = s.Notifications.clone()
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

const (
	maxLabels   = 64
	maxLabelLen = 60
)

// ValidateLabel checks a device's display name: some text, not too long, and
// nothing that would break a line or confuse a media server.
func ValidateLabel(l string) error {
	if strings.TrimSpace(l) == "" {
		return errors.New("can't be empty")
	}
	if l != strings.TrimSpace(l) {
		return errors.New("can't start or end with spaces")
	}
	if utf8.RuneCountInString(l) > maxLabelLen {
		return fmt.Errorf("can't be longer than %d characters", maxLabelLen)
	}
	for _, r := range l {
		if unicode.IsControl(r) {
			return errors.New("can't contain control characters")
		}
	}
	return nil
}

const maxPlexAdmins = 50

var clientIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

func (p *PlexSettings) validate() error {
	if p == nil {
		return nil
	}
	if len(p.Admins) > maxPlexAdmins {
		return fmt.Errorf("plex: at most %d accounts", maxPlexAdmins)
	}
	seen := map[int64]bool{}
	for _, a := range p.Admins {
		if a.ID <= 0 {
			return errors.New("plex: an account has no ID")
		}
		if seen[a.ID] {
			return fmt.Errorf("plex: account %d is listed twice", a.ID)
		}
		seen[a.ID] = true
		if len(a.Name) > maxLabelLen || strings.ContainsFunc(a.Name, unicode.IsControl) {
			return fmt.Errorf("plex: bad name for account %d", a.ID)
		}
	}
	if p.ClientID != "" && !clientIDRe.MatchString(p.ClientID) {
		return errors.New("plex: clientId is not valid")
	}
	return nil
}

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
	if s.LogLevel != "" && !slices.Contains(LogLevels, s.LogLevel) {
		return out, fmt.Errorf("logLevel must be one of %s", strings.Join(LogLevels, ", "))
	}
	for _, a := range s.Admin {
		p, err := parsePrefix(a)
		if err != nil {
			return out, fmt.Errorf("admin: %w", err)
		}
		out.admin = append(out.admin, p)
	}
	if err := s.Plex.validate(); err != nil {
		return out, err
	}
	if err := s.Notifications.validate(); err != nil {
		return out, err
	}
	if len(s.Labels) > maxLabels {
		return out, fmt.Errorf("at most %d device names", maxLabels)
	}
	for dev, label := range s.Labels {
		if !deviceName.MatchString(dev) {
			return out, fmt.Errorf("label for %q: not a device name", dev)
		}
		if err := ValidateLabel(label); err != nil {
			return out, fmt.Errorf("name for %q: %w", dev, err)
		}
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

// Warnings lists things in the settings that are allowed but probably not what
// the owner meant. Today that is a viewer address inside an admin range, which
// makes that viewer an admin too: they could open the admin page and change
// their own limits. It is a warning rather than an error so that a settings
// file that already works still loads.
func (s Settings) Warnings() []string {
	c, err := s.parse()
	if err != nil {
		return nil
	}
	return c.warnings()
}

func (c parsed) warnings() []string {
	var out []string
	for _, v := range c.viewers {
		for _, vp := range v.prefixes {
			for _, ap := range c.admin {
				if vp.Overlaps(ap) {
					out = append(out, fmt.Sprintf("viewer %q (%s) is inside the admin range %s, so they can open the admin page and change settings; narrow the admin range", v.Name, vp, ap))
				}
			}
		}
	}
	return out
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
