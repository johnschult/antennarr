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

	"github.com/johnschult/antennarr/internal/discovery"
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
	if !t0.InUse || !t0.ViaAntennarr || t0.Channel != "5.1" || t0.ChannelName != "WFRV" || t0.SignalStrength != 88 || t0.BitsPerSecond != 14200000 {
		t.Errorf("tuner0 = %+v", t0)
	}
	if !t1.InUse || t1.ViaAntennarr {
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
