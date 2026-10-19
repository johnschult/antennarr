// Package restart restarts a tuner when an admin asks, and checks that it did: sends "set /sys/restart
// self" (the way SiliconDust documents for hdhomerun_config), watches for the tuner to go away, and waits
// for it to come back. It runs each restart in the background, one per tuner, and keeps what is
// happening for the page to show. It never repeats a restart by itself.
package restart

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/johnschult/yagi/internal/hdhr"
	"github.com/johnschult/yagi/internal/tuner"
)

// Phase is how far a restart has got.
type Phase string

const (
	PhaseRestarting Phase = "restarting" // told to restart; waiting for it to go away and come back
	PhaseDone       Phase = "done"
	PhaseFailed     Phase = "failed"
)

// Status is what the page shows about one tuner's restart.
type Status struct {
	Phase    Phase     `json:"phase"`
	Error    string    `json:"error,omitempty"` // why it failed, in words for a person
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitempty"`
}

// Tuner is what a restart needs from the tuner's monitor.
type Tuner interface {
	SetRestarting(on bool)
	Note(kind, text string)
	Kick()
}

var _ Tuner = (*tuner.Monitor)(nil)

// Request is one restart.
type Request struct {
	Device string // the device's name in Yagi
	Title  string // what it is called to a person, for the timeline
	Host   string // the tuner's address, for its control port
	Tuner  Tuner
}

// Config for a Manager.
type Config struct {
	// Dial opens a control connection to a tuner's host; nil means its control port.
	Dial func(ctx context.Context, host string) (*hdhr.Conn, error)
	// OnChange is called when a status changes, so the page can be told. It must not block.
	OnChange func()
	Log      *slog.Logger
	Now      func() time.Time

	// GoneWithin is how long to wait to see the tuner stop answering after it was told to restart: if it
	// never does, it didn't restart. BackWithin is how long to wait for it to answer again. PollEvery is
	// how often to ask.
	GoneWithin time.Duration
	BackWithin time.Duration
	PollEvery  time.Duration
}

// ErrBusy is returned when a restart is already under way on that tuner.
var ErrBusy = errors.New("this tuner is already restarting")

// Manager runs restarts.
type Manager struct {
	cfg Config
	log *slog.Logger
	ctx context.Context
	wg  sync.WaitGroup

	mu       sync.Mutex
	statuses map[string]*Status
}

// New makes a Manager. Restarts run until ctx ends.
func New(ctx context.Context, cfg Config) *Manager {
	if cfg.Dial == nil {
		cfg.Dial = func(ctx context.Context, host string) (*hdhr.Conn, error) {
			return hdhr.Dial(ctx, net.JoinHostPort(host, hdhr.ControlPort))
		}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.GoneWithin == 0 {
		cfg.GoneWithin = 30 * time.Second
	}
	if cfg.BackWithin == 0 {
		cfg.BackWithin = 5 * time.Minute
	}
	if cfg.PollEvery == 0 {
		cfg.PollEvery = 500 * time.Millisecond
	}
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	return &Manager{cfg: cfg, log: log, ctx: ctx, statuses: map[string]*Status{}}
}

// Status is what is known about a tuner's latest restart, or nil if there has been none.
func (m *Manager) Status(device string) *Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.statuses[device]; ok {
		c := *s
		return &c
	}
	return nil
}

// Active says whether a restart is under way on the tuner.
func (m *Manager) Active(device string) bool {
	s := m.Status(device)
	return s != nil && s.Phase == PhaseRestarting
}

// Forget drops a finished restart's status, so the page stops showing it.
func (m *Manager) Forget(device string) {
	m.mu.Lock()
	if s, ok := m.statuses[device]; ok && s.Phase != PhaseRestarting {
		delete(m.statuses, device)
	}
	m.mu.Unlock()
	m.changed()
}

// Wait blocks until every restart has finished. It is for tests.
func (m *Manager) Wait() { m.wg.Wait() }

func (m *Manager) changed() {
	if m.cfg.OnChange != nil {
		m.cfg.OnChange()
	}
}

func (m *Manager) finish(device string, phase Phase, errText string) {
	m.mu.Lock()
	if s, ok := m.statuses[device]; ok {
		s.Phase, s.Error, s.Finished = phase, errText, m.cfg.Now()
	}
	m.mu.Unlock()
	m.changed()
}

// Start begins a restart in the background. Whether it is allowed at all is for the caller to have
// decided; this only refuses to run two at once on a tuner.
func (m *Manager) Start(r Request) error {
	if r.Host == "" || r.Tuner == nil {
		return errors.New("restart: no tuner to restart")
	}
	m.mu.Lock()
	if s, ok := m.statuses[r.Device]; ok && s.Phase == PhaseRestarting {
		m.mu.Unlock()
		return ErrBusy
	}
	m.statuses[r.Device] = &Status{Phase: PhaseRestarting, Started: m.cfg.Now()}
	m.mu.Unlock()
	m.changed()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.run(m.ctx, r)
	}()
	return nil
}

func (m *Manager) fail(r Request, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	m.log.Warn("tuner restart failed", "device", r.Device, "reason", msg)
	r.Tuner.Note("err", fmt.Sprintf("Restarting %s failed: %s", r.Title, msg))
	m.finish(r.Device, PhaseFailed, msg)
}

func (m *Manager) run(ctx context.Context, r Request) {
	r.Tuner.Note("info", fmt.Sprintf("Restarting %s", r.Title))
	m.log.Info("tuner restart starting", "device", r.Device)

	// 1. Ask. Nothing has been done to the tuner until this goes out, so failing to reach it costs nothing.
	conn, err := m.cfg.Dial(ctx, r.Host)
	if err != nil {
		m.fail(r, "%v. The tuner has not been touched.", err)
		return
	}
	// From here the tuner goes away and comes back, which is not an outage.
	r.Tuner.SetRestarting(true)
	defer r.Tuner.SetRestarting(false)

	dctx, cancel := context.WithTimeout(ctx, hdhr.ReplyTimeout*2)
	_, err = conn.Set(dctx, "/sys/restart", "self")
	cancel()
	conn.Close()
	var refused *hdhr.Error
	if errors.As(err, &refused) {
		m.fail(r, "the tuner refused (%s). It has not restarted.", refused.Message)
		return
	}
	// Any other end to the request (including no answer: a tuner that restarts at once doesn't give
	// one) is not a failure by itself. What the tuner does next is what says.

	// 2. It should stop answering. If it never does, it did not restart.
	if !m.waitFor(ctx, r.Host, false, m.cfg.GoneWithin) {
		if ctx.Err() != nil {
			m.fail(r, "stopped while waiting for the tuner to restart.")
			return
		}
		m.fail(r, "the tuner kept answering, so it does not look as if it restarted.")
		return
	}
	r.Tuner.Note("info", fmt.Sprintf("%s is restarting", r.Title))

	// 3. It should come back.
	if !m.waitFor(ctx, r.Host, true, m.cfg.BackWithin) {
		if ctx.Err() != nil {
			m.fail(r, "stopped while waiting for the tuner to come back.")
			return
		}
		m.fail(r, "the tuner has not come back after %s. Check that it has power and a network cable, and unplug it for ten seconds if it hasn't.", m.cfg.BackWithin.Round(time.Second))
		return
	}
	r.Tuner.Kick() // let the monitor see it too
	m.log.Info("tuner restarted", "device", r.Device)
	r.Tuner.Note("ok", fmt.Sprintf("%s restarted", r.Title))
	m.finish(r.Device, PhaseDone, "")
}

// answers says whether the tuner answers a question over its control port right now.
func (m *Manager) answers(ctx context.Context, host string) bool {
	ctx, cancel := context.WithTimeout(ctx, hdhr.ReplyTimeout*2)
	defer cancel()
	c, err := m.cfg.Dial(ctx, host)
	if err != nil {
		return false
	}
	defer c.Close()
	_, err = c.Get(ctx, "/sys/version")
	return err == nil
}

// waitFor polls until the tuner's answering is as wanted (up, or down), or the time is up.
func (m *Manager) waitFor(ctx context.Context, host string, up bool, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		if m.answers(ctx, host) == up {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(m.cfg.PollEvery):
		}
	}
	return false
}
