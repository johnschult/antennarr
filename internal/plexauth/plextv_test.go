package plexauth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePlexTV stands in for plex.tv. It checks the headers Antennarr must send
// and lets a test play the part of the person signing in.
type fakePlexTV struct {
	*httptest.Server
	mu       sync.Mutex
	nextID   int64
	pins     map[int64]*fakePin
	users    map[string]fakeUser // by token
	clientID string              // every request must carry the same one
	calls    map[string]int
	fail     bool // answer 500 to everything
	seenIDs  map[string]bool
}

type fakePin struct {
	code  string
	token string // set when the person "signs in"
	gone  bool
}

type fakeUser struct {
	ID              int64
	Username, Title string
}

func newFakePlexTV(t *testing.T) *fakePlexTV {
	t.Helper()
	f := &fakePlexTV{nextID: 1000, pins: map[int64]*fakePin{}, users: map[string]fakeUser{}, calls: map[string]int{}, seenIDs: map[string]bool{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *fakePlexTV) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[r.Method+" "+r.URL.Path]++
	f.seenIDs[r.Header.Get("X-Plex-Client-Identifier")] = true
	if r.Header.Get("Accept") != "application/json" || r.Header.Get("X-Plex-Product") != "Antennarr" || r.Header.Get("X-Plex-Client-Identifier") == "" {
		http.Error(w, "missing headers", http.StatusBadRequest)
		return
	}
	if f.fail {
		http.Error(w, "boom", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == "POST" && r.URL.Path == "/api/v2/pins":
		if r.URL.Query().Get("strong") != "true" {
			http.Error(w, "want a strong pin", http.StatusBadRequest)
			return
		}
		f.nextID++
		p := &fakePin{code: fmt.Sprintf("code%d-%s", f.nextID, strings.Repeat("x", 18))}
		f.pins[f.nextID] = p
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": f.nextID, "code": p.code, "authToken": nil, "expiresAt": time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)})
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v2/pins/"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/v2/pins/"), 10, 64)
		p, ok := f.pins[id]
		if !ok || p.gone {
			http.Error(w, `{"errors":[{"code":1020,"message":"Code not found or expired"}]}`, http.StatusNotFound)
			return
		}
		var tok any
		if p.token != "" {
			tok = p.token
		}
		json.NewEncoder(w).Encode(map[string]any{"id": id, "code": p.code, "authToken": tok})
	case r.Method == "GET" && r.URL.Path == "/api/v2/user":
		u, ok := f.users[r.Header.Get("X-Plex-Token")]
		if !ok {
			http.Error(w, `{"errors":[{"code":1001,"message":"User could not be authenticated"}]}`, http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": u.ID, "username": u.Username, "title": u.Title, "email": "secret@example.com"})
	default:
		http.NotFound(w, r)
	}
}

// signIn plays the person approving the PIN in their browser.
func (f *fakePlexTV) signIn(pinID int64, token string, u fakeUser) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pins[pinID].token = token
	f.users[token] = u
}

// addUser registers a token that plex.tv will recognise, without any PIN.
func (f *fakePlexTV) addUser(token string, u fakeUser) {
	f.mu.Lock()
	f.users[token] = u
	f.mu.Unlock()
}

func (f *fakePlexTV) expire(pinID int64) { f.mu.Lock(); f.pins[pinID].gone = true; f.mu.Unlock() }
func (f *fakePlexTV) count(call string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[call]
}
func (f *fakePlexTV) setFail(b bool) { f.mu.Lock(); f.fail = b; f.mu.Unlock() }
func (f *fakePlexTV) ids() map[string]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]bool{}
	for k, v := range f.seenIDs {
		out[k] = v
	}
	return out
}
