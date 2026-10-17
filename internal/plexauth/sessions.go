package plexauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/johnschult/antennarr/internal/access"
)

const maxSessions = 100

type session struct {
	user              access.PlexAccount
	created, lastUsed time.Time
}

// saveEvery is the most often a session's "last used" time is written to disk.
// Signing in and out are written at once; ordinary visits only move that time,
// which only matters if the server restarts, so a minute's lag is harmless.
const saveEvery = time.Minute

// sessions holds who is signed in, keyed by a hash of the session ID so that
// neither memory dumps nor the file can be used to impersonate anyone. With a
// file, sessions survive a restart (otherwise every restart signs everyone
// out); without one they live in memory only.
type sessions struct {
	mu        sync.Mutex
	m         map[string]*session // by hash of the ID
	idle, max time.Duration
	now       func() time.Time

	file      string // "" means memory only
	log       *slog.Logger
	lastSaved time.Time
}

func newSessions(idle, max time.Duration, now func() time.Time, file string, log *slog.Logger) *sessions {
	if log == nil {
		log = slog.Default()
	}
	s := &sessions{m: map[string]*session{}, idle: idle, max: max, now: now, file: file, log: log}
	s.load()
	return s
}

func hashID(id string) string {
	h := sha256.Sum256([]byte(id))
	return hex.EncodeToString(h[:])
}

// stored is how a session is written to the file.
type stored struct {
	Key      string             `json:"key"` // hash of the ID, never the ID
	User     access.PlexAccount `json:"user"`
	Created  time.Time          `json:"created"`
	LastUsed time.Time          `json:"lastUsed"`
}

func (s *sessions) load() {
	if s.file == "" {
		return
	}
	b, err := os.ReadFile(s.file)
	if err != nil {
		if !os.IsNotExist(err) {
			s.log.Warn("couldn't read the saved sign-ins; starting with none", "err", err)
		}
		return
	}
	var doc struct {
		Sessions []stored `json:"sessions"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		s.log.Warn("the saved sign-ins aren't readable; starting with none", "err", err)
		return
	}
	now := s.now()
	for _, st := range doc.Sessions {
		ss := &session{user: st.User, created: st.Created, lastUsed: st.LastUsed}
		if st.Key != "" && !s.expired(ss, now) {
			s.m[st.Key] = ss
		}
	}
}

// save writes the sessions out (s.mu held). A failure is logged, never fatal:
// the worst case is being signed out by the next restart, as before.
func (s *sessions) save() {
	if s.file == "" {
		return
	}
	s.lastSaved = s.now()
	doc := struct {
		Sessions []stored `json:"sessions"`
	}{Sessions: make([]stored, 0, len(s.m))}
	for key, ss := range s.m {
		doc.Sessions = append(doc.Sessions, stored{Key: key, User: ss.user, Created: ss.created, LastUsed: ss.lastUsed})
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err == nil {
		err = writeFileAtomic(s.file, b)
	}
	if err != nil {
		s.log.Warn("couldn't save sign-ins; a restart will sign everyone out", "err", err)
	}
}

// writeFileAtomic replaces path with data, readable only by this user.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op once renamed
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
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
	s.m[hashID(id)] = &session{user: u, created: now, lastUsed: now}
	s.save()
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
	key := hashID(id)
	ss, ok := s.m[key]
	if !ok {
		return access.PlexAccount{}, false
	}
	now := s.now()
	if s.expired(ss, now) {
		delete(s.m, key)
		s.save()
		return access.PlexAccount{}, false
	}
	ss.lastUsed = now
	if now.Sub(s.lastSaved) >= saveEvery {
		s.save()
	}
	return ss.user, true
}

func (s *sessions) delete(id string) {
	s.mu.Lock()
	if _, ok := s.m[hashID(id)]; ok {
		delete(s.m, hashID(id))
		s.save()
	}
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
