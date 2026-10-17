// Package api is the JSON/SSE interface the web UI talks to. It is meant to be
// served behind access.Policy.AdminOnly: it can change who may use the tuner.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/johnschult/antennarr/internal/access"
	"github.com/johnschult/antennarr/internal/live"
	"github.com/johnschult/antennarr/internal/tuner"
)

const (
	maxBody     = 64 << 10 // settings are tiny; anything bigger is a mistake or an attack
	keepAlive   = 20 * time.Second
	coalesceFor = 100 * time.Millisecond // bursts of changes become one event
)

// API serves /antennarr/api/*.
type API struct {
	Mon   *tuner.Monitor
	Pol   *access.Policy
	Store *access.Store
	Hub   *live.Hub
	Log   *slog.Logger
	// KeepAlive is how often an idle event stream gets a comment line, so
	// proxies don't drop it. Zero means 20s.
	KeepAlive time.Duration
}

// Prefix is where the API is mounted.
const Prefix = "/antennarr/api/"

func (a *API) Handler() http.Handler {
	if a.Log == nil {
		a.Log = slog.Default()
	}
	mux := http.NewServeMux()
	routes := []struct {
		path string
		by   map[string]http.HandlerFunc
	}{
		{"state", map[string]http.HandlerFunc{"GET": a.getState}},
		{"events", map[string]http.HandlerFunc{"GET": a.events}},
		{"viewers", map[string]http.HandlerFunc{"GET": a.listViewers, "POST": a.addViewer}},
		{"viewers/{name}", map[string]http.HandlerFunc{"PUT": a.replaceViewer, "DELETE": a.deleteViewer}},
		{"settings", map[string]http.HandlerFunc{"GET": a.getSettings, "PATCH": a.patchSettings}},
	}
	for _, rt := range routes {
		var allow []string
		for method, h := range rt.by {
			mux.HandleFunc(method+" "+Prefix+rt.path, h)
			allow = append(allow, method)
		}
		sort.Strings(allow)
		// Any other method on a known path: a JSON 405 instead of the mux's text one.
		mux.HandleFunc(Prefix+rt.path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Allow", strings.Join(allow, ", "))
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		})
	}
	mux.HandleFunc(Prefix, func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "no such endpoint")
	})
	return guardWrites(mux)
}

// ---- the state document ----

// Summary is the tuner's capacity at a glance.
type Summary struct {
	Total    int `json:"total"`    // tuners on the device
	InUse    int `json:"inUse"`    // in use by anyone
	Remote   int `json:"remote"`   // of those, through Antennarr
	Local    int `json:"local"`    // of those, the owner's own devices
	Reserved int `json:"reserved"` // always kept free for the owner
	Free     int `json:"free"`
}

// SettingsDoc is the part of the settings that isn't the viewer list.
type SettingsDoc struct {
	Mode         string   `json:"mode"` // "open" or "enforced"
	SharedCap    int      `json:"sharedCap"`
	OwnerReserve int      `json:"ownerReserve"`
	Admin        []string `json:"admin"`
}

// State is everything the dashboard shows.
type State struct {
	Time     time.Time             `json:"time"`
	Tuner    tuner.State           `json:"tuner"`
	Streams  []access.Active       `json:"streams"`
	Viewers  []access.ViewerStatus `json:"viewers"`
	Settings SettingsDoc           `json:"settings"`
	Summary  Summary               `json:"summary"`
	Editable bool                  `json:"editable"` // false: started without -config
}

func (a *API) state() State {
	t := a.Mon.Snapshot()
	set := a.Pol.Settings()
	total, via := t.InUse()

	free := t.TunerCount - total
	if free < 0 {
		free = 0
	}
	st := State{
		Time:    time.Now(),
		Tuner:   t,
		Streams: a.Pol.Active(),
		Viewers: a.Pol.ViewerStatuses(),
		Settings: SettingsDoc{
			Mode: a.Pol.Mode(), SharedCap: set.SharedCap, OwnerReserve: set.OwnerReserve, Admin: nonNil(set.Admin),
		},
		Summary:  Summary{Total: t.TunerCount, InUse: total, Remote: via, Local: total - via, Reserved: set.OwnerReserve, Free: free},
		Editable: a.Store.Editable(),
	}
	if st.Tuner.Slots == nil {
		st.Tuner.Slots = []tuner.Slot{}
	}
	if st.Tuner.Events == nil {
		st.Tuner.Events = []tuner.Event{}
	}
	return st
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (a *API) getState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.state())
}

// ---- live updates ----

// events streams the state as server-sent events: one right away, then one
// after each change, plus a comment every 20s so proxies keep the line open.
func (a *API) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming is not supported here")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // don't let nginx buffer the stream
	w.WriteHeader(http.StatusOK)

	send := func() error {
		b, err := json.Marshal(a.state())
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", b); err != nil {
			return err
		}
		fl.Flush()
		return nil
	}

	changes, cancel := a.Hub.Subscribe()
	defer cancel()
	if send() != nil {
		return
	}
	every := a.KeepAlive
	if every == 0 {
		every = keepAlive
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-changes:
			// Let a burst of changes settle, then send once.
			select {
			case <-time.After(coalesceFor):
			case <-r.Context().Done():
				return
			}
			select { // drop the signal(s) that arrived while we waited
			case <-changes:
			default:
			}
			if send() != nil {
				return
			}
		case <-tick.C:
			if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

// ---- viewers ----

type viewerInput struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	Limit     int      `json:"limit"`
}

func (v viewerInput) viewer() access.Viewer {
	return access.Viewer{Name: strings.TrimSpace(v.Name), Addresses: nonNil(v.Addresses), Limit: v.Limit}
}

func (a *API) listViewers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.Pol.ViewerStatuses())
}

func (a *API) viewerStatus(name string) (access.ViewerStatus, bool) {
	for _, v := range a.Pol.ViewerStatuses() {
		if strings.EqualFold(v.Name, name) {
			return v, true
		}
	}
	return access.ViewerStatus{}, false
}

func indexOf(vs []access.Viewer, name string) int {
	for i, v := range vs {
		if strings.EqualFold(v.Name, strings.TrimSpace(name)) {
			return i
		}
	}
	return -1
}

func (a *API) addViewer(w http.ResponseWriter, r *http.Request) {
	var in viewerInput
	if !decode(w, r, &in) {
		return
	}
	v := in.viewer()
	_, err := a.Store.Edit(func(s *access.Settings) error {
		if indexOf(s.Viewers, v.Name) >= 0 {
			return &httpErr{http.StatusConflict, fmt.Sprintf("viewer %q already exists", v.Name)}
		}
		s.Viewers = append(s.Viewers, v)
		return nil
	})
	if err != nil {
		a.fail(w, err)
		return
	}
	vs, _ := a.viewerStatus(v.Name)
	writeJSON(w, http.StatusCreated, vs)
}

func (a *API) replaceViewer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var in viewerInput
	if !decode(w, r, &in) {
		return
	}
	v := in.viewer()
	_, err := a.Store.Edit(func(s *access.Settings) error {
		i := indexOf(s.Viewers, name)
		if i < 0 {
			return &httpErr{http.StatusNotFound, fmt.Sprintf("no viewer named %q", name)}
		}
		s.Viewers[i] = v
		return nil
	})
	if err != nil {
		a.fail(w, err)
		return
	}
	vs, _ := a.viewerStatus(v.Name)
	writeJSON(w, http.StatusOK, vs)
}

func (a *API) deleteViewer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	_, err := a.Store.Edit(func(s *access.Settings) error {
		i := indexOf(s.Viewers, name)
		if i < 0 {
			return &httpErr{http.StatusNotFound, fmt.Sprintf("no viewer named %q", name)}
		}
		// With no viewers the tuner is open to everyone, so removing the last
		// one needs an explicit say-so.
		if len(s.Viewers) == 1 && r.URL.Query().Get("allowOpen") != "true" {
			return &httpErr{http.StatusConflict, "removing the last viewer would open the tuner to everyone; repeat with ?allowOpen=true to confirm"}
		}
		s.Viewers = append(s.Viewers[:i:i], s.Viewers[i+1:]...)
		return nil
	})
	if err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- settings ----

func (a *API) settingsDoc() SettingsDoc { return a.state().Settings }

func (a *API) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.settingsDoc())
}

// settingsPatch changes only the fields that are present.
type settingsPatch struct {
	SharedCap    *int      `json:"sharedCap"`
	OwnerReserve *int      `json:"ownerReserve"`
	Admin        *[]string `json:"admin"`
}

func (a *API) patchSettings(w http.ResponseWriter, r *http.Request) {
	var in settingsPatch
	if !decode(w, r, &in) {
		return
	}
	_, err := a.Store.Edit(func(s *access.Settings) error {
		if in.SharedCap != nil {
			s.SharedCap = *in.SharedCap
		}
		if in.OwnerReserve != nil {
			s.OwnerReserve = *in.OwnerReserve
		}
		if in.Admin != nil {
			s.Admin = *in.Admin
		}
		return nil
	})
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.settingsDoc())
}

// ---- plumbing ----

type httpErr struct {
	code int
	msg  string
}

func (e *httpErr) Error() string { return e.msg }

func (a *API) fail(w http.ResponseWriter, err error) {
	var he *httpErr
	var ve *access.ValidationError
	switch {
	case errors.As(err, &he):
		writeErr(w, he.code, he.msg)
	case errors.As(err, &ve):
		writeErr(w, http.StatusBadRequest, ve.Error())
	case errors.Is(err, access.ErrNoFile):
		writeErr(w, http.StatusConflict, err.Error())
	default:
		a.Log.Error("settings edit failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "could not save the settings")
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// decode reads a JSON body strictly. It writes the error response itself and
// returns false if the body is unusable.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		}
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, "invalid JSON: unexpected data after the object")
		return false
	}
	return true
}

// guardWrites is the CSRF defence for an API that trusts the caller's
// address: a web page the owner happens to visit must not be able to change
// settings from their browser. Writes have to be JSON (which a cross-origin
// page can't send without a CORS preflight that we never approve), and an
// Origin header, if present, has to be this very host.
func guardWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "" {
				if u, err := url.Parse(origin); err != nil || u.Host != r.Host {
					writeErr(w, http.StatusForbidden, "cross-origin requests are not allowed")
					return
				}
			}
			// Bodyless writes (DELETE) have no type to check.
			if r.ContentLength != 0 {
				mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil || mt != "application/json" {
					writeErr(w, http.StatusUnsupportedMediaType, "send application/json")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
