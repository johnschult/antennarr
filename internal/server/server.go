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
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/johnschult/antennarr/internal/access"
	"github.com/johnschult/antennarr/internal/api"
	"github.com/johnschult/antennarr/internal/claim"
	"github.com/johnschult/antennarr/internal/discovery"
	"github.com/johnschult/antennarr/internal/firmware"
	"github.com/johnschult/antennarr/internal/guide"
	"github.com/johnschult/antennarr/internal/hdhr"
	"github.com/johnschult/antennarr/internal/history"
	"github.com/johnschult/antennarr/internal/lineup"
	"github.com/johnschult/antennarr/internal/live"
	"github.com/johnschult/antennarr/internal/netinfo"
	"github.com/johnschult/antennarr/internal/notify"
	"github.com/johnschult/antennarr/internal/plexauth"
	"github.com/johnschult/antennarr/internal/proxy"
	"github.com/johnschult/antennarr/internal/restart"
	"github.com/johnschult/antennarr/internal/tuner"
	"github.com/johnschult/antennarr/internal/tunerinfo"
	"github.com/johnschult/antennarr/internal/ui"
	"github.com/johnschult/antennarr/internal/upgrade"
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
	// AdminListen is where the admin pages are served while there are no tuners
	// to put them on (default ":5004"). With tuners, the admin pages are on
	// each tuner's port instead.
	AdminListen string
	Poll        time.Duration
	Log         *slog.Logger
	// Locate is DefaultLocate unless a test supplies another.
	Locate Locator
	// Listeners are pre-bound listeners by device name; any device without one
	// is bound from its Listen address when Run starts.
	Listeners map[string]net.Listener
	// FirmwareURL and FirmwareInterval override where and how often the
	// firmware changelog is read; they exist for tests.
	FirmwareURL      string
	FirmwareInterval time.Duration
	// GuideURL and GuideIconBase override where the program guide and
	// channel logos come from; they exist for tests, which must never reach
	// SiliconDust.
	GuideURL      string
	GuideIconBase string
	// PlexURL and PlexAuthURL override plex.tv's addresses, for tests.
	PlexURL, PlexAuthURL string
	// PushoverURL overrides where Pushover messages go, for tests.
	PushoverURL string
	// FirmwareDownloadBase overrides where firmware images are downloaded from, and ControlPort the port
	// a tuner's control protocol is on (65001), for tests and the demo, which must never reach SiliconDust or
	// flash a real tuner. UpgradeConfig, if set, may change the install's timings.
	FirmwareDownloadBase string
	ControlPort          string
	UpgradeConfig        func(*upgrade.Config)
	// RestartConfig, if set, may change the timings of restarting a tuner (the demo and tests).
	RestartConfig func(*restart.Config)
	// Find looks for a tuner that has gone missing, at another address. Nil means a UDP discover and a scan
	// of the nearby network; tests supply one that doesn't, so a fake tuner that goes quiet never sends them
	// looking for a real one.
	Find tuner.Finder
	// OfflineGrace is how long a tuner must be offline before the owner is
	// messaged; zero means two minutes. Tests shorten it.
	OfflineGrace time.Duration
}

type device struct {
	cfg     access.Device
	mon     *tuner.Monitor
	handler http.Handler

	// Set when the device is serving.
	cancel context.CancelFunc
	srv    *http.Server
	ln     net.Listener
}

// Server runs the devices.
type Server struct {
	cfg       Config
	log       *slog.Logger
	policy    *access.Policy
	store     *access.Store
	hub       *live.Hub
	firmware  *firmware.Checker
	guide     *guide.Service
	notify    *notify.Notifier
	upgrade   *upgrade.Manager
	tunerinfo *tunerinfo.Reader
	restart   *restart.Manager
	history   *history.Recorder
	plex      *plexauth.Auth
	claim     *claim.Claimer
	admin     http.Handler // the admin pages, shared by every listener
	// fromFlags: the devices came from command-line flags, so there is no
	// devices list in the settings file to add to or remove from.
	fromFlags bool

	devMu   sync.RWMutex
	devices []*device

	mu        sync.Mutex
	announced map[string]string // device -> the firmware version we last told the owner about

	// fleetMu serializes adding and removing devices. The fields below it are
	// set by Run and used by them.
	fleetMu sync.Mutex
	runCtx  context.Context // nil until Run
	errc    chan error
	bare    *bareListener // serves the admin pages while there are no devices
}

// deviceList is a copy of the devices now running.
func (s *Server) deviceList() []*device {
	s.devMu.RLock()
	defer s.devMu.RUnlock()
	return append([]*device(nil), s.devices...)
}

// New builds the server: it finds each tuner and prepares everything, but
// listens on nothing until Run. With no tuners it still works: the admin pages
// are served on their own so tuners can be added from there.
func New(ctx context.Context, cfg Config) (*Server, error) {
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
	if cfg.AdminListen == "" {
		cfg.AdminListen = ":5004"
	}

	policy, err := access.NewPolicy(cfg.Settings, cfg.Log)
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg: cfg, log: cfg.Log, policy: policy, hub: live.NewHub(), announced: map[string]string{},
		fromFlags: len(cfg.Settings.Devices) == 0 && len(cfg.Devices) > 0,
	}
	s.firmware = firmware.NewChecker(firmware.Config{
		URL: cfg.FirmwareURL, Interval: cfg.FirmwareInterval, Logger: cfg.Log,
		// Read live, so switching the setting takes effect at once.
		Enabled:  func() bool { return policy.Settings().FirmwareCheckEnabled() },
		OnChange: func() { s.hub.Notify(); s.announceFirmware() },
	})

	s.guide = guide.NewService(guide.Config{
		URL: cfg.GuideURL, IconBase: cfg.GuideIconBase, Logger: cfg.Log,
		Enabled:  func() bool { return policy.Settings().GuideEnabled() },
		OnChange: s.hub.Notify,
	})

	// Messages to the owner. Nothing is contacted until a channel is set up, and the
	// settings are read on each event, so changing them needs no restart.
	s.notify = notify.New(notify.Config{
		Settings: policy.Settings, Log: cfg.Log, PushoverURL: cfg.PushoverURL, OfflineGrace: cfg.OfflineGrace,
		StatePath: notifiedFile(cfg.ConfigPath),
	})
	policy.SetEvents(s.notify)

	// Installing firmware: only ever started by an admin pressing the button, one tuner at a time.
	ucfg := upgrade.Config{
		Download: firmware.Downloader{Base: cfg.FirmwareDownloadBase},
		OnChange: s.hub.Notify,
		Log:      cfg.Log,
	}
	if cfg.ControlPort != "" {
		port := cfg.ControlPort
		ucfg.Dial = func(ctx context.Context, host string) (*hdhr.Conn, error) {
			return hdhr.Dial(ctx, net.JoinHostPort(host, port))
		}
	}
	if cfg.UpgradeConfig != nil {
		cfg.UpgradeConfig(&ucfg)
	}
	s.upgrade = upgrade.New(ctx, ucfg)
	dial := ucfg.Dial
	if dial == nil {
		dial = func(ctx context.Context, host string) (*hdhr.Conn, error) {
			return hdhr.Dial(ctx, net.JoinHostPort(host, hdhr.ControlPort))
		}
	}
	s.tunerinfo = &tunerinfo.Reader{Dial: dial}
	rcfg := restart.Config{Dial: dial, OnChange: s.hub.Notify, Log: cfg.Log}
	if cfg.RestartConfig != nil {
		cfg.RestartConfig(&rcfg)
	}
	s.restart = restart.New(ctx, rcfg)
	var hstore *history.Store
	if p := historyFile(cfg.ConfigPath); p != "" {
		hstore = &history.Store{Path: p}
	}
	s.history = history.New(hstore, time.Now())
	s.history.OnError = func(err error) { cfg.Log.Warn("couldn't save the activity history", "err", err) }

	// A stream starting or ending changes the tuners' own status too: tell the
	// dashboards now, and have the monitors look again straight away.
	policy.SetOnChange(func() {
		s.hub.Notify()
		s.firmware.Kick() // a no-op unless the firmware check was just switched on
		for _, d := range s.deviceList() {
			d.mon.Kick()
		}
	})

	s.store = access.NewStore(cfg.ConfigPath, policy)

	// While nobody has been made an admin, a one-time link in the log lets the
	// owner become one from another device.
	s.claim = claim.New(claim.Config{Policy: policy, Store: s.store, Log: cfg.Log})

	// Signing in with Plex is a second way to be an admin, next to the address.
	s.plex = plexauth.New(plexauth.Config{
		Base: cfg.PlexURL, AuthBase: cfg.PlexAuthURL,
		Settings: policy.Settings, Edit: s.store.Edit, IsAdmin: policy.IsAdmin, Log: cfg.Log,
		SessionsFile: sessionsFile(cfg.ConfigPath),
	})
	policy.SetSessionCheck(s.plex.IsSignedInAdmin)

	admin := http.NewServeMux()
	admin.Handle("/antennarr/streams", policy.AdminOnly(policy.ActiveHandler()))
	admin.Handle(api.Prefix, policy.AdminOnly((&api.API{
		Fleet: s, Firmware: s.firmware, Guide: s.guide, Notify: s.notify, Upgrade: s.upgrade, Device: s.tunerinfo, Restart: s.restart, History: s.history, Lineup: &lineup.Client{}, Control: dial, Pol: policy, Store: s.store, Hub: s.hub, Log: cfg.Log,
	}).Handler()))
	// The page itself is just static files with no data in them. It has to load
	// for someone who is not yet signed in, or there would be nowhere to sign in;
	// everything it shows comes from the admin-only API above.
	admin.Handle("/antennarr/", ui.Handler("/antennarr/"))
	s.admin = admin

	for _, d := range cfg.Devices {
		dev, err := s.build(ctx, d)
		if err != nil {
			return nil, err
		}
		s.devices = append(s.devices, dev)
		s.log.Info("device ready", "device", d.Name, "listen", d.Listen, "tuner", dev.mon.Snapshot().BaseURL, "tunerID", strings.ToUpper(d.ID))
	}

	if len(cfg.Settings.Viewers) == 0 {
		s.log.Warn("no viewers configured: nobody can use the tuners until you add one")
	}
	return s, nil
}

// build finds one tuner and prepares everything to serve it, without listening.
func (s *Server) build(ctx context.Context, d access.Device) (*device, error) {
	base, err := s.cfg.Locate(ctx, d.Tuner, d.ID)
	if err != nil {
		return nil, fmt.Errorf("device %q: %w", d.Name, err)
	}
	log := s.cfg.Log.With("device", d.Name)
	mon := tuner.NewMonitor(tuner.Config{
		Name: d.Name, BaseURL: base, ID: d.ID, StreamPort: d.StreamPort,
		Interval: s.cfg.Poll, Find: s.cfg.Find, OnChange: func() { s.hub.Notify(); s.announceFirmware(); s.guide.Kick() },
		OnTransition: func(online bool) { s.notify.TunerState(d.Name, s.title(d.Name), online) },
		Logger:       log,
	})
	s.policy.AddDevice(d.Name, mon, mon)
	s.guide.Watch(d.Name, mon.DeviceAuth)

	name := d.Name
	mux := http.NewServeMux()
	mux.Handle("/antennarr/", s.admin) // the admin pages are reachable on every device's port
	// Signing in has to be reachable by someone who isn't signed in yet; each
	// endpoint checks for itself what it will allow.
	mux.Handle("/antennarr/auth/", s.plex.Handler())
	mux.Handle(claim.Path, s.claim.Handler())
	tunerAPI := proxy.New(proxy.Config{
		Upstream: mon, Policy: s.policy.Device(name),
		Advertise: d.Advertise, DeviceID: d.ReportID, Logger: log,
	})
	mux.Handle("/", tunerAPI)
	// Someone who types the bare address into a browser wants the admin
	// page, not the tuner's own front page. Everyone else (viewers, media
	// servers) is handled as before.
	mux.Handle("GET /{$}", s.policy.AdminHome("/antennarr/", tunerAPI))

	log.Debug("device built", "listen", d.Listen, "tuner", base)
	return &device{cfg: d, mon: mon, handler: mux}, nil
}

// Policy is the access policy, for reloading settings.
func (s *Server) Policy() *access.Policy { return s.policy }

// Run serves every device until ctx ends or one of them fails.
func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	s.fleetMu.Lock()
	// Bind everything first, so a port that's already taken fails the whole
	// start-up instead of leaving some devices running and others not.
	devs := s.deviceList()
	lns := make([]net.Listener, len(devs))
	closeAll := func() {
		for _, l := range lns {
			if l != nil {
				l.Close()
			}
		}
	}
	for i, d := range devs {
		ln, ok := s.cfg.Listeners[d.cfg.Name]
		if !ok {
			var err error
			if ln, err = net.Listen("tcp", d.cfg.Listen); err != nil {
				closeAll()
				s.fleetMu.Unlock()
				return fmt.Errorf("device %q: %w", d.cfg.Name, err)
			}
		}
		lns[i] = ln
	}
	var bareLn net.Listener
	if len(devs) == 0 {
		ln, ok := s.cfg.Listeners[bareName]
		if !ok {
			var err error
			if ln, err = net.Listen("tcp", s.cfg.AdminListen); err != nil {
				s.fleetMu.Unlock()
				return fmt.Errorf("admin pages: %w", err)
			}
		}
		bareLn = ln
		lns = append(lns, ln)
	}

	s.runCtx = ctx
	s.errc = make(chan error, 8)
	s.logClaimLinks(lns)

	go s.firmware.Run(ctx)
	go s.guide.Run(ctx)
	go s.notify.Run(ctx)
	go s.recordHistory(ctx)

	for i, d := range devs {
		s.start(d, lns[i])
	}
	if bareLn != nil {
		s.startBare(bareLn)
	}
	errc := s.errc
	s.fleetMu.Unlock()

	var first error
	select {
	case <-ctx.Done():
	case first = <-errc:
	}
	cancel()

	s.fleetMu.Lock()
	servers := []*http.Server{}
	for _, d := range s.deviceList() {
		servers = append(servers, d.srv)
	}
	if s.bare != nil {
		servers = append(servers, s.bare.srv)
	}
	s.fleetMu.Unlock()
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	for _, srv := range servers {
		if srv != nil {
			_ = srv.Shutdown(shutdown)
		}
	}
	return first
}

// start begins serving a device on ln. The caller holds fleetMu.
func (s *Server) start(d *device, ln net.Listener) {
	dctx, cancel := context.WithCancel(s.runCtx)
	d.cancel = cancel
	go d.mon.Run(dctx)
	s.serve(d, ln)
}

// serve answers requests for a device on ln.
func (s *Server) serve(d *device, ln net.Listener) {
	d.ln = ln
	// No overall timeouts: streams run for hours. Only the header read is
	// bounded, so a stalled client can't hold a connection open for free.
	d.srv = &http.Server{Handler: d.handler, ReadHeaderTimeout: 10 * time.Second}
	s.log.Info("listening", "device", d.cfg.Name, "addr", ln.Addr().String())
	go func(srv *http.Server, name string) {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			select {
			case s.errc <- fmt.Errorf("device %q: %w", name, err):
			default:
			}
		}
	}(d.srv, d.cfg.Name)
}

// logClaimLinks writes the first-run setup link to the log, once for each
// address the machine can be reached at, if there is still an admin to claim.
func (s *Server) logClaimLinks(lns []net.Listener) {
	tok := s.claim.Token()
	if tok == "" || len(lns) == 0 {
		return
	}
	port := "5004"
	if a, ok := lns[0].Addr().(*net.TCPAddr); ok {
		port = fmt.Sprint(a.Port)
	}
	var hosts []string
	for _, a := range netinfo.Detect() {
		hosts = append(hosts, a.IP)
	}
	if len(hosts) == 0 {
		hosts = []string{"<this-machine>"}
	}
	s.log.Info("FIRST RUN: nobody is an admin yet. To set up, open one of these links from the device you want to use as the admin. It works once, and only from a private network")
	for _, u := range claim.URLs(tok, port, hosts) {
		s.log.Info("  setup link", "url", u)
	}
}

// announceFirmware tells the owner, once per release, when a tuner has newer
// firmware available, and forgets it again once the tuner is up to date so the
// next release is announced too.
func (s *Server) announceFirmware() {
	type note struct{ device, title, version, text string }
	var notes []note

	s.mu.Lock()
	for _, d := range s.deviceList() {
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
				title := s.title(d.cfg.Name)
				text := fmt.Sprintf("Firmware %s is available for %s (it is on %s)", st.Latest, title, st.Current)
				if st.DeviceReported != "" {
					text += ", as reported by the tuner"
				}
				notes = append(notes, note{d.cfg.Name, title, st.Latest, text})
			}
		case firmware.StateUpToDate:
			delete(s.announced, d.cfg.Name)
		}
	}
	s.mu.Unlock()

	// Outside the lock: noting an event calls back into the monitors.
	for _, n := range notes {
		for _, d := range s.deviceList() {
			if d.cfg.Name == n.device {
				d.mon.Note("info", n.text)
			}
		}
		s.notify.FirmwareFound(n.device, n.title, n.version, n.text)
	}
}

// title is what a tuner is called to the owner: its label, or its name.
func (s *Server) title(name string) string {
	if l := s.policy.Label(name); l != "" {
		return l
	}
	return name
}

// Reload applies new settings. Everything about viewers and limits takes
// effect at once; changes to the devices themselves (their ports and tuner
// addresses) need a restart, and that is reported rather than half-applied.
func (s *Server) Reload(next access.Settings) error {
	if err := s.policy.Update(next); err != nil {
		return err
	}
	if len(next.Devices) > 0 && !equal(restartKeys(next.Devices), restartKeys(s.runningConfigs())) {
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

// notifiedFile is where the firmware releases the owner was messaged about are
// kept, so a restart doesn't repeat them: beside the settings file, and empty when
// there isn't one.
func notifiedFile(configPath string) string {
	if configPath == "" {
		return ""
	}
	return strings.TrimSuffix(configPath, filepath.Ext(configPath)) + ".notified.json"
}

// sessionsFile is where Plex sign-ins are kept so they survive a restart: beside
// the settings file, which is where Antennarr is already allowed to write. With
// no settings file (started with flags only) there is nowhere to keep them.
func sessionsFile(configPath string) string {
	if configPath == "" {
		return ""
	}
	return strings.TrimSuffix(configPath, filepath.Ext(configPath)) + ".sessions.json"
}
