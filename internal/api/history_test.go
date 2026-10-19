package api

import (
	"net/http/httptest"
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
