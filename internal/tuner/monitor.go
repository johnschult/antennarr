// Package tuner watches one HDHomeRun: is it up, which tuners are in use and by
// whom, and where it has moved to if its address changes.
package tuner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/johnschult/antennarr/internal/discovery"
)

// Slot is one physical tuner on the device.
type Slot struct {
	Resource       string `json:"resource"` // tuner0, tuner1, ...
	InUse          bool   `json:"inUse"`
	Channel        string `json:"channel,omitempty"`
	ChannelName    string `json:"channelName,omitempty"`
	TargetIP       string `json:"targetIP,omitempty"`
	ViaAntennarr   bool   `json:"viaAntennarr"` // streaming to this host, i.e. through us
	SignalStrength int    `json:"signalStrength,omitempty"`
	SignalQuality  int    `json:"signalQuality,omitempty"`
	BitsPerSecond  int64  `json:"bitsPerSecond,omitempty"`
}

// Firmware is what the tuner itself says about its firmware in discover.json.
type Firmware struct {
	Name    string `json:"name,omitempty"`    // e.g. "hdhomerun_dvr_atsc3"
	Version string `json:"version,omitempty"` // e.g. "20260326": a date
	// UpgradeAvailable is the newer version the tuner knows of, if it says so.
	UpgradeAvailable string `json:"upgradeAvailable,omitempty"`
}

// Event is something worth telling the owner about.
type Event struct {
	Time time.Time `json:"time"`
	Kind string    `json:"kind"` // ok, info, warn, err
	Text string    `json:"text"`
}

// State is a snapshot of the tuner.
type State struct {
	Name         string    `json:"name,omitempty"` // the device's name in Antennarr
	Online       bool      `json:"online"`
	ID           string    `json:"id"`
	FriendlyName string    `json:"friendlyName,omitempty"`
	Model        string    `json:"model,omitempty"`
	BaseURL      string    `json:"baseURL"`
	TunerCount   int       `json:"tunerCount"`
	Firmware     Firmware  `json:"firmware"`
	Slots        []Slot    `json:"slots"`
	LastSeen     time.Time `json:"lastSeen"`
	Since        time.Time `json:"since"` // when Online last changed
	Events       []Event   `json:"events"`
}

// InUse counts slots in use, and how many of those are ours.
func (s State) InUse() (total, viaAntennarr int) {
	for _, sl := range s.Slots {
		if sl.InUse {
			total++
			if sl.ViaAntennarr {
				viaAntennarr++
			}
		}
	}
	return
}

// Finder looks for the tuner with the given DeviceID when it has gone missing.
// lastKnown is the base URL it was last seen at.
type Finder func(ctx context.Context, id, lastKnown string) (discovery.Device, error)

// Config for a Monitor.
type Config struct {
	Name       string        // the device's name, shown in the UI
	BaseURL    string        // where to look first
	ID         string        // DeviceID to follow; learned from BaseURL if empty
	StreamPort string        // tuner's streaming port, default 5004
	Interval   time.Duration // poll period, default 5s
	Failures   int           // consecutive failures before searching, default 3
	Find       Finder        // default: UDP discovery, then a /24 HTTP scan
	LocalIPs   func() []string
	// OnChange, if set, is called after every poll and every Note, so a UI can
	// refresh. It must not block.
	OnChange func()
	Logger   *slog.Logger
}

// Monitor polls the tuner and tracks where it lives.
type Monitor struct {
	cfg    Config
	client *http.Client
	log    *slog.Logger

	kick chan struct{}

	mu         sync.RWMutex
	st         State
	fails      int
	lastSearch time.Time
}

const maxEvents = 50

func NewMonitor(cfg Config) *Monitor {
	if cfg.StreamPort == "" {
		cfg.StreamPort = "5004"
	}
	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.Failures == 0 {
		cfg.Failures = 3
	}
	if cfg.LocalIPs == nil {
		cfg.LocalIPs = localIPs
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	m := &Monitor{cfg: cfg, client: &http.Client{Timeout: 3 * time.Second}, log: cfg.Logger, kick: make(chan struct{}, 1)}
	m.st = State{Name: cfg.Name, ID: strings.ToUpper(cfg.ID), BaseURL: discovery.NormalizeBase(cfg.BaseURL), Since: time.Now()}
	if cfg.Find == nil {
		m.cfg.Find = m.defaultFind
	}
	return m
}

// Snapshot returns a copy of the current state.
func (m *Monitor) Snapshot() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.st
	s.Slots = append([]Slot(nil), s.Slots...)
	s.Events = append([]Event(nil), s.Events...)
	return s
}

// Targets returns the tuner's API and streaming URLs as they are right now.
func (m *Monitor) Targets() (api, stream *url.URL) {
	base := m.Snapshot().BaseURL
	api, err := url.Parse(base)
	if err != nil {
		return nil, nil
	}
	s := *api
	s.Host = net.JoinHostPort(api.Hostname(), m.cfg.StreamPort)
	return api, &s
}

// Run polls until ctx is done.
func (m *Monitor) Run(ctx context.Context) {
	m.Poll(ctx)
	t := time.NewTicker(m.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.Poll(ctx)
		case <-m.kick:
			// Something probably just changed on the tuner (a stream started
			// or ended). Give it a moment to register, then look right away
			// instead of waiting out the interval. Kicks arriving meanwhile
			// collapse into this one.
			select {
			case <-time.After(kickDelay):
			case <-ctx.Done():
				return
			}
			m.Poll(ctx)
		}
	}
}

const kickDelay = 400 * time.Millisecond

// Kick asks Run for an early poll. It never blocks.
func (m *Monitor) Kick() {
	select {
	case m.kick <- struct{}{}:
	default:
	}
}

// Poll does one health check, and one search if the tuner has been gone long
// enough.
func (m *Monitor) Poll(ctx context.Context) {
	m.poll(ctx)
	if m.cfg.OnChange != nil {
		m.cfg.OnChange()
	}
}

func (m *Monitor) poll(ctx context.Context) {
	base := m.Snapshot().BaseURL
	if err := m.check(ctx, base); err != nil {
		m.fail(ctx, err)
		return
	}
	m.mu.Lock()
	m.fails = 0
	m.mu.Unlock()
}

type discoverDoc struct {
	FriendlyName     string
	ModelNumber      string
	DeviceID         string
	TunerCount       int
	FirmwareName     string
	FirmwareVersion  string
	UpgradeAvailable string
}

type statusEntry struct {
	Resource              string
	VctNumber             string
	VctName               string
	TargetIP              string
	SignalStrengthPercent int
	SignalQualityPercent  int
	NetworkRate           int64
}

func (m *Monitor) getJSON(ctx context.Context, base, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s%s: %s", base, path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// check confirms that the device at base is our tuner and records its state.
func (m *Monitor) check(ctx context.Context, base string) error {
	var d discoverDoc
	if err := m.getJSON(ctx, base, "/discover.json", &d); err != nil {
		return err
	}
	id := strings.ToUpper(d.DeviceID)

	m.mu.Lock()
	if m.st.ID == "" {
		m.st.ID = id // first contact: this is the tuner we follow from now on
		m.addEvent("info", fmt.Sprintf("Following tuner %s at %s", id, base))
	}
	want := m.st.ID
	m.mu.Unlock()
	if id != want {
		return fmt.Errorf("%s is tuner %s, not %s", base, id, want)
	}

	var raw []statusEntry
	statusErr := m.getJSON(ctx, base, "/status.json", &raw)
	local := map[string]bool{}
	for _, ip := range m.cfg.LocalIPs() {
		local[ip] = true
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	wasOnline := m.st.Online
	m.st.FriendlyName, m.st.Model, m.st.TunerCount = d.FriendlyName, d.ModelNumber, d.TunerCount
	m.st.Firmware = Firmware{Name: d.FirmwareName, Version: d.FirmwareVersion, UpgradeAvailable: d.UpgradeAvailable}
	m.st.LastSeen = time.Now()
	if statusErr == nil {
		m.st.Slots = slotsFrom(raw, local)
	}
	if !wasOnline {
		m.st.Online, m.st.Since = true, time.Now()
		m.addEvent("ok", fmt.Sprintf("Tuner online at %s", base))
		m.log.Info("tuner online", "id", id, "base", base)
	}
	return nil
}

func slotsFrom(raw []statusEntry, local map[string]bool) []Slot {
	var out []Slot
	for _, e := range raw {
		if !strings.HasPrefix(e.Resource, "tuner") {
			continue
		}
		s := Slot{Resource: e.Resource, TargetIP: e.TargetIP}
		if e.VctNumber != "" || e.TargetIP != "" {
			s.InUse = true
			s.Channel, s.ChannelName = e.VctNumber, e.VctName
			s.ViaAntennarr = local[e.TargetIP]
			s.SignalStrength, s.SignalQuality, s.BitsPerSecond = e.SignalStrengthPercent, e.SignalQualityPercent, e.NetworkRate
		}
		out = append(out, s)
	}
	return out
}

func (m *Monitor) fail(ctx context.Context, err error) {
	m.mu.Lock()
	m.fails++
	fails := m.fails
	id, last := m.st.ID, m.st.BaseURL
	if fails < m.cfg.Failures {
		m.mu.Unlock()
		m.log.Debug("tuner check failed", "n", fails, "err", err)
		return
	}
	if m.st.Online {
		m.st.Online, m.st.Since = false, time.Now()
		m.st.Slots = nil
		m.addEvent("err", fmt.Sprintf("Tuner offline: %v", err))
		m.log.Warn("tuner offline", "err", err)
	}
	// Don't hammer the network: search at most every 30s.
	if time.Since(m.lastSearch) < 30*time.Second {
		m.mu.Unlock()
		return
	}
	m.lastSearch = time.Now()
	m.mu.Unlock()

	if id == "" {
		return // never saw it, so there's nothing to search for
	}
	sctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	dev, ferr := m.cfg.Find(sctx, id, last)
	if ferr != nil || dev.BaseURL == "" || dev.BaseURL == last {
		return
	}
	m.mu.Lock()
	m.st.BaseURL = dev.BaseURL
	m.addEvent("info", fmt.Sprintf("Tuner moved %s → %s (found by DeviceID %s)", last, dev.BaseURL, id))
	m.mu.Unlock()
	m.log.Info("tuner moved", "from", last, "to", dev.BaseURL, "id", id)
	m.Poll(ctx) // confirm right away instead of waiting a full interval
}

// Note adds an event from outside the monitor, e.g. a refused stream.
func (m *Monitor) Note(kind, text string) {
	m.mu.Lock()
	m.addEvent(kind, text)
	m.mu.Unlock()
	if m.cfg.OnChange != nil {
		m.cfg.OnChange()
	}
}

// addEvent records an event; m.mu must be held.
func (m *Monitor) addEvent(kind, text string) {
	m.st.Events = append(m.st.Events, Event{Time: time.Now(), Kind: kind, Text: text})
	if n := len(m.st.Events); n > maxEvents {
		m.st.Events = m.st.Events[n-maxEvents:]
	}
}

// defaultFind tries UDP discovery, then scans the /24 around the last address.
func (m *Monitor) defaultFind(ctx context.Context, id, lastKnown string) (discovery.Device, error) {
	hostHint := ""
	if u, err := url.Parse(lastKnown); err == nil {
		hostHint = u.Hostname()
	}
	if devs, err := discovery.Discover(ctx, 2*time.Second); err == nil {
		for _, d := range devs {
			if d.ID == id {
				return d, nil
			}
		}
	}
	if ip := net.ParseIP(hostHint); ip != nil {
		if p, ok := discovery.PrefixAround(ip); ok {
			for _, d := range discovery.Scan(ctx, p, id) {
				if d.ID == id {
					return d, nil
				}
			}
		}
	}
	return discovery.Device{}, fmt.Errorf("tuner %s not found", id)
}

// localIPs lists this host's IPv4 addresses; a stream whose TargetIP is one of
// them is flowing through Antennarr.
func localIPs() []string {
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
			out = append(out, ipn.IP.String())
		}
	}
	return out
}
