package guide

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const secret = "S3CR3T-DEVICE-AUTH"

// fakeSD stands in for SiliconDust: the guide, and a logo.
type fakeSD struct {
	*httptest.Server
	guide    []byte
	hits     atomic.Int32
	iconHits atomic.Int32
	status   atomic.Int32 // 0 means 200
	etag     string
	mu       sync.Mutex
	gotAuth  []string
	iconType string
	iconBody []byte
}

func newFakeSD(t *testing.T) *fakeSD {
	t.Helper()
	g, err := os.ReadFile("testdata/xmltv.xml")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSD{guide: g, etag: `"v1"`, iconType: "image/png", iconBody: []byte("\x89PNG-fake")}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/xmltv":
			f.hits.Add(1)
			f.mu.Lock()
			f.gotAuth = append(f.gotAuth, r.URL.Query().Get("DeviceAuth"))
			f.mu.Unlock()
			if s := int(f.status.Load()); s != 0 {
				http.Error(w, "nope", s)
				return
			}
			if r.Header.Get("If-None-Match") == f.etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", f.etag)
			w.Write(f.guide)
		case strings.HasPrefix(r.URL.Path, "/channels/"):
			f.iconHits.Add(1)
			w.Header().Set("Content-Type", f.iconType)
			w.Write(f.iconBody)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	// Logos in the test guide live on SiliconDust's real host; point them at the
	// fake, which is what the service is told to trust.
	f.guide = []byte(strings.ReplaceAll(string(f.guide), "https://img.hdhomerun.com", f.URL))
	return f
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type rig struct {
	*Service
	sd   *fakeSD
	clk  *clock
	logs *bytes.Buffer
	on   atomic.Bool
	auth atomic.Value
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{sd: newFakeSD(t), clk: &clock{t: at}, logs: &bytes.Buffer{}}
	r.on.Store(true)
	r.auth.Store(secret)
	r.Service = NewService(Config{
		URL: r.sd.URL + "/api/xmltv", IconBase: r.sd.URL, Now: r.clk.now,
		Enabled: r.on.Load, Logger: slog.New(slog.NewTextHandler(r.logs, nil)),
	})
	r.Watch("antenna", func() string { return r.auth.Load().(string) })
	return r
}

func TestFetchesTheGuideForADeviceAndAnswersLookups(t *testing.T) {
	r := newRig(t)
	r.Refresh(context.Background())
	if got := r.sd.gotAuth; len(got) != 1 || got[0] != secret {
		t.Fatalf("SiliconDust was sent %v", got)
	}
	e, ok := r.Lookup("antenna", "11.1", at)
	if !ok || e.Name != "WXIADT" || !e.Icon || e.Now == nil || e.Now.Title != "Rosie's Rules" {
		t.Fatalf("lookup = %+v, %v", e, ok)
	}
	if _, ok := r.Lookup("antenna", "99.9", at); ok {
		t.Error("an unknown channel was found")
	}
	if _, ok := r.Lookup("nobody", "11.1", at); ok {
		t.Error("an unknown device was found")
	}
}

func TestNeverTalksToSiliconDustWhileOff(t *testing.T) {
	r := newRig(t)
	r.on.Store(false)
	r.Refresh(context.Background())
	if r.sd.hits.Load() != 0 {
		t.Fatal("fetched while off")
	}
	r.on.Store(true)
	r.Refresh(context.Background())
	if r.sd.hits.Load() != 1 {
		t.Fatalf("hits = %d after switching on", r.sd.hits.Load())
	}
}

func TestDevicesSharingATunerShareOneFetch(t *testing.T) {
	r := newRig(t)
	r.Watch("second", func() string { return secret })
	r.Watch("other-tuner", func() string { return "different" })
	r.Refresh(context.Background())
	if r.sd.hits.Load() != 2 {
		t.Fatalf("hits = %d, want one per distinct DeviceAuth", r.sd.hits.Load())
	}
	if _, ok := r.Lookup("second", "11.1", at); !ok {
		t.Error("the second device didn't get the shared guide")
	}
}

func TestWaitsForTheTunerToReportItsAuth(t *testing.T) {
	r := newRig(t)
	r.auth.Store("")
	r.Refresh(context.Background())
	if r.sd.hits.Load() != 0 {
		t.Fatal("fetched without a DeviceAuth")
	}
}

func TestRespectsMinGapAndUsesConditionalRequests(t *testing.T) {
	r := newRig(t)
	r.Refresh(context.Background())
	r.Refresh(context.Background()) // too soon
	if r.sd.hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1", r.sd.hits.Load())
	}
	r.clk.add(time.Minute)
	r.Refresh(context.Background()) // allowed; the guide hasn't changed
	if r.sd.hits.Load() != 2 {
		t.Fatalf("hits = %d, want 2", r.sd.hits.Load())
	}
	if _, ok := r.Lookup("antenna", "11.1", at); !ok {
		t.Error("a 304 must keep the guide we have")
	}
}

func TestAFailedRefreshKeepsTheGuideAndNeverLeaksTheAuth(t *testing.T) {
	r := newRig(t)
	r.Refresh(context.Background())
	r.sd.status.Store(500)
	r.clk.add(time.Minute)
	r.Refresh(context.Background())
	if _, ok := r.Lookup("antenna", "11.1", at); !ok {
		t.Error("the old guide was lost after a failed refresh")
	}

	// And an unreachable service: Go's own error text includes the URL.
	srv := NewService(Config{URL: "http://127.0.0.1:1/api/xmltv", Now: r.clk.now, Logger: slog.New(slog.NewTextHandler(r.logs, nil))})
	srv.Watch("antenna", func() string { return secret })
	srv.Refresh(context.Background())
	if strings.Contains(r.logs.String(), secret) {
		t.Fatalf("the DeviceAuth leaked into the log:\n%s", r.logs)
	}
	if !strings.Contains(r.logs.String(), "guide fetch failed") {
		t.Errorf("the failure wasn't logged:\n%s", r.logs)
	}
}

func TestRefusesAGuideThatIsTooBigOrNotAGuide(t *testing.T) {
	r := newRig(t)
	r.sd.guide = []byte("<html>hello")
	r.Refresh(context.Background())
	if _, ok := r.Lookup("antenna", "11.1", at); ok {
		t.Error("a non-guide was accepted")
	}

	r2 := newRig(t)
	r2.sd.guide = append([]byte("<tv>"), bytes.Repeat([]byte("<!-- padding -->"), maxGuide/16+10)...)
	r2.Refresh(context.Background())
	if _, ok := r2.Lookup("antenna", "11.1", at); ok {
		t.Error("an oversized guide was accepted")
	}
}

func TestKickOnlyFetchesWhatIsMissingOrStale(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.refresh(ctx, true) // never read: fetch
	if r.sd.hits.Load() != 1 {
		t.Fatalf("hits = %d", r.sd.hits.Load())
	}
	r.clk.add(time.Minute)
	r.refresh(ctx, true) // fresh: leave alone, however often the monitors kick
	if r.sd.hits.Load() != 1 {
		t.Fatalf("a fresh guide was fetched again: %d", r.sd.hits.Load())
	}
	r.clk.add(7 * time.Hour)
	r.refresh(ctx, true) // stale
	if r.sd.hits.Load() != 2 {
		t.Fatalf("a stale guide wasn't fetched: %d", r.sd.hits.Load())
	}
}

func TestIconsAreFetchedOnceFromTheGuidesAddressOnly(t *testing.T) {
	r := newRig(t)
	r.Refresh(context.Background())
	// 8.1's icon is on another host entirely.
	if _, _, err := r.Icon(context.Background(), "antenna", "8.1"); err != ErrNoIcon {
		t.Fatalf("a foreign host: %v", err)
	}
	if e, _ := r.Lookup("antenna", "8.1", at); e.Icon {
		t.Error("a foreign icon is advertised")
	}
}

func TestServesAnIconAndCachesIt(t *testing.T) {
	r := newRig(t)
	r.Refresh(context.Background())
	body, typ, err := r.Icon(context.Background(), "antenna", "11.1")
	if err != nil || typ != "image/png" || !bytes.HasPrefix(body, []byte("\x89PNG")) {
		t.Fatalf("icon = %q %q %v", body, typ, err)
	}
	if _, _, err := r.Icon(context.Background(), "antenna", "11.1"); err != nil || r.sd.iconHits.Load() != 1 {
		t.Fatalf("second fetch: err=%v hits=%d (want cached)", err, r.sd.iconHits.Load())
	}
}

func TestOnlyPlainImagesAreServed(t *testing.T) {
	for name, typ := range map[string]string{"svg": "image/svg+xml", "html": "text/html"} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.sd.iconType = typ
			r.Refresh(context.Background())
			if _, _, err := r.Icon(context.Background(), "antenna", "11.1"); err == nil {
				t.Errorf("%s was served", typ)
			}
		})
	}
}

func TestANilServiceKnowsNothing(t *testing.T) {
	var s *Service
	if _, ok := s.Lookup("a", "1.1", at); ok {
		t.Error("nil service found something")
	}
}

func TestRunFetchesAtStartAndStopsWithTheContext(t *testing.T) {
	r := newRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	deadline := time.Now().Add(3 * time.Second)
	for r.sd.hits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if r.sd.hits.Load() == 0 {
		t.Fatal("Run never fetched")
	}
	r.Kick()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run didn't stop")
	}
}
