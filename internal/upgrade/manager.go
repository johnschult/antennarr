// Package upgrade installs firmware on a tuner: it downloads SiliconDust's image, sends it to the tuner,
// waits for the tuner to restart, and checks that it came back running the new version. It runs each
// install in the background, one per tuner, and keeps what is happening for the page to show.
//
// It never retries sending an image by itself. A flash that has gone wrong is for a person to look at.
package upgrade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/johnschult/antennarr/internal/firmware"
	"github.com/johnschult/antennarr/internal/hdhr"
	"github.com/johnschult/antennarr/internal/tuner"
)

// Phase is how far an install has got.
type Phase string

const (
	PhaseDownloading Phase = "downloading" // fetching the image from SiliconDust; the tuner has not been touched
	PhaseSending     Phase = "sending"     // sending the image to the tuner
	PhaseRestarting  Phase = "restarting"  // the tuner has the image and is installing it and restarting
	PhaseVerifying   Phase = "verifying"   // the tuner has answered again: checking what it runs
	PhaseDone        Phase = "done"
	PhaseFailed      Phase = "failed"
)

// Active says whether an install is under way.
func (p Phase) Active() bool {
	return p == PhaseDownloading || p == PhaseSending || p == PhaseRestarting || p == PhaseVerifying
}

// Status is what the page shows about one tuner's install.
type Status struct {
	Phase    Phase     `json:"phase"`
	Release  string    `json:"release"`          // the version being installed
	From     string    `json:"from"`             // what the tuner ran before
	Percent  int       `json:"percent"`          // of the image sent, while sending
	Error    string    `json:"error,omitempty"`  // why it failed, in words for a person
	SHA256   string    `json:"sha256,omitempty"` // of the image that was downloaded
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitempty"`
}

// Tuner is what an install needs from the tuner's monitor.
type Tuner interface {
	SetUpdating(on bool)
	Note(kind, text string)
	Kick()
	Snapshot() tuner.State
}

// Request is one install.
type Request struct {
	Device  string // the device's name in Antennarr
	Title   string // what it is called to a person, for the timeline
	Host    string // the tuner's address, for its control port
	Family  string // the firmware family the tuner reports (hdhomerun_dvr_atsc3)
	Current string // what it runs now
	Release string // what to install
	Tuner   Tuner
}

// Downloader fetches an image.
type Downloader interface {
	Fetch(ctx context.Context, family, release string) (firmware.Image, error)
}

// Config for a Manager.
type Config struct {
	Download Downloader
	// Dial opens a control connection to a tuner's host; nil means its control port. Tests and the demo
	// use their own.
	Dial func(ctx context.Context, host string) (*hdhr.Conn, error)
	// OnChange is called when a status changes, so the page can be told. It must not block.
	OnChange func()
	Log      *slog.Logger
	Now      func() time.Time

	// RestartWait is how long to leave the tuner to install and restart before asking it anything, as
	// hdhomerun_config does (ten seconds). BackWithin is how long to wait for it to answer again, running
	// the new version. PollEvery is how often to ask. Deadline is the longest the whole install may take.
	RestartWait time.Duration
	BackWithin  time.Duration
	PollEvery   time.Duration
	Deadline    time.Duration
}

// ErrBusy is returned when an install is already under way on that tuner.
var ErrBusy = errors.New("an update is already under way on this tuner")

// Manager runs installs.
type Manager struct {
	cfg Config
	log *slog.Logger
	ctx context.Context
	wg  sync.WaitGroup

	mu       sync.Mutex
	statuses map[string]*Status
}

// New makes a Manager. Installs run until ctx ends, which stops any that is still downloading or sending.
func New(ctx context.Context, cfg Config) *Manager {
	if cfg.Download == nil {
		cfg.Download = firmware.Downloader{}
	}
	if cfg.Dial == nil {
		cfg.Dial = func(ctx context.Context, host string) (*hdhr.Conn, error) {
			return hdhr.Dial(ctx, net.JoinHostPort(host, hdhr.ControlPort))
		}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.RestartWait == 0 {
		cfg.RestartWait = 10 * time.Second
	}
	if cfg.BackWithin == 0 {
		cfg.BackWithin = 5 * time.Minute
	}
	if cfg.PollEvery == 0 {
		cfg.PollEvery = time.Second
	}
	if cfg.Deadline == 0 {
		cfg.Deadline = 10 * time.Minute
	}
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	return &Manager{cfg: cfg, log: log, ctx: ctx, statuses: map[string]*Status{}}
}

// Status is what is known about a tuner's latest install, or nil if there has been none.
func (m *Manager) Status(device string) *Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.statuses[device]; ok {
		c := *s
		return &c
	}
	return nil
}

// Active says whether an install is under way on the tuner.
func (m *Manager) Active(device string) bool {
	s := m.Status(device)
	return s != nil && s.Phase.Active()
}

// Forget drops a finished install's status, so the page stops showing it.
func (m *Manager) Forget(device string) {
	m.mu.Lock()
	if s, ok := m.statuses[device]; ok && !s.Phase.Active() {
		delete(m.statuses, device)
	}
	m.mu.Unlock()
	m.changed()
}

// Wait blocks until every install has finished. It is for tests.
func (m *Manager) Wait() { m.wg.Wait() }

func (m *Manager) changed() {
	if m.cfg.OnChange != nil {
		m.cfg.OnChange()
	}
}

// update changes a tuner's status.
func (m *Manager) update(device string, f func(*Status)) {
	m.mu.Lock()
	if s, ok := m.statuses[device]; ok {
		f(s)
	}
	m.mu.Unlock()
	m.changed()
}

// Start begins an install in the background. Whether the install is allowed at all is for the caller to
// have decided; this only refuses to run two at once on a tuner.
func (m *Manager) Start(r Request) error {
	if r.Host == "" || r.Tuner == nil {
		return errors.New("upgrade: no tuner to update")
	}
	m.mu.Lock()
	if s, ok := m.statuses[r.Device]; ok && s.Phase.Active() {
		m.mu.Unlock()
		return ErrBusy
	}
	m.statuses[r.Device] = &Status{Phase: PhaseDownloading, Release: r.Release, From: r.Current, Started: m.cfg.Now()}
	m.mu.Unlock()
	m.changed()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ctx, cancel := context.WithTimeout(m.ctx, m.cfg.Deadline)
		defer cancel()
		m.run(ctx, r)
	}()
	return nil
}

func (m *Manager) fail(r Request, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	m.log.Warn("firmware update failed", "device", r.Device, "release", r.Release, "reason", msg)
	r.Tuner.Note("err", fmt.Sprintf("Firmware update to %s failed: %s", r.Release, msg))
	m.update(r.Device, func(s *Status) { s.Phase, s.Error, s.Finished = PhaseFailed, msg, m.cfg.Now() })
}

func (m *Manager) run(ctx context.Context, r Request) {
	log := m.log.With("device", r.Device, "from", r.Current, "release", r.Release)
	r.Tuner.Note("info", fmt.Sprintf("Updating %s firmware from %s to %s", r.Title, r.Current, r.Release))
	log.Info("firmware update starting")

	// 1. Download. Nothing has been done to the tuner yet, so a failure here costs nothing.
	img, err := m.cfg.Download.Fetch(ctx, r.Family, r.Release)
	if err != nil {
		if errors.Is(err, firmware.ErrNotFound) {
			m.fail(r, "SiliconDust has no firmware file for %s %s to download. The tuner has not been changed.", r.Family, r.Release)
		} else {
			m.fail(r, "%v. The tuner has not been changed.", err)
		}
		return
	}
	log.Info("firmware downloaded", "bytes", len(img.Data), "sha256", img.SHA256)
	m.update(r.Device, func(s *Status) { s.Phase, s.SHA256, s.Percent = PhaseSending, img.SHA256, 0 })

	// From here the tuner goes away and comes back, which is not an outage.
	r.Tuner.SetUpdating(true)
	defer r.Tuner.SetUpdating(false)

	// 2. Send it.
	conn, err := m.cfg.Dial(ctx, r.Host)
	if err != nil {
		m.fail(r, "%v. The tuner has not been changed.", err)
		return
	}
	lastPct := -1
	err = conn.Upgrade(ctx, img.Data, func(sent, total int) {
		if pct := hdhr.Percent(sent, total); pct != lastPct {
			lastPct = pct
			m.update(r.Device, func(s *Status) { s.Percent = pct })
		}
	})
	conn.Close()
	var refused *hdhr.Error
	switch {
	case err == nil, errors.Is(err, hdhr.ErrNoReply):
		// Sent. A tuner that is already restarting doesn't answer, which is why that is not a failure.
	case errors.As(err, &refused):
		m.fail(r, "the tuner refused the firmware (%s). It is still running %s.", refused.Message, m.runningNow(ctx, r))
		return
	case ctx.Err() != nil && conn.EndSent():
		// Stopped after the tuner was told to install: it may be doing that now.
		m.fail(r, "the update was stopped after the firmware had been sent, so the tuner may be installing it. Give it five minutes, then check what it is running.")
		return
	case ctx.Err() != nil:
		m.fail(r, "the update was stopped before it finished sending: nothing was installed. The tuner is running %s.", m.runningNow(context.WithoutCancel(ctx), r))
		return
	default:
		m.fail(r, "sending the firmware didn't finish (%v). A tuner discards an image it didn't get whole, so it should still be running %s.", err, m.runningNow(ctx, r))
		return
	}
	log.Info("firmware sent")
	r.Tuner.Note("info", fmt.Sprintf("Firmware %s sent to %s; it is installing it and restarting", r.Release, r.Title))
	m.update(r.Device, func(s *Status) { s.Phase, s.Percent = PhaseRestarting, 100 })

	// 3. Leave it to install and restart (as hdhomerun_config does), then wait for it to say what it runs.
	if !sleep(ctx, m.cfg.RestartWait) {
		m.fail(r, "stopped while the tuner was restarting. Check what it is running.")
		return
	}
	version, err := m.waitForRelease(ctx, r)
	if err != nil {
		m.fail(r, "%v", err)
		return
	}
	m.update(r.Device, func(s *Status) { s.Phase = PhaseVerifying })
	r.Tuner.Kick() // let the monitor see it too
	log.Info("firmware installed", "version", version)
	r.Tuner.Note("ok", fmt.Sprintf("%s is now running firmware %s", r.Title, version))
	m.update(r.Device, func(s *Status) { s.Phase, s.Finished = PhaseDone, m.cfg.Now() })
}

// waitForRelease asks the tuner what it runs until it says it is the new release.
func (m *Manager) waitForRelease(ctx context.Context, r Request) (string, error) {
	deadline := m.cfg.Now().Add(m.cfg.BackWithin)
	var last string // what it last said it runs
	answered := false
	for m.cfg.Now().Before(deadline) {
		v, err := m.version(ctx, r.Host)
		switch {
		case err == nil && v == r.Release:
			return v, nil
		case err == nil:
			answered, last = true, v
		}
		if !sleep(ctx, m.cfg.PollEvery) {
			return "", errors.New("stopped while waiting for the tuner to come back. Check what it is running.")
		}
	}
	if answered {
		return "", fmt.Errorf("the tuner is back but still running %s, not %s: it did not take the update. Nothing more has been tried.", last, r.Release)
	}
	return "", fmt.Errorf("the tuner has not come back after %s. Give it a few more minutes, then unplug it for ten seconds and plug it in again.", m.cfg.BackWithin.Round(time.Second))
}

// runningNow says what the tuner reports, for a failure message; "an unknown version" if it won't say.
func (m *Manager) runningNow(ctx context.Context, r Request) string {
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for end := m.cfg.Now().Add(10 * time.Second); m.cfg.Now().Before(end); {
		if v, err := m.version(cctx, r.Host); err == nil && v != "" {
			return v
		}
		if !sleep(cctx, m.cfg.PollEvery) {
			break
		}
	}
	return "an unknown version"
}

func (m *Manager) version(ctx context.Context, host string) (string, error) {
	c, err := m.cfg.Dial(ctx, host)
	if err != nil {
		return "", err
	}
	defer c.Close()
	return c.Get(ctx, "/sys/version")
}

// sleep waits, and says whether the whole wait passed (false if ctx ended first).
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
