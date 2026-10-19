package restart

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/hdhr"
	"github.com/johnschult/yagi/internal/hdhr/hdhrtest"
)

// fakeTuner is the monitor, as far as a restart is concerned.
type fakeTuner struct {
	mu         sync.Mutex
	restarting []bool
	notes      []string
	kicks      int
}

func (f *fakeTuner) SetRestarting(on bool) {
	f.mu.Lock()
	f.restarting = append(f.restarting, on)
	f.mu.Unlock()
}
func (f *fakeTuner) Note(kind, text string) {
	f.mu.Lock()
	f.notes = append(f.notes, kind+": "+text)
	f.mu.Unlock()
}
func (f *fakeTuner) Kick() { f.mu.Lock(); f.kicks++; f.mu.Unlock() }
func (f *fakeTuner) flags() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]bool(nil), f.restarting...)
}
func (f *fakeTuner) lastNote() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.notes) == 0 {
		return ""
	}
	return f.notes[len(f.notes)-1]
}

func rig(t *testing.T, o hdhrtest.Options, tune func(*Config)) (*Manager, *hdhrtest.Server, *fakeTuner) {
	t.Helper()
	srv, err := hdhrtest.New("127.0.0.1:0", o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	cfg := Config{
		Dial:       func(ctx context.Context, host string) (*hdhr.Conn, error) { return hdhr.Dial(ctx, srv.Addr()) },
		GoneWithin: 3 * time.Second, BackWithin: 5 * time.Second, PollEvery: 30 * time.Millisecond,
	}
	if tune != nil {
		tune(&cfg)
	}
	return New(context.Background(), cfg), srv, &fakeTuner{}
}

func start(t *testing.T, m *Manager, tn *fakeTuner) {
	t.Helper()
	if err := m.Start(Request{Device: "antenna", Title: "Living room", Host: "tuner.test", Tuner: tn}); err != nil {
		t.Fatal(err)
	}
	m.Wait()
}

func TestARestartThatTakesIsDoneOnceTheTunerIsBack(t *testing.T) {
	m, srv, tn := rig(t, hdhrtest.Options{RestartFor: 400 * time.Millisecond}, nil)
	start(t, m, tn)
	s := m.Status("antenna")
	if s == nil || s.Phase != PhaseDone || s.Error != "" || s.Finished.IsZero() {
		t.Fatalf("status = %+v", s)
	}
	if srv.Restarts() != 1 {
		t.Errorf("the tuner was told to restart %d times", srv.Restarts())
	}
	if f := tn.flags(); len(f) != 2 || !f[0] || f[1] {
		t.Errorf("the monitor was told %v: expected on, then off", f)
	}
	if !strings.HasPrefix(tn.lastNote(), "ok:") {
		t.Errorf("last note %q", tn.lastNote())
	}
	if tn.kicks == 0 {
		t.Error("the monitor was not told to look again")
	}
}

func TestOnlyTheRestartCommandIsSent(t *testing.T) {
	m, srv, tn := rig(t, hdhrtest.Options{RestartFor: 200 * time.Millisecond}, nil)
	srv.Forget()
	start(t, m, tn)
	var sets int
	for _, f := range srv.Frames() {
		tlvs, _ := hdhr.ParseTLVs(f.Payload)
		var name, value string
		for _, tl := range tlvs {
			switch tl.Tag {
			case hdhr.TagGetSetName:
				name = strings.TrimRight(string(tl.Value), "\x00")
			case hdhr.TagGetSetValue:
				value = strings.TrimRight(string(tl.Value), "\x00")
			}
		}
		if value != "" || f.Type != hdhr.TypeGetSetReq {
			sets++
			if name != "/sys/restart" || value != "self" {
				t.Errorf("sent %q = %q", name, value)
			}
		}
	}
	if sets != 1 {
		t.Errorf("%d writes", sets)
	}
}

func TestATunerThatRestartsWithoutAnsweringStillCounts(t *testing.T) {
	m, _, tn := rig(t, hdhrtest.Options{RestartFor: 400 * time.Millisecond, RestartNoReply: true}, nil)
	start(t, m, tn)
	if s := m.Status("antenna"); s.Phase != PhaseDone {
		t.Fatalf("status = %+v", s)
	}
}

func TestATunerThatKeepsAnsweringDidNotRestart(t *testing.T) {
	m, _, tn := rig(t, hdhrtest.Options{}, func(c *Config) { c.GoneWithin = 400 * time.Millisecond })
	start(t, m, tn)
	s := m.Status("antenna")
	if s.Phase != PhaseFailed || !strings.Contains(s.Error, "kept answering") {
		t.Fatalf("status = %+v", s)
	}
	if f := tn.flags(); len(f) != 2 || f[1] {
		t.Errorf("the restarting flag was left set: %v", f)
	}
}

func TestATunerThatDoesNotComeBackIsSaidNotHidden(t *testing.T) {
	m, _, tn := rig(t, hdhrtest.Options{RestartFor: time.Minute}, func(c *Config) { c.BackWithin = 500 * time.Millisecond })
	start(t, m, tn)
	s := m.Status("antenna")
	if s.Phase != PhaseFailed || !strings.Contains(s.Error, "has not come back") {
		t.Fatalf("status = %+v", s)
	}
	if !strings.HasPrefix(tn.lastNote(), "err:") {
		t.Errorf("last note %q", tn.lastNote())
	}
}

func TestATunerThatRefusesHasNotRestarted(t *testing.T) {
	m, srv, tn := rig(t, hdhrtest.Options{Unknown: map[string]bool{"/sys/restart": true}}, nil)
	start(t, m, tn)
	s := m.Status("antenna")
	if s.Phase != PhaseFailed || !strings.Contains(s.Error, "refused") {
		t.Fatalf("status = %+v", s)
	}
	if srv.Restarts() != 0 {
		t.Error("counted a restart the tuner refused")
	}
}

func TestAnUnreachableTunerIsLeftAlone(t *testing.T) {
	m, _, tn := rig(t, hdhrtest.Options{}, func(c *Config) {
		c.Dial = func(context.Context, string) (*hdhr.Conn, error) { return nil, errors.New("no route") }
	})
	start(t, m, tn)
	s := m.Status("antenna")
	if s.Phase != PhaseFailed || !strings.Contains(s.Error, "not been touched") {
		t.Fatalf("status = %+v", s)
	}
	if len(tn.flags()) != 0 {
		t.Error("the tuner was marked as restarting although it was never asked to")
	}
}

func TestOnlyOneRestartAtATime(t *testing.T) {
	m, _, tn := rig(t, hdhrtest.Options{RestartFor: 600 * time.Millisecond}, nil)
	req := Request{Device: "antenna", Title: "Living room", Host: "tuner.test", Tuner: tn}
	if err := m.Start(req); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(req); !errors.Is(err, ErrBusy) {
		t.Errorf("a second start: %v", err)
	}
	if !m.Active("antenna") {
		t.Error("not active")
	}
	m.Wait()
	if m.Active("antenna") {
		t.Error("still active")
	}
}

func TestAFinishedRestartCanBeDismissedButARunningOneCannot(t *testing.T) {
	m, _, tn := rig(t, hdhrtest.Options{RestartFor: 600 * time.Millisecond}, nil)
	if err := m.Start(Request{Device: "antenna", Title: "x", Host: "h", Tuner: tn}); err != nil {
		t.Fatal(err)
	}
	m.Forget("antenna")
	if m.Status("antenna") == nil {
		t.Fatal("a running restart was forgotten")
	}
	m.Wait()
	m.Forget("antenna")
	if m.Status("antenna") != nil {
		t.Fatal("a finished one was not forgotten")
	}
}
