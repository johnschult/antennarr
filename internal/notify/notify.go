// Package notify tells the owner about things worth knowing (new tuner firmware,
// a tuner going offline, a viewer being refused) through the channels they set up:
// ntfy, Pushover or a webhook.
//
// Nothing is contacted until there is a channel, and each kind of event can be
// switched off. The settings are read on every event, so changing them needs no
// restart. Sending never blocks the code that noticed the event: events go onto a
// small queue and one goroutine sends them.
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/johnschult/yagi/internal/access"
)

// Event is something to tell the owner.
type Event struct {
	// Kind is one of access.NotifyFirmware, NotifyTuner, NotifyViewer, NotifyUnknown, NotifyWatching, NotifyRequest.
	Kind string
	// Device is the tuner's name, when the event is about one.
	Device  string
	Title   string
	Message string
	Time    time.Time
}

// Config is how a Notifier is built. Only Settings is required.
type Config struct {
	// Settings returns the current settings, which hold the channels and which
	// events are on.
	Settings func() access.Settings
	// Client sends the requests. Nil means NewClient().
	Client *http.Client
	Log    *slog.Logger
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// PushoverURL is where Pushover messages go; empty means Pushover's own.
	PushoverURL string
	// StatePath is a file that remembers which firmware release each tuner was
	// last told about, so a restart doesn't tell the owner again. Empty keeps it
	// in memory only.
	StatePath string
	// OfflineGrace is how long a tuner must stay offline before the owner is
	// told, so a reboot or a brief drop isn't a message. Zero means two minutes.
	OfflineGrace time.Duration
	// After runs f after d and returns a way to cancel it; nil means time.AfterFunc.
	// Tests replace it.
	After func(d time.Duration, f func()) (stop func() bool)
}

const (
	queueSize       = 32
	defaultGrace    = 2 * time.Minute
	refusedEvery    = 30 * time.Minute // the same viewer refused for the same reason on the same tuner
	startedEvery    = 30 * time.Minute // the same viewer starting to watch on the same tuner
	unknownEvery    = 24 * time.Hour   // the same unknown address
	unknownPerHour  = 6                // unknown addresses in all, however many there are
	cooldownKeepMax = 1000             // entries in the cooldown table before old ones are dropped
	unknownWindow   = time.Hour
	TestTitle       = "Yagi test"
	testMessage     = "If you can read this, notifications from Yagi are working."
)

// testKind is the kind of the message the Test button sends. It is not one the owner can switch off.
const testKind = "test"

// Notifier sends events to the channels in the settings.
type Notifier struct {
	settings    func() access.Settings
	client      *http.Client
	log         *slog.Logger
	now         func() time.Time
	pushoverURL string
	statePath   string
	grace       time.Duration
	after       func(d time.Duration, f func()) (stop func() bool)

	queue chan job

	mu       sync.Mutex
	known    map[string]string    // device -> the firmware release the owner was last told about
	last     map[string]time.Time // when each rate-limited event was last let through
	unknown  []time.Time          // when unknown addresses were let through, within the last hour
	offline  map[string]*offlineState
	shutdown bool
}

type job struct {
	ev       Event
	channels []access.NotifyChannel
}

// offlineState is one tuner's offline notice: waiting out the grace period, or already sent.
type offlineState struct {
	stop func() bool // cancels the wait; nil when it is not waiting
	gen  int         // which wait this is, so one that was canceled as it fired can tell
	sent bool        // the owner was told it is offline, so should be told when it is back
}

// New makes a Notifier. Call Run to start sending.
func New(cfg Config) *Notifier {
	n := &Notifier{
		settings:    cfg.Settings,
		client:      cfg.Client,
		log:         cfg.Log,
		now:         cfg.Now,
		pushoverURL: cfg.PushoverURL,
		statePath:   cfg.StatePath,
		grace:       cfg.OfflineGrace,
		after:       cfg.After,
		queue:       make(chan job, queueSize),
		last:        map[string]time.Time{},
		offline:     map[string]*offlineState{},
		known:       map[string]string{},
	}
	if n.client == nil {
		n.client = NewClient()
	}
	if n.log == nil {
		n.log = slog.Default()
	}
	if n.now == nil {
		n.now = time.Now
	}
	if n.pushoverURL == "" {
		n.pushoverURL = DefaultPushoverURL
	}
	if n.grace <= 0 {
		n.grace = defaultGrace
	}
	if n.after == nil {
		n.after = func(d time.Duration, f func()) func() bool { return time.AfterFunc(d, f).Stop }
	}
	n.load()
	return n
}

// load reads which firmware releases were told about. A missing or unreadable
// file just means telling again, so it is not an error.
func (n *Notifier) load() {
	if n.statePath == "" {
		return
	}
	b, err := os.ReadFile(n.statePath)
	if err != nil {
		return
	}
	var m map[string]string
	if json.Unmarshal(b, &m) == nil {
		n.known = m
	}
}

// save writes what was told about, atomically and readable only by the owner.
// n.mu must be held.
func (n *Notifier) save() {
	if n.statePath == "" {
		return
	}
	b, _ := json.MarshalIndent(n.known, "", "  ")
	tmp, err := os.CreateTemp(filepath.Dir(n.statePath), ".yagi-notified-*")
	if err != nil {
		n.log.Warn("couldn't remember which firmware was announced", "err", err)
		return
	}
	defer os.Remove(tmp.Name())
	_, werr := tmp.Write(append(b, '\n'))
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Chmod(tmp.Name(), 0o600) != nil || os.Rename(tmp.Name(), n.statePath) != nil {
		n.log.Warn("couldn't remember which firmware was announced", "path", n.statePath)
	}
}

// Run sends queued events until ctx ends.
func (n *Notifier) Run(ctx context.Context) {
	defer n.stopWaiting()
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-n.queue:
			n.deliver(ctx, j)
		}
	}
}

// deliver sends one event to each channel it was queued for. A channel that fails
// is logged and doesn't hold up the others. Nothing is retried: a message that
// arrives late and out of order is worse than one that doesn't.
func (n *Notifier) deliver(ctx context.Context, j job) {
	for _, ch := range j.channels {
		cctx, cancel := context.WithTimeout(ctx, sendTimeout)
		err := n.send(cctx, ch, j.ev)
		cancel()
		if err != nil && ctx.Err() == nil {
			n.log.Warn("notification not sent", "channel", ch.Name, "type", ch.Type, "err", err)
		}
	}
}

// Test sends one message to a channel, whatever the event settings, and says why not if it can't.
func (n *Notifier) Test(ctx context.Context, ch access.NotifyChannel) error {
	if err := ch.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	return n.send(ctx, ch, Event{Kind: testKind, Title: TestTitle, Message: testMessage, Time: n.now()})
}

// Send queues an event for every channel, if channels are set up and this kind of
// event is on. It returns at once. If the queue is full the event is dropped.
func (n *Notifier) Send(ev Event) { n.queueEvent(ev) }

// queueEvent is Send, saying whether the event was queued.
func (n *Notifier) queueEvent(ev Event) bool {
	set := n.settings()
	channels := set.NotifyChannels()
	if len(channels) == 0 || !set.NotifyEnabled(ev.Kind) {
		return false
	}
	if ev.Time.IsZero() {
		ev.Time = n.now()
	}
	select {
	case n.queue <- job{ev: ev, channels: channels}:
		return true
	default:
		n.log.Warn("notification dropped: too many waiting", "kind", ev.Kind)
		return false
	}
}

// allow lets an event through if the same key was not let through within every,
// and records that it was.
func (n *Notifier) allow(key string, every time.Duration) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.allowLocked(key, every)
}

func (n *Notifier) allowLocked(key string, every time.Duration) bool {
	now := n.now()
	if t, ok := n.last[key]; ok && now.Sub(t) < every {
		return false
	}
	if len(n.last) >= cooldownKeepMax { // forget what has expired, so the table can't grow without end
		for k, t := range n.last {
			if now.Sub(t) >= 24*time.Hour {
				delete(n.last, k)
			}
		}
	}
	n.last[key] = now
	return true
}

// FirmwareFound is newer firmware for a tuner. A release is only told about once
// per tuner, even across restarts. It counts as told once it is queued: if there
// were no channels or the event was off, the owner hears of it when the next
// release comes, not at every restart.
func (n *Notifier) FirmwareFound(device, title, version, text string) {
	n.mu.Lock()
	if n.known[device] == version {
		n.mu.Unlock()
		return
	}
	n.mu.Unlock()
	if !n.queueEvent(Event{Kind: access.NotifyFirmware, Device: device, Title: "New firmware for " + title, Message: text}) {
		return
	}
	n.mu.Lock()
	n.known[device] = version
	n.save()
	n.mu.Unlock()
}

// TunerState is called when a tuner is seen going offline or coming online. Going
// offline is only reported if it is still offline after the grace period, and
// coming online only if the owner was told it was offline, so a restart of
// Yagi, a first sighting, or a blip says nothing.
func (n *Notifier) TunerState(device, title string, online bool) {
	n.mu.Lock()
	if n.shutdown {
		n.mu.Unlock()
		return
	}
	st := n.offline[device]
	if st == nil {
		st = &offlineState{}
		n.offline[device] = st
	}
	if !online {
		if st.stop != nil || st.sent {
			n.mu.Unlock()
			return
		}
		st.gen++
		gen := st.gen
		st.stop = n.after(n.grace, func() {
			n.mu.Lock()
			// A newer wait, or none, means this one was canceled just as it fired.
			if st.stop == nil || st.gen != gen {
				n.mu.Unlock()
				return
			}
			st.stop = nil
			st.sent = true
			n.mu.Unlock()
			n.Send(Event{
				Kind: access.NotifyTuner, Device: device, Title: title + " is offline",
				Message: fmt.Sprintf("Yagi hasn't been able to reach it for %s. It will keep looking.", roundGrace(n.grace)),
			})
		})
		n.mu.Unlock()
		return
	}
	canceled := false
	if st.stop != nil {
		st.stop()
		st.stop = nil
		canceled = true
	}
	wasSent := st.sent
	st.sent = false
	n.mu.Unlock()
	if !canceled && wasSent {
		n.Send(Event{Kind: access.NotifyTuner, Device: device, Title: title + " is back online", Message: "Yagi can reach it again."})
	}
}

func roundGrace(d time.Duration) string {
	m := int(d / time.Minute)
	switch {
	case d == time.Minute:
		return "a minute"
	case m >= 2 && d%time.Minute == 0:
		return fmt.Sprintf("%d minutes", m)
	}
	return d.Round(time.Second).String()
}

// stopWaiting cancels any offline notice still waiting out its grace period.
func (n *Notifier) stopWaiting() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.shutdown = true
	for _, st := range n.offline {
		if st.stop != nil {
			st.stop()
			st.stop = nil
		}
	}
}

// Refused is a viewer turned away because of a limit. A tuner that is offline is
// not reported here (the tuner events cover it), and the same refusal is only
// reported once in a while, since a media server retries in a tight loop.
func (n *Notifier) Refused(device, title, viewer, reason string) {
	var why string
	switch reason {
	case access.ReasonViewerLimit:
		why = "is already using all the tuners they are allowed"
	case access.ReasonSharedCap:
		why = "found the tuners viewers share all in use"
	case access.ReasonOwnerReserve:
		why = "was turned away because the rest of the tuners are kept for you"
	default:
		return
	}
	if !n.settings().NotifyEnabled(access.NotifyViewer) || !n.allow("refused|"+device+"|"+viewer+"|"+reason, refusedEvery) {
		return
	}
	n.Send(Event{
		Kind: access.NotifyViewer, Device: device, Title: viewer + " was refused a tuner",
		Message: fmt.Sprintf("%s asked for a tuner on %s and %s.", viewer, title, why),
	})
}

// Requested is someone who isn't a viewer asking to use a tuner for the first time: a friend who has just added Yagi
// in their media server. It is on unless the owner switched it off, and it is only said when they are first seen.
func (n *Notifier) Requested(device, title, who string) {
	if !n.settings().NotifyEnabled(access.NotifyRequest) || !n.allow("requested|"+who, time.Hour) {
		return
	}
	n.Send(Event{
		Kind: access.NotifyRequest, Device: device, Title: who + " asked to use a tuner",
		Message: fmt.Sprintf("%s asked to use %s and isn't a viewer yet. Open the Viewers page to allow them.", who, title),
	})
}

// Started is a viewer given a tuner. It is off unless the owner switched it on, and a viewer is only reported once
// in a while per tuner, since flipping channels, or a media server that connects again, makes a stream each time.
func (n *Notifier) Started(device, title, viewer, channel string) {
	if !n.settings().NotifyEnabled(access.NotifyWatching) || !n.allow("started|"+device+"|"+viewer, startedEvery) {
		return
	}
	what := "started watching"
	if channel != "" {
		what += " channel " + channel
	}
	n.Send(Event{
		Kind: access.NotifyWatching, Device: device, Title: viewer + " started watching",
		Message: fmt.Sprintf("%s %s on %s.", viewer, what, title),
	})
}

// Unknown is an address that isn't a viewer asking to use a tuner. It is the
// noisiest event, because a tuner port that can be reached from the internet is
// scanned all day, so it is limited twice: each address is reported once a day, and
// no more than a few addresses an hour in all.
func (n *Notifier) Unknown(device, title, addr string) {
	if !n.settings().NotifyEnabled(access.NotifyUnknown) {
		return
	}
	n.mu.Lock()
	now := n.now()
	kept := n.unknown[:0]
	for _, t := range n.unknown {
		if now.Sub(t) < unknownWindow {
			kept = append(kept, t)
		}
	}
	n.unknown = kept
	// Over the hourly limit: say nothing, and don't use up this address's turn, so it is reported later.
	if len(n.unknown) >= unknownPerHour || !n.allowLocked("unknown|"+addr, unknownEvery) {
		n.mu.Unlock()
		return
	}
	n.unknown = append(n.unknown, now)
	n.mu.Unlock()
	n.Send(Event{
		Kind: access.NotifyUnknown, Device: device, Title: "An unknown address was refused",
		Message: fmt.Sprintf("%s tried to use %s and isn't a viewer.", addr, title),
	})
}
