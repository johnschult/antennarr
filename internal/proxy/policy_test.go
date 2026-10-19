package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakePolicy struct {
	authErr   error
	streamErr error
	count     int

	mu       sync.Mutex
	released int
	streams  int
}

func (f *fakePolicy) Authorize(*http.Request) error { return f.authErr }
func (f *fakePolicy) Stream(*http.Request) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	f.streams++
	return func() { f.mu.Lock(); f.released++; f.mu.Unlock() }, nil
}
func (f *fakePolicy) TunerCount(_ *http.Request, real int) int {
	if f.count >= 0 {
		return f.count
	}
	return real
}
func (f *fakePolicy) releasedCount() int { f.mu.Lock(); defer f.mu.Unlock(); return f.released }

// waitReleased waits for the slot to be given back. That happens when the
// handler returns, which can be a moment after the client has its response.
func (f *fakePolicy) waitReleased(want int) int {
	deadline := time.Now().Add(2 * time.Second)
	for f.releasedCount() < want && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	return f.releasedCount()
}

func proxyWithPolicy(t *testing.T, f *fakeTuner, pol Policy) *httptest.Server {
	t.Helper()
	api, _ := url.Parse(f.api.URL)
	stream, _ := url.Parse(f.stream.URL)
	srv := httptest.NewServer(New(Config{API: api, Stream: stream, Policy: pol}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPolicyAuthorizeRefusesEverything(t *testing.T) {
	pol := &fakePolicy{authErr: &Denied{Code: 403, Reason: "not-allowed", Text: "go away"}, count: -1}
	p := proxyWithPolicy(t, newFakeTuner(t), pol)

	for _, path := range []string{"/discover.json", "/lineup.json", "/status.json", "/auto/v5.1"} {
		resp, err := http.Get(p.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 403 || resp.Header.Get("X-Yagi-Reason") != "not-allowed" {
			t.Errorf("%s: %d reason=%q", path, resp.StatusCode, resp.Header.Get("X-Yagi-Reason"))
		}
		if string(body) != "go away\n" {
			t.Errorf("%s: body %q", path, body)
		}
	}
	if pol.streams != 0 {
		t.Error("a refused request still opened a stream")
	}
}

func TestPolicyStreamRefusalNeverTouchesTuner(t *testing.T) {
	f := newFakeTuner(t)
	pol := &fakePolicy{streamErr: &Denied{Code: 503, Reason: "viewer-limit", Text: "all 1 of your tuners are in use"}, count: -1}
	p := proxyWithPolicy(t, f, pol)

	resp, err := http.Get(p.URL + "/auto/v5.1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 503 || resp.Header.Get("X-Yagi-Reason") != "viewer-limit" {
		t.Errorf("%d reason=%q", resp.StatusCode, resp.Header.Get("X-Yagi-Reason"))
	}
	select {
	case <-f.streamGone:
		t.Error("the tuner was opened for a refused stream")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestPolicyPlainErrorBecomes403(t *testing.T) {
	pol := &fakePolicy{authErr: errors.New("nope"), count: -1}
	p := proxyWithPolicy(t, newFakeTuner(t), pol)
	resp, err := http.Get(p.URL + "/discover.json")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 || resp.Header.Get("X-Yagi-Reason") != "denied" {
		t.Errorf("%d reason=%q", resp.StatusCode, resp.Header.Get("X-Yagi-Reason"))
	}
}

func TestPolicyReleasesWhenViewerDisconnects(t *testing.T) {
	f := newFakeTuner(t)
	pol := &fakePolicy{count: -1}
	p := proxyWithPolicy(t, f, pol)

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/auto/v5.1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatal(err)
	}
	if pol.releasedCount() != 0 {
		t.Fatal("released while the stream was still running")
	}

	cancel()
	resp.Body.Close()
	if n := pol.waitReleased(1); n != 1 {
		t.Errorf("released %d times after disconnect, want exactly 1", n)
	}
}

func TestPolicyReleasesWhenTunerRefuses(t *testing.T) {
	var opened atomic.Int32
	api := http.NotFoundHandler()
	apiSrv := httptest.NewServer(api)
	defer apiSrv.Close()
	busy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		opened.Add(1)
		http.Error(w, "all tuners in use", http.StatusServiceUnavailable)
	}))
	defer busy.Close()

	pol := &fakePolicy{count: -1}
	a, _ := url.Parse(apiSrv.URL)
	s, _ := url.Parse(busy.URL)
	p := httptest.NewServer(New(Config{API: a, Stream: s, Policy: pol}))
	defer p.Close()

	resp, err := http.Get(p.URL + "/auto/v5.1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 503 || opened.Load() != 1 {
		t.Errorf("status %d, tuner opened %d times", resp.StatusCode, opened.Load())
	}
	if n := pol.waitReleased(1); n != 1 {
		t.Errorf("released %d times, want 1 (a busy tuner must not leak our slot)", n)
	}
}

func TestPolicyRewritesTunerCount(t *testing.T) {
	cases := []struct {
		policy int
		want   float64
	}{{policy: 2, want: 2}, {policy: 0, want: 0}, {policy: -1, want: 4}}
	for _, c := range cases {
		pol := &fakePolicy{count: c.policy}
		p := proxyWithPolicy(t, newFakeTuner(t), pol)
		var d map[string]any
		getJSON(t, p.URL+"/discover.json", &d)
		if d["TunerCount"] != c.want {
			t.Errorf("policy %d: TunerCount = %v, want %v", c.policy, d["TunerCount"], c.want)
		}
	}
}
