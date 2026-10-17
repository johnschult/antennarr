package firmware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// page is a fake changelog server that records what it is asked.
type page struct {
	*httptest.Server
	hits       atomic.Int32
	mu         sync.Mutex
	body       string
	status     int
	etag       string
	lastIfNone string
	lastUA     string
}

func newPage(t *testing.T) *page {
	t.Helper()
	b, err := os.ReadFile("testdata/changelog.html")
	if err != nil {
		t.Fatal(err)
	}
	p := &page{body: string(b), status: 200, etag: `W/"v1"`}
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.hits.Add(1)
		p.mu.Lock()
		defer p.mu.Unlock()
		p.lastIfNone, p.lastUA = r.Header.Get("If-None-Match"), r.UserAgent()
		if p.status == 200 && p.etag != "" && r.Header.Get("If-None-Match") == p.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if p.etag != "" {
			w.Header().Set("ETag", p.etag)
		}
		w.WriteHeader(p.status)
		w.Write([]byte(p.body))
	}))
	t.Cleanup(p.Close)
	return p
}

func (p *page) set(f func(*page)) { p.mu.Lock(); f(p); p.mu.Unlock() }

// fast is a checker with no minimum gap, so tests can refresh back to back.
func fast(p *page, mut ...func(*Config)) *Checker {
	cfg := Config{URL: p.URL, MinGap: time.Nanosecond, Interval: time.Hour}
	for _, m := range mut {
		m(&cfg)
	}
	return NewChecker(cfg)
}

func TestRefreshReadsThePageAndSaysWhereATunerStands(t *testing.T) {
	p := newPage(t)
	c := fast(p)
	if st := c.StatusFor(flex4k); st.State != StateUnknown || !strings.Contains(st.Error, "hasn't been fetched") {
		t.Errorf("before any fetch: %+v", st)
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := c.StatusFor(flex4k)
	if st.State != StateUpToDate || st.CheckedAt.IsZero() || st.Error != "" {
		t.Errorf("%+v", st)
	}
	old := flex4k
	old.Version = "20250815"
	if st := c.StatusFor(old); st.State != StateUpdateAvailable || st.Latest != "20260326" {
		t.Errorf("an older tuner: %+v", st)
	}
	if !strings.HasPrefix(p.lastUA, "Antennarr") {
		t.Errorf("User-Agent = %q: say who is asking", p.lastUA)
	}
}

func TestAnUnchangedPageIsNotDownloadedAgain(t *testing.T) {
	p := newPage(t)
	c := fast(p)
	c.Refresh(context.Background())
	first := c.CheckedAt()
	time.Sleep(5 * time.Millisecond)

	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.lastIfNone != `W/"v1"` {
		t.Errorf("If-None-Match = %q, want the ETag from last time", p.lastIfNone)
	}
	if !c.CheckedAt().After(first) {
		t.Error("a 304 should still count as having looked")
	}
	if st := c.StatusFor(flex4k); st.State != StateUpToDate {
		t.Errorf("the releases were lost after a 304: %+v", st)
	}

	// When the page does change, the new copy is used.
	p.set(func(p *page) {
		p.etag = `W/"v2"`
		p.body = strings.Replace(p.body, "<h4>Release 20260326:</h4>", "<h4>Release 20270101:</h4>", 1)
	})
	c.Refresh(context.Background())
	if st := c.StatusFor(flex4k); st.State != StateUpdateAvailable || st.Latest != "20270101" {
		t.Errorf("after the page changed: %+v", st)
	}
}

func TestRefreshesAreRateLimited(t *testing.T) {
	p := newPage(t)
	c := NewChecker(Config{URL: p.URL}) // the default 30s gap
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := c.Refresh(context.Background()); err != ErrTooSoon {
			t.Errorf("refresh %d: %v, want ErrTooSoon", i+2, err)
		}
	}
	if n := p.hits.Load(); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestAFailedFetchKeepsWhatWeHad(t *testing.T) {
	p := newPage(t)
	good := p.body
	c := fast(p)
	c.Refresh(context.Background())

	for name, mut := range map[string]func(*page){
		"server error": func(p *page) { p.status, p.etag = 500, "" },
		"page changed": func(p *page) { p.status, p.etag, p.body = 200, "", "<html><h1>Down for maintenance</h1></html>" },
	} {
		p.set(mut)
		if err := c.Refresh(context.Background()); err == nil {
			t.Errorf("%s: no error", name)
		}
		st := c.StatusFor(flex4k)
		if st.State != StateUpToDate || !strings.Contains(st.Error, "couldn't refresh") {
			t.Errorf("%s: should still answer from the last good copy, and say it is stale: %+v", name, st)
		}
		p.set(func(p *page) { p.status, p.etag, p.body = 200, "", good })
	}
}

func TestFailureBeforeAnySuccessIsUnknownWithTheReason(t *testing.T) {
	p := newPage(t)
	p.set(func(p *page) { p.status, p.etag = 503, "" })
	c := fast(p)
	if err := c.Refresh(context.Background()); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("err = %v", err)
	}
	if st := c.StatusFor(flex4k); st.State != StateUnknown || !strings.Contains(st.Error, "503") {
		t.Errorf("%+v", st)
	}
}

func TestAnUnreachableSiteIsAnError(t *testing.T) {
	p := newPage(t)
	url := p.URL
	p.Close()
	c := NewChecker(Config{URL: url, MinGap: time.Nanosecond})
	if err := c.Refresh(context.Background()); err == nil {
		t.Error("expected an error")
	}
	if st := c.StatusFor(flex4k); st.State != StateUnknown || st.Error == "" {
		t.Errorf("%+v", st)
	}
}

func TestNothingIsFetchedWhileTurnedOff(t *testing.T) {
	p := newPage(t)
	var on atomic.Bool
	c := fast(p, func(c *Config) { c.Enabled = on.Load })

	if err := c.Refresh(context.Background()); err == nil {
		t.Error("a disabled check should say so")
	}
	if p.hits.Load() != 0 {
		t.Error("a request was made while the check was off")
	}
	if st := c.StatusFor(flex4k); st.State != StateDisabled {
		t.Errorf("%+v", st)
	}
	// The tuner's own report is still worth showing: it needs no outside request.
	d := flex4k
	d.UpgradeAvailable = "20270101"
	if st := c.StatusFor(d); st.State != StateUpdateAvailable {
		t.Errorf("device report while off: %+v", st)
	}

	on.Store(true) // switched on while running
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := c.StatusFor(flex4k); st.State != StateUpToDate {
		t.Errorf("after enabling: %+v", st)
	}
}

func TestOnChangeAfterEveryAttempt(t *testing.T) {
	p := newPage(t)
	var n atomic.Int32
	c := fast(p, func(c *Config) { c.OnChange = func() { n.Add(1) } })
	c.Refresh(context.Background())
	p.set(func(p *page) { p.status, p.etag = 500, "" })
	c.Refresh(context.Background())
	if n.Load() != 2 {
		t.Errorf("OnChange called %d times, want 2 (a failure is news too)", n.Load())
	}
}

func TestRunLooksAtOnceThenOnTheInterval(t *testing.T) {
	p := newPage(t)
	c := fast(p, func(c *Config) { c.Interval = 40 * time.Millisecond; c.MinGap = time.Nanosecond })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()

	deadline := time.Now().Add(3 * time.Second)
	for p.hits.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if p.hits.Load() < 3 {
		t.Errorf("only %d fetches: the first should be immediate and then one per interval", p.hits.Load())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestRunDoesNotLookWhileTurnedOff(t *testing.T) {
	p := newPage(t)
	c := fast(p, func(c *Config) { c.Interval = 20 * time.Millisecond; c.Enabled = func() bool { return false } })
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	c.Run(ctx)
	if p.hits.Load() != 0 {
		t.Errorf("%d requests while disabled", p.hits.Load())
	}
}

func TestKickMakesRunLookEarly(t *testing.T) {
	p := newPage(t)
	var on atomic.Bool
	c := fast(p, func(c *Config) { c.Enabled = on.Load }) // interval is an hour: only a kick can trigger a look
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	if p.hits.Load() != 0 {
		t.Fatal("looked while off")
	}

	on.Store(true)
	for i := 0; i < 10; i++ {
		c.Kick() // never blocks, however many
	}
	deadline := time.Now().Add(2 * time.Second)
	for p.hits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if p.hits.Load() == 0 {
		t.Error("a kick did not trigger a look")
	}
}

func TestNewCheckerDefaults(t *testing.T) {
	c := NewChecker(Config{})
	if c.cfg.URL != DefaultURL || c.cfg.Interval != 12*time.Hour || c.cfg.MinGap != 30*time.Second {
		t.Errorf("defaults = %+v", c.cfg)
	}
	if !strings.HasPrefix(DefaultURL, "https://") {
		t.Error("the default must be https")
	}
}

func TestStatusNeedsAFirmwareVersion(t *testing.T) {
	p := newPage(t)
	c := fast(p)
	c.Refresh(context.Background())
	d := flex4k
	d.Version = ""
	if st := c.StatusFor(d); st.State != StateUnknown || !strings.Contains(st.Error, "didn't report a firmware version") {
		t.Errorf("%+v", st)
	}
}
