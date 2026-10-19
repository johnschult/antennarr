package lineup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/lineup/linetest"
)

func fake(t *testing.T) *linetest.Server {
	t.Helper()
	s := linetest.New([]linetest.Channel{
		{Number: "2.1", Name: "WSB-HD", HD: true},
		{Number: "2.2", Name: "Laff", State: "favorite"},
		{Number: "10.3", Name: "Old", State: "disabled"},
		{Number: "703", Name: "Locked", Unsubscribed: true},
	}, 300*time.Millisecond)
	t.Cleanup(s.Close)
	return s
}

func TestFoundListsEveryChannelWithItsState(t *testing.T) {
	s := fake(t)
	got, err := Client{}.Found(context.Background(), s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("%d channels: %+v", len(got), got)
	}
	want := map[string]State{"2.1": Enabled, "2.2": Favorite, "10.3": Disabled, "703": Enabled}
	for _, c := range got {
		if c.State != want[c.Number] {
			t.Errorf("%s is %s, want %s", c.Number, c.State, want[c.Number])
		}
	}
	if !got[0].HD || got[0].Video != "MPEG2" || got[0].Name != "WSB-HD" {
		t.Errorf("first = %+v", got[0])
	}
	if got[3].Subscribed {
		t.Error("a channel the tuner has but can't play is marked as changeable")
	}
	if reqs := s.Requests(); len(reqs) != 1 || reqs[0] != "GET /lineup.json?show=found" {
		t.Errorf("requests = %v", reqs)
	}
}

func TestOddEntriesAreLeftOutNotTrusted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"GuideNumber":"2.1","GuideName":"  Good   name "},{"GuideNumber":"../../etc","GuideName":"x"},{"GuideNumber":"2.1&scan=start","GuideName":"y"},{"GuideNumber":"","GuideName":"z"}]`))
	}))
	defer srv.Close()
	got, err := Client{}.Found(context.Background(), srv.URL)
	if err != nil || len(got) != 1 || got[0].Number != "2.1" || got[0].Name != "Good name" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestSetStateSendsTheTunersOwnMarks(t *testing.T) {
	s := fake(t)
	c := Client{}
	ctx := context.Background()
	for st, want := range map[State]string{Disabled: "disabled", Favorite: "favorite", Enabled: "enabled"} {
		s.Forget()
		if err := c.SetState(ctx, s.URL, "2.1", st); err != nil {
			t.Fatal(err)
		}
		if got := s.State("2.1"); got != want {
			t.Errorf("after %s the tuner has %q", st, got)
		}
		wantReq := map[State]string{Disabled: "x", Favorite: "%2B", Enabled: "-"}[st] + "2.1"
		if reqs := s.Requests(); len(reqs) != 1 || reqs[0] != "POST /lineup.post?favorite="+wantReq {
			t.Errorf("requests = %v, want a favorite=%s", reqs, wantReq)
		}
	}
}

func TestNothingOddIsEverSent(t *testing.T) {
	s := fake(t)
	c := Client{}
	for _, number := range []string{"", "x", "2.1&scan=start", "2.1/../x", "../2.1", "2.1\n", "1234567", "2.", ".1", " 2.1"} {
		if err := c.SetState(context.Background(), s.URL, number, Disabled); err == nil {
			t.Errorf("%q was sent", number)
		}
	}
	for _, st := range []State{"", "gone", "x", "+"} {
		if err := c.SetState(context.Background(), s.URL, "2.1", st); err == nil {
			t.Errorf("state %q was sent", st)
		}
	}
	if reqs := s.Requests(); len(reqs) != 0 {
		t.Errorf("requests = %v", reqs)
	}
}

func TestScanStatusAndScanning(t *testing.T) {
	s := fake(t)
	c := Client{}
	ctx := context.Background()
	st, err := c.Scan(ctx, s.URL)
	if err != nil || st.InProgress || !st.Possible || st.Source != "Antenna" || len(st.Sources) != 2 {
		t.Fatalf("status = %+v, %v", st, err)
	}
	if err := c.StartScan(ctx, s.URL, "Cable"); err != nil {
		t.Fatal(err)
	}
	st, _ = c.Scan(ctx, s.URL)
	if !st.InProgress || st.Possible || st.Source != "Cable" || st.Found != 12 {
		t.Fatalf("status while scanning = %+v", st)
	}
	if err := c.AbortScan(ctx, s.URL); err != nil {
		t.Fatal(err)
	}
	if s.Scanning() {
		t.Error("still scanning after Abort")
	}
	reqs := strings.Join(s.Requests(), "\n")
	for _, want := range []string{"POST /lineup.post?scan=start&source=Cable", "POST /lineup.post?scan=abort"} {
		if !strings.Contains(reqs, want) {
			t.Errorf("no %q in\n%s", want, reqs)
		}
	}
}

func TestAScanSourceIsEscaped(t *testing.T) {
	s := fake(t)
	Client{}.StartScan(context.Background(), s.URL, "A&b=c d")
	if reqs := s.Requests(); len(reqs) != 1 || strings.Contains(reqs[0], "b=c") && !strings.Contains(reqs[0], "%26") {
		t.Errorf("requests = %v", reqs)
	}
}

func TestErrorsDoNotQuoteTheAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 500) }))
	_, err := Client{}.Found(context.Background(), srv.URL)
	if err == nil || strings.Contains(err.Error(), srv.URL) {
		t.Errorf("error = %v", err)
	}
	srv.Close()
	_, err = Client{}.Found(context.Background(), srv.URL)
	if err == nil || strings.Contains(err.Error(), srv.URL) {
		t.Errorf("error = %v", err)
	}
}

func TestAnswersThatAreNotLineupsAreRefused(t *testing.T) {
	for name, body := range map[string]string{"html": "<html>", "an object": `{"a":1}`, "nothing": ""} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		if _, err := (Client{}).Found(context.Background(), srv.URL); err == nil {
			t.Errorf("%s was accepted", name)
		}
		if _, err := (Client{}).Scan(context.Background(), srv.URL); err == nil && name != "an object" { // an object is a status with nothing in it
			t.Errorf("%s was accepted as a status", name)
		}
		srv.Close()
	}
}

func TestARedirectIsNotFollowed(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("followed a redirect"); w.Write([]byte("[]")) }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
	defer srv.Close()
	if _, err := (Client{}).Found(context.Background(), srv.URL); err == nil {
		t.Error("a redirect was taken for an answer")
	}
}
