package access

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
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

const testDevice = "antenna"

// testPolicy is a policy together with the view of it for one device, so tests
// can call Stream, Authorize and TunerCount directly.
type testPolicy struct {
	*Policy
	*DeviceView
}

func newPolicy(t *testing.T, s Settings, mon *fakeMon) (*testPolicy, *fakeNotes) {
	t.Helper()
	n := &fakeNotes{}
	p, err := NewPolicy(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.AddDevice(testDevice, mon, n)
	return &testPolicy{p, p.Device(testDevice)}, n
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

// oneViewer is a single viewer on 192.168.1.9 with plenty of tuners: the setup
// for tests that need someone allowed to stream.
func oneViewer() Settings {
	s := DefaultSettings()
	s.Viewers = []Viewer{{Name: "Guest", Addresses: []string{"192.168.1.9"}, Limit: 8}}
	return s
}

var twoViewers = Settings{Viewers: []Viewer{
	{Name: "John", Addresses: []string{"100.67.117.32"}, Limit: 2},
	{Name: "Mom", Addresses: []string{"10.8.0.0/24"}, Limit: 1},
}}

// Until a viewer is added nobody can use the tuner: the install fails closed.
func TestNoViewersMeansNobodyCanStream(t *testing.T) {
	p, _ := newPolicy(t, DefaultSettings(), &fakeMon{})
	for _, remote := range []string{"127.0.0.1:1", "192.168.1.9:1", "100.64.0.7:1", "203.0.113.9:1"} {
		if err := p.Authorize(req(remote)); err == nil {
			t.Errorf("%s was allowed to use the tuner with no viewers", remote)
		}
	}
	if p.Mode() != "no-viewers" {
		t.Errorf("mode = %q", p.Mode())
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
	s := oneViewer()
	p, _ := newPolicy(t, s, mon)

	var rels []func()
	for i := 0; i < 3; i++ { // 4 tuners - 1 reserved = 3 for viewers
		rel, err := p.Stream(req("192.168.1.9:1"))
		if err != nil {
			t.Fatalf("stream %d: %v", i+1, err)
		}
		rels = append(rels, rel)
	}
	_, err := p.Stream(req("192.168.1.9:1"))
	if d := denied(t, err); d.Reason != ReasonOwnerReserve {
		t.Errorf("fourth stream: %+v", d)
	}
	rels[0]()
	if _, err := p.Stream(req("192.168.1.9:1")); err != nil {
		t.Errorf("after one ended: %v", err)
	}
}

func TestOwnerReserveCountsLocalUseButNotOurOwn(t *testing.T) {
	mon := &fakeMon{}
	// The owner's TVs hold two tuners. A stream of ours shows up in status too,
	// but must not be counted twice (we count our own streams ourselves).
	mon.set(true, 4, local, local, ours, idle)
	p, _ := newPolicy(t, oneViewer(), mon)

	// 4 total - 1 reserved - 2 local = 1 available for viewers.
	if _, err := p.Stream(req("192.168.1.9:1")); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := p.Stream(req("192.168.1.9:1"))
	if d := denied(t, err); d.Reason != ReasonOwnerReserve {
		t.Errorf("second: %+v", d)
	}
}

func TestOwnerReserveZeroUsesEveryTuner(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 2)
	zero := oneViewer()
	zero.OwnerReserve = 0
	p, _ := newPolicy(t, zero, mon)
	for i := 0; i < 2; i++ {
		if _, err := p.Stream(req("192.168.1.9:1")); err != nil {
			t.Fatalf("stream %d: %v", i+1, err)
		}
	}
	if _, err := p.Stream(req("192.168.1.9:1")); err == nil {
		t.Error("a third stream fit on a two-tuner device")
	}
}

func TestUnknownTunerCountSkipsReserveCheck(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 0)
	p, _ := newPolicy(t, oneViewer(), mon)
	if _, err := p.Stream(req("192.168.1.9:1")); err != nil {
		t.Errorf("refused when the tuner count is unknown: %v", err)
	}
}

func TestOfflineTunerRefusesStreams(t *testing.T) {
	mon := &fakeMon{}
	mon.set(false, 4)
	p, _ := newPolicy(t, oneViewer(), mon)
	_, err := p.Stream(req("192.168.1.9:1"))
	if d := denied(t, err); d.Code != http.StatusServiceUnavailable || d.Reason != ReasonOffline {
		t.Errorf("%+v", d)
	}
}

func TestStreamFromUnknownAddressIsForbidden(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	p, _ := newPolicy(t, twoViewers, mon)
	_, err := p.Stream(req("192.168.1.9:1"))
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
		t.Errorf("invalid update changed behavior: %v", err)
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

func TestAdminHomeRedirectsAdminsOnly(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tuner front page") })

	t.Run("no viewers: only this machine is an admin", func(t *testing.T) {
		p, _ := newPolicy(t, DefaultSettings(), &fakeMon{})
		h := p.AdminHome("/antennarr/", next)
		for remote, want := range map[string]int{"127.0.0.1:1": http.StatusFound, "192.168.1.9:1": http.StatusOK} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req(remote))
			if rec.Code != want {
				t.Errorf("%s: %d, want %d", remote, rec.Code, want)
			}
		}
	})

	t.Run("enforced: admins are sent to the UI, others reach the tuner side", func(t *testing.T) {
		s := twoViewers
		s.Admin = []string{"192.168.68.0/22"}
		p, _ := newPolicy(t, s, &fakeMon{})
		h := p.AdminHome("/antennarr/", next)
		for remote, want := range map[string]int{
			"127.0.0.1:1":     http.StatusFound, // loopback
			"192.168.68.9:1":  http.StatusFound, // configured admin range
			"100.67.117.32:1": http.StatusOK,    // a viewer is not an admin: gets the normal handler
			"203.0.113.9:1":   http.StatusOK,
		} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req(remote))
			if rec.Code != want {
				t.Errorf("%s: %d, want %d", remote, rec.Code, want)
			}
			if want == http.StatusOK && rec.Body.String() != "tuner front page" {
				t.Errorf("%s: next handler not reached: %q", remote, rec.Body)
			}
		}
	})
}

func TestAdminHomeSendsEveryBrowserToTheAdminPage(t *testing.T) {
	p, _ := newPolicy(t, twoViewers, &fakeMon{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tuner front page") })
	h := p.AdminHome("/antennarr/", next)

	for _, remote := range []string{"203.0.113.9:1", "100.67.117.32:1"} { // not admins: a stranger and a viewer
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		r.Header.Set("Accept", "text/html,application/xhtml+xml")
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/antennarr/" {
			t.Errorf("a browser from %s: %d %q, want a redirect to the admin page", remote, rec.Code, rec.Header().Get("Location"))
		}
	}
	// A media server or script (no HTML in Accept) still reaches the tuner's side.
	for _, accept := range []string{"", "*/*", "application/json"} {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "203.0.113.9:1"
		if accept != "" {
			r.Header.Set("Accept", accept)
		}
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusOK || rec.Body.String() != "tuner front page" {
			t.Errorf("Accept %q: %d %q, want the tuner's own handler", accept, rec.Code, rec.Body)
		}
	}
}

func TestIPv6LinkLocalAddressesCanBeListedDespiteTheirInterfaceName(t *testing.T) {
	s := twoViewers
	s.Admin = []string{"fe80::/10"}
	s.Viewers = append(append([]Viewer(nil), s.Viewers...), Viewer{Name: "Phone", Addresses: []string{"fe80::1822:6815:bce3:f6e9"}, Limit: 1})
	p, _ := newPolicy(t, s, &fakeMon{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") })
	for _, remote := range []string{"[fe80::1822:6815:bce3:f6e9%en0]:50152", "[fe80::1822:6815:bce3:f6e9]:50152"} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		if !p.IsAdmin(r) {
			t.Errorf("%s should be an admin through fe80::/10", remote)
		}
		rec := httptest.NewRecorder()
		p.AdminOnly(next).ServeHTTP(rec, r)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: %d from AdminOnly", remote, rec.Code)
		}
	}
	// And a single link-local address works as a viewer's address.
	r := httptest.NewRequest("GET", "/discover.json", nil)
	r.RemoteAddr = "[fe80::1822:6815:bce3:f6e9%en0]:1"
	if err := p.Authorize(r); err != nil {
		t.Errorf("a viewer listed by its link-local address was refused: %v", err)
	}
}

// Admin never depends on viewers: it is this machine, an admin address or a
// signed-in Plex account, whether or not any viewer exists.
func TestAdminIsIndependentOfViewers(t *testing.T) {
	for name, s := range map[string]Settings{"no viewers": DefaultSettings(), "viewers": twoViewers} {
		s.Admin = []string{"192.168.68.0/22"}
		p, _ := newPolicy(t, s, &fakeMon{})
		for remote, want := range map[string]bool{
			"127.0.0.1:1":    true,
			"[::1]:1":        true,
			"192.168.68.9:1": true,
			"192.168.1.9:1":  false,
			"203.0.113.9:1":  false,
			"100.64.0.7:1":   false,
		} {
			if got := p.IsAdmin(req(remote)); got != want {
				t.Errorf("%s: %s admin = %v, want %v", name, remote, got, want)
			}
		}
	}
}

func TestHasAdmin(t *testing.T) {
	p, _ := newPolicy(t, DefaultSettings(), &fakeMon{})
	if p.HasAdmin() {
		t.Error("a fresh install has no admin")
	}
	s := DefaultSettings()
	s.Admin = []string{"100.64.0.1"}
	p, _ = newPolicy(t, s, &fakeMon{})
	if !p.HasAdmin() {
		t.Error("an admin address is an admin")
	}
	s = DefaultSettings()
	s.Plex = &PlexSettings{Enabled: true, Admins: []PlexAccount{{ID: 1, Name: "john"}}}
	p, _ = newPolicy(t, s, &fakeMon{})
	if !p.HasAdmin() {
		t.Error("an allowed Plex account is an admin")
	}
	s.Plex.Enabled = false
	p, _ = newPolicy(t, s, &fakeMon{})
	if p.HasAdmin() {
		t.Error("Plex accounts count only while Plex sign-in is on")
	}
}

func TestPrivateAddr(t *testing.T) {
	for remote, want := range map[string]bool{
		"127.0.0.1": true, "::1": true, "192.168.1.9": true, "10.1.2.3": true, "172.16.5.5": true,
		"100.64.0.7": true, "fd7a:115c:a1e0::1": true, "fe80::1": true,
		"203.0.113.9": false, "8.8.8.8": false, "2001:db8::1": false, "100.128.0.1": false, "172.32.0.1": false,
	} {
		ip := netip.MustParseAddr(remote)
		if got := PrivateAddr(ip); got != want {
			t.Errorf("PrivateAddr(%s) = %v, want %v", remote, got, want)
		}
	}
}

// fakeEvents records what the policy tells the notifier about.
type fakeEvents struct {
	mu      sync.Mutex
	refused []string
	unknown []string
	started []string
}

func (f *fakeEvents) Started(device, title, viewer, channel string) {
	f.mu.Lock()
	f.started = append(f.started, device+"|"+title+"|"+viewer+"|"+channel)
	f.mu.Unlock()
}

func (f *fakeEvents) Refused(device, title, viewer, reason string) {
	f.mu.Lock()
	f.refused = append(f.refused, device+"|"+title+"|"+viewer+"|"+reason)
	f.mu.Unlock()
}
func (f *fakeEvents) Unknown(device, title, addr string) {
	f.mu.Lock()
	f.unknown = append(f.unknown, device+"|"+title+"|"+addr)
	f.mu.Unlock()
}
func (f *fakeEvents) get() (refused, unknown []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.refused...), append([]string(nil), f.unknown...)
}

func TestARefusalReachesTheEventsOnceAMinuteWithTheTunersName(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	s := twoViewers
	s.Labels = map[string]string{testDevice: "Living room"}
	p, _ := newPolicy(t, s, mon)
	ev := &fakeEvents{}
	p.SetEvents(ev)
	now := time.Unix(1_000_000, 0)
	p.now = func() time.Time { return now }

	mom := req("10.8.0.5:1")
	if _, err := p.Stream(mom); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		p.Stream(mom)
	}
	refused, unknown := ev.get()
	if len(refused) != 1 || refused[0] != "antenna|Living room|Mom|viewer-limit" || len(unknown) != 0 {
		t.Errorf("refused = %v, unknown = %v", refused, unknown)
	}
	now = now.Add(61 * time.Second)
	p.Stream(mom)
	if refused, _ := ev.get(); len(refused) != 2 {
		t.Errorf("a minute later: %v", refused)
	}
}

func TestAStreamGivenToAViewerReachesTheEventsWithTheChannel(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	s := twoViewers
	s.Labels = map[string]string{testDevice: "Living room"}
	p, _ := newPolicy(t, s, mon)
	ev := &fakeEvents{}
	p.SetEvents(ev)
	r := req("10.8.0.5:1")
	r.URL.Path = "/auto/v5.1"
	if _, err := p.Stream(r); err != nil {
		t.Fatal(err)
	}
	ev.mu.Lock()
	got := append([]string(nil), ev.started...)
	ev.mu.Unlock()
	if len(got) != 1 || got[0] != "antenna|Living room|Mom|5.1" {
		t.Errorf("started = %v", got)
	}
}

func TestARefusedOrUnknownRequestIsNotAStart(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	p, _ := newPolicy(t, twoViewers, mon)
	ev := &fakeEvents{}
	p.SetEvents(ev)
	mom := req("10.8.0.5:1")
	p.Stream(mom)                  // given
	p.Stream(mom)                  // refused: at its limit
	p.Stream(req("203.0.113.9:1")) // not a viewer
	ev.mu.Lock()
	n := len(ev.started)
	ev.mu.Unlock()
	if n != 1 {
		t.Errorf("%d starts, want 1", n)
	}
}

func TestAnUnknownAddressReachesTheEventsFromAuthorizeAndStream(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	p, _ := newPolicy(t, twoViewers, mon)
	ev := &fakeEvents{}
	p.SetEvents(ev)

	if err := p.Authorize(req("203.0.113.9:5555")); err == nil {
		t.Fatal("an unknown address was allowed")
	}
	if _, err := p.Stream(req("203.0.113.10:5555")); err == nil {
		t.Fatal("an unknown address got a stream")
	}
	_, unknown := ev.get()
	if len(unknown) != 2 || unknown[0] != "antenna|antenna|203.0.113.9" || unknown[1] != "antenna|antenna|203.0.113.10" {
		t.Errorf("unknown = %v", unknown)
	}

	// A viewer, even refused, is not an unknown address.
	if err := p.Authorize(req("10.8.0.5:1")); err != nil {
		t.Fatal(err)
	}
	if _, unknown := ev.get(); len(unknown) != 2 {
		t.Errorf("a viewer was reported as unknown: %v", unknown)
	}
}

func TestPolicyWorksWithoutEvents(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	p, _ := newPolicy(t, twoViewers, mon)
	p.Authorize(req("203.0.113.9:1")) // nobody is listening: it must not panic
	mom := req("10.8.0.5:1")
	p.Stream(mom)
	p.Stream(mom)
}
