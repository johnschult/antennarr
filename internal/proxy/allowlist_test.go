package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// recordingTuner is a tuner API that notes every request it receives, so a
// test can prove a request never got as far as the tuner.
type recordingTuner struct {
	srv  *httptest.Server
	mu   sync.Mutex
	seen []string
}

func newRecordingTuner(t *testing.T) *recordingTuner {
	t.Helper()
	rt := &recordingTuner{}
	rt.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rt.mu.Lock()
		rt.seen = append(rt.seen, r.Method+" "+r.URL.RequestURI())
		rt.mu.Unlock()
		io.WriteString(w, `[]`)
	}))
	t.Cleanup(rt.srv.Close)
	return rt
}

func (rt *recordingTuner) requests() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string(nil), rt.seen...)
}

func proxyFor(t *testing.T, rt *recordingTuner) *httptest.Server {
	t.Helper()
	u, _ := url.Parse(rt.srv.URL)
	srv := httptest.NewServer(New(Config{API: u, Stream: u}))
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, method, u string) int {
	t.Helper()
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// A viewer must not reach the tuner's management endpoints: rescans, firmware
// upgrade, its own web pages, or anything else that isn't on the list.
func TestManagementPathsNeverReachTheTuner(t *testing.T) {
	rt := newRecordingTuner(t)
	srv := proxyFor(t, rt)

	cases := []struct{ method, path string }{
		{"POST", "/lineup.post?scan=start"},
		{"GET", "/lineup.post?scan=start"},
		{"POST", "/lineup.post?scan=abort"},
		{"GET", "/upgrade"},
		{"POST", "/upgrade"},
		{"GET", "/tuners.html"},
		{"GET", "/tuners.html?page=2"},
		{"GET", "/system.json"},
		{"GET", "/"},
		{"GET", "/device.xml"},
		{"GET", "/lineup.xml"},
		{"GET", "/%2e%2e/upgrade"},
		{"GET", "/status.json/../upgrade"},
		{"PUT", "/status.json/x"},
		{"GET", "/auto"},
		{"GET", "/auto/"},
		{"GET", "/tuner/v5.1"},
		{"GET", "/tunerX/v5.1"},
		{"GET", "/tuner0"},
		{"GET", "/tuner0/"},
	}
	for _, c := range cases {
		code := do(t, c.method, srv.URL+c.path)
		if code != http.StatusNotFound && code != http.StatusMethodNotAllowed && code != http.StatusMovedPermanently {
			t.Errorf("%s %s = %d, want a refusal", c.method, c.path, code)
		}
	}
	if got := rt.requests(); len(got) != 0 {
		t.Errorf("the tuner received requests it should never have seen: %v", got)
	}
}

// The status pages a media server reads still pass through, read-only.
func TestStatusPagesStillPassThrough(t *testing.T) {
	rt := newRecordingTuner(t)
	srv := proxyFor(t, rt)

	for _, path := range []string{"/status.json", "/lineup_status.json"} {
		if code := do(t, "GET", srv.URL+path); code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, code)
		}
		if code := do(t, "HEAD", srv.URL+path); code != http.StatusOK {
			t.Errorf("HEAD %s = %d, want 200", path, code)
		}
		for _, m := range []string{"POST", "PUT", "DELETE"} {
			if code := do(t, m, srv.URL+path); code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s = %d, want 405", m, path, code)
			}
		}
	}
	for _, r := range rt.requests() {
		if r[:3] == "POS" || r[:3] == "PUT" || r[:3] == "DEL" {
			t.Errorf("tuner saw a write: %s", r)
		}
	}
}

// discover.json and lineup.json are rewritten by Antennarr, so they take the
// same read-only rule.
func TestRewrittenPagesAreReadOnly(t *testing.T) {
	rt := newRecordingTuner(t)
	srv := proxyFor(t, rt)
	for _, path := range []string{"/discover.json", "/lineup.json"} {
		if code := do(t, "POST", srv.URL+path); code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s = %d, want 405", path, code)
		}
	}
	if got := rt.requests(); len(got) != 0 {
		t.Errorf("tuner saw %v", got)
	}
}

// Streams are GET only, so every stream is metered.
func TestStreamsAreGetOnly(t *testing.T) {
	f := newFakeTuner(t)
	p := newProxy(t, f, "")
	for _, m := range []string{"HEAD", "POST", "PUT", "DELETE"} {
		if code := do(t, m, p.URL+"/auto/v5.1"); code != http.StatusMethodNotAllowed {
			t.Errorf("%s /auto/v5.1 = %d, want 405", m, code)
		}
	}
}

func TestIsStream(t *testing.T) {
	yes := []string{"/auto/v5.1", "/auto/ch577000000-3", "/tuner0/v5.1", "/tuner12/ch577000000"}
	no := []string{
		"/", "/auto", "/auto/", "/auto/v5.1/extra", "/tuner", "/tuners.html", "/tuner0",
		"/tuner0/", "/tunerX/v5.1", "/tuner/v5.1", "/status.json", "/lineup.post", "/autox/v5.1",
	}
	for _, p := range yes {
		if !isStream(p) {
			t.Errorf("isStream(%q) = false, want true", p)
		}
	}
	for _, p := range no {
		if isStream(p) {
			t.Errorf("isStream(%q) = true, want false", p)
		}
	}
}
