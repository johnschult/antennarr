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

// fakePlexTV stands in for plex.tv. It checks the headers Yagi must send
// and lets a test play the part of the person signing in.
type fakePlexTV struct {
	*httptest.Server
	mu      sync.Mutex
	nextID  int64
	pins    map[int64]*fakePin
	users   map[string]fakeUser // by token
	calls   map[string]int
	fail    bool // answer 500 to everything
	seenIDs map[string]bool
	// resources is what each token's account has, as plex.tv would send it, by token.
	resources map[string][]map[string]any
	// padResources makes the answer to /resources larger than any client should read.
	padResources bool
	// queries records the raw query of each request, to see that a token is never in a URL.
	queries []string
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
	f := &fakePlexTV{nextID: 1000, pins: map[int64]*fakePin{}, users: map[string]fakeUser{}, calls: map[string]int{}, seenIDs: map[string]bool{}, resources: map[string][]map[string]any{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *fakePlexTV) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[r.Method+" "+r.URL.Path]++
	f.queries = append(f.queries, r.URL.RawQuery)
	f.seenIDs[r.Header.Get("X-Plex-Client-Identifier")] = true
	if r.Header.Get("Accept") != "application/json" || r.Header.Get("X-Plex-Product") != "Yagi" || r.Header.Get("X-Plex-Client-Identifier") == "" {
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
	case r.Method == "GET" && r.URL.Path == "/api/v2/resources":
		if q := r.URL.Query(); q.Get("includeHttps") != "1" || q.Get("includeRelay") != "0" {
			http.Error(w, "want https connections and no relays", http.StatusBadRequest)
			return
		}
		list, ok := f.resources[r.Header.Get("X-Plex-Token")]
		if _, known := f.users[r.Header.Get("X-Plex-Token")]; !ok && !known {
			http.Error(w, `{"errors":[{"code":1001,"message":"User could not be authenticated"}]}`, http.StatusUnauthorized)
			return
		}
		if f.padResources {
			list = append(list, map[string]any{"name": strings.Repeat("x", 2<<20), "provides": "server", "clientIdentifier": "pad"})
		}
		json.NewEncoder(w).Encode(list)
	default:
		http.NotFound(w, r)
	}
}

// setResources gives a token's account these resources, and makes the token a known one.
func (f *fakePlexTV) setResources(token string, list ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resources[token] = list
	if _, ok := f.users[token]; !ok {
		f.users[token] = fakeUser{ID: 99, Username: "someone"}
	}
}

// server is a resource that is a Plex Media Server, shaped as plex.tv sends it: with its own access token and public
// address as well, which Yagi must not pass on.
func server(name, machineID string, owned bool, conns ...map[string]any) map[string]any {
	return map[string]any{
		"name": name, "product": "Plex Media Server", "clientIdentifier": machineID, "provides": "server", "owned": owned,
		"accessToken": "server-access-token-" + machineID, "publicAddress": "203.0.113.50", "connections": conns,
	}
}

// conn is one of a resource's connections.
func conn(address string, port int, local, relay bool) map[string]any {
	return map[string]any{
		"protocol": "http", "address": address, "port": port, "local": local, "relay": relay, "IPv6": strings.Contains(address, ":"),
		"uri": fmt.Sprintf("http://%s:%d", address, port),
	}
}

// signIn plays the person approving the PIN in their browser.
func (f *fakePlexTV) signIn(pinID int64, token string, u fakeUser) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pins[pinID].token = token
	f.users[token] = u
}

// addUser registers a token that plex.tv will recognize, without any PIN.
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
func (f *fakePlexTV) setPad(b bool) { f.mu.Lock(); f.padResources = b; f.mu.Unlock() }
func (f *fakePlexTV) rawQueries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.queries...)
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
