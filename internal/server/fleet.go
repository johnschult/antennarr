package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/johnschult/yagi/internal/access"
	"github.com/johnschult/yagi/internal/api"
	"github.com/johnschult/yagi/internal/claim"
)

// bareName is the key in Config.Listeners for the listener that serves the
// admin pages while there are no devices. It exists for tests.
const bareName = "(admin)"

// bareListener serves just the admin pages, for when there are no tuners to
// put them on. Without it a start with no tuner found would have nowhere to
// add one from.
type bareListener struct {
	srv *http.Server
	ln  net.Listener
}

// Devices lists the tuners now being fronted.
func (s *Server) Devices() []api.Device {
	s.devMu.RLock()
	defer s.devMu.RUnlock()
	list := s.devices
	out := make([]api.Device, len(list))
	for i, d := range list {
		out[i] = api.Device{Name: d.cfg.Name, Listen: d.cfg.Listen, Mon: d.mon}
	}
	return out
}

// runningConfigs is the settings of the devices now running.
func (s *Server) runningConfigs() []access.Device {
	s.devMu.RLock()
	defer s.devMu.RUnlock()
	list := s.devices
	out := make([]access.Device, len(list))
	for i, d := range list {
		out[i] = d.cfg
	}
	return out
}

// canEditDevices says why tuners can't be added or removed, if they can't.
func (s *Server) canEditDevices() error {
	if s.runCtx == nil || s.runCtx.Err() != nil {
		return api.NewError(http.StatusServiceUnavailable, "Yagi is not running")
	}
	if !s.store.Editable() {
		return api.NewError(http.StatusConflict, "there is no settings file to save tuners in")
	}
	if s.fromFlags {
		return api.NewError(http.StatusConflict, "the tuner was set with command-line flags; list your tuners under \"devices\" in the settings file to change them here")
	}
	return nil
}

// AddDevice starts fronting a tuner and saves it to the settings. Anything
// left out (the name, the port) is chosen; a tuner given only by address has
// its DeviceID read from it. Nothing is kept unless all of it worked.
func (s *Server) AddDevice(ctx context.Context, d access.Device) (access.Device, error) {
	s.fleetMu.Lock()
	defer s.fleetMu.Unlock()
	if err := s.canEditDevices(); err != nil {
		return d, err
	}
	if d.Tuner == "" && d.ID == "" {
		return d, api.NewError(http.StatusBadRequest, "give the tuner's address, or its DeviceID")
	}
	if d.Name == "" {
		d.Name = s.freeName(d.ID)
	}
	if d.Listen == "" {
		d.Listen = s.nextListen()
	}
	set := s.policy.Settings()
	set.Devices = append(set.Devices, d)
	if err := set.Validate(); err != nil {
		return d, api.NewError(http.StatusBadRequest, err.Error())
	}

	dev, err := s.build(ctx, d)
	if err != nil {
		return d, api.NewError(http.StatusBadRequest, err.Error())
	}
	// Look at it once now, so a wrong address is refused instead of saved as a
	// tuner that never answers. With a DeviceID given it may simply be unplugged.
	pctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	dev.mon.Poll(pctx)
	cancel()
	snap := dev.mon.Snapshot()
	if !snap.Online && d.ID == "" {
		s.drop(dev)
		return d, api.NewError(http.StatusUnprocessableEntity, fmt.Sprintf("couldn't reach an HDHomeRun at %s", d.Tuner))
	}
	if d.ID == "" {
		d.ID = strings.ToUpper(snap.ID)
		dev.cfg.ID = d.ID
	}
	for _, other := range s.deviceList() {
		if d.ID != "" && strings.EqualFold(other.cfg.ID, d.ID) {
			s.drop(dev)
			return d, api.NewError(http.StatusConflict, fmt.Sprintf("that tuner is already added, as %q", other.cfg.Name))
		}
	}

	ln, swapped, err := s.listen(d)
	if err != nil {
		s.drop(dev)
		return d, api.NewError(http.StatusConflict, fmt.Sprintf("can't listen on %s: %v", d.Listen, err))
	}
	if _, err := s.store.Edit(func(set *access.Settings) error {
		set.Devices = append(set.Devices, dev.cfg)
		return nil
	}); err != nil {
		ln.Close()
		s.drop(dev)
		if swapped {
			s.openBare()
		}
		return d, err
	}

	s.devMu.Lock()
	s.devices = append(s.devices, dev)
	s.devMu.Unlock()
	s.start(dev, ln)
	s.hub.Notify()
	s.log.Info("tuner added", "device", dev.cfg.Name, "id", dev.cfg.ID, "listen", dev.cfg.Listen)
	return dev.cfg, nil
}

// RemoveDevice stops fronting a tuner. Streams on it are cut, and its name is
// dropped from the labels and from any per-device viewer limits so the settings
// stay valid.
func (s *Server) RemoveDevice(name string) error {
	s.fleetMu.Lock()
	defer s.fleetMu.Unlock()
	if err := s.canEditDevices(); err != nil {
		return err
	}
	var dev *device
	for _, d := range s.deviceList() {
		if d.cfg.Name == name {
			dev = d
		}
	}
	if dev == nil {
		return api.NewError(http.StatusNotFound, fmt.Sprintf("no device named %q", name))
	}
	if _, err := s.store.Edit(func(set *access.Settings) error {
		kept := set.Devices[:0:0]
		for _, d := range set.Devices {
			if d.Name != name {
				kept = append(kept, d)
			}
		}
		set.Devices = kept
		delete(set.Labels, name)
		for i := range set.Viewers {
			delete(set.Viewers[i].Limits, name)
			if len(set.Viewers[i].Limits) == 0 {
				set.Viewers[i].Limits = nil
			}
		}
		return nil
	}); err != nil {
		return err
	}

	s.devMu.Lock()
	for i, d := range s.devices {
		if d == dev {
			s.devices = append(s.devices[:i:i], s.devices[i+1:]...)
			break
		}
	}
	s.devMu.Unlock()
	s.drop(dev)
	s.log.Info("tuner removed", "device", name)
	if len(s.deviceList()) == 0 {
		s.openBare() // the admin pages were on that device's port
	}
	s.hub.Notify()
	return nil
}

// SetListen moves a tuner to another port (or address). The new one is opened
// before the old one is let go, so a port that is taken leaves everything as it
// was. Streams on the old port are cut after a moment, and anything that
// connects to it, a viewer's media server for one, has to be pointed at the new
// one.
func (s *Server) SetListen(name, listen string) (access.Device, error) {
	s.fleetMu.Lock()
	defer s.fleetMu.Unlock()
	if err := s.canEditDevices(); err != nil {
		return access.Device{}, err
	}
	var dev *device
	for _, d := range s.deviceList() {
		if d.cfg.Name == name {
			dev = d
		}
	}
	if dev == nil {
		return access.Device{}, api.NewError(http.StatusNotFound, fmt.Sprintf("no device named %q", name))
	}
	listen = strings.TrimSpace(listen)
	if listen != "" && strings.Trim(listen, "0123456789") == "" {
		listen = ":" + listen // just a port number
	}
	if listen == dev.cfg.Listen {
		return dev.cfg, nil
	}
	cfg := dev.cfg
	cfg.Listen = listen
	set := s.policy.Settings()
	for i := range set.Devices {
		if set.Devices[i].Name == name {
			set.Devices[i] = cfg
		}
	}
	if err := set.Validate(); err != nil {
		return dev.cfg, api.NewError(http.StatusBadRequest, err.Error())
	}

	if err := s.adminPortTaken(cfg); err != nil {
		return dev.cfg, api.NewError(http.StatusConflict, err.Error())
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return dev.cfg, api.NewError(http.StatusConflict, fmt.Sprintf("can't listen on %s: %v", listen, err))
	}
	if _, err := s.store.Edit(func(set *access.Settings) error {
		for i := range set.Devices {
			if set.Devices[i].Name == name {
				set.Devices[i].Listen = listen
			}
		}
		return nil
	}); err != nil {
		ln.Close()
		return dev.cfg, err
	}

	old := dev.srv
	s.devMu.Lock()
	dev.cfg.Listen = listen
	s.devMu.Unlock()
	s.serve(dev, ln)
	if old != nil {
		go retire(old) // closes its listener; the reply to this request still gets out
	}
	s.hub.Notify()
	s.log.Info("tuner moved to another port", "device", name, "listen", listen)
	return dev.cfg, nil
}

// drop forgets a device everywhere and, if it was serving, stops it. Streams
// still running are given a moment and then cut, in the background so that a
// request being answered right now (the one removing it, say) still gets its
// reply.
func (s *Server) drop(d *device) {
	s.policy.RemoveDevice(d.cfg.Name)
	s.guide.Unwatch(d.cfg.Name)
	s.mu.Lock()
	delete(s.announced, d.cfg.Name)
	s.mu.Unlock()
	if d.cancel != nil {
		d.cancel()
	}
	if d.ln != nil {
		d.ln.Close()
	}
	if d.srv != nil {
		go retire(d.srv)
	}
}

// retire stops a server: idle connections now, the rest after a second.
func retire(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if srv.Shutdown(ctx) != nil {
		_ = srv.Close()
	}
}

// freeName picks a name for a new device: "tuner" if it is free, then one with
// the DeviceID, then a number.
func (s *Server) freeName(id string) string {
	taken := map[string]bool{}
	for _, d := range s.deviceList() {
		taken[strings.ToLower(d.cfg.Name)] = true
	}
	cands := []string{"tuner"}
	if id != "" {
		cands = append(cands, "tuner-"+strings.ToLower(id))
	}
	for _, c := range cands {
		if !taken[c] {
			return c
		}
	}
	for i := 2; ; i++ {
		if c := "tuner-" + strconv.Itoa(i); !taken[c] {
			return c
		}
	}
}

// nextListen picks a port for a new device. The first one takes over the port
// the admin pages are on, so the page stays where it is; later ones go one past
// the highest in use.
func (s *Server) nextListen() string {
	if s.cfg.AdminSeparate {
		// The admin pages keep their own port, so a tuner never takes it over: start at 5004, and go
		// one past the highest in use, stepping over the admin port.
		port := 5004
		for _, d := range s.deviceList() {
			if _, p, err := net.SplitHostPort(d.cfg.Listen); err == nil {
				if n, err := strconv.Atoi(p); err == nil && n >= port {
					port = n + 1
				}
			}
		}
		for samePort(s.cfg.AdminListen, ":"+strconv.Itoa(port)) {
			port++
		}
		return ":" + strconv.Itoa(port)
	}
	if s.bare != nil {
		if _, port, err := net.SplitHostPort(s.bare.ln.Addr().String()); err == nil {
			return ":" + port
		}
	}
	highest := 0
	for _, d := range s.deviceList() {
		if _, p, err := net.SplitHostPort(d.cfg.Listen); err == nil {
			if n, err := strconv.Atoi(p); err == nil && n > highest {
				highest = n
			}
		}
	}
	if highest == 0 {
		if _, p, err := net.SplitHostPort(s.cfg.AdminListen); err == nil {
			return ":" + p
		}
		return ":5004"
	}
	return ":" + strconv.Itoa(highest+1)
}

// listen binds a device's port. If the admin-only listener has it, that is let
// go first: the device serves the admin pages too. swapped says that happened.
//
// The admin-only listener is let go before the port is tried, not after it fails: whether binding
// ":5004" fails while "127.0.0.1:5004" is held depends on the system (it does on Linux and does not on
// macOS), and where it doesn't, the old listener would be left serving the port, and answering what the
// device should with a 404.
func (s *Server) listen(d access.Device) (ln net.Listener, swapped bool, err error) {
	if ln, ok := s.cfg.Listeners[d.Name]; ok {
		return ln, false, nil
	}
	if err := s.adminPortTaken(d); err != nil {
		return nil, false, err
	}
	if !s.cfg.AdminSeparate && s.bare != nil && samePort(s.bare.ln.Addr().String(), d.Listen) {
		s.stopBare()
		if ln, err = net.Listen("tcp", d.Listen); err == nil {
			return ln, true, nil
		}
		s.openBare()
		return nil, false, err
	}
	ln, err = net.Listen("tcp", d.Listen)
	return ln, false, err
}

// adminPortTaken says why a tuner can't use its port, when the admin pages have a port of their own and
// it is that one. It is nil otherwise.
func (s *Server) adminPortTaken(d access.Device) error {
	if s.cfg.AdminSeparate && samePort(s.cfg.AdminListen, d.Listen) {
		return fmt.Errorf("%s is the port of the admin pages (-admin-listen): give the tuner another", d.Listen)
	}
	return nil
}

func samePort(a, b string) bool {
	_, pa, errA := net.SplitHostPort(a)
	_, pb, errB := net.SplitHostPort(b)
	return errA == nil && errB == nil && pa == pb
}

// startBare serves the admin pages alone on ln. The caller holds fleetMu.
func (s *Server) startBare(ln net.Listener) {
	mux := http.NewServeMux()
	mux.Handle("/", s.admin) // the pages, their API, and everything else a browser asks this port for
	mux.Handle("/auth/", s.plex.Handler())
	mux.Handle(claim.Path, s.claim.Handler())
	mux.HandleFunc("GET "+legacyPrefix, redirectLegacy)
	mux.HandleFunc("GET "+renamedPrefix, redirectRenamed)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	ln = s.gate(ln)
	s.bare = &bareListener{srv: srv, ln: ln}
	s.log.Info("no tuners yet: serving the admin pages so one can be added", "addr", ln.Addr().String())
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			select {
			case s.errc <- fmt.Errorf("admin pages: %w", err):
			default:
			}
		}
	}()
}

// stopBare lets go of the admin-only listener: no new connections now, and the
// ones in use (the page that asked for this) are given a moment to finish.
func (s *Server) stopBare() {
	if s.bare == nil {
		return
	}
	s.bare.ln.Close()
	go retire(s.bare.srv)
	s.bare = nil
}

// openBare serves the admin pages alone again, after the last device went.
func (s *Server) openBare() {
	if s.bare != nil {
		return
	}
	ln, err := net.Listen("tcp", s.cfg.AdminListen)
	if err != nil {
		s.log.Error("no tuners left and the admin pages can't be served; restart Yagi", "addr", s.cfg.AdminListen, "err", err)
		return
	}
	s.startBare(ln)
}
