package upgrade

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/firmware"
	"github.com/johnschult/yagi/internal/hdhr"
	"github.com/johnschult/yagi/internal/hdhr/hdhrtest"
	"github.com/johnschult/yagi/internal/tuner"
)

// ---- fakes ----

// fakeTuner is the monitor, recording what the install tells it.
type fakeTuner struct {
	mu       sync.Mutex
	updating []bool // every SetUpdating, in order
	notes    []string
	kicks    int
}

func (f *fakeTuner) SetUpdating(on bool) {
	f.mu.Lock()
	f.updating = append(f.updating, on)
	f.mu.Unlock()
}
func (f *fakeTuner) Note(kind, text string) {
	f.mu.Lock()
	f.notes = append(f.notes, kind+": "+text)
	f.mu.Unlock()
}
func (f *fakeTuner) Kick()                 { f.mu.Lock(); f.kicks++; f.mu.Unlock() }
func (f *fakeTuner) Snapshot() tuner.State { return tuner.State{} }
func (f *fakeTuner) flags() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]bool(nil), f.updating...)
}
func (f *fakeTuner) text() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.notes, "\n")
}

// fakeDownload hands out an image, or an error, and counts.
type fakeDownload struct {
	mu    sync.Mutex
	img   []byte
	err   error
	calls []string
}

func (d *fakeDownload) Fetch(ctx context.Context, family, release string) (firmware.Image, error) {
	d.mu.Lock()
	d.calls = append(d.calls, family+"/"+release)
	d.mu.Unlock()
	if d.err != nil {
		return firmware.Image{}, d.err
	}
	return firmware.Image{Family: family, Release: release, Data: d.img, SHA256: "abc123"}, nil
}

func image(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*13 + 5)
	}
	return b
}

type rig struct {
	m     *Manager
	srv   *hdhrtest.Server
	tun   *fakeTuner
	dl    *fakeDownload
	req   Request
	chg   int
	snaps []Status // the status after every change
	mu    sync.Mutex
}

// newRig builds a Manager talking to a fake tuner on a port of its own, with short waits.
func newRig(t *testing.T, o hdhrtest.Options) *rig {
	t.Helper()
	srv, err := hdhrtest.New("127.0.0.1:0", o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	r := &rig{srv: srv, tun: &fakeTuner{}, dl: &fakeDownload{img: image(5000)}}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	r.m = New(ctx, Config{
		Download: r.dl,
		Dial:     func(ctx context.Context, _ string) (*hdhr.Conn, error) { return hdhr.Dial(ctx, srv.Addr()) },
		OnChange: func() {
			sn := r.m.Status("antenna")
			r.mu.Lock()
			r.chg++
			if sn != nil {
				r.snaps = append(r.snaps, *sn)
			}
			r.mu.Unlock()
		},
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RestartWait: 40 * time.Millisecond, BackWithin: 1500 * time.Millisecond, PollEvery: 20 * time.Millisecond, Deadline: 20 * time.Second,
	})
	r.req = Request{Device: "antenna", Title: "Living room", Host: "tuner.test", Family: "hdhomerun_dvr_atsc3", Current: "20250815", Release: "20260326", Tuner: r.tun}
	return r
}

func (r *rig) run(t *testing.T) *Status {
	t.Helper()
	if err := r.m.Start(r.req); err != nil {
		t.Fatal(err)
	}
	r.m.Wait()
	s := r.m.Status("antenna")
	if s == nil {
		t.Fatal("no status")
	}
	return s
}

func (r *rig) endMarkers() int {
	n := 0
	for _, f := range r.srv.Frames() {
		if f.Type == hdhr.TypeUpgradeReq && len(f.Payload) == 4 && binary.BigEndian.Uint32(f.Payload) == 0xFFFFFFFF {
			n++
		}
	}
	return n
}

// ---- the good path ----

func TestAnInstallDownloadsSendsWaitsAndChecksTheVersion(t *testing.T) {
	r := newRig(t, hdhrtest.Options{RebootFor: 200 * time.Millisecond, VersionAfter: "20260326"})
	s := r.run(t)

	if s.Phase != PhaseDone || s.Error != "" || s.Release != "20260326" || s.From != "20250815" || s.SHA256 != "abc123" || s.Percent != 100 {
		t.Fatalf("status = %+v", s)
	}
	if s.Finished.IsZero() || s.Started.IsZero() {
		t.Errorf("times: %+v", s)
	}
	if got := r.srv.Image(); string(got) != string(r.dl.img) {
		t.Errorf("the tuner got %d bytes, want the %d downloaded", len(got), len(r.dl.img))
	}
	if len(r.dl.calls) != 1 || r.dl.calls[0] != "hdhomerun_dvr_atsc3/20260326" {
		t.Errorf("downloads = %v", r.dl.calls)
	}
	if f := r.tun.flags(); len(f) != 2 || !f[0] || f[1] {
		t.Errorf("the monitor was told %v, want it marked updating and then not", f)
	}
	if r.tun.kicks == 0 {
		t.Error("the monitor was not asked to look again")
	}
	for _, want := range []string{"Updating Living room firmware from 20250815 to 20260326", "sent to Living room", "is now running firmware 20260326"} {
		if !strings.Contains(r.tun.text(), want) {
			t.Errorf("the timeline lacks %q:\n%s", want, r.tun.text())
		}
	}
	if r.endMarkers() != 1 {
		t.Errorf("%d end markers: an image is sent once", r.endMarkers())
	}
}

func TestATunerThatRestartsBeforeAnsweringTheEndIsStillSuccess(t *testing.T) {
	r := newRig(t, hdhrtest.Options{DropAfterEnd: true, RebootFor: 150 * time.Millisecond, VersionAfter: "20260326"})
	if s := r.run(t); s.Phase != PhaseDone {
		t.Fatalf("status = %+v", s)
	}
}

func TestTheStatusMovesThroughThePhasesInOrderAndProgressNeverGoesBack(t *testing.T) {
	r := newRig(t, hdhrtest.Options{RebootFor: 100 * time.Millisecond, VersionAfter: "20260326"})
	r.run(t)

	r.mu.Lock()
	snaps := append([]Status(nil), r.snaps...)
	r.mu.Unlock()
	order := []Phase{PhaseDownloading, PhaseSending, PhaseRestarting, PhaseVerifying, PhaseDone}
	rank := map[Phase]int{}
	for i, p := range order {
		rank[p] = i
	}
	last, pct := -1, 0
	seen := map[Phase]bool{}
	for _, sn := range snaps {
		i, ok := rank[sn.Phase]
		if !ok {
			t.Fatalf("a phase that is not one of ours: %q", sn.Phase)
		}
		if i < last {
			t.Errorf("went from %s back to %s", order[last], sn.Phase)
		}
		last = i
		seen[sn.Phase] = true
		if sn.Percent < pct {
			t.Errorf("progress went from %d%% back to %d%%", pct, sn.Percent)
		}
		pct = sn.Percent
	}
	for _, p := range order {
		if !seen[p] {
			t.Errorf("the page was never told the phase %q: %v", p, snaps)
		}
	}
	mid := false
	for _, sn := range snaps {
		if sn.Phase == PhaseSending && sn.Percent > 0 && sn.Percent < 100 {
			mid = true
		}
	}
	if !mid {
		t.Error("no progress between 0 and 100 while sending")
	}
}

// ---- what goes wrong ----

func TestADownloadThatFailsLeavesTheTunerAlone(t *testing.T) {
	for name, err := range map[string]error{"not there": firmware.ErrNotFound, "network": errors.New("couldn't download the firmware from SiliconDust: no route")} {
		r := newRig(t, hdhrtest.Options{})
		r.dl.err = err
		s := r.run(t)
		if s.Phase != PhaseFailed || !strings.Contains(s.Error, "has not been changed") {
			t.Errorf("%s: status = %+v", name, s)
		}
		if n := len(r.srv.Frames()); n != 0 {
			t.Errorf("%s: %d frames were sent to the tuner", name, n)
		}
		if f := r.tun.flags(); len(f) != 0 {
			t.Errorf("%s: the monitor was told %v before anything was done to the tuner", name, f)
		}
		if !strings.Contains(r.tun.text(), "failed") {
			t.Errorf("%s: no failure on the timeline", name)
		}
	}
}

func TestATunerThatCannotBeReachedIsLeftAlone(t *testing.T) {
	r := newRig(t, hdhrtest.Options{})
	r.srv.Close() // nothing is listening
	s := r.run(t)
	if s.Phase != PhaseFailed || !strings.Contains(s.Error, "couldn't reach") || !strings.Contains(s.Error, "has not been changed") {
		t.Errorf("status = %+v", s)
	}
	if f := r.tun.flags(); len(f) != 2 || f[1] {
		t.Errorf("updating flags %v: they must be cleared after a failure", f)
	}
}

func TestATunerThatRefusesTheImageSaysSoAndWhatItStillRuns(t *testing.T) {
	r := newRig(t, hdhrtest.Options{RejectUpgrade: "ERROR: invalid firmware"})
	s := r.run(t)
	if s.Phase != PhaseFailed || !strings.Contains(s.Error, "invalid firmware") || !strings.Contains(s.Error, "still running 20250815") {
		t.Fatalf("status = %+v", s)
	}
	if r.endMarkers() != 1 {
		t.Errorf("%d end markers: a refused flash is not tried again", r.endMarkers())
	}
	if f := r.tun.flags(); len(f) != 2 || f[1] {
		t.Errorf("updating flags %v", f)
	}
}

func TestAnUploadThatDiesBeforeTheEndIsAFailureThatChangedNothing(t *testing.T) {
	r := newRig(t, hdhrtest.Options{DropBeforeEnd: 2})
	s := r.run(t)
	if s.Phase != PhaseFailed || !strings.Contains(s.Error, "didn't finish") || !strings.Contains(s.Error, "still be running 20250815") {
		t.Fatalf("status = %+v", s)
	}
	if r.srv.Upgraded() {
		t.Error("the tuner took an image that was never completed")
	}
	if r.endMarkers() != 0 {
		t.Error("an end marker was sent for an upload that failed")
	}
}

func TestATunerThatNeverComesBackIsToldToThePerson(t *testing.T) {
	r := newRig(t, hdhrtest.Options{DropAfterEnd: true, RebootFor: time.Hour, VersionAfter: "20260326"})
	s := r.run(t)
	if s.Phase != PhaseFailed || !strings.Contains(s.Error, "has not come back") || !strings.Contains(s.Error, "unplug it") {
		t.Fatalf("status = %+v", s)
	}
	if f := r.tun.flags(); len(f) != 2 || f[1] {
		t.Errorf("the monitor was left marked as updating: %v", f)
	}
	if r.endMarkers() != 1 {
		t.Errorf("%d end markers: it must not send the image again to a tuner that is not answering", r.endMarkers())
	}
}

func TestATunerThatComesBackOnTheOldVersionDidNotTakeIt(t *testing.T) {
	r := newRig(t, hdhrtest.Options{DropAfterEnd: true, RebootFor: 100 * time.Millisecond}) // VersionAfter empty: unchanged
	s := r.run(t)
	if s.Phase != PhaseFailed || !strings.Contains(s.Error, "still running 20250815, not 20260326") || !strings.Contains(s.Error, "did not take") {
		t.Fatalf("status = %+v", s)
	}
	if r.endMarkers() != 1 {
		t.Errorf("%d end markers: it is not tried again", r.endMarkers())
	}
}

func TestStoppingBeforeTheImageIsSentChangesNothing(t *testing.T) {
	srv, err := hdhrtest.New("127.0.0.1:0", hdhrtest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	tun := &fakeTuner{}
	ctx, cancel := context.WithCancel(context.Background())
	m := New(ctx, Config{
		Download: &fakeDownload{img: image(2000)},
		Dial: func(ctx context.Context, _ string) (*hdhr.Conn, error) {
			cancel() // Yagi is stopping, just as it connects
			return hdhr.Dial(context.Background(), srv.Addr())
		},
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RestartWait: 20 * time.Millisecond, BackWithin: 500 * time.Millisecond, PollEvery: 10 * time.Millisecond,
	})
	if err := m.Start(Request{Device: "antenna", Title: "T", Host: "h", Family: "hdhomerun_dvr_atsc3", Current: "20250815", Release: "20260326", Tuner: tun}); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	s := m.Status("antenna")
	if s.Phase != PhaseFailed || srv.Upgraded() {
		t.Errorf("status = %+v, upgraded = %v", s, srv.Upgraded())
	}
	if f := tun.flags(); len(f) != 2 || f[1] {
		t.Errorf("updating flags %v", f)
	}
}

// ---- running them ----

func TestOnlyOneInstallAtATimeOnATuner(t *testing.T) {
	r := newRig(t, hdhrtest.Options{RebootFor: 300 * time.Millisecond, VersionAfter: "20260326"})
	if err := r.m.Start(r.req); err != nil {
		t.Fatal(err)
	}
	if !r.m.Active("antenna") {
		t.Error("not active")
	}
	if err := r.m.Start(r.req); !errors.Is(err, ErrBusy) {
		t.Errorf("a second start: %v", err)
	}
	r.m.Wait()
	if r.m.Active("antenna") {
		t.Error("still active")
	}
	if len(r.dl.calls) != 1 {
		t.Errorf("%d downloads", len(r.dl.calls))
	}
	// and after it has finished another may begin
	r.srv.SetValue("/sys/version", "20250815")
	r.dl.img = image(4000)
	if err := r.m.Start(r.req); err != nil {
		t.Errorf("a new install after the last: %v", err)
	}
	r.m.Wait()
}

func TestTunersAreIndependent(t *testing.T) {
	a := newRig(t, hdhrtest.Options{RebootFor: 100 * time.Millisecond, VersionAfter: "20260326"})
	b := newRig(t, hdhrtest.Options{RejectUpgrade: "no"})
	a.req.Device = "one"
	if err := a.m.Start(a.req); err != nil {
		t.Fatal(err)
	}
	if err := b.m.Start(b.req); err != nil {
		t.Fatal(err)
	}
	a.m.Wait()
	b.m.Wait()
	if a.m.Status("one").Phase != PhaseDone || b.m.Status("antenna").Phase != PhaseFailed {
		t.Errorf("%+v / %+v", a.m.Status("one"), b.m.Status("antenna"))
	}
}

func TestStatusIsACopyAndForgetClearsAFinishedOneOnly(t *testing.T) {
	r := newRig(t, hdhrtest.Options{RebootFor: 100 * time.Millisecond, VersionAfter: "20260326"})
	r.run(t)
	s := r.m.Status("antenna")
	s.Phase = "tampered"
	if r.m.Status("antenna").Phase != PhaseDone {
		t.Error("changing the returned status changed the manager's")
	}
	r.m.Forget("antenna")
	if r.m.Status("antenna") != nil {
		t.Error("a finished status was not forgotten")
	}
	if r.m.Status("never-seen") != nil {
		t.Error("a status for a tuner that never had one")
	}

	// A running install is not forgotten.
	r2 := newRig(t, hdhrtest.Options{RebootFor: 400 * time.Millisecond, VersionAfter: "20260326"})
	r2.m.Start(r2.req)
	r2.m.Forget("antenna")
	if r2.m.Status("antenna") == nil {
		t.Error("an install under way was forgotten")
	}
	r2.m.Wait()
}

func TestTheOwnerIsToldAsThingsChange(t *testing.T) {
	r := newRig(t, hdhrtest.Options{RebootFor: 100 * time.Millisecond, VersionAfter: "20260326"})
	r.run(t)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.chg < 3 {
		t.Errorf("OnChange was called %d times: at least the start, progress and the end", r.chg)
	}
}

func TestARequestWithNoTunerIsRefused(t *testing.T) {
	r := newRig(t, hdhrtest.Options{})
	for name, req := range map[string]Request{"no host": {Device: "a", Tuner: r.tun}, "no monitor": {Device: "a", Host: "h"}} {
		if err := r.m.Start(req); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if r.m.Status("a") != nil {
		t.Error("a refused request left a status")
	}
}

func TestNotesAndStatusNeverCarryTheImage(t *testing.T) {
	r := newRig(t, hdhrtest.Options{RejectUpgrade: "bad"})
	r.dl.img = []byte(strings.Repeat("SECRETIMAGEBYTES", 400))
	s := r.run(t)
	if strings.Contains(s.Error, "SECRET") || strings.Contains(r.tun.text(), "SECRET") || strings.Contains(fmt.Sprintf("%+v", s), "SECRET") {
		t.Error("the image leaked into a message")
	}
}
