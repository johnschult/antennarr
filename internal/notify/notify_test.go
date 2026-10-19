package notify

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/access"
)

// ---- fakes ----

// hit is one request a fake server received.
type hit struct {
	path   string
	query  string
	header http.Header
	body   string
}

// server records what it is sent. A test must never reach a real service.
type server struct {
	*httptest.Server
	hits   chan hit
	status int
	reply  string
}

func newServer(t *testing.T) *server {
	t.Helper()
	s := &server{hits: make(chan hit, 64), status: 200}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.hits <- hit{path: r.URL.Path, query: r.URL.RawQuery, header: r.Header.Clone(), body: string(b)}
		w.WriteHeader(s.status)
		io.WriteString(w, s.reply)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *server) next(t *testing.T) hit {
	t.Helper()
	select {
	case h := <-s.hits:
		return h
	case <-time.After(3 * time.Second):
		t.Fatal("nothing was sent")
		return hit{}
	}
}

func (s *server) none(t *testing.T) {
	t.Helper()
	select {
	case h := <-s.hits:
		t.Fatalf("something was sent: %+v", h)
	case <-time.After(150 * time.Millisecond):
	}
}

// clock is a time the test moves.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// timers runs the offline grace period by hand.
type timers struct {
	mu      sync.Mutex
	pending map[int]func()
	next    int
}

func (ts *timers) after(d time.Duration, f func()) func() bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.pending == nil {
		ts.pending = map[int]func(){}
	}
	ts.next++
	id := ts.next
	ts.pending[id] = f
	return func() bool {
		ts.mu.Lock()
		defer ts.mu.Unlock()
		_, ok := ts.pending[id]
		delete(ts.pending, id)
		return ok
	}
}

func (ts *timers) count() int { ts.mu.Lock(); defer ts.mu.Unlock(); return len(ts.pending) }

// fire runs every wait that has not been canceled.
func (ts *timers) fire() {
	ts.mu.Lock()
	var fs []func()
	for id, f := range ts.pending {
		fs = append(fs, f)
		delete(ts.pending, id)
	}
	ts.mu.Unlock()
	for _, f := range fs {
		f()
	}
}

type rig struct {
	n      *Notifier
	set    *access.Settings
	clock  *clock
	timers *timers
	srv    *server
}

func ntfyChannel(srv *server) access.NotifyChannel {
	return access.NotifyChannel{ID: "phone", Name: "Phone", Type: access.ChannelNtfy, URL: srv.URL, Topic: "antennarr"}
}

// newRig starts a Notifier with one ntfy channel pointing at a fake server.
func newRig(t *testing.T, mutate ...func(*access.Settings, *server)) *rig {
	t.Helper()
	r := &rig{srv: newServer(t), clock: &clock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}, timers: &timers{}}
	r.set = &access.Settings{Notifications: &access.Notifications{Channels: []access.NotifyChannel{ntfyChannel(r.srv)}}}
	for _, m := range mutate {
		m(r.set, r.srv)
	}
	var mu sync.Mutex
	r.n = New(Config{
		Settings:    func() access.Settings { mu.Lock(); defer mu.Unlock(); return r.set.Clone() },
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:         r.clock.now,
		After:       r.timers.after,
		PushoverURL: r.srv.URL + "/1/messages.json",
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.n.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return r
}

func decode(t *testing.T, h hit) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(h.body), &m); err != nil {
		t.Fatalf("not JSON: %q", h.body)
	}
	return m
}

// ---- the three kinds of channel ----

func TestNtfyGetsATopicTitleMessageAndToken(t *testing.T) {
	r := newRig(t, func(s *access.Settings, _ *server) { s.Notifications.Channels[0].Token = "tk_secret" })
	r.n.FirmwareFound("antenna", "Living room", "20260326", "Version 20260326 is out.")
	h := r.srv.next(t)
	m := decode(t, h)
	if m["topic"] != "antennarr" || m["title"] != "New firmware for Living room" || m["message"] != "Version 20260326 is out." {
		t.Errorf("body = %v", m)
	}
	if h.header.Get("Authorization") != "Bearer tk_secret" || h.header.Get("Content-Type") != "application/json" {
		t.Errorf("headers = %v", h.header)
	}
	if !strings.HasPrefix(h.header.Get("User-Agent"), "Antennarr") {
		t.Errorf("user agent = %q", h.header.Get("User-Agent"))
	}
}

func TestNtfyWithoutATokenSendsNoAuthorization(t *testing.T) {
	r := newRig(t)
	r.n.FirmwareFound("a", "A", "v-x", "x")
	if got := r.srv.next(t).header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q", got)
	}
}

func TestPushoverIsAFormWithTheTokenAndUser(t *testing.T) {
	r := newRig(t, func(s *access.Settings, _ *server) {
		s.Notifications.Channels = []access.NotifyChannel{{ID: "po", Name: "Pushover", Type: access.ChannelPushover, Token: "app-token", User: "user-key"}}
	})
	r.n.FirmwareFound("a", "A", "v-hello", "hello")
	h := r.srv.next(t)
	form, _ := url.ParseQuery(h.body)
	if h.path != "/1/messages.json" || form.Get("token") != "app-token" || form.Get("user") != "user-key" || form.Get("title") != "New firmware for A" || form.Get("message") != "hello" {
		t.Errorf("%s %v", h.path, form)
	}
	if h.header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Errorf("content type %q", h.header.Get("Content-Type"))
	}
}

func TestWebhookGetsJSONWithTheEventAndTheTime(t *testing.T) {
	r := newRig(t, func(s *access.Settings, srv *server) {
		s.Notifications.Channels = []access.NotifyChannel{{ID: "h", Name: "Hook", Type: access.ChannelWebhook, URL: srv.URL + "/api/webhook/abc?key=zzz", Token: "bearer"}}
	})
	r.n.FirmwareFound("antenna", "Living room", "v1", "x")
	h := r.srv.next(t)
	m := decode(t, h)
	if h.path != "/api/webhook/abc" || h.query != "key=zzz" {
		t.Errorf("the address was changed: %s?%s", h.path, h.query)
	}
	if m["event"] != "firmware" || m["device"] != "antenna" || m["message"] != "x" || m["time"] != "2026-10-18T09:00:00Z" {
		t.Errorf("body = %v", m)
	}
	if h.header.Get("Authorization") != "Bearer bearer" {
		t.Errorf("Authorization = %q", h.header.Get("Authorization"))
	}
}

func TestEveryChannelGetsTheEventAndOneThatFailsDoesNotStopTheRest(t *testing.T) {
	bad := newServer(t)
	bad.status = 500
	r := newRig(t, func(s *access.Settings, srv *server) {
		s.Notifications.Channels = []access.NotifyChannel{
			{ID: "a", Name: "A", Type: access.ChannelNtfy, URL: bad.URL, Topic: "t"},
			{ID: "b", Name: "B", Type: access.ChannelWebhook, URL: srv.URL + "/hook"},
		}
	})
	r.n.FirmwareFound("a", "A", "v-x", "x")
	bad.next(t)
	if h := r.srv.next(t); h.path != "/hook" {
		t.Errorf("the second channel got %s", h.path)
	}
}

// ---- what is sent, and what is not ----

func TestNothingIsSentWithoutChannelsOrWhenTheKindIsOff(t *testing.T) {
	r := newRig(t, func(s *access.Settings, _ *server) { s.Notifications.Events.Firmware = new(bool) })
	r.n.FirmwareFound("a", "A", "v-x", "x") // firmware is switched off
	r.srv.none(t)

	r2 := newRig(t, func(s *access.Settings, _ *server) { s.Notifications.Channels = nil })
	r2.n.FirmwareFound("a", "A", "v-x", "x") // no channels
	r2.srv.none(t)

	r3 := newRig(t, func(s *access.Settings, _ *server) { s.Notifications = nil })
	r3.n.FirmwareFound("a", "A", "v-x", "x") // never set up
	r3.srv.none(t)
}

func TestSettingsAreReadOnEveryEventSoChangesNeedNoRestart(t *testing.T) {
	r := newRig(t)
	r.n.FirmwareFound("a", "A", "v-first", "first")
	r.srv.next(t)
	off := false
	r.set.Notifications.Events.Firmware = &off
	r.n.FirmwareFound("a", "A", "v-second", "second")
	r.srv.none(t)
}

// ---- tuner offline and online ----

func TestOfflineIsOnlyReportedAfterTheGracePeriod(t *testing.T) {
	r := newRig(t)
	r.n.TunerState("antenna", "Living room", false)
	r.srv.none(t) // not yet
	if r.timers.count() != 1 {
		t.Fatalf("waiting: %d", r.timers.count())
	}
	r.n.TunerState("antenna", "Living room", false) // still down: no second wait
	if r.timers.count() != 1 {
		t.Errorf("a second wait was started: %d", r.timers.count())
	}
	r.timers.fire()
	m := decode(t, r.srv.next(t))
	if m["title"] != "Living room is offline" || !strings.Contains(m["message"].(string), "2 minutes") {
		t.Errorf("body = %v", m)
	}
	r.n.TunerState("antenna", "Living room", false) // still down after telling: say nothing more
	r.timers.fire()
	r.srv.none(t)
}

func TestABlipIsNotReportedAtAll(t *testing.T) {
	r := newRig(t)
	r.n.TunerState("antenna", "Living room", false)
	r.n.TunerState("antenna", "Living room", true) // back before the grace period
	r.timers.fire()
	r.srv.none(t)
	if r.timers.count() != 0 {
		t.Errorf("the wait was not canceled: %d", r.timers.count())
	}
}

func TestBackOnlineIsOnlyReportedIfOfflineWas(t *testing.T) {
	r := newRig(t)
	r.n.TunerState("antenna", "Living room", true) // the first sighting at startup
	r.srv.none(t)

	r.n.TunerState("antenna", "Living room", false)
	r.timers.fire()
	r.srv.next(t) // offline
	r.n.TunerState("antenna", "Living room", true)
	if m := decode(t, r.srv.next(t)); m["title"] != "Living room is back online" {
		t.Errorf("body = %v", m)
	}
	r.n.TunerState("antenna", "Living room", true) // online again: nothing new
	r.srv.none(t)

	// and it can go offline again later
	r.n.TunerState("antenna", "Living room", false)
	r.timers.fire()
	r.srv.next(t)
}

func TestAWaitThatWasCanceledAsItFiredDoesNotSend(t *testing.T) {
	r := newRig(t)
	r.n.TunerState("antenna", "L", false)
	var stale func()
	r.timers.mu.Lock()
	for _, f := range r.timers.pending {
		stale = f // keep the first wait's callback, as if it had already started
	}
	r.timers.mu.Unlock()
	r.n.TunerState("antenna", "L", true)  // cancels it
	r.n.TunerState("antenna", "L", false) // a second, newer wait
	stale()                               // the old one fires late
	r.srv.none(t)
	r.timers.fire() // only the newer one counts
	r.srv.next(t)
}

func TestEachTunerHasItsOwnWait(t *testing.T) {
	r := newRig(t)
	r.n.TunerState("a", "A", false)
	r.n.TunerState("b", "B", false)
	r.n.TunerState("a", "A", true) // A is back; B is still down
	r.timers.fire()
	if m := decode(t, r.srv.next(t)); m["title"] != "B is offline" {
		t.Errorf("body = %v", m)
	}
	r.srv.none(t)
}

func TestTunerEventsCanBeSwitchedOff(t *testing.T) {
	r := newRig(t, func(s *access.Settings, _ *server) { s.Notifications.Events.Tuner = new(bool) })
	r.n.TunerState("a", "A", false)
	r.timers.fire()
	r.srv.none(t)
}

func TestShuttingDownCancelsTheWait(t *testing.T) {
	r := newRig(t)
	r.n.TunerState("a", "A", false)
	r.n.stopWaiting()
	if r.timers.count() != 0 {
		t.Errorf("the wait is still pending: %d", r.timers.count())
	}
	r.n.TunerState("a", "A", false) // after shutdown nothing is started
	if r.timers.count() != 0 {
		t.Errorf("a wait was started after shutdown")
	}
}

// ---- viewers refused ----

func TestARefusalIsReportedOnceInAWhile(t *testing.T) {
	r := newRig(t)
	r.n.Refused("antenna", "Living room", "Mom", access.ReasonViewerLimit)
	m := decode(t, r.srv.next(t))
	if m["title"] != "Mom was refused a tuner" || m["message"] != "Mom asked for a tuner on Living room and is already using all the tuners they are allowed." {
		t.Errorf("body = %v", m)
	}
	r.n.Refused("antenna", "Living room", "Mom", access.ReasonViewerLimit) // a media server retrying
	r.srv.none(t)
	r.n.Refused("antenna", "Living room", "Mom", access.ReasonSharedCap) // a different reason
	if m := decode(t, r.srv.next(t)); !strings.Contains(m["message"].(string), "share") {
		t.Errorf("body = %v", m)
	}
	r.n.Refused("antenna", "Living room", "Dad", access.ReasonViewerLimit) // a different viewer
	r.srv.next(t)
	r.n.Refused("antenna", "Living room", "Dad", access.ReasonOwnerReserve)
	if m := decode(t, r.srv.next(t)); !strings.Contains(m["message"].(string), "kept for you") {
		t.Errorf("body = %v", m)
	}

	r.clock.advance(refusedEvery + time.Second)
	r.n.Refused("antenna", "Living room", "Mom", access.ReasonViewerLimit) // after a while
	r.srv.next(t)
}

func TestARefusalBecauseTheTunerIsOfflineOrTheAddressIsUnknownIsNotAViewerEvent(t *testing.T) {
	r := newRig(t)
	r.n.Refused("a", "A", "Mom", access.ReasonOffline)
	r.n.Refused("a", "A", "Mom", access.ReasonNotAllowed)
	r.srv.none(t)
}

func TestViewerEventsCanBeSwitchedOff(t *testing.T) {
	r := newRig(t, func(s *access.Settings, _ *server) { s.Notifications.Events.Viewer = new(bool) })
	r.n.Refused("a", "A", "Mom", access.ReasonViewerLimit)
	r.srv.none(t)
	// and a switched-off event doesn't use up the turn it would have had
	on := true
	r.set.Notifications.Events.Viewer = &on
	r.n.Refused("a", "A", "Mom", access.ReasonViewerLimit)
	r.srv.next(t)
}

// ---- unknown addresses ----

func unknownOn(s *access.Settings, _ *server) { on := true; s.Notifications.Events.Unknown = &on }

func TestUnknownAddressesAreOffUntilSwitchedOn(t *testing.T) {
	r := newRig(t)
	r.n.Unknown("antenna", "Living room", "203.0.113.9")
	r.srv.none(t)
}

func TestAnUnknownAddressIsReportedOnceADay(t *testing.T) {
	r := newRig(t, unknownOn)
	r.n.Unknown("antenna", "Living room", "203.0.113.9")
	m := decode(t, r.srv.next(t))
	if m["title"] != "An unknown address was refused" || m["message"] != "203.0.113.9 tried to use Living room and isn't a viewer." {
		t.Errorf("body = %v", m)
	}
	r.n.Unknown("antenna", "Living room", "203.0.113.9")
	r.srv.none(t)
	r.clock.advance(unknownEvery + time.Second)
	r.n.Unknown("antenna", "Living room", "203.0.113.9")
	r.srv.next(t)
}

func TestUnknownAddressesAreCappedPerHourAndTheRestWaitTheirTurn(t *testing.T) {
	r := newRig(t, unknownOn)
	for i := 0; i < unknownPerHour+3; i++ {
		r.n.Unknown("antenna", "L", "203.0.113."+string(rune('1'+i)))
	}
	for i := 0; i < unknownPerHour; i++ {
		r.srv.next(t)
	}
	r.srv.none(t) // a scanner trying nine addresses sends six messages, not nine

	// An address held back by the cap was not marked as reported, so it is told of next hour.
	r.clock.advance(unknownWindow + time.Second)
	r.n.Unknown("antenna", "L", "203.0.113."+string(rune('1'+unknownPerHour)))
	r.srv.next(t)
}

// ---- the queue ----

func TestAFullQueueDropsEventsRatherThanBlocking(t *testing.T) {
	r := &rig{srv: newServer(t)}
	set := access.Settings{Notifications: &access.Notifications{Channels: []access.NotifyChannel{ntfyChannel(r.srv)}}}
	n := New(Config{Settings: func() access.Settings { return set }, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}) // never Run
	done := make(chan struct{})
	go func() {
		for i := 0; i < queueSize*3; i++ {
			n.Send(Event{Kind: access.NotifyFirmware, Title: "t", Message: "m"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Send blocked on a full queue")
	}
	if len(n.queue) != queueSize {
		t.Errorf("queue holds %d, want %d", len(n.queue), queueSize)
	}
}

// ---- the test button ----

func TestTestSendsOneMessageWhateverTheEventSettings(t *testing.T) {
	r := newRig(t, func(s *access.Settings, _ *server) {
		off := false
		s.Notifications.Events = access.NotifyEvents{Firmware: &off, Tuner: &off, Viewer: &off, Unknown: &off}
	})
	if err := r.n.Test(context.Background(), r.set.Notifications.Channels[0]); err != nil {
		t.Fatal(err)
	}
	m := decode(t, r.srv.next(t))
	if m["title"] != TestTitle {
		t.Errorf("body = %v", m)
	}
	if tags, _ := m["tags"].([]any); len(tags) != 1 || tags[0] != "white_check_mark" {
		t.Errorf("a test should not look like a warning: %v", m["tags"])
	}
}

func TestTestWorksOnAChannelThatIsNotSavedYet(t *testing.T) {
	r := newRig(t, func(s *access.Settings, _ *server) { s.Notifications = nil })
	ch := ntfyChannel(r.srv)
	if err := r.n.Test(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	r.srv.next(t)
}

func TestTestSaysWhyItFailed(t *testing.T) {
	r := newRig(t)
	r.srv.status = 401
	r.srv.reply = `{"error":"forbidden: bad token"}`
	err := r.n.Test(context.Background(), r.set.Notifications.Channels[0])
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "bad token") {
		t.Errorf("error = %v", err)
	}

	bad := r.set.Notifications.Channels[0]
	bad.Topic = "has a space"
	if err := r.n.Test(context.Background(), bad); err == nil || !strings.Contains(err.Error(), "topic") {
		t.Errorf("an invalid channel: %v", err)
	}
}

func TestTestFailsPlainlyWhenNothingIsListening(t *testing.T) {
	r := newRig(t)
	dead := newServer(t)
	ch := ntfyChannel(dead)
	dead.Close()
	err := r.n.Test(context.Background(), ch)
	if err == nil || !strings.Contains(err.Error(), "couldn't reach") {
		t.Errorf("error = %v", err)
	}
}

// ---- secrets ----

func TestErrorsAndLogsNeverRepeatTheWebhookAddressOrTokens(t *testing.T) {
	srv := newServer(t)
	srv.status = 500
	secretURL := srv.URL + "/api/webhooks/123/SECRETPATH?key=SECRETQUERY"
	var log strings.Builder
	n := New(Config{
		Settings: func() access.Settings { return access.Settings{} },
		Log:      slog.New(slog.NewTextHandler(&log, nil)),
	})
	for name, ch := range map[string]access.NotifyChannel{
		"a webhook that answers badly": {ID: "h", Name: "Hook", Type: access.ChannelWebhook, URL: secretURL, Token: "SECRETTOKEN"},
		"one nothing listens on":       {ID: "h", Name: "Hook", Type: access.ChannelWebhook, URL: "http://127.0.0.1:1/api/webhooks/123/SECRETPATH?key=SECRETQUERY", Token: "SECRETTOKEN"},
	} {
		err := n.Test(context.Background(), ch)
		if err == nil {
			t.Fatalf("%s: no error", name)
		}
		for _, s := range []string{"SECRETPATH", "SECRETQUERY", "SECRETTOKEN"} {
			if strings.Contains(err.Error(), s) {
				t.Errorf("%s: the error repeats %q: %v", name, s, err)
			}
		}
	}

	// And the log of a failed send, which goes through deliver.
	n.deliver(context.Background(), job{ev: Event{Kind: "test", Title: "t", Message: "m", Time: time.Now()}, channels: []access.NotifyChannel{
		{ID: "h", Name: "Hook", Type: access.ChannelWebhook, URL: secretURL, Token: "SECRETTOKEN"},
		{ID: "p", Name: "Po", Type: access.ChannelPushover, Token: "SECRETAPP", User: "SECRETUSER"},
	}})
	for _, s := range []string{"SECRETPATH", "SECRETQUERY", "SECRETTOKEN", "SECRETAPP", "SECRETUSER"} {
		if strings.Contains(log.String(), s) {
			t.Errorf("the log repeats %q:\n%s", s, log.String())
		}
	}
	if !strings.Contains(log.String(), "notification not sent") {
		t.Errorf("a failed send was not logged: %q", log.String())
	}
}

func TestARedirectIsNotFollowed(t *testing.T) {
	target := newServer(t)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/elsewhere", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirect.Close)
	n := New(Config{Settings: func() access.Settings { return access.Settings{} }})
	err := n.Test(context.Background(), access.NotifyChannel{ID: "h", Name: "H", Type: access.ChannelWebhook, URL: redirect.URL, Token: "tok"})
	if err == nil || !strings.Contains(err.Error(), "307") {
		t.Errorf("error = %v", err)
	}
	target.none(t) // the token went nowhere else
}

func TestALongReplyIsCutDown(t *testing.T) {
	srv := newServer(t)
	srv.status = 400
	srv.reply = strings.Repeat("x", 5000)
	n := New(Config{Settings: func() access.Settings { return access.Settings{} }})
	err := n.Test(context.Background(), access.NotifyChannel{ID: "h", Name: "H", Type: access.ChannelWebhook, URL: srv.URL})
	if err == nil || len(err.Error()) > 400 {
		t.Errorf("error = %v", err)
	}
}

// ---- firmware is told about once per release ----

func TestAReleaseIsToldOnceAndANewOneIsToldAgain(t *testing.T) {
	r := newRig(t)
	r.n.FirmwareFound("antenna", "Living room", "20260326", "a")
	r.srv.next(t)
	r.n.FirmwareFound("antenna", "Living room", "20260326", "a") // seen again at the next check
	r.srv.none(t)
	r.n.FirmwareFound("other", "Cable", "20260326", "a") // another tuner has its own turn
	r.srv.next(t)
	r.n.FirmwareFound("antenna", "Living room", "20261001", "b") // a newer release
	r.srv.next(t)
}

func TestARestartDoesNotTellAgain(t *testing.T) {
	state := filepath.Join(t.TempDir(), "antennarr.notified.json")
	srv := newServer(t)
	set := access.Settings{Notifications: &access.Notifications{Channels: []access.NotifyChannel{ntfyChannel(srv)}}}
	build := func() *Notifier {
		n := New(Config{Settings: func() access.Settings { return set }, StatePath: state, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go n.Run(ctx)
		return n
	}

	build().FirmwareFound("antenna", "Living room", "20260326", "a")
	srv.next(t)
	if info, err := os.Stat(state); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the state file: %v %v", info, err)
	}

	restarted := build() // Watchtower replaced the container
	restarted.FirmwareFound("antenna", "Living room", "20260326", "a")
	srv.none(t)
	restarted.FirmwareFound("antenna", "Living room", "20261001", "b")
	srv.next(t)
}

func TestAReleaseNotToldBecauseItWasOffIsToldWhenItIsOn(t *testing.T) {
	r := newRig(t, func(s *access.Settings, _ *server) { s.Notifications.Events.Firmware = new(bool) })
	r.n.FirmwareFound("a", "A", "v1", "x")
	r.srv.none(t)
	on := true
	r.set.Notifications.Events.Firmware = &on
	r.n.FirmwareFound("a", "A", "v1", "x") // still the same release, and it was never sent
	r.srv.next(t)
}

func TestAnUnreadableStateFileJustMeansTellingAgain(t *testing.T) {
	state := filepath.Join(t.TempDir(), "n.json")
	os.WriteFile(state, []byte("{not json"), 0o600)
	srv := newServer(t)
	set := access.Settings{Notifications: &access.Notifications{Channels: []access.NotifyChannel{ntfyChannel(srv)}}}
	n := New(Config{Settings: func() access.Settings { return set }, StatePath: state, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go n.Run(ctx)
	n.FirmwareFound("a", "A", "v1", "x")
	srv.next(t)
}

func TestAStatePathThatCannotBeWrittenDoesNotStopTheMessage(t *testing.T) {
	srv := newServer(t)
	set := access.Settings{Notifications: &access.Notifications{Channels: []access.NotifyChannel{ntfyChannel(srv)}}}
	n := New(Config{Settings: func() access.Settings { return set }, StatePath: filepath.Join(t.TempDir(), "missing", "n.json"), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go n.Run(ctx)
	n.FirmwareFound("a", "A", "v1", "x")
	srv.next(t)
}

// ---- where a notification may go ----

func TestTheClientRefusesAddressesNoServerIsAt(t *testing.T) {
	for address, ok := range map[string]bool{
		"127.0.0.1:8080":              true,
		"[::1]:8080":                  true,
		"192.168.1.20:80":             true,
		"10.0.0.5:2586":               true,
		"203.0.113.9:443":             true,
		"169.254.169.254:80":          false, // the cloud metadata service
		"[fe80::1]:80":                false,
		"[::ffff:169.254.169.254]:80": false,
		"0.0.0.0:80":                  false,
		"224.0.0.1:80":                false,
		"not an address":              false,
		"example.com:80":              false, // a name should never reach here: it is looked up first
	} {
		err := guard("tcp", address, nil)
		if (err == nil) != ok {
			t.Errorf("guard(%s) = %v, want allowed=%v", address, err, ok)
		}
	}
}

func TestANotificationToALinkLocalAddressNeverConnects(t *testing.T) {
	// The real client, as used outside tests. 169.254.x.x is refused before anything is sent, so this is quick.
	n := New(Config{Settings: func() access.Settings { return access.Settings{} }})
	start := time.Now()
	err := n.post(context.Background(), "http://169.254.169.254/latest/meta-data/", "application/json", []byte("{}"), "SECRETTOKEN")
	if err == nil || !strings.Contains(err.Error(), "can't be used") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "SECRETTOKEN") || strings.Contains(err.Error(), "latest/meta-data") {
		t.Errorf("the error repeats what it shouldn't: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("took %v: it tried to connect", time.Since(start))
	}
}

func TestAnAddressItIsAllowedToSendToStillWorksThroughTheRealClient(t *testing.T) {
	srv := newServer(t) // on 127.0.0.1
	n := New(Config{Settings: func() access.Settings { return access.Settings{} }})
	if err := n.Test(context.Background(), ntfyChannel(srv)); err != nil {
		t.Fatal(err)
	}
	srv.next(t)
	// and by name, which is looked up and then checked
	u, _ := url.Parse(srv.URL)
	named := ntfyChannel(srv)
	named.URL = "http://localhost:" + u.Port()
	if err := n.Test(context.Background(), named); err != nil {
		t.Fatal(err)
	}
	srv.next(t)
}
