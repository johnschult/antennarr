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

// StateSource reports what the tuner is doing (the tuner.Monitor).
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

const noteEvery = time.Minute // at most one event per viewer+reason per minute

// Policy implements proxy.Policy.
type Policy struct {
	mon   StateSource
	notes Notifier
	log   *slog.Logger
	now   func() time.Time

	mu       sync.Mutex
	cfg      parsed
	next     uint64
	active   map[uint64]*Active
	lastNote map[string]time.Time
	lastSeen map[string]time.Time // by viewer name
	onChange func()
}

// openViewer stands in for everyone while no viewers are configured.
var openViewer = parsedViewer{Viewer: Viewer{Name: "anyone", Limit: -1}}

var _ proxy.Policy = (*Policy)(nil)

func NewPolicy(s Settings, mon StateSource, notes Notifier, log *slog.Logger) (*Policy, error) {
	cfg, err := s.parse()
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	return &Policy{
		mon: mon, notes: notes, log: log, now: time.Now, cfg: cfg,
		active: map[uint64]*Active{}, lastNote: map[string]time.Time{}, lastSeen: map[string]time.Time{},
	}, nil
}

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
	cfg, err := s.parse()
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.cfg = cfg
	p.mu.Unlock()
	p.changed()
	return nil
}

// Settings returns a copy of the current settings.
func (p *Policy) Settings() Settings {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.cfg.Settings
	s.Viewers = append([]Viewer(nil), s.Viewers...)
	s.Admin = append([]string(nil), s.Admin...)
	return s
}

// Active lists streams in progress, oldest first.
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

func clientIP(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}

// identify finds the viewer a request belongs to, preferring the most specific
// matching address. p.mu must be held.
func (p *Policy) identify(r *http.Request) (parsedViewer, bool) {
	if len(p.cfg.viewers) == 0 {
		return openViewer, true
	}
	ip, ok := clientIP(r)
	if !ok {
		return parsedViewer{}, false
	}
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

func (p *Policy) Authorize(r *http.Request) error {
	p.mu.Lock()
	v, ok := p.identify(r)
	if ok && len(p.cfg.viewers) > 0 {
		p.lastSeen[v.Name] = p.now()
	}
	p.mu.Unlock()
	if !ok {
		return &proxy.Denied{Code: http.StatusForbidden, Reason: ReasonNotAllowed, Text: "this address is not allowed to use this tuner"}
	}
	return nil
}

func (p *Policy) TunerCount(r *http.Request, real int) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := real - p.cfg.OwnerReserve
	if c := p.cfg.SharedCap; c > 0 && c < n {
		n = c
	}
	if v, ok := p.identify(r); ok && v.Limit >= 0 && v.Limit < n {
		n = v.Limit
	}
	if n < 0 {
		n = 0
	}
	return n
}

func (p *Policy) Stream(r *http.Request) (func(), error) {
	st := p.mon.Snapshot()

	p.mu.Lock()
	v, ok := p.identify(r)
	if !ok {
		p.mu.Unlock()
		return nil, &proxy.Denied{Code: http.StatusForbidden, Reason: ReasonNotAllowed, Text: "this address is not allowed to use this tuner"}
	}

	deny := func(reason, text string) (func(), error) {
		p.mu.Unlock()
		p.note(v.Name, reason, text)
		return nil, &proxy.Denied{Code: http.StatusServiceUnavailable, Reason: reason, Text: text}
	}

	if !st.Online {
		return deny(ReasonOffline, "the tuner is offline")
	}

	mine, all := 0, len(p.active)
	for _, a := range p.active {
		if a.Viewer == v.Name {
			mine++
		}
	}
	if v.Limit >= 0 && mine >= v.Limit {
		return deny(ReasonViewerLimit, fmt.Sprintf("all %d of your tuners are in use", v.Limit))
	}
	if c := p.cfg.SharedCap; c > 0 && all >= c {
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
		if all+1+local > st.TunerCount-p.cfg.OwnerReserve {
			return deny(ReasonOwnerReserve, "the remaining tuners are reserved for the owner")
		}
	}

	p.next++
	a := &Active{ID: p.next, Viewer: v.Name, Client: r.RemoteAddr, Path: r.URL.Path, Channel: channelFromPath(r.URL.Path), Started: p.now()}
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
	InUse    int        `json:"inUse"`
	LastSeen *time.Time `json:"lastSeen"` // nil: never been in contact
}

// ViewerStatuses lists the configured viewers with live information.
func (p *Policy) ViewerStatuses() []ViewerStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ViewerStatus, 0, len(p.cfg.viewers))
	for _, v := range p.cfg.viewers {
		vs := ViewerStatus{Viewer: v.Viewer}
		for _, a := range p.active {
			if a.Viewer == v.Name {
				vs.InUse++
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

// Mode is "open" while no viewers are configured, otherwise "enforced".
func (p *Policy) Mode() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.cfg.viewers) == 0 {
		return "open"
	}
	return "enforced"
}

// note tells the owner about a refusal, at most once a minute per viewer and
// reason, since media servers retry in a tight loop.
func (p *Policy) note(viewer, reason, text string) {
	if p.notes == nil {
		return
	}
	key := viewer + "|" + reason
	p.mu.Lock()
	if t, ok := p.lastNote[key]; ok && p.now().Sub(t) < noteEvery {
		p.mu.Unlock()
		return
	}
	p.lastNote[key] = p.now()
	p.mu.Unlock()
	p.notes.Note("warn", fmt.Sprintf("%s was refused a tuner: %s", viewer, text))
}

// AdminOnly restricts a handler (the /antennarr/* endpoints) to the owner.
// With no viewers configured everything is open; otherwise only loopback and
// the configured admin addresses get through.
func (p *Policy) AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !p.IsAdmin(r) {
			http.Error(w, "admin only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// IsAdmin reports whether the request may use the /antennarr/* admin endpoints.
func (p *Policy) IsAdmin(r *http.Request) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.cfg.viewers) == 0 {
		return true
	}
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

// AdminHome sends admins who open the bare address in a browser to the admin
// page at `to`; everyone else is handed to next, as if this wasn't here.
func (p *Policy) AdminHome(to string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.IsAdmin(r) {
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
