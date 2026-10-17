package access

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/proxy"
	"github.com/johnschult/antennarr/internal/tuner"
)

type fakeMon struct {
	mu sync.Mutex
	st tuner.State
}

func (f *fakeMon) Snapshot() tuner.State { f.mu.Lock(); defer f.mu.Unlock(); return f.st }
func (f *fakeMon) set(online bool, tuners int, slots ...tuner.Slot) {
	f.mu.Lock()
	f.st = tuner.State{Online: online, TunerCount: tuners, Slots: slots}
	f.mu.Unlock()
}

type fakeNotes struct {
	mu    sync.Mutex
	notes []string
}

func (f *fakeNotes) Note(kind, text string) {
	f.mu.Lock()
	f.notes = append(f.notes, kind+": "+text)
	f.mu.Unlock()
}
func (f *fakeNotes) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.notes) }

var (
	local = tuner.Slot{InUse: true, TargetIP: "192.168.1.77"} // an owner's TV
	ours  = tuner.Slot{InUse: true, ViaAntennarr: true}       // a stream through us
	idle  = tuner.Slot{}
	_     = idle
)

func newPolicy(t *testing.T, s Settings, mon *fakeMon) (*Policy, *fakeNotes) {
	t.Helper()
	n := &fakeNotes{}
	p, err := NewPolicy(s, mon, n, nil)
	if err != nil {
		t.Fatal(err)
	}
	return p, n
}

func req(remote string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/auto/v5.1", nil)
	r.RemoteAddr = remote
	return r
}

func denied(t *testing.T, err error) *proxy.Denied {
	t.Helper()
	var d *proxy.Denied
	if !errors.As(err, &d) {
		t.Fatalf("error %v is not a *proxy.Denied", err)
	}
	return d
}

var twoViewers = Settings{Viewers: []Viewer{
	{Name: "John", Addresses: []string{"100.67.117.32"}, Limit: 2},
	{Name: "Mom", Addresses: []string{"10.8.0.0/24"}, Limit: 1},
}}

func TestOpenModeAllowsAnyone(t *testing.T) {
	p, _ := newPolicy(t, DefaultSettings(), &fakeMon{})
	if err := p.Authorize(req("203.0.113.9:5555")); err != nil {
		t.Errorf("open mode refused a client: %v", err)
	}
	if !p.isAdmin(req("203.0.113.9:5555")) {
		t.Error("open mode should leave /antennarr open")
	}
}

func TestAllowlist(t *testing.T) {
	p, _ := newPolicy(t, twoViewers, &fakeMon{})
	for remote, want := range map[string]bool{
		"100.67.117.32:41000":          true,
		"10.8.0.77:41000":              true, // inside Mom's CIDR
		"10.8.1.77:41000":              false,
		"100.67.117.33:41000":          false,
		"203.0.113.9:41000":            false,
		"[::ffff:100.67.117.32]:41000": true, // IPv4 seen over a dual-stack socket
		"not-an-address":               false,
	} {
		err := p.Authorize(req(remote))
		if (err == nil) != want {
			t.Errorf("%s: allowed=%v, want %v (%v)", remote, err == nil, want, err)
		}
		if err != nil {
			if d := denied(t, err); d.Code != http.StatusForbidden || d.Reason != ReasonNotAllowed {
				t.Errorf("%s: %+v", remote, d)
			}
		}
	}
}

func TestMostSpecificAddressWins(t *testing.T) {
	s := Settings{Viewers: []Viewer{
		{Name: "Office", Addresses: []string{"10.0.0.0/8"}, Limit: 1},
		{Name: "Alice", Addresses: []string{"10.1.2.3"}, Limit: 3},
	}}
	mon := &fakeMon{}
	mon.set(true, 8)
	p, _ := newPolicy(t, s, mon)

	if got := p.TunerCount(req("10.1.2.3:1"), 8); got != 3 {
		t.Errorf("Alice's address: TunerCount = %d, want 3", got)
	}
	if got := p.TunerCount(req("10.9.9.9:1"), 8); got != 1 {
		t.Errorf("office address: TunerCount = %d, want 1", got)
	}
}

func TestViewerLimit(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	p, _ := newPolicy(t, twoViewers, mon)
	mom := req("10.8.0.5:1")

	rel, err := p.Stream(mom)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Stream(mom)
	d := denied(t, err)
	if d.Code != http.StatusServiceUnavailable || d.Reason != ReasonViewerLimit {
		t.Errorf("second stream: %+v", d)
	}

	// John isn't affected by Mom's limit.
	if _, err := p.Stream(req("100.67.117.32:1")); err != nil {
		t.Errorf("John refused: %v", err)
	}

	rel()
	rel() // releasing twice must not free a second tuner
	if _, err := p.Stream(mom); err != nil {
		t.Errorf("after release: %v", err)
	}
	if _, err := p.Stream(mom); err == nil {
		t.Error("double release let a second stream in")
	}
}

func TestLimitZeroPausesViewer(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	s := Settings{Viewers: []Viewer{{Name: "Cabin", Addresses: []string{"10.8.0.6"}, Limit: 0}}}
	p, _ := newPolicy(t, s, mon)
	if _, err := p.Stream(req("10.8.0.6:1")); denied(t, err).Reason != ReasonViewerLimit {
		t.Error("a paused viewer got a tuner")
	}
	if err := p.Authorize(req("10.8.0.6:1")); err != nil {
		t.Errorf("a paused viewer should still reach discover/lineup: %v", err)
	}
}

func TestSharedCapAcrossViewers(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	s := twoViewers
	s.SharedCap = 2
	p, _ := newPolicy(t, s, mon)

	if _, err := p.Stream(req("100.67.117.32:1")); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Stream(req("10.8.0.5:1")); err != nil {
		t.Fatal(err)
	}
	_, err := p.Stream(req("100.67.117.32:2"))
	if d := denied(t, err); d.Reason != ReasonSharedCap {
		t.Errorf("third stream: %+v", d)
	}
}

func TestOwnerReserveCountsOurStreamsImmediately(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4) // nothing in use on the tuner yet
	s := Settings{OwnerReserve: 1}
	p, _ := newPolicy(t, s, mon)

	var rels []func()
	for i := 0; i < 3; i++ { // 4 tuners - 1 reserved = 3 for viewers
		rel, err := p.Stream(req("203.0.113.9:1"))
		if err != nil {
			t.Fatalf("stream %d: %v", i+1, err)
		}
		rels = append(rels, rel)
	}
	_, err := p.Stream(req("203.0.113.9:1"))
	if d := denied(t, err); d.Reason != ReasonOwnerReserve {
		t.Errorf("fourth stream: %+v", d)
	}
	rels[0]()
	if _, err := p.Stream(req("203.0.113.9:1")); err != nil {
		t.Errorf("after one ended: %v", err)
	}
}

func TestOwnerReserveCountsLocalUseButNotOurOwn(t *testing.T) {
	mon := &fakeMon{}
	// The owner's TVs hold two tuners. A stream of ours shows up in status too,
	// but must not be counted twice (we count our own streams ourselves).
	mon.set(true, 4, local, local, ours, idle)
	p, _ := newPolicy(t, Settings{OwnerReserve: 1}, mon)

	// 4 total - 1 reserved - 2 local = 1 available for viewers.
	if _, err := p.Stream(req("203.0.113.9:1")); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := p.Stream(req("203.0.113.9:1"))
	if d := denied(t, err); d.Reason != ReasonOwnerReserve {
		t.Errorf("second: %+v", d)
	}
}

func TestOwnerReserveZeroUsesEveryTuner(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 2)
	p, _ := newPolicy(t, Settings{}, mon)
	for i := 0; i < 2; i++ {
		if _, err := p.Stream(req("203.0.113.9:1")); err != nil {
			t.Fatalf("stream %d: %v", i+1, err)
		}
	}
	if _, err := p.Stream(req("203.0.113.9:1")); err == nil {
		t.Error("a third stream fit on a two-tuner device")
	}
}

func TestUnknownTunerCountSkipsReserveCheck(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 0)
	p, _ := newPolicy(t, Settings{OwnerReserve: 1}, mon)
	if _, err := p.Stream(req("203.0.113.9:1")); err != nil {
		t.Errorf("refused when the tuner count is unknown: %v", err)
	}
}

func TestOfflineTunerRefusesStreams(t *testing.T) {
	mon := &fakeMon{}
	mon.set(false, 4)
	p, _ := newPolicy(t, DefaultSettings(), mon)
	_, err := p.Stream(req("203.0.113.9:1"))
	if d := denied(t, err); d.Code != http.StatusServiceUnavailable || d.Reason != ReasonOffline {
		t.Errorf("%+v", d)
	}
}

func TestStreamFromUnknownAddressIsForbidden(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	p, _ := newPolicy(t, twoViewers, mon)
	_, err := p.Stream(req("203.0.113.9:1"))
	if d := denied(t, err); d.Code != http.StatusForbidden {
		t.Errorf("%+v", d)
	}
}

func TestTunerCountAdvertised(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	john, mom := req("100.67.117.32:1"), req("10.8.0.5:1")
	cases := []struct {
		name string
		s    Settings
		r    *http.Request
		real int
		want int
	}{
		{"open, no reserve", Settings{}, john, 4, 4},
		{"reserve subtracted", Settings{OwnerReserve: 1}, john, 4, 3},
		{"viewer limit lower", Settings{OwnerReserve: 1, Viewers: twoViewers.Viewers}, john, 4, 2},
		{"viewer limit 1", Settings{Viewers: twoViewers.Viewers}, mom, 4, 1},
		{"shared cap lowest", Settings{SharedCap: 1, Viewers: twoViewers.Viewers}, john, 4, 1},
		{"reserve exceeds tuners", Settings{OwnerReserve: 9}, john, 4, 0},
	}
	for _, c := range cases {
		p, _ := newPolicy(t, c.s, mon)
		if got := p.TunerCount(c.r, c.real); got != c.want {
			t.Errorf("%s: TunerCount = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestUpdate(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	p, _ := newPolicy(t, twoViewers, mon)
	rel, err := p.Stream(req("10.8.0.5:1")) // Mom, limit 1
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// Invalid settings are refused and change nothing.
	if err := p.Update(Settings{Viewers: []Viewer{{Name: ""}}}); err == nil {
		t.Fatal("accepted invalid settings")
	}
	if err := p.Authorize(req("100.67.117.32:1")); err != nil {
		t.Errorf("invalid update changed behaviour: %v", err)
	}

	// Valid settings take effect; Mom's running stream is left alone.
	if err := p.Update(Settings{Viewers: []Viewer{{Name: "Mom", Addresses: []string{"10.8.0.0/24"}, Limit: 2}}}); err != nil {
		t.Fatal(err)
	}
	if err := p.Authorize(req("100.67.117.32:1")); err == nil {
		t.Error("John should no longer be allowed")
	}
	if len(p.Active()) != 1 {
		t.Errorf("Update dropped a running stream: %+v", p.Active())
	}
	if _, err := p.Stream(req("10.8.0.5:2")); err != nil {
		t.Errorf("Mom's raised limit not applied: %v", err)
	}
}

func TestSettingsReturnsCopy(t *testing.T) {
	p, _ := newPolicy(t, twoViewers, &fakeMon{})
	s := p.Settings()
	s.Viewers[0].Name = "tampered"
	if p.Settings().Viewers[0].Name == "tampered" {
		t.Error("Settings() exposed internal state")
	}
}

func TestRefusalsAreNotedOncePerMinute(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	p, notes := newPolicy(t, twoViewers, mon)
	now := time.Unix(1_000_000, 0)
	p.now = func() time.Time { return now }

	mom := req("10.8.0.5:1")
	if _, err := p.Stream(mom); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ { // a media server retrying in a loop
		p.Stream(mom)
	}
	if notes.count() != 1 {
		t.Fatalf("%d notes for a retry loop, want 1", notes.count())
	}
	now = now.Add(61 * time.Second)
	p.Stream(mom)
	if notes.count() != 2 {
		t.Errorf("%d notes a minute later, want 2", notes.count())
	}
	if notes.notes[0] != "warn: Mom was refused a tuner: all 1 of your tuners are in use" {
		t.Errorf("note text = %q", notes.notes[0])
	}
}

func TestActiveListsStreamsOldestFirst(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	p, _ := newPolicy(t, twoViewers, mon)
	p.Stream(req("100.67.117.32:1"))
	p.Stream(req("10.8.0.5:1"))

	a := p.Active()
	if len(a) != 2 || a[0].Viewer != "John" || a[1].Viewer != "Mom" || a[0].ID >= a[1].ID {
		t.Fatalf("active = %+v", a)
	}

	rec := httptest.NewRecorder()
	p.ActiveHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/antennarr/streams", nil))
	var got []Active
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got) != 2 {
		t.Errorf("handler body %s (%v)", rec.Body, err)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("headers = %v", rec.Header())
	}
}

func TestAdminOnly(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "secret") })
	s := twoViewers
	s.Admin = []string{"100.64.0.0/10"}
	p, _ := newPolicy(t, s, &fakeMon{})
	h := p.AdminOnly(ok)

	for remote, want := range map[string]int{
		"127.0.0.1:5000":     200, // loopback
		"[::1]:5000":         200,
		"100.100.1.1:5000":   200, // configured admin range
		"100.67.117.32:5000": 200, // John is inside 100.64.0.0/10 too
		"10.8.0.5:5000":      403, // a viewer, but not an admin
		"203.0.113.9:5000":   403,
		"garbage":            403,
	} {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/antennarr/status", nil)
		r.RemoteAddr = remote
		h.ServeHTTP(rec, r)
		if rec.Code != want {
			t.Errorf("%s: status %d, want %d", remote, rec.Code, want)
		}
	}
}

// Many requests racing for the last tuners must never overshoot the limit.
func TestConcurrentStreamsNeverExceedLimit(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 8)
	s := Settings{SharedCap: 3, Viewers: []Viewer{
		{Name: "A", Addresses: []string{"10.0.0.1"}, Limit: 2},
		{Name: "B", Addresses: []string{"10.0.0.2"}, Limit: 2},
	}}
	p, _ := newPolicy(t, s, mon)

	var granted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 60; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			remote := "10.0.0.1:1"
			if i%2 == 1 {
				remote = "10.0.0.2:1"
			}
			if _, err := p.Stream(req(remote)); err == nil {
				granted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if granted.Load() != 3 || len(p.Active()) != 3 {
		t.Errorf("granted %d, active %d; want exactly 3 (the shared cap)", granted.Load(), len(p.Active()))
	}
}
