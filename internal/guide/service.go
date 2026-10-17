package guide

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultURL is SiliconDust's guide service; it is given a tuner's DeviceAuth.
const DefaultURL = "https://api.hdhomerun.com/api/xmltv"

// IconHost is the only host channel logos are fetched from. The logo address
// comes from the guide, which is outside data, so it is never fetched blindly.
const IconHost = "img.hdhomerun.com"

const (
	maxGuide = 32 << 20  // the real guide is about 3.5 MB
	maxIcon  = 512 << 10 // logos are a few KB
)

// Config for a Service.
type Config struct {
	URL      string        // default DefaultURL; tests point it at a fake
	Client   *http.Client  // default: 60s timeout
	Interval time.Duration // how often to look; default 6h
	MinGap   time.Duration // least time between fetches; default 30s
	// Enabled says whether looking is allowed right now (the setting can be
	// switched off while running). Nil means always.
	Enabled func() bool
	// OnChange is called after a fetch attempt, so the dashboards can update.
	OnChange func()
	Logger   *slog.Logger
	Now      func() time.Time
	// IconBase replaces https://IconHost when fetching logos; for tests.
	IconBase string
}

// Service keeps each tuner's guide fresh. It sends the tuner's DeviceAuth to
// SiliconDust, which is how their guide knows which channels to list, and
// nothing at all while turned off. Tuners that share a DeviceAuth share a fetch.
type Service struct {
	cfg    Config
	client *http.Client
	log    *slog.Logger
	kick   chan struct{}

	mu      sync.Mutex
	devices map[string]func() string // device -> its current DeviceAuth
	feeds   map[string]*feed         // by DeviceAuth
	icons   map[string]icon          // by icon URL
}

type feed struct {
	channels  map[string]*Channel
	etag      string
	modified  string
	checkedAt time.Time
	attempted time.Time
	lastErr   string
}

type icon struct {
	body []byte
	typ  string
}

func NewService(cfg Config) *Service {
	if cfg.URL == "" {
		cfg.URL = DefaultURL
	}
	if cfg.Interval == 0 {
		cfg.Interval = 6 * time.Hour
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
	if cfg.IconBase == "" {
		cfg.IconBase = "https://" + IconHost
	}
	s := &Service{
		cfg: cfg, client: cfg.Client, log: cfg.Logger, kick: make(chan struct{}, 1),
		devices: map[string]func() string{}, feeds: map[string]*feed{}, icons: map[string]icon{},
	}
	if s.client == nil {
		s.client = &http.Client{Timeout: 60 * time.Second}
	}
	return s
}

func (s *Service) enabled() bool { return s.cfg.Enabled == nil || s.cfg.Enabled() }

// Watch tells the service about a device and how to ask for its DeviceAuth.
func (s *Service) Watch(device string, auth func() string) {
	s.mu.Lock()
	s.devices[device] = auth
	s.mu.Unlock()
}

// Run looks now and then every Interval until ctx ends. Kick makes it look
// early, but only for a guide that has never been read or has gone stale (for
// when the setting is switched on, or a tuner has just appeared).
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(s.cfg.Interval)
	defer t.Stop()
	s.refreshIfEnabled(ctx, false)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.refreshIfEnabled(ctx, false)
		case <-s.kick:
			s.refreshIfEnabled(ctx, true)
		}
	}
}

// Kick asks Run to look soon. It never blocks.
func (s *Service) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

func (s *Service) refreshIfEnabled(ctx context.Context, onlyStale bool) {
	if s.enabled() {
		s.refresh(ctx, onlyStale)
	}
}

// Refresh fetches the guide for every distinct DeviceAuth that hasn't been
// fetched within MinGap. A guide that is already held is kept if a fetch fails.
func (s *Service) Refresh(ctx context.Context) { s.refreshIfEnabled(ctx, false) }

func (s *Service) refresh(ctx context.Context, onlyStale bool) {
	s.mu.Lock()
	auths := map[string]bool{}
	for _, get := range s.devices {
		if a := get(); a != "" {
			auths[a] = true
		}
	}
	s.mu.Unlock()

	changed := false
	for auth := range auths {
		if onlyStale && !s.stale(auth) {
			continue
		}
		changed = s.refreshOne(ctx, auth) || changed
	}
	if changed && s.cfg.OnChange != nil {
		s.cfg.OnChange()
	}
}

// stale is true for a guide never read, or not read within an Interval.
func (s *Service) stale(auth string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.feeds[auth]
	return f == nil || f.checkedAt.IsZero() || s.cfg.Now().Sub(f.checkedAt) >= s.cfg.Interval
}

func (s *Service) refreshOne(ctx context.Context, auth string) bool {
	s.mu.Lock()
	f := s.feeds[auth]
	if f == nil {
		f = &feed{}
		s.feeds[auth] = f
	}
	if !f.attempted.IsZero() && s.cfg.Now().Sub(f.attempted) < s.cfg.MinGap {
		s.mu.Unlock()
		return false
	}
	f.attempted = s.cfg.Now()
	etag, modified := f.etag, f.modified
	s.mu.Unlock()

	chans, newTag, newMod, notModified, err := s.fetch(ctx, auth, etag, modified)

	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case err != nil:
		f.lastErr = err.Error()
		s.log.Warn("guide fetch failed", "err", err) // err never contains the DeviceAuth: see fetch
	case notModified:
		f.checkedAt, f.lastErr = s.cfg.Now(), ""
	default:
		f.channels, f.etag, f.modified = chans, newTag, newMod
		f.checkedAt, f.lastErr = s.cfg.Now(), ""
	}
	return err == nil && !notModified
}

func (s *Service) fetch(ctx context.Context, auth, etag, modified string) (chans map[string]*Channel, newTag, newMod string, notModified bool, err error) {
	u, err := url.Parse(s.cfg.URL)
	if err != nil {
		return nil, "", "", false, errors.New("the guide address is not valid")
	}
	q := u.Query()
	q.Set("DeviceAuth", auth)
	u.RawQuery = q.Encode()

	// Errors from here on must not carry u: it holds the DeviceAuth.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", "", false, errors.New("could not build the guide request")
	}
	req.Header.Set("User-Agent", "Antennarr (guide)")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if modified != "" {
		req.Header.Set("If-Modified-Since", modified)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // drop the URL, which holds the DeviceAuth
		}
		return nil, "", "", false, fmt.Errorf("couldn't reach the guide service: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		return nil, etag, modified, true, nil
	case http.StatusOK:
	default:
		return nil, "", "", false, fmt.Errorf("the guide service answered %s", resp.Status)
	}
	lr := &io.LimitedReader{R: resp.Body, N: maxGuide + 1}
	chans, err = Parse(lr, s.cfg.Now())
	if lr.N <= 0 {
		return nil, "", "", false, errors.New("the guide is larger than expected, so it was ignored")
	}
	if err != nil {
		return nil, "", "", false, err
	}
	return chans, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), false, nil
}

// Entry is what a tile shows for a channel.
type Entry struct {
	Name string     `json:"name,omitempty"`
	Icon bool       `json:"icon"` // whether a logo can be fetched with Icon
	Now  *Programme `json:"now,omitempty"`
}

// Lookup says what the guide knows about a channel on a device right now.
func (s *Service) Lookup(device, channel string, at time.Time) (Entry, bool) {
	ch := s.channel(device, channel)
	if ch == nil {
		return Entry{}, false
	}
	return Entry{Name: ch.Name, Icon: validIcon(ch.Icon, s.cfg.IconBase), Now: ch.Now(at)}, true
}

func (s *Service) channel(device, channel string) *Channel {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	get := s.devices[device]
	if get == nil {
		return nil
	}
	f := s.feeds[get()]
	if f == nil {
		return nil
	}
	return f.channels[channel]
}

// validIcon accepts a logo address only on the expected host, over https.
func validIcon(raw, base string) bool {
	u, err := url.Parse(raw)
	b, err2 := url.Parse(base)
	if err != nil || err2 != nil {
		return false
	}
	return u.Scheme == b.Scheme && u.Host == b.Host && strings.HasPrefix(u.Path, "/")
}

// ErrNoIcon means the guide has no logo for that channel.
var ErrNoIcon = errors.New("no logo for that channel")

// Icon returns the channel's logo, fetched from SiliconDust once and kept.
// The address always comes from the guide, never from the request.
func (s *Service) Icon(ctx context.Context, device, channel string) (body []byte, contentType string, err error) {
	ch := s.channel(device, channel)
	if ch == nil || !validIcon(ch.Icon, s.cfg.IconBase) {
		return nil, "", ErrNoIcon
	}
	s.mu.Lock()
	if ic, ok := s.icons[ch.Icon]; ok {
		s.mu.Unlock()
		return ic.body, ic.typ, nil
	}
	s.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ch.Icon, nil)
	if err != nil {
		return nil, "", ErrNoIcon
	}
	req.Header.Set("User-Agent", "Antennarr (guide)")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("couldn't fetch the logo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("the logo request answered %s", resp.Status)
	}
	typ := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(typ, "image/") || strings.Contains(typ, "svg") { // svg can carry script
		return nil, "", errors.New("that isn't an image")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxIcon+1))
	if err != nil || len(b) > maxIcon {
		return nil, "", errors.New("the logo is too large")
	}
	s.mu.Lock()
	if len(s.icons) > 400 { // a few hundred channels at most; never grow without bound
		s.icons = map[string]icon{}
	}
	s.icons[ch.Icon] = icon{b, typ}
	s.mu.Unlock()
	return b, typ, nil
}
