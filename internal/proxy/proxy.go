// Package proxy fronts one HDHomeRun tuner and rewrites its HTTP API so that
// remote media servers only ever talk to Antennarr, never to the tuner's LAN IP.
package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// Config describes the tuner behind the proxy.
type Config struct {
	// API is the tuner's HTTP API root (port 80), e.g. http://192.168.1.132.
	API *url.URL
	// Stream is where /auto/... and /tunerN/... streams live (port 5004).
	Stream *url.URL
	// Advertise is the base URL handed to clients, e.g. http://host:5004.
	// Empty means "whatever host the client connected with".
	Advertise string
	// DeviceID, if set, replaces the tuner's DeviceID in discover.json. Only
	// useful when a media server already knows the real tuner (e.g. testing
	// against your own), since it dedupes devices by ID.
	DeviceID string
	Logger   *slog.Logger
}

// Proxy is an http.Handler.
type Proxy struct {
	cfg    Config
	client *http.Client
	api    *httputil.ReverseProxy
	stream *httputil.ReverseProxy
	log    *slog.Logger
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
	p.api = p.reverse(cfg.API, transport)
	p.stream = p.reverse(cfg.Stream, transport)
	return p
}

func (p *Proxy) reverse(target *url.URL, rt http.RoundTripper) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Host = target.Host
			r.Out.Header.Del("X-Forwarded-For")
		},
		Transport:     rt,
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// A cancelled request is just the viewer going away; the tuner
			// connection has already been closed with it.
			if r.Context().Err() != nil {
				return
			}
			p.log.Warn("upstream error", "path", r.URL.Path, "err", err)
			http.Error(w, "tuner unreachable", http.StatusBadGateway)
		},
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !isStream(r.URL.Path) {
		p.log.Info("request", "method", r.Method, "path", r.URL.RequestURI(), "client", r.RemoteAddr, "ua", r.UserAgent())
	}
	switch {
	case r.URL.Path == "/discover.json":
		p.discover(w, r)
	case r.URL.Path == "/lineup.json":
		p.lineup(w, r)
	case isStream(r.URL.Path):
		p.log.Info("stream open", "path", r.URL.Path, "client", r.RemoteAddr)
		start := time.Now()
		// ReverseProxy panics with http.ErrAbortHandler when the viewer drops
		// mid-stream, so the close has to be logged from a defer.
		defer func() {
			p.log.Info("stream closed", "path", r.URL.Path, "client", r.RemoteAddr, "after", time.Since(start).Round(time.Second))
		}()
		p.stream.ServeHTTP(w, r)
	default:
		p.api.ServeHTTP(w, r)
	}
}

func isStream(path string) bool {
	return strings.HasPrefix(path, "/auto/") || strings.HasPrefix(path, "/tuner")
}

// base is the address clients should use to reach us.
func (p *Proxy) base(r *http.Request) string {
	if p.cfg.Advertise != "" {
		return strings.TrimRight(p.cfg.Advertise, "/")
	}
	return "http://" + r.Host
}

func (p *Proxy) fetchJSON(r *http.Request, path string, v any) error {
	u := *p.cfg.API
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
	writeJSON(w, d)
}

func (p *Proxy) lineup(w http.ResponseWriter, r *http.Request) {
	var channels []map[string]any
	if err := p.fetchJSON(r, "/lineup.json", &channels); err != nil {
		p.log.Warn("lineup failed", "err", err)
		http.Error(w, "tuner unreachable", http.StatusBadGateway)
		return
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
