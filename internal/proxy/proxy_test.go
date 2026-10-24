package proxy

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type fakeTuner struct {
	api, stream *httptest.Server
	streamGone  chan struct{} // closed when the stream handler sees its request end
}

func newFakeTuner(t *testing.T) *fakeTuner {
	t.Helper()
	f := &fakeTuner{streamGone: make(chan struct{})}
	var streamURL string

	f.stream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mpeg")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		for {
			select {
			case <-r.Context().Done():
				close(f.streamGone)
				return
			case <-time.After(5 * time.Millisecond):
				_, _ = w.Write([]byte("TSPACKET"))
				w.(http.Flusher).Flush()
			}
		}
	}))
	streamURL = f.stream.URL

	f.api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/discover.json":
			io.WriteString(w, `{"FriendlyName":"Fake FLEX","DeviceID":"10ABCC87","TunerCount":4,"BaseURL":"http://192.168.1.132","LineupURL":"http://192.168.1.132/lineup.json"}`)
		case "/lineup.json":
			io.WriteString(w, `[{"GuideNumber":"5.1","GuideName":"WFRV","HD":1,"URL":"`+strings.Replace(streamURL, "127.0.0.1", "192.168.1.132", 1)+`/auto/v5.1?transcode=heavy"}]`)
		case "/status.json":
			io.WriteString(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() { f.api.Close(); f.stream.Close() })
	return f
}

func newProxy(t *testing.T, f *fakeTuner, advertise string) *httptest.Server {
	t.Helper()
	api, _ := url.Parse(f.api.URL)
	stream, _ := url.Parse(f.stream.URL)
	srv := httptest.NewServer(New(Config{API: api, Stream: stream, Advertise: advertise}))
	t.Cleanup(srv.Close)
	return srv
}

func getJSON(t *testing.T, u string, v any) {
	t.Helper()
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverRewritesURLsAndKeepsRest(t *testing.T) {
	p := newProxy(t, newFakeTuner(t), "")
	var d map[string]any
	getJSON(t, p.URL+"/discover.json", &d)

	if d["BaseURL"] != p.URL {
		t.Errorf("BaseURL = %v, want %v", d["BaseURL"], p.URL)
	}
	if d["LineupURL"] != p.URL+"/lineup.json" {
		t.Errorf("LineupURL = %v", d["LineupURL"])
	}
	if d["DeviceID"] != "10ABCC87" || d["TunerCount"] != float64(4) || d["FriendlyName"] != "Fake FLEX" {
		t.Errorf("other fields changed: %v", d)
	}
}

func TestLineupPointsAtProxyKeepingPathAndQuery(t *testing.T) {
	p := newProxy(t, newFakeTuner(t), "")
	var lineup []map[string]any
	getJSON(t, p.URL+"/lineup.json", &lineup)

	if len(lineup) != 1 {
		t.Fatalf("got %d channels", len(lineup))
	}
	if got, want := lineup[0]["URL"], p.URL+"/auto/v5.1?transcode=heavy"; got != want {
		t.Errorf("URL = %v, want %v", got, want)
	}
	if lineup[0]["GuideName"] != "WFRV" || lineup[0]["HD"] != float64(1) {
		t.Errorf("other fields changed: %v", lineup[0])
	}
}

func TestAdvertiseOverridesHost(t *testing.T) {
	p := newProxy(t, newFakeTuner(t), "http://yagi.example.ts.net:5004/")
	var d map[string]any
	getJSON(t, p.URL+"/discover.json", &d)
	if d["BaseURL"] != "http://yagi.example.ts.net:5004" {
		t.Errorf("BaseURL = %v", d["BaseURL"])
	}
}

func TestDeviceIDOverride(t *testing.T) {
	f := newFakeTuner(t)
	api, _ := url.Parse(f.api.URL)
	stream, _ := url.Parse(f.stream.URL)
	srv := httptest.NewServer(New(Config{API: api, Stream: stream, DeviceID: "10ABCD86"}))
	defer srv.Close()

	var d map[string]any
	getJSON(t, srv.URL+"/discover.json", &d)
	if d["DeviceID"] != "10ABCD86" {
		t.Errorf("DeviceID = %v, want 10ABCD86", d["DeviceID"])
	}
}

func TestOtherAPIPathsPassThrough(t *testing.T) {
	p := newProxy(t, newFakeTuner(t), "")
	resp, err := http.Get(p.URL + "/status.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if b, _ := io.ReadAll(resp.Body); strings.TrimSpace(string(b)) != "[]" {
		t.Errorf("body = %q", b)
	}
}

func TestStreamPassesThroughAndFreesTunerOnDisconnect(t *testing.T) {
	f := newFakeTuner(t)
	p := newProxy(t, f, "")

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/auto/v5.1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len("TSPACKET")*3)
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatalf("reading stream: %v", err)
	}
	if !strings.HasPrefix(string(buf), "TSPACKET") {
		t.Errorf("unexpected stream bytes %q", buf)
	}

	// The viewer goes away: the connection to the tuner must close with it,
	// otherwise the tuner stays busy for nobody.
	cancel()
	resp.Body.Close()
	select {
	case <-f.streamGone:
	case <-time.After(2 * time.Second):
		t.Fatal("tuner stream was still open 2s after the viewer disconnected")
	}
}

func TestTunerDownIs502(t *testing.T) {
	f := newFakeTuner(t)
	p := newProxy(t, f, "")
	f.api.Close()
	resp, err := http.Get(p.URL + "/discover.json")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
}

func TestRoutineRequestsAreLoggedAtDebugNotInfo(t *testing.T) {
	f := newFakeTuner(t)
	api, _ := url.Parse(f.api.URL)
	stream, _ := url.Parse(f.stream.URL)
	ask := func(level slog.Level) string {
		var out strings.Builder
		log := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: level}))
		srv := httptest.NewServer(New(Config{API: api, Stream: stream, Logger: log}))
		defer srv.Close()
		var v any
		getJSON(t, srv.URL+"/discover.json", &v)
		getJSON(t, srv.URL+"/lineup.json", &v)
		return out.String()
	}
	if got := ask(slog.LevelInfo); strings.Contains(got, "msg=request") {
		t.Errorf("routine requests are logged at info:\n%s", got)
	}
	if got := ask(slog.LevelDebug); strings.Count(got, "msg=request") != 2 {
		t.Errorf("debug should show both requests:\n%s", got)
	}
}

func TestTunerRefusingAStreamIsLoggedAndPassedOn(t *testing.T) {
	f := newFakeTuner(t)
	f.stream.Close()
	f.stream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-HDHomeRun-Error", "805 - Tuner in use")
		http.Error(w, "All tuners in use", http.StatusServiceUnavailable)
	}))
	t.Cleanup(f.stream.Close)
	api, _ := url.Parse(f.api.URL)
	stream, _ := url.Parse(f.stream.URL)
	var out strings.Builder
	srv := httptest.NewServer(New(Config{API: api, Stream: stream, Logger: slog.New(slog.NewTextHandler(&out, nil))}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/auto/v5.1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, the tuner's answer should reach the viewer as it is", resp.StatusCode)
	}
	srv.Close() // waits for the handler, which logs "stream closed" after the answer is sent
	got := out.String()
	for _, want := range []string{"msg=\"tuner refused stream\"", "status=\"503 Service Unavailable\"", "x-hdhomerun-error=\"805 - Tuner in use\""} {
		if !strings.Contains(got, want) {
			t.Errorf("log lacks %s:\n%s", want, got)
		}
	}
}

func TestStreamThatWorksIsNotLoggedAsRefused(t *testing.T) {
	f := newFakeTuner(t)
	api, _ := url.Parse(f.api.URL)
	stream, _ := url.Parse(f.stream.URL)
	var out strings.Builder
	srv := httptest.NewServer(New(Config{API: api, Stream: stream, Logger: slog.New(slog.NewTextHandler(&out, nil))}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/auto/v5.1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	resp.Body.Close()
	srv.Close() // waits for the handler, which is still logging "stream closed"
	if strings.Contains(out.String(), "refused") {
		t.Errorf("a stream that worked was logged as refused:\n%s", out.String())
	}
}

func TestEmptyLineupIsLogged(t *testing.T) {
	f := newFakeTuner(t)
	f.api.Close()
	f.api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[]`)
	}))
	t.Cleanup(f.api.Close)
	api, _ := url.Parse(f.api.URL)
	stream, _ := url.Parse(f.stream.URL)
	var out strings.Builder
	srv := httptest.NewServer(New(Config{API: api, Stream: stream, Logger: slog.New(slog.NewTextHandler(&out, nil))}))
	defer srv.Close()

	var v any
	for i := 0; i < 5; i++ { // Plex asks over and over
		getJSON(t, srv.URL+"/lineup.json", &v)
	}
	srv.Close()
	if n := strings.Count(out.String(), "tuner has no channels"); n != 1 {
		t.Errorf("an empty lineup was logged %d times for 5 requests, want once:\n%s", n, out.String())
	}
}
