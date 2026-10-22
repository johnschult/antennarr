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

// adminName is the key in Config.Listeners for the listener that serves the
// admin pages while there are no devices. It exists for tests.
const adminName = "(admin)"

// adminListener serves the admin pages, on a port of their own that no tuner uses.
type adminListener struct {
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

	ln, err := s.listen(d)
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

// nextListen picks a port for a new device: 5004, or one past the highest in use, stepping over the admin
// port, which no tuner uses.
func (s *Server) nextListen() string {
	port := 5004
	for _, d := range s.deviceList() {
		if n, err := strconv.Atoi(portOfListen(d.cfg.Listen)); err == nil && n >= port {
			port = n + 1
		}
	}
	for samePort(s.cfg.AdminListen, ":"+strconv.Itoa(port)) {
		port++
	}
	return ":" + strconv.Itoa(port)
}

// listen binds a device's port. The admin pages' port is never a tuner's.
func (s *Server) listen(d access.Device) (net.Listener, error) {
	if ln, ok := s.cfg.Listeners[d.Name]; ok {
		return ln, nil
	}
	if err := s.adminPortTaken(d); err != nil {
		return nil, err
	}
	return net.Listen("tcp", d.Listen)
}

// adminPortTaken says why a tuner can't use its port, when it is the admin pages'. It is nil otherwise.
func (s *Server) adminPortTaken(d access.Device) error {
	if samePort(s.cfg.AdminListen, d.Listen) {
		return fmt.Errorf("%s is the port of the admin pages (%s): give the tuner another", d.Listen, s.cfg.AdminListen)
	}
	return nil
}

func samePort(a, b string) bool {
	_, pa, errA := net.SplitHostPort(a)
	_, pb, errB := net.SplitHostPort(b)
	return errA == nil && errB == nil && pa == pb
}

// adminMux is the admin pages on their own: what a port with no tuner on it answers.
func (s *Server) adminMux() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", s.admin) // the pages, their API, and everything else a browser asks this port for
	mux.Handle("/auth/", s.plex.Handler())
	mux.Handle(claim.Path, s.claim.Handler())
	mux.HandleFunc("GET "+legacyPrefix, redirectLegacy)
	mux.HandleFunc("GET "+renamedPrefix, redirectRenamed)
	return mux
}

// startAdmin serves the admin pages on ln. The caller holds fleetMu.
func (s *Server) startAdmin(ln net.Listener) {
	srv := &http.Server{Handler: s.adminMux(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	ln = s.gate(ln)
	s.adminLn = &adminListener{srv: srv, ln: ln}
	s.log.Info("serving the admin pages", "addr", ln.Addr().String())
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			select {
			case s.errc <- fmt.Errorf("admin pages: %w", err):
			default:
			}
		}
	}()
}
