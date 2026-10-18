package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// newProxyFor fronts a hand-written API handler; the stream upstream is a
// closed server unless the test gives one.
func newProxyFor(t *testing.T, api http.Handler, stream *url.URL) *httptest.Server {
	t.Helper()
	apiSrv := httptest.NewServer(api)
	t.Cleanup(apiSrv.Close)
	apiURL, _ := url.Parse(apiSrv.URL)
	if stream == nil {
		dead := httptest.NewServer(http.NotFoundHandler())
		stream, _ = url.Parse(dead.URL)
		dead.Close() // nothing is listening here any more
	}
	srv := httptest.NewServer(New(Config{API: apiURL, Stream: stream}))
	t.Cleanup(srv.Close)
	return srv
}

func status(t *testing.T, u string) int {
	t.Helper()
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestStreamUpstreamDownIs502(t *testing.T) {
	p := newProxyFor(t, http.NotFoundHandler(), nil)
	if got := status(t, p.URL+"/auto/v5.1"); got != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", got)
	}
}

func TestBadTunerResponsesAre502(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"non-200":      func(w http.ResponseWriter, r *http.Request) { http.Error(w, "busy", http.StatusServiceUnavailable) },
		"invalid json": func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "<html>not json") },
		"wrong shape":  func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `"just a string"`) },
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			p := newProxyFor(t, h, nil)
			for _, path := range []string{"/discover.json", "/lineup.json"} {
				if got := status(t, p.URL+path); got != http.StatusBadGateway {
					t.Errorf("%s: status = %d, want 502", path, got)
				}
			}
		})
	}
}

func TestQueryStringIsForwardedToTuner(t *testing.T) {
	var mu sync.Mutex
	var seen string
	p := newProxyFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = r.URL.RawQuery
		mu.Unlock()
		io.WriteString(w, `[]`)
	}), nil)

	status(t, p.URL+"/lineup.json?show=found&tuning")
	mu.Lock()
	defer mu.Unlock()
	if seen != "show=found&tuning" {
		t.Errorf("tuner saw query %q, want %q", seen, "show=found&tuning")
	}
}

func TestLineupLeavesOddEntriesAlone(t *testing.T) {
	p := newProxyFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[
			{"GuideNumber":"1.1","GuideName":"no url"},
			{"GuideNumber":"1.2","URL":42},
			{"GuideNumber":"1.3","URL":"http://192.168.1.9:5004/auto/v1.3"}
		]`)
	}), nil)

	var lineup []map[string]any
	getJSON(t, p.URL+"/lineup.json", &lineup)
	if len(lineup) != 3 {
		t.Fatalf("got %d entries, want 3", len(lineup))
	}
	if _, has := lineup[0]["URL"]; has {
		t.Errorf("entry without URL gained one: %v", lineup[0])
	}
	if lineup[1]["URL"] != float64(42) {
		t.Errorf("non-string URL changed: %v", lineup[1]["URL"])
	}
	if lineup[2]["URL"] != p.URL+"/auto/v1.3" {
		t.Errorf("valid URL = %v", lineup[2]["URL"])
	}
}

func TestRebase(t *testing.T) {
	cases := []struct{ raw, base, want string }{
		{"http://192.168.1.132:5004/auto/v5.1", "http://relay:5004", "http://relay:5004/auto/v5.1"},
		{"http://192.168.1.132/auto/v5.1?a=b&c=d", "https://relay.example", "https://relay.example/auto/v5.1?a=b&c=d"},
		{"http://tuner/tuner0/v2.1", "http://100.64.0.1:8080", "http://100.64.0.1:8080/tuner0/v2.1"},
		{"://garbage", "http://relay", "://garbage"},       // unparseable input is left alone
		{"http://tuner/x", "://garbage", "http://tuner/x"}, // unparseable base leaves the URL alone
	}
	for _, c := range cases {
		if got := rebase(c.raw, c.base); got != c.want {
			t.Errorf("rebase(%q, %q) = %q, want %q", c.raw, c.base, got, c.want)
		}
	}
}

// movingUpstream lets a test relocate the tuner while the proxy is running.
type movingUpstream struct {
	mu          sync.Mutex
	api, stream *url.URL
}

func (m *movingUpstream) Targets() (*url.URL, *url.URL) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.api, m.stream
}

func (m *movingUpstream) set(api, stream *url.URL) {
	m.mu.Lock()
	m.api, m.stream = api, stream
	m.mu.Unlock()
}

func TestFollowsUpstreamWhenItMoves(t *testing.T) {
	a, b := newFakeTuner(t), newFakeTuner(t)
	parse := func(f *fakeTuner) (*url.URL, *url.URL) {
		api, _ := url.Parse(f.api.URL)
		stream, _ := url.Parse(f.stream.URL)
		return api, stream
	}
	up := &movingUpstream{}
	up.set(parse(a))
	srv := httptest.NewServer(New(Config{Upstream: up}))
	defer srv.Close()

	var d map[string]any
	getJSON(t, srv.URL+"/discover.json", &d)
	if d["FriendlyName"] != "Fake FLEX" {
		t.Fatalf("first tuner: %v", d)
	}

	a.api.Close() // the old address goes dark...
	a.stream.Close()
	up.set(parse(b)) // ...and the tuner turns up somewhere else

	getJSON(t, srv.URL+"/discover.json", &d)
	if d["DeviceID"] != "10ABCC87" {
		t.Errorf("after the move: %v", d)
	}
	if got := status(t, srv.URL+"/status.json"); got != http.StatusOK {
		t.Errorf("pass-through after the move = %d", got)
	}
}

func TestUnknownUpstreamIs502(t *testing.T) {
	srv := httptest.NewServer(New(Config{Upstream: &movingUpstream{}})) // no address yet
	defer srv.Close()
	for _, path := range []string{"/discover.json", "/lineup.json", "/status.json", "/auto/v5.1"} {
		if got := status(t, srv.URL+path); got != http.StatusBadGateway {
			t.Errorf("%s = %d, want 502", path, got)
		}
	}
}

// The tuner must see its own address as Host, not Antennarr's, or firmware
// that checks Host would reject the request.
func TestTunerSeesItsOwnHostHeader(t *testing.T) {
	p := newProxyFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, r.Host) // a tuner that echoes the Host it was called with
	}), nil)
	resp, err := http.Get(p.URL + "/anything")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	proxyHost := strings.TrimPrefix(p.URL, "http://")
	if got := string(body); got == "" || got == proxyHost {
		t.Errorf("tuner saw Host %q; want its own address, not the proxy's %q", got, proxyHost)
	}
}

func TestMediaServersAlwaysGetTheTunersOwnName(t *testing.T) {
	// The name the owner gives a tuner is for the web page only. What a media server is told is the
	// tuner's own, so it doesn't change when the owner renames it (and Plex keeps showing the model).
	f := newFakeTuner(t)
	api, _ := url.Parse(f.api.URL)
	stream, _ := url.Parse(f.stream.URL)
	srv := httptest.NewServer(New(Config{API: api, Stream: stream}))
	defer srv.Close()

	var d map[string]any
	getJSON(t, srv.URL+"/discover.json", &d)
	if d["FriendlyName"] != "Fake FLEX" || d["DeviceID"] != "10ABCC87" || d["TunerCount"] != float64(4) {
		t.Errorf("discover.json = %v", d)
	}
}
