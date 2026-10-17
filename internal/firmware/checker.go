package firmware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// DefaultURL is SiliconDust's public firmware changelog.
const DefaultURL = "https://www.silicondust.com/support/downloads/firmware-changelog.html"

// ErrTooSoon is returned by Refresh when it was asked again too quickly.
var ErrTooSoon = errors.New("the changelog was checked a moment ago")

// Config for a Checker.
type Config struct {
	URL      string        // default DefaultURL
	Client   *http.Client  // default: 20s timeout
	Interval time.Duration // how often to look; default 12h
	// MinGap is the least time between fetches, however often Refresh is
	// called; default 30s.
	MinGap time.Duration
	// Enabled says whether looking is currently allowed (the setting can be
	// switched off while running). Nil means always.
	Enabled func() bool
	// OnChange is called after a fetch attempt, so status can be re-reported.
	OnChange func()
	Logger   *slog.Logger
	Now      func() time.Time
}

// Checker keeps a copy of the changelog fresh. It makes one small request to
// SiliconDust's website every Interval, with no information about you or your
// tuners in it, and none at all while turned off.
type Checker struct {
	cfg    Config
	client *http.Client
	log    *slog.Logger
	kick   chan struct{}

	mu        sync.Mutex
	releases  []Release
	etag      string
	modified  string
	checkedAt time.Time // last successful look (including "not modified")
	attempted time.Time // last attempt, successful or not
	lastErr   string
}

func NewChecker(cfg Config) *Checker {
	if cfg.URL == "" {
		cfg.URL = DefaultURL
	}
	if cfg.Interval == 0 {
		cfg.Interval = 12 * time.Hour
	}
	if cfg.MinGap == 0 {
		cfg.MinGap = 30 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	c := &Checker{cfg: cfg, client: cfg.Client, log: cfg.Logger, kick: make(chan struct{}, 1)}
	if c.client == nil {
		c.client = &http.Client{Timeout: 20 * time.Second}
	}
	return c
}

func (c *Checker) enabled() bool { return c.cfg.Enabled == nil || c.cfg.Enabled() }

// Run looks now and then every Interval until ctx ends. Kick makes it look
// early (for when the setting is switched on).
func (c *Checker) Run(ctx context.Context) {
	t := time.NewTicker(c.cfg.Interval)
	defer t.Stop()
	c.refreshIfEnabled(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.refreshIfEnabled(ctx)
		case <-c.kick:
			c.refreshIfEnabled(ctx)
		}
	}
}

// Kick asks Run to look soon. It never blocks.
func (c *Checker) Kick() {
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

func (c *Checker) refreshIfEnabled(ctx context.Context) {
	if !c.enabled() {
		return
	}
	if err := c.Refresh(ctx); err != nil && !errors.Is(err, ErrTooSoon) {
		c.log.Warn("firmware changelog check failed", "err", err)
	}
}

// Refresh fetches the changelog, unless that happened within MinGap. It uses
// a conditional request, so an unchanged page costs almost nothing.
func (c *Checker) Refresh(ctx context.Context) error {
	if !c.enabled() {
		return errors.New("the firmware check is turned off")
	}
	c.mu.Lock()
	if !c.attempted.IsZero() && c.cfg.Now().Sub(c.attempted) < c.cfg.MinGap {
		c.mu.Unlock()
		return ErrTooSoon
	}
	c.attempted = c.cfg.Now()
	etag, modified := c.etag, c.modified
	c.mu.Unlock()

	releases, newTag, newMod, notModified, err := c.fetch(ctx, etag, modified)

	c.mu.Lock()
	switch {
	case err != nil:
		c.lastErr = err.Error()
	case notModified:
		c.checkedAt, c.lastErr = c.cfg.Now(), ""
	default:
		c.releases, c.etag, c.modified = releases, newTag, newMod
		c.checkedAt, c.lastErr = c.cfg.Now(), ""
	}
	c.mu.Unlock()

	if c.cfg.OnChange != nil {
		c.cfg.OnChange()
	}
	return err
}

func (c *Checker) fetch(ctx context.Context, etag, modified string) (rs []Release, newTag, newMod string, notModified bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.URL, nil)
	if err != nil {
		return nil, "", "", false, err
	}
	req.Header.Set("User-Agent", "Antennarr (firmware check)")
	req.Header.Set("Accept", "text/html")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if modified != "" {
		req.Header.Set("If-Modified-Since", modified)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", "", false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		return nil, etag, modified, true, nil
	case http.StatusOK:
	default:
		return nil, "", "", false, fmt.Errorf("the changelog page answered %s", resp.Status)
	}
	rs, err = Parse(resp.Body)
	if err != nil {
		return nil, "", "", false, err
	}
	return rs, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), false, nil
}

// StatusFor says where a tuner stands. It never touches the network.
func (c *Checker) StatusFor(d Device) Status {
	if c == nil { // no checker configured: only the tuner's own report can say anything
		st := Evaluate(nil, d)
		if st.State == StateUnknown {
			st.Error = "the firmware check is not available"
		}
		return st
	}
	c.mu.Lock()
	releases, checked, lastErr := c.releases, c.checkedAt, c.lastErr
	c.mu.Unlock()

	st := Evaluate(releases, d)
	st.CheckedAt = checked
	switch {
	case !c.enabled() && st.State != StateUpdateAvailable:
		st.State, st.Error = StateDisabled, "the firmware check is turned off"
	case st.State == StateUnknown:
		switch {
		case lastErr != "":
			st.Error = lastErr
		case releases == nil:
			st.Error = "the changelog hasn't been fetched yet"
		case !versionRe.MatchString(d.Version):
			st.Error = "the tuner didn't report a firmware version"
		}
	case lastErr != "":
		// Still showing what we last read, but say it is getting stale.
		st.Error = "couldn't refresh the changelog: " + lastErr
	}
	return st
}

// CheckedAt is when the changelog was last read successfully.
func (c *Checker) CheckedAt() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.checkedAt
}
