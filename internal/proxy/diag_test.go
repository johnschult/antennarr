package proxy

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/diag"
)

// diagProxy is a proxy to f with diagnostics mode on (or off), whose log and troubles can be read.
func diagProxy(t *testing.T, f *fakeTuner, on bool) (srv *httptest.Server, log func() string, troubles func() []string) {
	t.Helper()
	api, _ := url.Parse(f.api.URL)
	stream, _ := url.Parse(f.stream.URL)
	var mu sync.Mutex
	var out strings.Builder
	var got []string
	m := &diag.Mode{}
	if on {
		m.Enable(time.Hour)
		t.Cleanup(m.Disable)
	}
	srv = httptest.NewServer(New(Config{
		API: api, Stream: stream, Diag: m,
		OnTrouble: func(what string) { mu.Lock(); got = append(got, what); mu.Unlock() },
		Logger:    slog.New(slog.NewTextHandler(&lockedWriter{mu: &mu, w: &out}, nil)),
	}))
	t.Cleanup(srv.Close)
	return srv, func() string { mu.Lock(); defer mu.Unlock(); return out.String() },
		func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), got...) }
}

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(b)
}

func TestDiagnosticsRecordsARequestWithHowItWent(t *testing.T) {
	f := newFakeTuner(t)
	srv, log, _ := diagProxy(t, f, true)
	var v any
	getJSON(t, srv.URL+"/lineup.json", &v)
	srv.Close()
	got := log()
	for _, want := range []string{`msg="diagnostics: request"`, "path=/lineup.json", "status=200", "took="} {
		if !strings.Contains(got, want) {
			t.Errorf("log lacks %s:\n%s", want, got)
		}
	}
}

func TestDiagnosticsRecordsHowAStreamEnded(t *testing.T) {
	f := newFakeTuner(t)
	srv, log, _ := diagProxy(t, f, true)
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/auto/v5.1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	io.ReadFull(resp.Body, buf)
	cancel()
	resp.Body.Close()
	srv.Close()
	got := log()
	for _, want := range []string{`msg="diagnostics: stream"`, "status=200", `ended="the viewer left"`, "waited="} {
		if !strings.Contains(got, want) {
			t.Errorf("log lacks %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, "bytes=0 ") {
		t.Errorf("no bytes counted:\n%s", got)
	}
}

func TestNothingExtraIsRecordedWhenDiagnosticsIsOff(t *testing.T) {
	f := newFakeTuner(t)
	srv, log, troubles := diagProxy(t, f, false)
	var v any
	getJSON(t, srv.URL+"/lineup.json", &v)
	srv.Close()
	if strings.Contains(log(), "diagnostics") || len(troubles()) != 0 {
		t.Errorf("recorded with the mode off:\n%s", log())
	}
}

func TestATunerRefusalIsTroubleOnlyInDiagnosticsMode(t *testing.T) {
	for _, on := range []bool{true, false} {
		f := newFakeTuner(t)
		f.stream.Close()
		f.stream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "All tuners in use", http.StatusServiceUnavailable)
		}))
		t.Cleanup(f.stream.Close)
		srv, _, troubles := diagProxy(t, f, on)
		resp, err := http.Get(srv.URL + "/auto/v5.1")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		srv.Close()
		if got := troubles(); on != (len(got) == 1 && strings.Contains(got[0], "503")) || (!on && len(got) != 0) {
			t.Errorf("on=%v: troubles = %v", on, got)
		}
	}
}

func TestAnEmptyLineupIsTroubleInDiagnosticsMode(t *testing.T) {
	f := newFakeTuner(t)
	f.api.Close()
	f.api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `[]`) }))
	t.Cleanup(f.api.Close)
	srv, _, troubles := diagProxy(t, f, true)
	var v any
	getJSON(t, srv.URL+"/lineup.json", &v)
	srv.Close()
	if got := troubles(); len(got) != 1 || !strings.Contains(got[0], "no channels") {
		t.Errorf("troubles = %v", got)
	}
}

func TestAStreamThatEndsAtOnceIsTrouble(t *testing.T) {
	f := newFakeTuner(t)
	f.stream.Close()
	f.stream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "TS") // a stream that starts and stops
	}))
	t.Cleanup(f.stream.Close)
	srv, _, troubles := diagProxy(t, f, true)
	resp, err := http.Get(srv.URL + "/auto/v5.1")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	srv.Close()
	if got := troubles(); len(got) != 1 || !strings.Contains(got[0], "almost as soon") {
		t.Errorf("troubles = %v", got)
	}
}
