package plexauth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/johnschult/antennarr/internal/access"
)

const maxSessions = 100

type session struct {
	user              access.PlexAccount
	created, lastUsed time.Time
}

// sessions holds who is signed in. It lives in memory: restarting Antennarr
// signs everyone out, which is a fine thing for a restart to do.
type sessions struct {
	mu        sync.Mutex
	m         map[string]*session
	idle, max time.Duration
	now       func() time.Time
}

func newSessions(idle, max time.Duration, now func() time.Time) *sessions {
	return &sessions{m: map[string]*session{}, idle: idle, max: max, now: now}
}

func randomID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // no randomness means no safe way to carry on
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// create signs a user in and returns the session ID for their cookie.
func (s *sessions) create(u access.PlexAccount) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.sweep(now)
	for len(s.m) >= maxSessions { // drop the longest idle
		var oldest string
		for id, ss := range s.m {
			if oldest == "" || ss.lastUsed.Before(s.m[oldest].lastUsed) {
				oldest = id
			}
		}
		delete(s.m, oldest)
	}
	id := randomID(32)
	s.m[id] = &session{user: u, created: now, lastUsed: now}
	return id
}

// get returns who a session belongs to, if it is still good, and counts the
// visit so an active session doesn't time out.
func (s *sessions) get(id string) (access.PlexAccount, bool) {
	if id == "" {
		return access.PlexAccount{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.m[id]
	if !ok {
		return access.PlexAccount{}, false
	}
	now := s.now()
	if s.expired(ss, now) {
		delete(s.m, id)
		return access.PlexAccount{}, false
	}
	ss.lastUsed = now
	return ss.user, true
}

func (s *sessions) delete(id string) {
	s.mu.Lock()
	delete(s.m, id)
	s.mu.Unlock()
}

func (s *sessions) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}

func (s *sessions) expired(ss *session, now time.Time) bool {
	return now.Sub(ss.lastUsed) > s.idle || now.Sub(ss.created) > s.max
}

func (s *sessions) sweep(now time.Time) {
	for id, ss := range s.m {
		if s.expired(ss, now) {
			delete(s.m, id)
		}
	}
}
