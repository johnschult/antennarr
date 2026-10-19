// Package linetest is a fake tuner's channel lineup, for tests and the demo: it serves lineup.json and
// lineup_status.json and takes lineup.post as the real tuner's page does, and records every request.
package linetest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// Channel is a channel the fake tuner has found.
type Channel struct {
	Number       string
	Name         string
	HD           bool
	State        string // "enabled" (the zero value), "favorite" or "disabled"
	Unsubscribed bool
}

// Server is the fake tuner.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	channels []*Channel
	sources  []string
	source   string
	scanning bool
	started  time.Time
	scanFor  time.Duration
	requests []string
	// IgnoreChanges makes the tuner accept a change and do nothing, as a tuner that didn't take it.
	IgnoreChanges bool
	// CannotScan says scanning isn't possible right now.
	CannotScan bool
}

// New starts a fake tuner with channels, sources "Antenna" and "Cable", and a scan that takes scanFor.
func New(channels []Channel, scanFor time.Duration) *Server {
	s := &Server{sources: []string{"Antenna", "Cable"}, source: "Antenna", scanFor: scanFor}
	for i := range channels {
		c := channels[i]
		s.channels = append(s.channels, &c)
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// Handler is the fake tuner's web side, for mounting in another server (the demo's).
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.handle) }

// Requests is "METHOD path?query" for every request received, in order.
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

// Forget clears the record.
func (s *Server) Forget() {
	s.mu.Lock()
	s.requests = nil
	s.mu.Unlock()
}

// State is what a channel is set to now ("" if the tuner hasn't got it).
func (s *Server) State(number string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.channels {
		if c.Number == number {
			if c.State == "" {
				return "enabled"
			}
			return c.State
		}
	}
	return ""
}

// Scanning says whether a scan is running.
func (s *Server) Scanning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tick()
	return s.scanning
}

func (s *Server) tick() {
	if s.scanning && time.Since(s.started) >= s.scanFor {
		s.scanning = false
	}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, r.Method+" "+r.URL.RequestURI())
	s.tick()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/lineup.json":
		var out []map[string]any
		for _, c := range s.channels {
			if r.URL.Query().Get("show") != "found" && c.State == "disabled" {
				continue
			}
			m := map[string]any{"GuideNumber": c.Number, "GuideName": c.Name, "VideoCodec": "MPEG2", "AudioCodec": "AC3", "URL": "http://tuner/auto/v" + c.Number}
			if c.HD {
				m["HD"] = 1
			}
			if r.URL.Query().Get("show") == "found" {
				if c.State == "disabled" {
					m["Enabled"] = 0
				}
				if c.State == "favorite" {
					m["Favorite"] = 1
				}
				if c.Unsubscribed {
					m["Subscribed"] = 0
				}
			}
			out = append(out, m)
		}
		json.NewEncoder(w).Encode(out)
	case r.Method == http.MethodGet && r.URL.Path == "/lineup_status.json":
		st := map[string]any{"ScanInProgress": b2i(s.scanning), "ScanPossible": b2i(!s.CannotScan && !s.scanning), "Source": s.source, "SourceList": s.sources}
		if s.scanning {
			st["Progress"] = int(100 * time.Since(s.started) / s.scanFor)
			st["Found"] = 12
		}
		json.NewEncoder(w).Encode(st)
	case r.Method == http.MethodPost && r.URL.Path == "/lineup.post":
		q := r.URL.Query()
		switch {
		case q.Get("favorite") != "":
			v := q.Get("favorite")
			if len(v) < 2 || !strings.ContainsAny(v[:1], "+-x") {
				http.Error(w, "bad", http.StatusBadRequest)
				return
			}
			for _, c := range s.channels {
				if c.Number == v[1:] && !s.IgnoreChanges {
					c.State = map[string]string{"+": "favorite", "-": "enabled", "x": "disabled"}[v[:1]]
				}
			}
		case q.Get("scan") == "start":
			if s.CannotScan || s.scanning {
				http.Error(w, "no", http.StatusConflict)
				return
			}
			if src := q.Get("source"); src != "" {
				s.source = src
			}
			s.scanning, s.started = true, time.Now()
		case q.Get("scan") == "abort":
			s.scanning = false
		default:
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
