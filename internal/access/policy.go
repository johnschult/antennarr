package access

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/johnschult/antennarr/internal/proxy"
	"github.com/johnschult/antennarr/internal/tuner"
)

// StateSource reports what a tuner is doing (the tuner.Monitor).
type StateSource interface {
	Snapshot() tuner.State
}

// Notifier receives events worth showing the owner (the tuner.Monitor).
type Notifier interface {
	Note(kind, text string)
}

// Active is one stream flowing through us right now.
type Active struct {
	ID      uint64    `json:"id"`
	Device  string    `json:"device"`
	Viewer  string    `json:"viewer"`
	Client  string    `json:"client"`
	Path    string    `json:"path"`
	Channel string    `json:"channel,omitempty"` // e.g. "5.1", when the path names one
	Started time.Time `json:"started"`
}

// Refusal reasons, sent to clients as X-Antennarr-Reason.
const (
	ReasonNotAllowed   = "not-allowed"
	ReasonViewerLimit  = "viewer-limit"
	ReasonSharedCap    = "shared-cap"
	ReasonOwnerReserve = "owner-reserve"
	ReasonOffline      = "tuner-offline"
)

const noteEvery = time.Minute // at most one event per viewer+device+reason per minute

// Policy holds who may use the tuners and how much, across all devices.
// Viewers and settings are shared; stream counts, limits and the owner
// reserve are tracked per device. Each device gets a DeviceView, which is what
// that device's proxy consults.
type Policy struct {
	log *slog.Logger
	now func() time.Time

	mu       sync.Mutex
	cfg      parsed
	devices  map[string]*deviceState
	next     uint64
	active   map[uint64]*Active
	lastNote map[string]time.Time
	lastSeen map[string]time.Time // by viewer name
	onChange func()
	session  func(*http.Request) bool // an admin signed in some other way (Plex)
}

type deviceState struct {
	mon   StateSource
	notes Notifier
}

func NewPolicy(s Settings, log *slog.Logger) (*Policy, error) {
	cfg, err := s.Clone().parse() // our own copy: the caller may go on changing theirs
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	for _, w := range cfg.warnings() {
		log.Warn(w)
	}
	return &Policy{
		log: log, now: time.Now, cfg: cfg,
		devices: map[string]*deviceState{}, active: map[uint64]*Active{},
		lastNote: map[string]time.Time{}, lastSeen: map[string]time.Time{},
	}, nil
}

// AddDevice registers a device by name, with the source of its tuner status
// and where to report events about it.
func (p *Policy) AddDevice(name string, mon StateSource, notes Notifier) {
	p.mu.Lock()
	p.devices[name] = &deviceState{mon: mon, notes: notes}
	p.mu.Unlock()
}

// Device returns the view of the policy for one device's proxy.
func (p *Policy) Device(name string) *DeviceView { return &DeviceView{p: p, name: name} }

// SetOnChange registers a func called (without locks held) whenever streams
// start or end or the settings change. It must not block.
func (p *Policy) SetOnChange(f func()) {
	p.mu.Lock()
	p.onChange = f
	p.mu.Unlock()
}

func (p *Policy) changed() {
	p.mu.Lock()
	f := p.onChange
	p.mu.Unlock()
	if f != nil {
		f()
	}
}

// Update swaps in new settings. Streams already running are left alone.
func (p *Policy) Update(s Settings) error {
	cfg, err := s.Clone().parse() // our own copy: the caller may go on changing theirs
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.cfg = cfg
	p.mu.Unlock()
	for _, w := range cfg.warnings() {
		p.log.Warn(w)
	}
	p.changed()
	return nil
}

// Settings returns a copy of the current settings.
func (p *Policy) Settings() Settings {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg.Settings.Clone()
}

// Active lists streams in progress on every device, oldest first.
func (p *Policy) Active() []Active {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Active, 0, len(p.active))
	for _, a := range p.active {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Label is the display name set for a device, or "" if it has none.
func (p *Policy) Label(device string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg.Labels[device]
}

// Limits reports the owner reserve and shared cap in force for a device.
func (p *Policy) Limits(device string) (ownerReserve, sharedCap int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg.reserveFor(device), p.cfg.capFor(device)
}

func clientIP(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	// An IPv6 link-local address arrives with the name of the interface it came in on
	// (fe80::1%en0). A network range never "contains" an address with a zone, so drop it.
	return a.Unmap().WithZone(""), true
}

// identify finds the viewer a request belongs to, preferring the most specific
// matching address. p.mu must be held.
func (p *Policy) identify(r *http.Request) (parsedViewer, bool) {
	ip, ok := clientIP(r)
	if !ok {
		return parsedViewer{}, false
	}
	// With no viewers configured nobody matches: the tuner stays closed until
	// the owner adds one.
	best, bestBits, found := parsedViewer{}, -1, false
	for _, v := range p.cfg.viewers {
		for _, pre := range v.prefixes {
			if pre.Contains(ip) && pre.Bits() > bestBits {
				best, bestBits, found = v, pre.Bits(), true
			}
		}
	}
	return best, found
}

// cgnat is 100.64.0.0/10, the range Tailscale hands out.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// privateAddr reports whether ip is on a network that isn't the public
// internet: loopback, private (10/8, 172.16/12, 192.168/16, fc00::/7, which
// includes Tailscale's IPv6 range), link-local, or Tailscale's 100.64.0.0/10.
func privateAddr(ip netip.Addr) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || cgnat.Contains(ip)
}

// ClientAddr is the address a request came from, without any IPv6 zone and
// with IPv4-mapped addresses unmapped. ok is false if it can't be read.
func ClientAddr(r *http.Request) (netip.Addr, bool) { return clientIP(r) }

// PrivateAddr reports whether ip is on a network that isn't the public
// internet: loopback, private, link-local or Tailscale.
func PrivateAddr(ip netip.Addr) bool { return privateAddr(ip) }

// HasAdmin reports whether anyone other than this machine can open the admin
// pages: an address under admin, or an allowed Plex account. While it is false
// the owner has not yet "claimed" the install.
func (p *Policy) HasAdmin() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.cfg.admin) > 0 || (p.cfg.Settings.PlexEnabled() && len(p.cfg.Plex.Admins) > 0)
}

func notAllowed() error {
	return &proxy.Denied{Code: http.StatusForbidden, Reason: ReasonNotAllowed, Text: "this address is not allowed to use this tuner"}
}

// DeviceView is the policy as one device's proxy sees it.
type DeviceView struct {
	p    *Policy
	name string
}

var _ proxy.Policy = (*DeviceView)(nil)

func (d *DeviceView) Authorize(r *http.Request) error {
	p := d.p
	p.mu.Lock()
	v, ok := p.identify(r)
	if ok && len(p.cfg.viewers) > 0 {
		p.lastSeen[v.Name] = p.now()
	}
	p.mu.Unlock()
	if !ok {
		return notAllowed()
	}
	return nil
}

// TunerCount is how many tuners to tell this viewer the device has.
func (d *DeviceView) TunerCount(r *http.Request, real int) int {
	p := d.p
	p.mu.Lock()
	defer p.mu.Unlock()
	n := real - p.cfg.reserveFor(d.name)
	if c := p.cfg.capFor(d.name); c > 0 && c < n {
		n = c
	}
	if v, ok := p.identify(r); ok {
		if l := v.limitFor(d.name); l >= 0 && l < n {
			n = l
		}
	}
	if n < 0 {
		n = 0
	}
	return n
}

func (d *DeviceView) Stream(r *http.Request) (func(), error) {
	p := d.p
	p.mu.Lock()
	ds := p.devices[d.name]
	p.mu.Unlock()

	var st tuner.State
	if ds != nil {
		st = ds.mon.Snapshot()
	}

	p.mu.Lock()
	v, ok := p.identify(r)
	if !ok {
		p.mu.Unlock()
		return nil, notAllowed()
	}

	deny := func(reason, text string) (func(), error) {
		p.mu.Unlock()
		p.note(d.name, v.Name, reason, text)
		return nil, &proxy.Denied{Code: http.StatusServiceUnavailable, Reason: reason, Text: text}
	}

	if ds == nil || !st.Online {
		return deny(ReasonOffline, "the tuner is offline")
	}

	mine, all := 0, 0
	for _, a := range p.active {
		if a.Device != d.name {
			continue
		}
		all++
		if a.Viewer == v.Name {
			mine++
		}
	}
	if l := v.limitFor(d.name); l >= 0 && mine >= l {
		return deny(ReasonViewerLimit, fmt.Sprintf("all %d of your tuners are in use", l))
	}
	if c := p.cfg.capFor(d.name); c > 0 && all >= c {
		return deny(ReasonSharedCap, "all shared tuners are in use")
	}
	// Tuners in use by the owner's own devices come from the tuner's status;
	// ours are counted here, immediately, rather than waiting for the next poll.
	if st.TunerCount > 0 {
		local := 0
		for _, s := range st.Slots {
			if s.InUse && !s.ViaAntennarr {
				local++
			}
		}
		if all+1+local > st.TunerCount-p.cfg.reserveFor(d.name) {
			return deny(ReasonOwnerReserve, "the remaining tuners are reserved for the owner")
		}
	}

	p.next++
	a := &Active{ID: p.next, Device: d.name, Viewer: v.Name, Client: r.RemoteAddr, Path: r.URL.Path, Channel: channelFromPath(r.URL.Path), Started: p.now()}
	p.active[a.ID] = a
	p.mu.Unlock()
	p.changed()

	var once sync.Once
	return func() {
		once.Do(func() {
			p.mu.Lock()
			delete(p.active, a.ID)
			p.mu.Unlock()
			p.changed()
		})
	}, nil
}

// note tells the owner about a refusal, at most once a minute per viewer,
// device and reason, since media servers retry in a tight loop.
func (p *Policy) note(device, viewer, reason, text string) {
	p.mu.Lock()
	ds := p.devices[device]
	var notes Notifier
	if ds != nil {
		notes = ds.notes
	}
	key := device + "|" + viewer + "|" + reason
	if notes == nil || (!p.lastNote[key].IsZero() && p.now().Sub(p.lastNote[key]) < noteEvery) {
		p.mu.Unlock()
		return
	}
	p.lastNote[key] = p.now()
	where := ""
	if len(p.devices) > 1 { // only mention the device when there is more than one
		title := p.cfg.Labels[device]
		if title == "" {
			title = device
		}
		where = " on " + title
	}
	p.mu.Unlock()
	notes.Note("warn", fmt.Sprintf("%s was refused a tuner%s: %s", viewer, where, text))
}

// channelFromPath pulls "5.1" out of /auto/v5.1 or /tuner0/v5.1. Streams
// tuned by frequency (/auto/ch...) have no channel number.
func channelFromPath(path string) string {
	i := strings.LastIndex(path, "/")
	seg := path[i+1:]
	if len(seg) > 1 && seg[0] == 'v' && seg[1] >= '0' && seg[1] <= '9' {
		return seg[1:]
	}
	return ""
}

// ViewerStatus is a configured viewer plus what they're doing right now.
type ViewerStatus struct {
	Viewer
	InUse         int            `json:"inUse"`         // across all devices
	InUseByDevice map[string]int `json:"inUseByDevice"` // only devices they're using
	LastSeen      *time.Time     `json:"lastSeen"`      // nil: never been in contact
}

// ViewerStatuses lists the configured viewers with live information.
func (p *Policy) ViewerStatuses() []ViewerStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ViewerStatus, 0, len(p.cfg.viewers))
	for _, v := range p.cfg.viewers {
		vs := ViewerStatus{Viewer: v.Viewer.clone(), InUseByDevice: map[string]int{}}
		for _, a := range p.active {
			if a.Viewer == v.Name {
				vs.InUse++
				vs.InUseByDevice[a.Device]++
			}
		}
		if t, ok := p.lastSeen[v.Name]; ok {
			t := t
			vs.LastSeen = &t
		}
		out = append(out, vs)
	}
	return out
}

func (v Viewer) clone() Viewer {
	out := v
	out.Addresses = append([]string(nil), v.Addresses...)
	if v.Limits != nil {
		out.Limits = make(map[string]int, len(v.Limits))
		for k, n := range v.Limits {
			out.Limits[k] = n
		}
	}
	return out
}

// Mode is "no-viewers" while no viewers are configured (so nobody can use the
// tuner yet), otherwise "enforced".
func (p *Policy) Mode() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.cfg.viewers) == 0 {
		return "no-viewers"
	}
	return "enforced"
}

// AdminOnly restricts a handler (the /antennarr/* endpoints) to the owner.
// Only loopback, the configured admin addresses and a signed-in Plex admin get
// through.
func (p *Policy) AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !p.IsAdmin(r) {
			http.Error(w, "admin only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SetSessionCheck adds a second way to be an admin, alongside the address:
// a request for which check returns true is an admin too. Used for signing in
// with Plex.
func (p *Policy) SetSessionCheck(check func(*http.Request) bool) {
	p.mu.Lock()
	p.session = check
	p.mu.Unlock()
}

// IsAdmin reports whether the request may use the /antennarr/* admin
// endpoints: from an admin address, or signed in as an admin.
func (p *Policy) IsAdmin(r *http.Request) bool {
	if p.isAdminAddress(r) {
		return true
	}
	p.mu.Lock()
	check := p.session
	p.mu.Unlock()
	return check != nil && check(r) // asked without our lock: it reads the settings
}

// isAdminAddress is the address-based part: loopback, or an admin address.
func (p *Policy) isAdminAddress(r *http.Request) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	ip, ok := clientIP(r)
	if !ok {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, pre := range p.cfg.admin {
		if pre.Contains(ip) {
			return true
		}
	}
	return false
}

// IsAdminAddress reports whether the request comes from an admin address,
// ignoring any sign-in. Plex sign-in uses it to tell the two apart.
func (p *Policy) IsAdminAddress(r *http.Request) bool { return p.isAdminAddress(r) }

// AdminHome sends admins who open the bare address to the admin page at `to`,
// and so is any other browser (a request that accepts HTML): that page is where
// someone not yet allowed in is told so and what to do, which beats a bare
// refusal from the tuner's side. Everything else (media servers, scripts) is
// handed to next, as if this wasn't here.
func (p *Policy) AdminHome(to string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.IsAdmin(r) || strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.Redirect(w, r, to, http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ActiveHandler serves the streams in progress as JSON.
func (p *Policy) ActiveHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(p.Active())
	})
}
