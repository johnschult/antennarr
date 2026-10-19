package history

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)

func at(sec int) time.Time { return t0.Add(time.Duration(sec) * time.Second) }

func obs(tuner, channel string, kind Kind, viewer string) Observation {
	return Observation{Device: "antenna", Tuner: tuner, Channel: channel, ChannelName: "WSB-HD", Kind: kind, Viewer: viewer}
}

func TestASessionRunsFromWhenItWasFirstSeenToWhenItWasLastSeen(t *testing.T) {
	r := New(nil, t0)
	v := obs("tuner0", "2.1", KindViewer, "Alex")
	r.Observe(at(0), []Observation{v})
	r.Observe(at(300), []Observation{v})
	r.Observe(at(600), nil) // stopped between 300 and 600: it was last seen at 300
	got := r.Sessions(at(-3600))
	if len(got) != 1 || !got[0].Start.Equal(at(0)) || !got[0].End.Equal(at(300)) || got[0].Open || got[0].Viewer != "Alex" || got[0].Channel != "2.1" {
		t.Fatalf("sessions = %+v", got)
	}
}

func TestASessionStillGoingIsListedAsOpen(t *testing.T) {
	r := New(nil, t0)
	r.Observe(at(0), []Observation{obs("tuner0", "2.1", KindLocal, "")})
	r.Observe(at(120), []Observation{obs("tuner0", "2.1", KindLocal, "")})
	got := r.Sessions(at(-3600))
	if len(got) != 1 || !got[0].Open || !got[0].End.Equal(at(120)) {
		t.Fatalf("sessions = %+v", got)
	}
}

func TestChangingChannelOrViewerStartsANewSession(t *testing.T) {
	r := New(nil, t0)
	r.Observe(at(0), []Observation{obs("tuner0", "2.1", KindViewer, "Alex")})
	r.Observe(at(60), []Observation{obs("tuner0", "2.1", KindViewer, "Alex")})
	r.Observe(at(120), []Observation{obs("tuner0", "5.1", KindViewer, "Alex")}) // another channel
	r.Observe(at(180), []Observation{obs("tuner0", "5.1", KindViewer, "Alex")})
	r.Observe(at(240), []Observation{obs("tuner0", "5.1", KindViewer, "Sam")}) // another viewer
	r.Observe(at(300), []Observation{obs("tuner0", "5.1", KindViewer, "Sam")})
	r.Observe(at(360), nil)
	got := r.Sessions(at(-3600))
	if len(got) != 3 {
		t.Fatalf("%d sessions: %+v", len(got), got)
	}
	if got[0].Channel != "2.1" || got[1].Channel != "5.1" || got[1].Viewer != "Alex" || got[2].Viewer != "Sam" {
		t.Errorf("sessions = %+v", got)
	}
}

func TestSeveralTunersAreSeparateSessions(t *testing.T) {
	r := New(nil, t0)
	both := []Observation{obs("tuner0", "2.1", KindViewer, "Alex"), obs("tuner1", "69.1", KindLocal, "")}
	r.Observe(at(0), both)
	r.Observe(at(60), both)
	r.Observe(at(120), []Observation{obs("tuner1", "69.1", KindLocal, "")}) // Alex stopped
	r.Observe(at(180), nil)
	got := r.Sessions(at(-3600))
	if len(got) != 2 || got[0].Tuner != "tuner0" || !got[0].End.Equal(at(60)) || got[1].Tuner != "tuner1" || !got[1].End.Equal(at(120)) {
		t.Fatalf("sessions = %+v", got)
	}
}

func TestFlickingThroughChannelsIsNotKept(t *testing.T) {
	r := New(nil, t0)
	r.Observe(at(0), []Observation{obs("tuner0", "2.1", KindLocal, "")})
	r.Observe(at(5), []Observation{obs("tuner0", "5.1", KindLocal, "")})
	r.Observe(at(10), []Observation{obs("tuner0", "7.1", KindLocal, "")})
	r.Observe(at(14), nil)
	if got := r.Sessions(at(-3600)); len(got) != 0 {
		t.Fatalf("kept %+v", got)
	}
}

func TestSomethingOnATunerWeCannotNameIsNotASession(t *testing.T) {
	r := New(nil, t0)
	r.Observe(at(0), []Observation{{Device: "antenna", Tuner: "tuner0", Kind: KindLocal}})
	r.Observe(at(60), []Observation{{Device: "antenna", Tuner: "tuner0", Kind: KindLocal}})
	r.Observe(at(120), nil)
	if got := r.Sessions(at(-3600)); len(got) != 0 {
		t.Fatalf("kept %+v", got)
	}
}

func TestTheProgramsThatWereOnAreKeptInOrderWithoutRepeats(t *testing.T) {
	r := New(nil, t0)
	o := obs("tuner0", "2.1", KindViewer, "Alex")
	for i, title := range []string{"News", "News", "", "Football", "Football", "News"} {
		o.Title = title
		r.Observe(at(i*60), []Observation{o})
	}
	r.Observe(at(1000), nil)
	got := r.Sessions(at(-3600))
	if len(got) != 1 || strings.Join(got[0].Titles, "|") != "News|Football|News" {
		t.Fatalf("titles = %+v", got)
	}
}

func TestTitlesAreCappedInNumberAndLength(t *testing.T) {
	r := New(nil, t0)
	o := obs("tuner0", "2.1", KindViewer, "Alex")
	for i := 0; i < 20; i++ {
		o.Title = string(rune('a'+i)) + strings.Repeat("x", 300)
		r.Observe(at(i*60), []Observation{o})
	}
	r.Observe(at(2000), nil)
	got := r.Sessions(at(-3600))
	if len(got[0].Titles) != maxTitles || len(got[0].Titles[0]) != maxTitle {
		t.Fatalf("%d titles, first %d long", len(got[0].Titles), len(got[0].Titles[0]))
	}
}

func TestCloseEndsWhatIsStillGoing(t *testing.T) {
	r := New(nil, t0)
	r.Observe(at(0), []Observation{obs("tuner0", "2.1", KindLocal, "")})
	r.Observe(at(90), []Observation{obs("tuner0", "2.1", KindLocal, "")})
	r.Close()
	got := r.Sessions(at(-3600))
	if len(got) != 1 || got[0].Open || !got[0].End.Equal(at(90)) {
		t.Fatalf("sessions = %+v", got)
	}
}

func TestSessionsAreKeptInTheFileAndComeBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	r := New(&Store{Path: path}, t0)
	o := obs("tuner0", "2.1", KindViewer, "Alex")
	o.Title = "News"
	r.Observe(at(0), []Observation{o})
	r.Observe(at(300), []Observation{o})
	r.Observe(at(360), nil)

	again := New(&Store{Path: path}, at(400))
	got := again.Sessions(at(-3600))
	if len(got) != 1 || got[0].Viewer != "Alex" || got[0].Titles[0] != "News" || !got[0].End.Equal(at(300)) {
		t.Fatalf("after a restart: %+v", got)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file: %v %v", info, err)
	}
}

func TestOldSessionsAreDroppedFromMemoryAndFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	st := &Store{Path: path}
	old := Session{Device: "antenna", Tuner: "tuner0", Channel: "2.1", Kind: KindLocal, Start: t0.Add(-40 * 24 * time.Hour), End: t0.Add(-40*24*time.Hour + time.Hour)}
	recent := Session{Device: "antenna", Tuner: "tuner0", Channel: "5.1", Kind: KindLocal, Start: t0.Add(-24 * time.Hour), End: t0.Add(-23 * time.Hour)}
	st.Append(old)
	st.Append(recent)

	r := New(st, t0)
	if got := r.Sessions(t0.Add(-60 * 24 * time.Hour)); len(got) != 1 || got[0].Channel != "5.1" {
		t.Fatalf("loaded %+v", got)
	}
	if b, _ := os.ReadFile(path); strings.Contains(string(b), `"2.1"`) {
		t.Error("the old session is still in the file")
	}

	// and as time passes while running
	r.Observe(t0.Add(31*24*time.Hour), nil)
	if got := r.Sessions(t0.Add(-60 * 24 * time.Hour)); len(got) != 0 {
		t.Fatalf("after a month: %+v", got)
	}
	if b, _ := os.ReadFile(path); strings.Contains(string(b), `"5.1"`) {
		t.Error("the expired session is still in the file")
	}
}

func TestAFileWithJunkInItStillLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	good := `{"device":"antenna","tuner":"tuner0","channel":"2.1","kind":"local","start":"2026-10-19T19:00:00Z","end":"2026-10-19T19:30:00Z"}`
	backwards := `{"device":"antenna","tuner":"tuner0","channel":"2.1","kind":"local","start":"2026-10-19T19:30:00Z","end":"2026-10-19T19:00:00Z"}`
	nothing := `{"device":"antenna","tuner":"tuner0","kind":"local","start":"2026-10-19T19:00:00Z","end":"2026-10-19T19:30:00Z"}`
	os.WriteFile(path, []byte("not json\n"+good+"\n"+backwards+"\n"+nothing+"\n\n{\n"), 0o600)
	got := New(&Store{Path: path}, t0).Sessions(t0.Add(-24 * time.Hour))
	if len(got) != 1 {
		t.Fatalf("loaded %+v", got)
	}
}

func TestAFileThatCannotBeWrittenIsReportedAndTheSessionKept(t *testing.T) {
	var errs []error
	r := New(&Store{Path: filepath.Join(t.TempDir(), "no", "such", "dir", "h.jsonl")}, t0)
	r.OnError = func(err error) { errs = append(errs, err) }
	o := obs("tuner0", "2.1", KindLocal, "")
	r.Observe(at(0), []Observation{o})
	r.Observe(at(60), []Observation{o})
	r.Observe(at(120), nil)
	if len(errs) != 1 || !errors.Is(errs[0], os.ErrNotExist) {
		t.Errorf("errors = %v", errs)
	}
	if got := r.Sessions(at(-3600)); len(got) != 1 {
		t.Errorf("the session was lost with the file: %+v", got)
	}
}

func TestSessionsSinceLeavesOutOlderOnes(t *testing.T) {
	r := New(nil, t0)
	o := obs("tuner0", "2.1", KindLocal, "")
	r.Observe(at(0), []Observation{o})
	r.Observe(at(60), []Observation{o})
	r.Observe(at(120), nil)
	if got := r.Sessions(at(61)); len(got) != 0 {
		t.Errorf("a session that ended before since: %+v", got)
	}
	if got := r.Sessions(at(30)); len(got) != 1 {
		t.Errorf("a session that was going at since: %+v", got)
	}
}

func stream(bps int64, quality int) []Observation {
	o := obs("tuner0", "2.1", KindViewer, "Alex")
	o.BitsPerSecond, o.Quality = bps, quality
	return []Observation{o}
}

func TestBytesAreTheRateOverTheTimeWatched(t *testing.T) {
	r := New(nil, t0)
	r.Observe(at(0), stream(8_000_000, 0))  // 1 MB/s
	r.Observe(at(10), stream(8_000_000, 0)) // 10 seconds at 1 MB/s
	r.Observe(at(20), stream(16_000_000, 0))
	// the second stretch is the average of 8 and 16 Mbps: 12 Mbps, 1.5 MB/s, for 10 seconds
	r.Observe(at(30), nil)
	got := r.Sessions(at(-3600))
	if len(got) != 1 || got[0].Bytes != 10_000_000+15_000_000 {
		t.Fatalf("bytes = %+v", got)
	}
}

func TestAGapTooLongToBelieveIsNotCountedAsWatching(t *testing.T) {
	r := New(nil, t0)
	r.Observe(at(0), stream(8_000_000, 0))
	r.Observe(at(10), stream(8_000_000, 0))
	r.Observe(at(10+int(maxGap/time.Second)+30), stream(8_000_000, 0)) // Yagi was away for a minute and a half
	r.Observe(at(10+int(maxGap/time.Second)+40), stream(8_000_000, 0))
	r.Observe(at(1000), nil)
	got := r.Sessions(at(-3600))
	if len(got) != 1 || got[0].Bytes != 10_000_000+10_000_000 {
		t.Fatalf("bytes = %+v: the long gap should add nothing", got)
	}
}

func TestNoBitrateAddsNoBytes(t *testing.T) {
	r := New(nil, t0)
	r.Observe(at(0), stream(0, 0))
	r.Observe(at(30), stream(0, 0))
	r.Observe(at(60), nil)
	if got := r.Sessions(at(-3600)); len(got) != 1 || got[0].Bytes != 0 {
		t.Fatalf("sessions = %+v", got)
	}
}

func TestQualityIsTheAverageOfWhatWasReported(t *testing.T) {
	r := New(nil, t0)
	for i, q := range []int{100, 80, 0, 60} { // 0 is "not reported", not zero quality
		r.Observe(at(i*10), stream(1_000_000, q))
	}
	r.Observe(at(100), nil)
	got := r.Sessions(at(-3600))
	if len(got) != 1 || got[0].Quality != 80 {
		t.Fatalf("quality = %+v, want the average of 100, 80 and 60", got)
	}
}

func TestQualityIsLeftOutWhenTheTunerNeverSaysAndCappedAtAHundred(t *testing.T) {
	r := New(nil, t0)
	r.Observe(at(0), stream(1_000_000, 0))
	r.Observe(at(30), stream(1_000_000, 0))
	r.Observe(at(60), nil)
	if got := r.Sessions(at(-3600)); got[0].Quality != 0 {
		t.Errorf("quality = %d for a tuner that doesn't say", got[0].Quality)
	}
	r.Observe(at(100), stream(1_000_000, 250))
	r.Observe(at(130), stream(1_000_000, 250))
	r.Observe(at(160), nil)
	if got := r.Sessions(at(-3600)); got[1].Quality != 100 {
		t.Errorf("quality = %d for odd readings", got[1].Quality)
	}
}

func TestBytesAndQualitySurviveTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.jsonl")
	r := New(&Store{Path: path}, t0)
	r.Observe(at(0), stream(8_000_000, 90))
	r.Observe(at(20), stream(8_000_000, 70))
	r.Observe(at(40), nil)
	got := New(&Store{Path: path}, at(100)).Sessions(at(-3600))
	if len(got) != 1 || got[0].Bytes != 20_000_000 || got[0].Quality != 80 {
		t.Fatalf("after a restart: %+v", got)
	}
}

func TestAnOpenSessionHasItsBytesAndQualityToo(t *testing.T) {
	r := New(nil, t0)
	r.Observe(at(0), stream(8_000_000, 90))
	r.Observe(at(20), stream(8_000_000, 90))
	got := r.Sessions(at(-3600))
	if len(got) != 1 || !got[0].Open || got[0].Bytes != 20_000_000 || got[0].Quality != 90 {
		t.Fatalf("open session = %+v", got)
	}
}
