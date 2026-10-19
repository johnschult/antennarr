package api

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/history"
)

func TestHistoryListsSessionsInTheRange(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	r := history.New(nil, time.Now())
	now := time.Now()
	obs := func(ch string) []history.Observation {
		return []history.Observation{{Device: "antenna", Tuner: "tuner0", Channel: ch, ChannelName: "WSB", Kind: history.KindViewer, Viewer: "John", Title: "News"}}
	}
	r.Observe(now.Add(-10*24*time.Hour), obs("2.1"))
	r.Observe(now.Add(-10*24*time.Hour+time.Hour), obs("2.1"))
	r.Observe(now.Add(-10*24*time.Hour+2*time.Hour), nil)
	r.Observe(now.Add(-2*time.Hour), obs("5.1"))
	r.Observe(now.Add(-time.Hour), obs("5.1"))
	e.api.History = r

	code, v := e.call(t, "GET", "/antennarr/api/history", "")
	d := obj(t, v)
	got, _ := d["sessions"].([]any)
	if code != 200 || d["days"] != float64(7) || len(got) != 1 || obj(t, got[0])["channel"] != "5.1" || obj(t, got[0])["viewer"] != "John" || obj(t, got[0])["open"] != true {
		t.Fatalf("%d %v", code, v)
	}
	_, v = e.call(t, "GET", "/antennarr/api/history?days=30", "")
	if got, _ := obj(t, v)["sessions"].([]any); len(got) != 2 {
		t.Fatalf("30 days: %v", v)
	}
}

func TestHistoryIsAnEmptyListWhenThereIsNone(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	_, v := e.call(t, "GET", "/antennarr/api/history", "")
	if s, ok := obj(t, v)["sessions"].([]any); !ok || len(s) != 0 {
		t.Errorf("sessions = %v, want [] (never null)", obj(t, v)["sessions"])
	}
}

func TestHistoryRefusesOddRanges(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	for _, q := range []string{"days=0", "days=31", "days=-1", "days=abc", "days="} {
		code, _ := e.call(t, "GET", "/antennarr/api/history?"+q, "")
		want := 400
		if q == "days=" {
			want = 200 // no value is the default
		}
		if code != want {
			t.Errorf("%s: %d, want %d", q, code, want)
		}
	}
}

func TestHistoryIsForAdminsOnly(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/antennarr/api/history", nil)
	req.RemoteAddr = "100.67.117.32:1" // John is a viewer, not an admin
	e.pol.AdminOnly(e.api.Handler()).ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Errorf("status %d", rec.Code)
	}
}

func TestSettingsReportTheHistorySwitchAndItClosesTheRoute(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	e.api.History = history.New(nil, time.Now())
	_, v := e.call(t, "GET", "/antennarr/api/settings", "")
	if obj(t, v)["history"] != true {
		t.Errorf("default = %v, want on", obj(t, v)["history"])
	}
	if code, _ := e.call(t, "GET", "/antennarr/api/history", ""); code != 200 {
		t.Fatalf("while on: %d", code)
	}
	code, v := e.call(t, "PATCH", "/antennarr/api/settings", `{"history":false}`)
	if code != 200 || obj(t, v)["history"] != false {
		t.Fatalf("%d %v", code, v)
	}
	if fileSettings(t, e).HistoryEnabled() {
		t.Error("the choice was not saved")
	}
	if code, v := e.call(t, "GET", "/antennarr/api/history", ""); code != 404 {
		t.Errorf("while off: %d %v", code, v)
	}
	e.call(t, "PATCH", "/antennarr/api/settings", `{"sharedCap":2}`)
	if fileSettings(t, e).HistoryEnabled() {
		t.Error("an unrelated patch switched it back on")
	}
	if code, _ := e.call(t, "PATCH", "/antennarr/api/settings", `{"history":"yes"}`); code != 400 {
		t.Errorf("a non-boolean: %d", code)
	}
	e.call(t, "PATCH", "/antennarr/api/settings", `{"history":true}`)
	if code, _ := e.call(t, "GET", "/antennarr/api/history", ""); code != 200 {
		t.Errorf("switched back on: %d", code)
	}
}

func TestClearingTheHistoryForgetsEverythingEvenWhileItIsOff(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	path := filepath.Join(t.TempDir(), "h.jsonl")
	r := history.New(&history.Store{Path: path}, time.Now())
	now := time.Now()
	o := []history.Observation{{Device: "antenna", Tuner: "tuner0", Channel: "2.1", Kind: history.KindLocal}}
	r.Observe(now.Add(-3*time.Hour), o)
	r.Observe(now.Add(-2*time.Hour), o)
	r.Observe(now.Add(-time.Hour), nil) // a finished session, in the file
	r.Observe(now.Add(-30*time.Minute), o)
	r.Observe(now, o) // and one still going
	e.api.History = r
	if b, _ := os.ReadFile(path); len(b) == 0 {
		t.Fatal("nothing was written to the file to clear")
	}

	e.call(t, "PATCH", "/antennarr/api/settings", `{"history":false}`)
	if code, _ := e.call(t, "DELETE", "/antennarr/api/history", ""); code != 204 {
		t.Fatalf("clear: %d", code)
	}
	if got := r.Sessions(now.Add(-24 * time.Hour)); len(got) != 0 {
		t.Errorf("still in memory: %+v", got)
	}
	if b, _ := os.ReadFile(path); len(b) != 0 {
		t.Errorf("still in the file: %s", b)
	}
	e.call(t, "PATCH", "/antennarr/api/settings", `{"history":true}`)
	if _, v := e.call(t, "GET", "/antennarr/api/history", ""); len(obj(t, v)["sessions"].([]any)) != 0 {
		t.Errorf("history after clearing: %v", v)
	}
}

func TestOnlyAdminsCanClearTheHistory(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/antennarr/api/history", nil)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "100.67.117.32:1" // John is a viewer, not an admin
	e.pol.AdminOnly(e.api.Handler()).ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Errorf("status %d", rec.Code)
	}
}
