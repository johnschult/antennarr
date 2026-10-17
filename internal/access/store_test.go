package access

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newStore(t *testing.T, initial Settings) (*Store, *Policy, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "antennarr.json")
	if err := SaveSettings(path, initial); err != nil {
		t.Fatal(err)
	}
	mon := &fakeMon{}
	mon.set(true, 4)
	pol, _ := newPolicy(t, initial, mon)
	return NewStore(path, pol), pol, path
}

func TestEditSavesThenApplies(t *testing.T) {
	st, pol, path := newStore(t, twoViewers)

	got, err := st.Edit(func(s *Settings) error {
		s.SharedCap = 2
		s.Viewers[0].Limit = 1
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.SharedCap != 2 || pol.Settings().SharedCap != 2 || pol.Settings().Viewers[0].Limit != 1 {
		t.Errorf("not applied: %+v", pol.Settings())
	}
	onDisk, _, err := LoadSettings(path)
	if err != nil || onDisk.SharedCap != 2 || onDisk.Viewers[0].Limit != 1 {
		t.Errorf("not saved: %+v (%v)", onDisk, err)
	}
}

func TestEditWithoutFileIsRefused(t *testing.T) {
	pol, _ := newPolicy(t, twoViewers, &fakeMon{})
	st := NewStore("", pol)
	if st.Editable() {
		t.Error("a store with no path claims to be editable")
	}
	_, err := st.Edit(func(s *Settings) error { s.SharedCap = 9; return nil })
	if !errors.Is(err, ErrNoFile) {
		t.Errorf("err = %v, want ErrNoFile", err)
	}
	if pol.Settings().SharedCap != 0 {
		t.Error("an edit with nowhere to save it was applied anyway")
	}
}

func TestInvalidEditChangesNothing(t *testing.T) {
	st, pol, path := newStore(t, twoViewers)
	before, _ := os.ReadFile(path)

	_, err := st.Edit(func(s *Settings) error {
		s.Viewers = append(s.Viewers, Viewer{Name: "John", Addresses: []string{"1.2.3.4"}}) // duplicate name
		return nil
	})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want a ValidationError", err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("the file changed")
	}
	if len(pol.Settings().Viewers) != 2 {
		t.Error("the running settings changed")
	}
}

func TestEditCallbackErrorChangesNothing(t *testing.T) {
	st, pol, _ := newStore(t, twoViewers)
	boom := errors.New("boom")
	_, err := st.Edit(func(s *Settings) error { s.SharedCap = 7; return boom })
	if !errors.Is(err, boom) || pol.Settings().SharedCap != 0 {
		t.Errorf("err=%v sharedCap=%d", err, pol.Settings().SharedCap)
	}
}

func TestFailedSaveLeavesRunningSettingsAlone(t *testing.T) {
	_, pol, _ := newStore(t, twoViewers)
	st := NewStore(filepath.Join(t.TempDir(), "missing", "dir", "a.json"), pol) // can't be written

	_, err := st.Edit(func(s *Settings) error { s.SharedCap = 3; return nil })
	if err == nil {
		t.Fatal("expected a save error")
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		t.Error("a write failure was reported as a validation problem")
	}
	if pol.Settings().SharedCap != 0 {
		t.Error("settings were applied even though they couldn't be saved")
	}
}

func TestEditsSerialize(t *testing.T) {
	st, pol, path := newStore(t, twoViewers)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := st.Edit(func(s *Settings) error {
				s.Viewers = append(s.Viewers, Viewer{Name: fmt.Sprintf("v%d", i), Addresses: []string{fmt.Sprintf("10.9.0.%d", i+1)}, Limit: 1})
				return nil
			})
			if err != nil {
				t.Errorf("edit %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if n := len(pol.Settings().Viewers); n != 22 {
		t.Errorf("%d viewers after 20 concurrent adds, want 22 (an edit was lost)", n)
	}
	if onDisk, _, _ := LoadSettings(path); len(onDisk.Viewers) != 22 {
		t.Errorf("%d viewers on disk, want 22", len(onDisk.Viewers))
	}
}

func TestChannelFromPath(t *testing.T) {
	cases := map[string]string{
		"/auto/v5.1":          "5.1",
		"/auto/v2.1":          "2.1",
		"/tuner0/v11.3":       "11.3",
		"/auto/v7":            "7",
		"/auto/ch593000000-3": "", // tuned by frequency
		"/auto/":              "",
		"/auto/video":         "", // starts with v but isn't a channel
		"/discover.json":      "",
		"":                    "",
	}
	for in, want := range cases {
		if got := channelFromPath(in); got != want {
			t.Errorf("channelFromPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStreamRecordsChannel(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	p, _ := newPolicy(t, DefaultSettings(), mon)
	r := req("203.0.113.9:1")
	r.URL.Path = "/auto/v5.1"
	if _, err := p.Stream(r); err != nil {
		t.Fatal(err)
	}
	if a := p.Active(); len(a) != 1 || a[0].Channel != "5.1" {
		t.Errorf("active = %+v", a)
	}
}

func TestViewerStatusesAndLastSeen(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	p, _ := newPolicy(t, twoViewers, mon)
	now := time.Unix(2_000_000, 0)
	p.now = func() time.Time { return now }

	for _, v := range p.ViewerStatuses() {
		if v.LastSeen != nil || v.InUse != 0 {
			t.Errorf("%s: fresh viewer has activity: %+v", v.Name, v)
		}
	}

	p.Authorize(req("100.67.117.32:1")) // John's media server says hello
	p.Authorize(req("203.0.113.9:1"))   // a stranger must not be recorded
	p.Stream(req("100.67.117.32:2"))

	got := map[string]ViewerStatus{}
	for _, v := range p.ViewerStatuses() {
		got[v.Name] = v
	}
	if j := got["John"]; j.LastSeen == nil || !j.LastSeen.Equal(now) || j.InUse != 1 {
		t.Errorf("John = %+v", j)
	}
	if m := got["Mom"]; m.LastSeen != nil || m.InUse != 0 {
		t.Errorf("Mom = %+v", m)
	}
	if len(got) != 2 {
		t.Errorf("a stranger appeared in the viewer list: %+v", got)
	}
}

func TestOpenModeRecordsNoViewers(t *testing.T) {
	p, _ := newPolicy(t, DefaultSettings(), &fakeMon{})
	p.Authorize(req("203.0.113.9:1"))
	if n := len(p.ViewerStatuses()); n != 0 {
		t.Errorf("%d viewers in open mode", n)
	}
	if p.Mode() != "open" {
		t.Errorf("mode = %q", p.Mode())
	}
	p.Update(twoViewers)
	if p.Mode() != "enforced" {
		t.Errorf("mode = %q after adding viewers", p.Mode())
	}
}

func TestOnChangeFires(t *testing.T) {
	mon := &fakeMon{}
	mon.set(true, 4)
	p, _ := newPolicy(t, twoViewers, mon)
	var n atomic.Int32
	p.SetOnChange(func() { n.Add(1) })

	rel, err := p.Stream(req("100.67.117.32:1"))
	if err != nil {
		t.Fatal(err)
	}
	if n.Load() != 1 {
		t.Errorf("after stream start: %d changes, want 1", n.Load())
	}
	rel()
	rel() // a second release is a no-op, so no second notification
	if n.Load() != 2 {
		t.Errorf("after release: %d changes, want 2", n.Load())
	}
	p.Update(twoViewers)
	if n.Load() != 3 {
		t.Errorf("after update: %d changes, want 3", n.Load())
	}
	p.Update(Settings{Viewers: []Viewer{{Name: ""}}}) // invalid: refused, nothing changed
	if n.Load() != 3 {
		t.Errorf("a refused update notified: %d", n.Load())
	}
	p.Stream(req("203.0.113.9:1")) // refused stream
	if n.Load() != 3 {
		t.Errorf("a refused stream notified: %d", n.Load())
	}
}
