// Package server wires the pieces together: for each device a monitor, a
// proxy and a listener, around one shared access policy, API and web UI.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/johnschult/antennarr/internal/access"
	"github.com/johnschult/antennarr/internal/api"
	"github.com/johnschult/antennarr/internal/discovery"
	"github.com/johnschult/antennarr/internal/firmware"
	"github.com/johnschult/antennarr/internal/live"
	"github.com/johnschult/antennarr/internal/proxy"
	"github.com/johnschult/antennarr/internal/tuner"
	"github.com/johnschult/antennarr/internal/ui"
)

// Locator finds where a tuner is to start with: from its address if given,
// otherwise by searching the network for its DeviceID.
type Locator func(ctx context.Context, addr, id string) (baseURL string, err error)

// Config is everything needed to run.
type Config struct {
	// Settings as read from the settings file (or the defaults).
	Settings access.Settings
	// Devices to run: Settings.Devices, or one built from command-line flags.
	// Kept apart from Settings so that a flags-only setup never writes a
	// devices list into the settings file when someone edits a viewer.
	Devices    []access.Device
	ConfigPath string // where edits are saved; empty means read-only
	Poll       time.Duration
	Log        *slog.Logger
	// Locate is DefaultLocate unless a test supplies another.
	Locate Locator
	// Listeners are pre-bound listeners by device name; any device without one
	// is bound from its Listen address when Run starts.
	Listeners map[string]net.Listener
	// FirmwareURL and FirmwareInterval override where and how often the
	// firmware changelog is read; they exist for tests.
	FirmwareURL      string
	FirmwareInterval time.Duration
}

type device struct {
	cfg     access.Device
	mon     *tuner.Monitor
	handler http.Handler
}

// Server runs the devices.
type Server struct {
	cfg      Config
	log      *slog.Logger
	policy   *access.Policy
	hub      *live.Hub
	devices  []*device
	firmware *firmware.Checker

	mu        sync.Mutex
	announced map[string]string // device -> the firmware version we last told the owner about
}

// New builds the server: it finds each tuner and prepares everything, but
// listens on nothing until Run.
func New(ctx context.Context, cfg Config) (*Server, error) {
	if len(cfg.Devices) == 0 {
		return nil, errors.New("no devices to run")
	}
	if err := (access.Settings{Devices: cfg.Devices}).Validate(); err != nil {
		return nil, err
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.Poll == 0 {
		cfg.Poll = 5 * time.Second
	}
	if cfg.Locate == nil {
		cfg.Locate = DefaultLocate
	}

	policy, err := access.NewPolicy(cfg.Settings, cfg.Log)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, log: cfg.Log, policy: policy, hub: live.NewHub(), announced: map[string]string{}}
	s.firmware = firmware.NewChecker(firmware.Config{
		URL: cfg.FirmwareURL, Interval: cfg.FirmwareInterval, Logger: cfg.Log,
		// Read live, so switching the setting takes effect at once.
		Enabled:  func() bool { return policy.Settings().FirmwareCheckEnabled() },
		OnChange: func() { s.hub.Notify(); s.announceFirmware() },
	})

	var apiDevices []api.Device
	for _, d := range cfg.Devices {
		base, err := cfg.Locate(ctx, d.Tuner, d.ID)
		if err != nil {
			return nil, fmt.Errorf("device %q: %w", d.Name, err)
		}
		log := cfg.Log.With("device", d.Name)
		mon := tuner.NewMonitor(tuner.Config{
			Name: d.Name, BaseURL: base, ID: d.ID, StreamPort: d.StreamPort,
			Interval: cfg.Poll, OnChange: func() { s.hub.Notify(); s.announceFirmware() }, Logger: log,
		})
		policy.AddDevice(d.Name, mon, mon)
		s.devices = append(s.devices, &device{cfg: d, mon: mon})
		apiDevices = append(apiDevices, api.Device{Name: d.Name, Listen: d.Listen, Mon: mon})
		log.Info("device ready", "listen", d.Listen, "tuner", base, "tunerID", strings.ToUpper(d.ID))
	}

	// A stream starting or ending changes the tuners' own status too: tell the
	// dashboards now, and have the monitors look again straight away.
	policy.SetOnChange(func() {
		s.hub.Notify()
		s.firmware.Kick() // a no-op unless the firmware check was just switched on
		for _, d := range s.devices {
			d.mon.Kick()
		}
	})

	admin := http.NewServeMux()
	admin.Handle("/antennarr/streams", policy.AdminOnly(policy.ActiveHandler()))
	admin.Handle(api.Prefix, policy.AdminOnly((&api.API{
		Devices: apiDevices, Firmware: s.firmware, Pol: policy, Store: access.NewStore(cfg.ConfigPath, policy), Hub: s.hub, Log: cfg.Log,
	}).Handler()))
	admin.Handle("/antennarr/", policy.AdminOnly(ui.Handler("/antennarr/"))) // the web UI

	for _, d := range s.devices {
		name := d.cfg.Name
		mux := http.NewServeMux()
		mux.Handle("/antennarr/", admin) // the admin pages are reachable on every device's port
		tunerAPI := proxy.New(proxy.Config{
			Upstream: d.mon, Policy: policy.Device(d.cfg.Name),
			Advertise: d.cfg.Advertise, DeviceID: d.cfg.ReportID, Logger: cfg.Log.With("device", d.cfg.Name),
			FriendlyName: func() string { return policy.Label(name) },
		})
		mux.Handle("/", tunerAPI)
		// Someone who types the bare address into a browser wants the admin
		// page, not the tuner's own front page. Everyone else (viewers, media
		// servers) is handled as before.
		mux.Handle("GET /{$}", policy.AdminHome("/antennarr/", tunerAPI))
		d.handler = mux
	}

	if len(cfg.Settings.Viewers) == 0 {
		s.log.Warn("no viewers configured: anyone who can reach these ports can use the tuners")
	}
	return s, nil
}

// Policy is the access policy, for reloading settings.
func (s *Server) Policy() *access.Policy { return s.policy }

// Run serves every device until ctx ends or one of them fails.
func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Bind everything first, so a port that's already taken fails the whole
	// start-up instead of leaving some devices running and others not.
	lns := make([]net.Listener, len(s.devices))
	for i, d := range s.devices {
		ln, ok := s.cfg.Listeners[d.cfg.Name]
		if !ok {
			var err error
			if ln, err = net.Listen("tcp", d.cfg.Listen); err != nil {
				for _, l := range lns[:i] {
					l.Close()
				}
				return fmt.Errorf("device %q: %w", d.cfg.Name, err)
			}
		}
		lns[i] = ln
	}

	go s.firmware.Run(ctx)

	servers := make([]*http.Server, len(s.devices))
	errc := make(chan error, len(s.devices))
	for i, d := range s.devices {
		go d.mon.Run(ctx)
		// No overall timeouts: streams run for hours. Only the header read is
		// bounded, so a stalled client can't hold a connection open for free.
		servers[i] = &http.Server{Handler: d.handler, ReadHeaderTimeout: 10 * time.Second}
		s.log.Info("listening", "device", d.cfg.Name, "addr", lns[i].Addr().String())
		go func(srv *http.Server, ln net.Listener, name string) {
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- fmt.Errorf("device %q: %w", name, err)
			}
		}(servers[i], lns[i], d.cfg.Name)
	}

	var first error
	select {
	case <-ctx.Done():
	case first = <-errc:
	}
	cancel()
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	for _, srv := range servers {
		_ = srv.Shutdown(shutdown)
	}
	return first
}

// announceFirmware tells the owner, once per release, when a tuner has newer
// firmware available, and forgets it again once the tuner is up to date so the
// next release is announced too.
func (s *Server) announceFirmware() {
	type note struct{ device, text string }
	var notes []note

	s.mu.Lock()
	for _, d := range s.devices {
		t := d.mon.Snapshot()
		if !t.Online {
			continue
		}
		st := s.firmware.StatusFor(firmware.Device{
			Model: t.Model, FirmwareName: t.Firmware.Name, Version: t.Firmware.Version, UpgradeAvailable: t.Firmware.UpgradeAvailable,
		})
		switch st.State {
		case firmware.StateUpdateAvailable:
			if s.announced[d.cfg.Name] != st.Latest {
				s.announced[d.cfg.Name] = st.Latest
				title := s.policy.Label(d.cfg.Name)
				if title == "" {
					title = d.cfg.Name
				}
				text := fmt.Sprintf("Firmware %s is available for %s (it is on %s)", st.Latest, title, st.Current)
				if st.DeviceReported != "" {
					text += ", as reported by the tuner"
				}
				notes = append(notes, note{d.cfg.Name, text})
			}
		case firmware.StateUpToDate:
			delete(s.announced, d.cfg.Name)
		}
	}
	s.mu.Unlock()

	// Outside the lock: noting an event calls back into the monitors.
	for _, n := range notes {
		for _, d := range s.devices {
			if d.cfg.Name == n.device {
				d.mon.Note("info", n.text)
			}
		}
	}
}

// Reload applies new settings. Everything about viewers and limits takes
// effect at once; changes to the devices themselves (their ports and tuner
// addresses) need a restart, and that is reported rather than half-applied.
func (s *Server) Reload(next access.Settings) error {
	if err := s.policy.Update(next); err != nil {
		return err
	}
	if len(next.Devices) > 0 && !equal(restartKeys(next.Devices), restartKeys(s.cfg.Devices)) {
		s.log.Warn("the devices in the settings file changed; restart Antennarr to apply that (everything else was reloaded)")
	}
	s.log.Info("settings reloaded", "viewers", len(next.Viewers))
	return nil
}

// ReloadFile re-reads the settings file and applies it. A bad file is
// reported and the running settings are kept.
func (s *Server) ReloadFile(path string) error {
	next, _, err := access.LoadSettings(path)
	if err != nil {
		return err
	}
	return s.Reload(next)
}

// restartKeys describes the parts of each device that can't change while
// running. Per-device limits are left out: those reload live.
func restartKeys(ds []access.Device) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = strings.Join([]string{d.Name, d.Listen, d.Tuner, strings.ToUpper(d.ID), d.StreamPort, d.Advertise, strings.ToUpper(d.ReportID)}, "|")
	}
	sort.Strings(out)
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// DefaultLocate uses the tuner's address if there is one, otherwise searches
// the network for its DeviceID.
func DefaultLocate(ctx context.Context, addr, id string) (string, error) {
	if addr != "" {
		u, err := ParseTuner(addr)
		if err != nil {
			return "", err
		}
		return u.String(), nil
	}
	if id == "" {
		return "", errors.New("needs a tuner address or a DeviceID to search for")
	}
	fctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	devs, err := discovery.Discover(fctx, 3*time.Second)
	if err != nil && len(devs) == 0 {
		return "", fmt.Errorf("searching for tuner %s: %w", id, err)
	}
	for _, d := range devs {
		if strings.EqualFold(d.ID, id) {
			return d.BaseURL, nil
		}
	}
	return "", fmt.Errorf("tuner %s not found on the network; give its address", strings.ToUpper(id))
}

// ParseTuner accepts "192.168.1.132", "host:80" or a full URL.
func ParseTuner(s string) (*url.URL, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("bad tuner address %q", s)
	}
	return u, nil
}
