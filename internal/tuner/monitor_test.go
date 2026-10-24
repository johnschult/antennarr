package tuner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/discovery"
)

// fakeHDHR is a tuner whose DeviceID and status.json can be changed mid-test.
type fakeHDHR struct {
	*httptest.Server
	mu     sync.Mutex
	id     string
	status string
	extra  string // more discover.json fields, e.g. ,"UpgradeAvailable":"20270101"
}

func newHDHR(id string) *fakeHDHR {
	f := &fakeHDHR{id: id, status: `[{"Resource":"tuner0"},{"Resource":"tuner1"}]`}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/discover.json":
			fmt.Fprintf(w, `{"FriendlyName":"Fake FLEX","ModelNumber":"HDFX-4K","DeviceID":%q,"TunerCount":2,"FirmwareName":"hdhomerun_dvr_atsc3","FirmwareVersion":"20260326"%s}`, f.id, f.extra)
		case "/status.json":
			io.WriteString(w, f.status)
		default:
			http.NotFound(w, r)
		}
	}))
	return f
}

func (f *fakeHDHR) setStatus(s string) { f.mu.Lock(); f.status = s; f.mu.Unlock() }

func monitorFor(base, id string, find Finder) *Monitor {
	return NewMonitor(Config{
		BaseURL:  base,
		ID:       id,
		Failures: 2,
		Find:     find,
		LocalIPs: func() []string { return []string{"192.168.1.20"} },
	})
}

func neverFind(context.Context, string, string) (discovery.Device, error) {
	return discovery.Device{}, errors.New("not found")
}

func TestPollGoesOnlineAndReadsSlots(t *testing.T) {
	f := newHDHR("10ABCC87")
	defer f.Close()
	f.setStatus(`[
		{"Resource":"tuner0","VctNumber":"5.1","VctName":"WFRV","TargetIP":"192.168.1.20","SignalStrengthPercent":88,"SignalQualityPercent":100,"NetworkRate":14200000},
		{"Resource":"tuner1","VctNumber":"2.1","VctName":"WBAY","TargetIP":"192.168.1.77"},
		{"Resource":"tuner2"},
		{"Resource":"tuner3","TargetIP":"192.168.1.88"},
		{"Resource":"system","Something":"else"}
	]`)

	m := monitorFor(f.URL, "10abcc87", neverFind)
	m.Poll(context.Background())
	s := m.Snapshot()

	if !s.Online || s.FriendlyName != "Fake FLEX" || s.Model != "HDFX-4K" || s.TunerCount != 2 {
		t.Fatalf("state = %+v", s)
	}
	if len(s.Slots) != 4 {
		t.Fatalf("%d slots, want 4 (system entry must be ignored): %+v", len(s.Slots), s.Slots)
	}
	t0, t1, t2, t3 := s.Slots[0], s.Slots[1], s.Slots[2], s.Slots[3]
	if !t0.InUse || !t0.ViaProxy || t0.Channel != "5.1" || t0.ChannelName != "WFRV" || t0.SignalStrength != 88 || t0.BitsPerSecond != 14200000 {
		t.Errorf("tuner0 = %+v", t0)
	}
	if !t1.InUse || t1.ViaProxy {
		t.Errorf("tuner1 should be in use by someone else: %+v", t1)
	}
	if t2.InUse {
		t.Errorf("tuner2 should be idle: %+v", t2)
	}
	if !t3.InUse { // a target with no channel yet still holds the tuner
		t.Errorf("tuner3 should count as in use: %+v", t3)
	}
	if total, via := s.InUse(); total != 3 || via != 1 {
		t.Errorf("InUse() = %d, %d; want 3, 1", total, via)
	}
}

func TestLearnsIDOnFirstContact(t *testing.T) {
	f := newHDHR("10ABCC87")
	defer f.Close()
	m := monitorFor(f.URL, "", neverFind)
	m.Poll(context.Background())
	if got := m.Snapshot().ID; got != "10ABCC87" {
		t.Errorf("ID = %q, want it learned from the tuner", got)
	}
}

func TestTargets(t *testing.T) {
	for _, base := range []string{"http://192.168.1.132/", "http://192.168.1.132:80", "http://192.168.1.132"} {
		m := NewMonitor(Config{BaseURL: base, StreamPort: "5004", Find: neverFind})
		api, stream := m.Targets()
		if api.String() != "http://192.168.1.132" || stream.String() != "http://192.168.1.132:5004" {
			t.Errorf("%s: api=%v stream=%v", base, api, stream)
		}
		if got := m.Snapshot().BaseURL; got != "http://192.168.1.132" {
			t.Errorf("%s: BaseURL = %q, want it normalized", base, got)
		}
	}
}

func TestUnparseableBaseGivesNoTargets(t *testing.T) {
	m := NewMonitor(Config{BaseURL: "http://[::1", Find: neverFind})
	if api, stream := m.Targets(); api != nil || stream != nil {
		t.Errorf("got %v, %v for an unparseable base", api, stream)
	}
}

func TestOfflineAfterConsecutiveFailures(t *testing.T) {
	f := newHDHR("10ABCC87")
	m := monitorFor(f.URL, "10ABCC87", neverFind)
	m.Poll(context.Background())
	if !m.Snapshot().Online {
		t.Fatal("should start online")
	}
	f.Close()

	m.Poll(context.Background()) // 1st failure: tolerated
	if !m.Snapshot().Online {
		t.Error("one blip should not mark the tuner offline")
	}
	m.Poll(context.Background()) // 2nd: offline
	s := m.Snapshot()
	if s.Online || len(s.Slots) != 0 {
		t.Errorf("state = %+v", s)
	}
	if last := s.Events[len(s.Events)-1]; last.Kind != "err" || !strings.Contains(last.Text, "offline") {
		t.Errorf("last event = %+v", last)
	}
}

func TestRecoversWhenTunerReturns(t *testing.T) {
	f := newHDHR("10ABCC87")
	defer f.Close()
	m := monitorFor(f.URL, "10ABCC87", neverFind)
	m.Poll(context.Background())

	f.mu.Lock()
	f.id = "SOMEONE1" // wrong device answers at this address
	f.mu.Unlock()
	m.Poll(context.Background())
	m.Poll(context.Background())
	if m.Snapshot().Online {
		t.Fatal("a different tuner at the same address must not count as ours")
	}

	f.mu.Lock()
	f.id = "10ABCC87"
	f.mu.Unlock()
	m.Poll(context.Background())
	s := m.Snapshot()
	if !s.Online {
		t.Fatal("should be online again")
	}
	if last := s.Events[len(s.Events)-1]; last.Kind != "ok" {
		t.Errorf("last event = %+v", last)
	}
}

func TestFollowsTunerToNewAddress(t *testing.T) {
	old, moved := newHDHR("10ABCC87"), newHDHR("10ABCC87")
	defer moved.Close()

	var calls atomic.Int32
	find := func(_ context.Context, id, last string) (discovery.Device, error) {
		calls.Add(1)
		if id != "10ABCC87" || last != old.URL {
			t.Errorf("Find(%q, %q)", id, last)
		}
		return discovery.Device{ID: id, BaseURL: moved.URL}, nil
	}
	m := monitorFor(old.URL, "10ABCC87", find)
	m.Poll(context.Background())
	old.Close() // DHCP gave the tuner a new address

	m.Poll(context.Background())
	m.Poll(context.Background()) // threshold reached: search, then re-check

	s := m.Snapshot()
	if s.BaseURL != moved.URL || !s.Online {
		t.Fatalf("BaseURL=%s online=%v, want %s online", s.BaseURL, s.Online, moved.URL)
	}
	api, _ := m.Targets()
	if api.String() != moved.URL {
		t.Errorf("Targets() still points at %v", api)
	}
	var sawMove bool
	for _, e := range s.Events {
		sawMove = sawMove || strings.Contains(e.Text, "moved")
	}
	if !sawMove {
		t.Errorf("no 'moved' event in %+v", s.Events)
	}
}

func TestOnMovedIsCalledOnceTheTunerAnswersAtTheNewAddress(t *testing.T) {
	old, moved := newHDHR("10ABCC87"), newHDHR("10ABCC87")
	defer moved.Close()
	find := func(_ context.Context, id, _ string) (discovery.Device, error) {
		return discovery.Device{ID: id, BaseURL: moved.URL}, nil
	}
	m := monitorFor(old.URL, "10ABCC87", find)
	var from, to []string
	m.cfg.OnMoved = func(f, t string) { from, to = append(from, f), append(to, t) }
	m.Poll(context.Background())
	old.Close()
	m.Poll(context.Background())
	m.Poll(context.Background())

	if len(to) != 1 || from[0] != old.URL || to[0] != moved.URL {
		t.Errorf("OnMoved called with from=%v to=%v, want once, %s to %s", from, to, old.URL, moved.URL)
	}
}

func TestOnMovedIsNotCalledIfNothingAnswersAtTheNewAddress(t *testing.T) {
	old, dead := newHDHR("10ABCC87"), newHDHR("10ABCC87")
	dead.Close() // found by the search, but gone again, or not answering
	find := func(_ context.Context, id, _ string) (discovery.Device, error) {
		return discovery.Device{ID: id, BaseURL: dead.URL}, nil
	}
	m := monitorFor(old.URL, "10ABCC87", find)
	called := false
	m.cfg.OnMoved = func(string, string) { called = true }
	m.Poll(context.Background())
	old.Close()
	m.Poll(context.Background())
	m.Poll(context.Background())
	if called {
		t.Error("an address the tuner didn't answer at was reported as its new one")
	}
}

func TestATunerNotSeenYetIsSearchedForAtOnce(t *testing.T) {
	// The saved address is out of date at start-up: there is nothing to wait for.
	old, moved := newHDHR("10ABCC87"), newHDHR("10ABCC87")
	defer moved.Close()
	old.Close()
	var calls atomic.Int32
	find := func(_ context.Context, id, _ string) (discovery.Device, error) {
		calls.Add(1)
		return discovery.Device{ID: id, BaseURL: moved.URL}, nil
	}
	m := monitorFor(old.URL, "10ABCC87", find) // two failures are normally needed
	m.Poll(context.Background())               // one poll: the very first
	if s := m.Snapshot(); s.BaseURL != moved.URL || !s.Online || calls.Load() != 1 {
		t.Errorf("BaseURL=%s online=%v searches=%d, want %s online after one search", s.BaseURL, s.Online, calls.Load(), moved.URL)
	}
}

func TestSearchIsRateLimited(t *testing.T) {
	f := newHDHR("10ABCC87")
	m := monitorFor(f.URL, "10ABCC87", nil)
	var calls atomic.Int32
	m.cfg.Find = func(context.Context, string, string) (discovery.Device, error) {
		calls.Add(1)
		return discovery.Device{}, errors.New("not found")
	}
	m.Poll(context.Background())
	f.Close()
	for i := 0; i < 10; i++ {
		m.Poll(context.Background())
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("Find called %d times in a burst of polls, want 1", n)
	}
}

func TestNoSearchWithoutKnownID(t *testing.T) {
	var calls atomic.Int32
	m := monitorFor("http://127.0.0.1:1", "", func(context.Context, string, string) (discovery.Device, error) {
		calls.Add(1)
		return discovery.Device{}, errors.New("nope")
	})
	for i := 0; i < 5; i++ {
		m.Poll(context.Background())
	}
	if calls.Load() != 0 {
		t.Error("searched for a tuner whose ID was never learned")
	}
}

func TestStatusFailureKeepsTunerOnline(t *testing.T) {
	f := newHDHR("10ABCC87")
	defer f.Close()
	m := monitorFor(f.URL, "10ABCC87", neverFind)
	m.Poll(context.Background())
	f.setStatus(`<html>oops`)
	m.Poll(context.Background())
	m.Poll(context.Background())
	if !m.Snapshot().Online {
		t.Error("a bad status.json should not take the tuner offline; discover.json still answers")
	}
}

func TestEventsAreCapped(t *testing.T) {
	m := NewMonitor(Config{Find: neverFind})
	m.mu.Lock()
	for i := 0; i < maxEvents+25; i++ {
		m.addEvent("info", fmt.Sprint(i))
	}
	m.mu.Unlock()
	ev := m.Snapshot().Events
	if len(ev) != maxEvents || ev[len(ev)-1].Text != fmt.Sprint(maxEvents+24) {
		t.Errorf("%d events, last %q", len(ev), ev[len(ev)-1].Text)
	}
}

func TestSnapshotIsACopy(t *testing.T) {
	f := newHDHR("10ABCC87")
	defer f.Close()
	m := monitorFor(f.URL, "10ABCC87", neverFind)
	m.Poll(context.Background())
	s := m.Snapshot()
	s.Slots[0].Channel = "tampered"
	s.Events[0].Text = "tampered"
	again := m.Snapshot()
	if again.Slots[0].Channel == "tampered" || again.Events[0].Text == "tampered" {
		t.Error("mutating a snapshot changed the monitor's state")
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	f := newHDHR("10ABCC87")
	defer f.Close()
	m := NewMonitor(Config{BaseURL: f.URL, ID: "10ABCC87", Interval: 10 * time.Millisecond, Find: neverFind})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()

	deadline := time.Now().Add(2 * time.Second)
	for !m.Snapshot().Online && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !m.Snapshot().Online {
		t.Fatal("Run never polled")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}

func TestOnChangeFiresAfterPollAndNote(t *testing.T) {
	f := newHDHR("10ABCC87")
	defer f.Close()
	var n atomic.Int32
	m := NewMonitor(Config{BaseURL: f.URL, ID: "10ABCC87", Find: neverFind, OnChange: func() { n.Add(1) }})

	m.Poll(context.Background())
	if n.Load() != 1 {
		t.Errorf("after a poll: %d calls, want 1", n.Load())
	}
	f.Close()
	m.Poll(context.Background()) // a failing poll is a change too (the UI may want to say so)
	if n.Load() != 2 {
		t.Errorf("after a failed poll: %d calls, want 2", n.Load())
	}
	m.Note("warn", "hello")
	if n.Load() != 3 {
		t.Errorf("after Note: %d calls, want 3", n.Load())
	}
	ev := m.Snapshot().Events
	if last := ev[len(ev)-1]; last.Kind != "warn" || last.Text != "hello" {
		t.Errorf("last event = %+v", last)
	}
}

func TestKickPollsEarly(t *testing.T) {
	f := newHDHR("10ABCC87")
	defer f.Close()
	// An interval so long that only a kick can produce a second poll.
	var polls atomic.Int32
	m := NewMonitor(Config{BaseURL: f.URL, ID: "10ABCC87", Interval: time.Hour, Find: neverFind,
		OnChange: func() { polls.Add(1) }})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	waitFor := func(n int32) bool {
		deadline := time.Now().Add(3 * time.Second)
		for polls.Load() < n && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		return polls.Load() >= n
	}
	if !waitFor(1) {
		t.Fatal("no initial poll")
	}

	f.setStatus(`[{"Resource":"tuner0","VctNumber":"5.1","TargetIP":"192.168.1.77"},{"Resource":"tuner1"}]`)
	for i := 0; i < 20; i++ { // a burst of kicks
		m.Kick()
	}
	if !waitFor(2) {
		t.Fatal("kick did not trigger a poll")
	}
	if total, _ := m.Snapshot().InUse(); total != 1 {
		t.Errorf("the early poll did not pick up the change: %d in use", total)
	}
	time.Sleep(2 * kickDelay)
	if n := polls.Load(); n > 3 {
		t.Errorf("20 kicks caused %d polls, want them collapsed", n)
	}
}

func TestKickNeverBlocks(t *testing.T) {
	m := NewMonitor(Config{Find: neverFind})
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			m.Kick() // nobody is running the loop
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Kick blocked")
	}
}

func TestSnapshotCarriesTheDeviceName(t *testing.T) {
	f := newHDHR("10ABCC87")
	defer f.Close()
	m := NewMonitor(Config{Name: "antenna", BaseURL: f.URL, ID: "10ABCC87", Find: neverFind})
	if got := m.Snapshot().Name; got != "antenna" {
		t.Errorf("name before any poll = %q", got)
	}
	m.Poll(context.Background())
	if got := m.Snapshot().Name; got != "antenna" {
		t.Errorf("name after a poll = %q", got)
	}
}

func TestFirmwareIsReadFromTheTuner(t *testing.T) {
	f := newHDHR("10ABCC87")
	defer f.Close()
	m := NewMonitor(Config{BaseURL: f.URL, ID: "10ABCC87", Find: neverFind})
	m.Poll(context.Background())
	fw := m.Snapshot().Firmware
	if fw.Name != "hdhomerun_dvr_atsc3" || fw.Version != "20260326" || fw.UpgradeAvailable != "" {
		t.Errorf("firmware = %+v", fw)
	}

	// A tuner that knows of a newer version says so in the same document.
	f.mu.Lock()
	f.extra = `,"UpgradeAvailable":"20270101"`
	f.mu.Unlock()
	m.Poll(context.Background())
	if got := m.Snapshot().Firmware.UpgradeAvailable; got != "20270101" {
		t.Errorf("UpgradeAvailable = %q", got)
	}
}

func TestOnTransitionSaysOnceWhenTheTunerGoesOfflineAndComesBack(t *testing.T) {
	f := newHDHR("10ABCC87")
	defer f.Close()
	var mu sync.Mutex
	var seen []bool
	var m *Monitor
	m = NewMonitor(Config{
		BaseURL: f.URL, ID: "10ABCC87", Failures: 2, Find: neverFind,
		LocalIPs: func() []string { return nil },
		OnTransition: func(online bool) {
			m.Snapshot() // would deadlock if the monitor's lock were still held
			mu.Lock()
			seen = append(seen, online)
			mu.Unlock()
		},
	})
	got := func() []bool { mu.Lock(); defer mu.Unlock(); return append([]bool(nil), seen...) }
	same := func(a, b []bool) bool {
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

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Poll(context.Background())
		m.Poll(context.Background()) // still online: nothing more
		if !same(got(), []bool{true}) {
			t.Errorf("after coming online: %v", got())
		}

		f.mu.Lock()
		f.id = "SOMEONE1"
		f.mu.Unlock()
		m.Poll(context.Background()) // first failure: a blip is not a transition
		if !same(got(), []bool{true}) {
			t.Errorf("after one failure: %v", got())
		}
		m.Poll(context.Background()) // second: offline
		m.Poll(context.Background()) // still offline: nothing more
		if !same(got(), []bool{true, false}) {
			t.Errorf("after going offline: %v", got())
		}

		f.mu.Lock()
		f.id = "10ABCC87"
		f.mu.Unlock()
		m.Poll(context.Background())
		m.Poll(context.Background())
		if !same(got(), []bool{true, false, true}) {
			t.Errorf("after coming back: %v", got())
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the callback ran with the monitor's lock held")
	}
}

func TestNoOnTransitionIsFine(t *testing.T) {
	f := newHDHR("10ABCC87")
	m := monitorFor(f.URL, "10ABCC87", neverFind)
	m.Poll(context.Background())
	f.Close()
	m.Poll(context.Background())
	m.Poll(context.Background())
	if m.Snapshot().Online {
		t.Error("should be offline")
	}
}

func TestAnUpdatingTunerGoingAwayAndComingBackIsNotAnOutage(t *testing.T) {
	f := newHDHR("10ABCC87")
	defer f.Close()
	var mu sync.Mutex
	var transitions []bool
	searched := 0
	m := NewMonitor(Config{
		BaseURL: f.URL, ID: "10ABCC87", Failures: 2,
		Find: func(context.Context, string, string) (discovery.Device, error) {
			mu.Lock()
			searched++
			mu.Unlock()
			return discovery.Device{}, errors.New("not found")
		},
		LocalIPs:     func() []string { return nil },
		OnTransition: func(online bool) { mu.Lock(); transitions = append(transitions, online); mu.Unlock() },
	})
	m.Poll(context.Background())
	mu.Lock()
	transitions = nil // the first sighting is not what is being looked at
	mu.Unlock()
	eventsBefore := len(m.Snapshot().Events)

	m.SetUpdating(true)
	if !m.Snapshot().Updating {
		t.Fatal("not marked as updating")
	}
	f.mu.Lock()
	f.id = "SOMEONE1" // it is gone: whatever answers is not our tuner
	f.mu.Unlock()
	for i := 0; i < 4; i++ {
		m.Poll(context.Background())
	}
	s := m.Snapshot()
	if s.Online {
		t.Error("a tuner that does not answer is shown online")
	}
	if !s.Updating {
		t.Error("the update flag was lost")
	}
	if len(s.Events) != eventsBefore {
		t.Errorf("events were recorded for a planned restart: %+v", s.Events[eventsBefore:])
	}

	f.mu.Lock()
	f.id = "10ABCC87"
	f.mu.Unlock()
	m.Poll(context.Background())
	m.Poll(context.Background())
	s = m.Snapshot()
	if !s.Online || len(s.Events) != eventsBefore {
		t.Errorf("back online: online=%v, events %+v", s.Online, s.Events[eventsBefore:])
	}
	mu.Lock()
	defer mu.Unlock()
	if len(transitions) != 0 || searched != 0 {
		t.Errorf("transitions %v, searches %d: a planned restart should cause neither", transitions, searched)
	}
}

func TestAfterTheUpdateTheTunerIsWatchedAsUsual(t *testing.T) {
	f := newHDHR("10ABCC87")
	m := monitorFor(f.URL, "10ABCC87", neverFind)
	m.Poll(context.Background())
	m.SetUpdating(true)
	m.SetUpdating(false)
	if m.Snapshot().Updating {
		t.Fatal("still updating")
	}
	f.Close()
	m.Poll(context.Background())
	m.Poll(context.Background())
	s := m.Snapshot()
	if s.Online || s.Events[len(s.Events)-1].Kind != "err" {
		t.Errorf("an outage after the update was not reported: %+v", s.Events)
	}
}

func TestTheUpdatingFlagCannotStickForever(t *testing.T) {
	f := newHDHR("10ABCC87")
	m := monitorFor(f.URL, "10ABCC87", neverFind)
	m.Poll(context.Background())
	m.SetUpdating(true)
	m.mu.Lock()
	m.updatingSince = time.Now().Add(-maxUpdating - time.Minute) // an upgrade that died without clearing it
	m.mu.Unlock()
	f.Close()
	m.Poll(context.Background())
	m.Poll(context.Background())
	s := m.Snapshot()
	if s.Updating || s.Events[len(s.Events)-1].Kind != "err" {
		t.Errorf("updating=%v events=%+v: a stale flag should not hide an outage", s.Updating, s.Events)
	}
}

func TestARestartedTunerGoingAwayIsNotAnOutage(t *testing.T) {
	f := newHDHR("10ABCC87")
	defer f.Close()
	var transitions []bool
	var mu sync.Mutex
	m := NewMonitor(Config{
		BaseURL: f.URL, ID: "10ABCC87", Failures: 2, Find: neverFind, LocalIPs: func() []string { return nil },
		OnTransition: func(online bool) { mu.Lock(); transitions = append(transitions, online); mu.Unlock() },
	})
	m.Poll(context.Background())
	mu.Lock()
	transitions = nil
	mu.Unlock()
	before := len(m.Snapshot().Events)

	m.SetRestarting(true)
	if !m.Snapshot().Restarting {
		t.Fatal("not marked as restarting")
	}
	f.mu.Lock()
	f.id = "SOMEONE1"
	f.mu.Unlock()
	for i := 0; i < 4; i++ {
		m.Poll(context.Background())
	}
	f.mu.Lock()
	f.id = "10ABCC87"
	f.mu.Unlock()
	m.Poll(context.Background())
	m.Poll(context.Background())
	s := m.Snapshot()
	if !s.Online || !s.Restarting || len(s.Events) != before {
		t.Errorf("online=%v restarting=%v events %+v", s.Online, s.Restarting, s.Events[before:])
	}
	mu.Lock()
	defer mu.Unlock()
	if len(transitions) != 0 {
		t.Errorf("transitions %v for a restart someone asked for", transitions)
	}
}

func TestTheRestartingFlagCannotStickForever(t *testing.T) {
	f := newHDHR("10ABCC87")
	m := monitorFor(f.URL, "10ABCC87", neverFind)
	m.Poll(context.Background())
	m.SetRestarting(true)
	m.mu.Lock()
	m.restartingSince = time.Now().Add(-maxRestarting - time.Minute)
	m.mu.Unlock()
	f.Close()
	m.Poll(context.Background())
	m.Poll(context.Background())
	s := m.Snapshot()
	if s.Restarting || s.Events[len(s.Events)-1].Kind != "err" {
		t.Errorf("restarting=%v events=%+v: a stale flag should not hide an outage", s.Restarting, s.Events)
	}
}

func TestClaimStreamsDecidesFromYagisOwnStreams(t *testing.T) {
	// A slot is {Channel, ViaProxy as the address check set it (the address is this host's)}.
	type sl struct {
		ch   string
		host bool
	}
	cases := []struct {
		name    string
		slots   []sl
		streams []string
		want    []bool
	}{
		{"Yagi's own stream, from this host", []sl{{"5.1", true}}, []string{"5.1"}, []bool{true}},
		{"a media server on the same machine, straight from the tuner", []sl{{"5.1", true}}, nil, []bool{false}},
		{"Yagi in a container: the tuner sees an address Yagi doesn't know", []sl{{"5.1", false}}, []string{"5.1"}, []bool{true}},
		{"a media server on another machine, straight from the tuner", []sl{{"5.1", false}}, nil, []bool{false}},
		{"Yagi's stream and another machine's on one channel", []sl{{"5.1", false}, {"5.1", true}}, []string{"5.1"}, []bool{false, true}},
		{"Yagi's stream and the same machine's on one channel: one of them is Yagi's", []sl{{"5.1", true}, {"5.1", true}}, []string{"5.1"}, []bool{true, false}},
		{"two Yagi streams, two channels", []sl{{"5.1", true}, {"2.1", true}}, []string{"2.1", "5.1"}, []bool{true, true}},
		{"a stream whose path named no channel takes a slot from this host", []sl{{"5.1", true}, {"2.1", false}}, []string{""}, []bool{true, false}},
		{"a stream not yet tuned has no channel, so it isn't claimed by a named one", []sl{{"", true}}, []string{"5.1"}, []bool{false}},
	}
	for _, c := range cases {
		slots := make([]Slot, len(c.slots))
		for i, x := range c.slots {
			slots[i] = Slot{InUse: true, Channel: x.ch, ViaProxy: x.host}
		}
		claimStreams(slots, c.streams)
		for i, w := range c.want {
			if slots[i].ViaProxy != w {
				t.Errorf("%s: slot %d ViaProxy = %v, want %v", c.name, i, slots[i].ViaProxy, w)
			}
		}
	}
}

func TestPollMarksOnlyYagisOwnStreamsAsViaProxy(t *testing.T) {
	f := newHDHR("10ABCC87")
	defer f.Close()
	// Someone is watching 5.1 from this machine's own address: Yagi's stream or a media server next to it.
	f.setStatus(`[{"Resource":"tuner0","VctNumber":"5.1","VctName":"WFRV","TargetIP":"192.168.1.20"},{"Resource":"tuner1"}]`)
	open := []string{}
	m := monitorFor(f.URL, "10ABCC87", neverFind)
	m.cfg.Streams = func() []string { return open }

	m.Poll(context.Background())
	if total, via := m.Snapshot().InUse(); total != 1 || via != 0 {
		t.Errorf("with no stream open in Yagi: in use %d, via Yagi %d; want 1, 0", total, via)
	}
	open = []string{"5.1"}
	m.Poll(context.Background())
	if total, via := m.Snapshot().InUse(); total != 1 || via != 1 {
		t.Errorf("with a stream open in Yagi on 5.1: in use %d, via Yagi %d; want 1, 1", total, via)
	}
}

func TestSearchResultIsKeptWhileTheTunerIsMissingAndClearedWhenItIsBack(t *testing.T) {
	f := newHDHR("10ABCC87")
	m := monitorFor(f.URL, "10ABCC87", neverFind)
	m.Poll(context.Background())
	if m.Snapshot().Search != nil {
		t.Fatal("a search is shown for a tuner that answers")
	}
	f.Close()
	m.Poll(context.Background())
	m.Poll(context.Background()) // the threshold: search, and find nothing
	s := m.Snapshot().Search
	if s == nil || s.Found || s.At.IsZero() {
		t.Fatalf("Search = %+v, want a search that found nothing", s)
	}
	back := newHDHR("10ABCC87")
	defer back.Close()
	m.cfg.Find = func(_ context.Context, id, _ string) (discovery.Device, error) {
		return discovery.Device{ID: id, BaseURL: back.URL}, nil
	}
	m.mu.Lock()
	m.lastSearch = time.Time{} // don't wait out the rate limit
	m.mu.Unlock()
	m.Poll(context.Background())
	if st := m.Snapshot(); !st.Online || st.Search != nil {
		t.Errorf("online=%v Search=%+v, want it online and the search forgotten", st.Online, st.Search)
	}
}

func TestARefusedStreamIsKeptAsAProblemAndNotedOncePerMinute(t *testing.T) {
	m := monitorFor("http://127.0.0.1:1", "10ABCC87", neverFind)
	changed := 0
	m.cfg.OnChange = func() { changed++ }
	for i := 0; i < 3; i++ {
		m.StreamRefused("503 Service Unavailable", "805 - Tuner in use")
	}
	s := m.Snapshot()
	if len(s.Problems) != 1 || s.Problems[0].Kind != ProblemRefused || !strings.Contains(s.Problems[0].Text, "503 Service Unavailable") || !strings.Contains(s.Problems[0].Text, "805 - Tuner in use") {
		t.Fatalf("Problems = %+v", s.Problems)
	}
	warns := 0
	for _, e := range s.Events {
		if e.Kind == "warn" {
			warns++
		}
	}
	if warns != 1 {
		t.Errorf("%d warn events for 3 refusals in a row, want 1: %+v", warns, s.Events)
	}
	if changed != 3 {
		t.Errorf("the page was told %d times, want 3", changed)
	}
}

func TestAnEmptyLineupIsAProblemUntilTheTunerHasChannelsAgain(t *testing.T) {
	m := monitorFor("http://127.0.0.1:1", "10ABCC87", neverFind)
	changed := 0
	m.cfg.OnChange = func() { changed++ }
	m.Lineup(40) // the usual case says nothing
	if len(m.Snapshot().Problems) != 0 || changed != 0 {
		t.Fatalf("a normal lineup was reported: %+v, told %d times", m.Snapshot().Problems, changed)
	}
	m.Lineup(0)
	if p := m.Snapshot().Problems; len(p) != 1 || p[0].Kind != ProblemNoChannels {
		t.Fatalf("Problems = %+v after an empty lineup", p)
	}
	m.Lineup(40)
	s := m.Snapshot()
	if len(s.Problems) != 0 {
		t.Errorf("the problem stayed after the channels came back: %+v", s.Problems)
	}
	if last := s.Events[len(s.Events)-1]; last.Kind != "ok" || !strings.Contains(last.Text, "channels again") {
		t.Errorf("last event = %+v, want a note that the channels are back", last)
	}
}

func TestAStreamYagiTurnedAwayIsKeptAsAProblem(t *testing.T) {
	m := monitorFor("http://127.0.0.1:1", "10ABCC87", neverFind)
	m.StreamDenied("John was refused a tuner: the remaining tuners are reserved for the owner")
	m.StreamDenied("Ann was refused a tuner: all shared tuners are in use")
	p := m.Snapshot().Problems
	if len(p) != 1 || p[0].Kind != ProblemDenied || !strings.Contains(p[0].Text, "Ann was refused") {
		t.Errorf("Problems = %+v, want only the latest refusal", p)
	}
	if len(m.Snapshot().Events) != 0 {
		t.Errorf("an event was added as well, but the policy adds its own: %+v", m.Snapshot().Events)
	}
}

func TestRetriesAreOneEpisodeUntilTheTroubleHasBeenQuietForAWhile(t *testing.T) {
	m := monitorFor("http://127.0.0.1:1", "10ABCC87", neverFind)
	m.StreamRefused("503 Service Unavailable", "")
	first := m.Snapshot().Problems[0]
	time.Sleep(5 * time.Millisecond)
	m.StreamRefused("503 Service Unavailable", "") // the media server tries again
	again := m.Snapshot().Problems[0]
	if !again.Time.Equal(first.Time) || !again.Last.After(first.Last) {
		t.Errorf("a retry: started %v then %v, last %v then %v; want the same start and a later last", first.Time, again.Time, first.Last, again.Last)
	}

	// Quiet for longer than an episode lasts: the next one is a new one.
	m.mu.Lock()
	m.st.Problems[0].Last = time.Now().Add(-problemEpisode - time.Minute)
	m.mu.Unlock()
	m.StreamRefused("503 Service Unavailable", "")
	if next := m.Snapshot().Problems[0]; !next.Time.After(first.Time) {
		t.Errorf("after a quiet spell the start is still %v, want a new episode", next.Time)
	}
}

func TestAnEmptyLineupIsOneEpisodeUntilChannelsReturn(t *testing.T) {
	m := monitorFor("http://127.0.0.1:1", "10ABCC87", neverFind)
	m.Lineup(0)
	first := m.Snapshot().Problems[0]
	m.mu.Lock()
	m.st.Problems[0].Last = time.Now().Add(-3 * time.Hour) // however long ago, until it is cleared
	m.mu.Unlock()
	m.Lineup(0)
	if got := m.Snapshot().Problems[0]; !got.Time.Equal(first.Time) {
		t.Errorf("an empty lineup, still empty hours later, started a new episode: %v then %v", first.Time, got.Time)
	}
	m.Lineup(40)
	time.Sleep(5 * time.Millisecond)
	m.Lineup(0)
	if got := m.Snapshot().Problems[0]; !got.Time.After(first.Time) {
		t.Errorf("empty again after channels came back should be a new episode, got start %v", got.Time)
	}
}
