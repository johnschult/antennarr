// Package proxy fronts one HDHomeRun tuner and rewrites its HTTP API so that
// remote media servers only ever talk to Yagi, never to the tuner's LAN IP.
package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// Config describes the tuner behind the proxy.
type Config struct {
	// API is the tuner's HTTP API root (port 80), e.g. http://192.168.1.132.
	API *url.URL
	// Stream is where /auto/... and /tunerN/... streams live (port 5004).
	Stream *url.URL
	// Upstream, if set, replaces API and Stream and may move over time (see
	// the tuner package, which follows the tuner by DeviceID).
	Upstream Upstream
	// Policy, if set, gates every request and meters streams.
	Policy Policy
	// Advertise is the base URL handed to clients, e.g. http://host:5004.
	// Empty means "whatever host the client connected with".
	Advertise string
	// DeviceID, if set, replaces the tuner's DeviceID in discover.json. Only
	// useful when a media server already knows the real tuner (e.g. testing
	// against your own), since it dedupes devices by ID.
	DeviceID string
	// Observer, if set, is told what the tuner answered to a stream or lineup request, so the owner can be shown
	// the trouble a media server ran into. Its methods must not block.
	Observer Observer
	Logger   *slog.Logger
}

// Observer hears what the tuner answers to the requests Yagi passes on.
type Observer interface {
	// StreamRefused is called when the tuner answers a stream request with something other than 200: the status
	// line and, if the tuner gave one, its reason (its X-HDHomeRun-Error header).
	StreamRefused(status, reason string)
	// Lineup is called with the number of channels the tuner returned.
	Lineup(channels int)
}

// Proxy is an http.Handler.
type Proxy struct {
	cfg    Config
	up     Upstream
	client *http.Client
	api    *httputil.ReverseProxy
	stream *httputil.ReverseProxy
	log    *slog.Logger

	// emptyLineupLogged is when "tuner has no channels" was last logged (Unix nanoseconds). Plex asks for the
	// lineup often, and a tuner that stays empty would otherwise fill the log with the same line.
	emptyLineupLogged atomic.Int64
}

func New(cfg Config) *Proxy {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	// Streams are raw MPEG-TS that run for hours: no compression, no response
	// timeout, and flush every write straight through.
	transport := &http.Transport{
		DisableCompression:    true,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConnsPerHost:   4,
	}
	p := &Proxy{
		cfg:    cfg,
		client: &http.Client{Transport: transport, Timeout: 15 * time.Second},
		log:    cfg.Logger,
	}
	if cfg.Upstream == nil {
		cfg.Upstream = staticUpstream{cfg.API, cfg.Stream}
	}
	p.up = cfg.Upstream
	p.api = p.reverse(func() *url.URL { a, _ := p.up.Targets(); return a }, transport)
	p.stream = p.reverse(func() *url.URL { _, s := p.up.Targets(); return s }, transport)
	p.stream.ModifyResponse = p.logTunerRefusal
	return p
}

// logTunerRefusal says so when the tuner turns a stream request away (all its
// tuners busy, no signal lock, ...). The answer goes on to the viewer as it is,
// so without this the log shows the same as for a stream that worked.
func (p *Proxy) logTunerRefusal(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	attrs := []any{"path", resp.Request.URL.Path, "status", resp.Status}
	// The tuner explains itself in a header of its own (X-HDHomeRun-Error); log whatever it sends.
	for k, v := range resp.Header {
		if strings.HasPrefix(k, "X-Hdhomerun") {
			attrs = append(attrs, strings.ToLower(k), strings.Join(v, ", "))
		}
	}
	p.log.Warn("tuner refused stream", attrs...)
	if p.cfg.Observer != nil {
		p.cfg.Observer.StreamRefused(resp.Status, strings.Join(resp.Header.Values("X-HDHomeRun-Error"), ", "))
	}
	return nil
}

// Upstream says where the tuner is right now; it can change while we run.
type Upstream interface {
	Targets() (api, stream *url.URL)
}

type staticUpstream struct{ api, stream *url.URL }

func (s staticUpstream) Targets() (*url.URL, *url.URL) { return s.api, s.stream }

func (p *Proxy) reverse(target func() *url.URL, rt http.RoundTripper) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			t := target()
			if t == nil {
				t = &url.URL{Scheme: "http", Host: "invalid.invalid"} // fails fast into the 502 handler
			}
			r.SetURL(t)
			r.Out.Host = t.Host
			r.Out.Header.Del("X-Forwarded-For")
		},
		Transport:     rt,
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// A canceled request is just the viewer going away; the tuner
			// connection has already been closed with it.
			if r.Context().Err() != nil {
				return
			}
			p.log.Warn("upstream error", "path", r.URL.Path, "err", err)
			http.Error(w, "tuner unreachable", http.StatusBadGateway)
		},
	}
}

// Policy decides who may use the tuner. The proxy asks it before doing
// anything, so a nil Policy means open access.
type Policy interface {
	// Authorize is called for every request; an error refuses it.
	Authorize(r *http.Request) error
	// Stream is called before a stream is opened. It reserves a tuner for the
	// viewer and returns the func to call when the stream ends.
	Stream(r *http.Request) (release func(), err error)
	// TunerCount is the number of tuners to tell this viewer about.
	TunerCount(r *http.Request, real int) int
}

// Denied is a refusal from a Policy.
type Denied struct {
	Code   int    // HTTP status: 403 not allowed, 503 no tuner for you right now
	Reason string // short machine-readable cause, sent as X-Yagi-Reason
	Text   string // for the viewer
}

func (d *Denied) Error() string { return d.Text }

func (p *Proxy) deny(w http.ResponseWriter, err error) {
	var d *Denied
	if !errors.As(err, &d) {
		d = &Denied{Code: http.StatusForbidden, Reason: "denied", Text: err.Error()}
	}
	w.Header().Set("X-Yagi-Reason", d.Reason)
	http.Error(w, d.Text, d.Code)
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !isStream(r.URL.Path) {
		// Media servers ask for discover.json and lineup.json over and over; that is detail for debugging, not news.
		// What matters is logged above debug: a stream opening and closing, and anything refused.
		p.log.Debug("request", "method", r.Method, "path", r.URL.RequestURI(), "client", r.RemoteAddr, "ua", r.UserAgent())
	}
	if p.cfg.Policy != nil {
		if err := p.cfg.Policy.Authorize(r); err != nil {
			p.log.Warn("refused", "path", r.URL.Path, "client", r.RemoteAddr, "why", err)
			p.deny(w, err)
			return
		}
	}
	switch {
	case r.URL.Path == "/discover.json":
		if !readOnly(w, r) {
			return
		}
		p.discover(w, r)
	case r.URL.Path == "/lineup.json":
		if !readOnly(w, r) {
			return
		}
		p.lineup(w, r)
	case isStream(r.URL.Path):
		// A stream is opened by GET only; anything else would reach the tuner
		// without being counted against the viewer's limit.
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if p.cfg.Policy != nil {
			release, err := p.cfg.Policy.Stream(r)
			if err != nil {
				p.log.Warn("stream refused", "path", r.URL.Path, "client", r.RemoteAddr, "why", err)
				p.deny(w, err)
				return
			}
			defer release() // runs even when ReverseProxy aborts the handler
		}
		p.log.Info("stream open", "path", r.URL.Path, "client", r.RemoteAddr)
		start := time.Now()
		// ReverseProxy panics with http.ErrAbortHandler when the viewer drops
		// mid-stream, so the close has to be logged from a defer.
		defer func() {
			p.log.Info("stream closed", "path", r.URL.Path, "client", r.RemoteAddr, "after", time.Since(start).Round(time.Second))
		}()
		p.stream.ServeHTTP(w, r)
	case passthrough[r.URL.Path]:
		if !readOnly(w, r) {
			return
		}
		p.api.ServeHTTP(w, r)
	default:
		// Everything else the tuner serves (rescans, firmware upgrade, its web
		// pages, ...) is for its owner, not for the people it is shared with.
		// Say so in the log: a media server that really needs another path
		// shows up here, and the path can be added to passthrough.
		p.log.Warn("path not served", "method", r.Method, "path", r.URL.RequestURI(), "client", r.RemoteAddr, "ua", r.UserAgent())
		http.NotFound(w, r)
	}
}

// passthrough lists the tuner endpoints, besides the ones Yagi rewrites
// itself (discover.json, lineup.json) and the streams, that are handed to the
// tuner as they are. They are read-only status pages. Anything not listed here
// is refused with a 404, so a viewer can't reach the tuner's management API.
var passthrough = map[string]bool{
	"/status.json":        true,
	"/lineup_status.json": true,
}

// readOnly lets GET and HEAD through and answers anything else with a 405.
func readOnly(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

// isStream reports whether path asks for a live stream: /auto/<channel> or
// /tunerN/<channel>. It is deliberately narrower than "starts with /tuner",
// which would also take the tuner's own pages (/tuners.html) for a stream.
func isStream(path string) bool {
	rest, ok := strings.CutPrefix(path, "/auto/")
	if !ok {
		rest, ok = strings.CutPrefix(path, "/tuner")
		if !ok {
			return false
		}
		i := strings.IndexByte(rest, '/')
		if i < 1 {
			return false
		}
		for _, c := range rest[:i] {
			if c < '0' || c > '9' {
				return false
			}
		}
		rest = rest[i+1:]
	}
	return rest != "" && !strings.Contains(rest, "/")
}

// base is the address clients should use to reach us.
func (p *Proxy) base(r *http.Request) string {
	if p.cfg.Advertise != "" {
		return strings.TrimRight(p.cfg.Advertise, "/")
	}
	return "http://" + r.Host
}

func (p *Proxy) fetchJSON(r *http.Request, path string, v any) error {
	api, _ := p.up.Targets()
	if api == nil {
		return errors.New("tuner address unknown")
	}
	u := *api
	u.Path = path
	u.RawQuery = r.URL.RawQuery
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tuner returned %s for %s", resp.Status, path)
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber() // keep numbers byte-for-byte so we only change what we mean to
	return dec.Decode(v)
}

func writeJSON(w http.ResponseWriter, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.Copy(w, &buf)
}

func (p *Proxy) discover(w http.ResponseWriter, r *http.Request) {
	var d map[string]any
	if err := p.fetchJSON(r, "/discover.json", &d); err != nil {
		p.log.Warn("discover failed", "err", err)
		http.Error(w, "tuner unreachable", http.StatusBadGateway)
		return
	}
	base := p.base(r)
	d["BaseURL"] = base
	d["LineupURL"] = base + "/lineup.json"
	if p.cfg.DeviceID != "" {
		d["DeviceID"] = p.cfg.DeviceID
	}
	if p.cfg.Policy != nil {
		if n, ok := d["TunerCount"].(json.Number); ok {
			if real, err := n.Int64(); err == nil {
				d["TunerCount"] = p.cfg.Policy.TunerCount(r, int(real))
			}
		}
	}
	writeJSON(w, d)
}

func (p *Proxy) lineup(w http.ResponseWriter, r *http.Request) {
	var channels []map[string]any
	if err := p.fetchJSON(r, "/lineup.json", &channels); err != nil {
		p.log.Warn("lineup failed", "err", err)
		http.Error(w, "tuner unreachable", http.StatusBadGateway)
		return
	}
	if len(channels) == 0 {
		// Plex then shows an empty guide, and the log would show an ordinary request.
		now := time.Now().UnixNano()
		if last := p.emptyLineupLogged.Load(); now-last >= int64(time.Minute) && p.emptyLineupLogged.CompareAndSwap(last, now) {
			p.log.Warn("tuner has no channels", "client", r.RemoteAddr)
		}
	}
	p.log.Debug("lineup", "channels", len(channels), "client", r.RemoteAddr)
	if p.cfg.Observer != nil {
		p.cfg.Observer.Lineup(len(channels))
	}
	base := p.base(r)
	for _, ch := range channels {
		raw, ok := ch["URL"].(string)
		if !ok {
			continue
		}
		ch["URL"] = rebase(raw, base)
	}
	writeJSON(w, channels)
}

// rebase swaps the scheme and host of raw for those of base, keeping path and
// query. If raw doesn't parse it is returned untouched.
func rebase(raw, base string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	b, err := url.Parse(base)
	if err != nil {
		return raw
	}
	u.Scheme, u.Host = b.Scheme, b.Host
	return u.String()
}
