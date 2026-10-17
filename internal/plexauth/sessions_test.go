package plexauth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/access"
)

// cookieOf is the session cookie the rig's browser holds.
func (r *rig) cookieOf() *http.Cookie {
	r.t.Helper()
	for _, c := range r.client.Jar.Cookies(mustURL(r.srv.URL + "/antennarr/")) {
		if c.Name == CookieName {
			return c
		}
	}
	r.t.Fatal("no session cookie")
	return nil
}

// adminWith reports whether this rig's Auth accepts a request carrying cookie c.
func (r *rig) adminWith(c *http.Cookie) bool {
	req := httptest.NewRequest("GET", "/x", nil)
	req.AddCookie(c)
	return r.auth.IsSignedInAdmin(req)
}

func TestASignInSurvivesARestart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "antennarr.sessions.json")
	before := newRig(t, func(c *Config) { c.SessionsFile = file })
	before.signedIn()
	cookie := before.cookieOf()

	// A new Auth over the same file is a restarted Antennarr.
	after := newRig(t, func(c *Config) { c.SessionsFile = file })
	if !after.adminWith(cookie) {
		t.Fatal("the browser's cookie no longer works after a restart")
	}
	if after.auth.sess.len() != 1 {
		t.Errorf("%d sessions after the restart", after.auth.sess.len())
	}
}

func TestWithoutAFileARestartSignsEveryoneOut(t *testing.T) {
	before := newRig(t)
	before.signedIn()
	after := newRig(t)
	if after.adminWith(before.cookieOf()) {
		t.Fatal("a session appeared out of nowhere")
	}
}

func TestSigningOutStaysSignedOutAcrossARestart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "s.json")
	before := newRig(t, func(c *Config) { c.SessionsFile = file })
	before.signedIn()
	cookie := before.cookieOf()
	if code, _, _ := before.post("/antennarr/auth/logout", "{}"); code != 200 && code != 204 {
		t.Fatalf("logout: %d", code)
	}
	after := newRig(t, func(c *Config) { c.SessionsFile = file })
	if after.adminWith(cookie) {
		t.Fatal("a signed-out session came back after a restart")
	}
}

func TestOnlyAHashOfTheSessionIDIsWrittenAndOnlyTheOwnerCanReadIt(t *testing.T) {
	file := filepath.Join(t.TempDir(), "s.json")
	r := newRig(t, func(c *Config) { c.SessionsFile = file })
	r.signedIn()
	id := r.cookieOf().Value

	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), id) {
		t.Fatalf("the session ID itself was written to disk:\n%s", b)
	}
	if !strings.Contains(string(b), hashID(id)) {
		t.Errorf("the hash of the session ID isn't in the file:\n%s", b)
	}
	if strings.Contains(string(b), "SECRET-TOKEN") {
		t.Error("the Plex token was written to disk")
	}
	if fi, _ := os.Stat(file); fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestSessionsThatRanOutAreNotRestored(t *testing.T) {
	file := filepath.Join(t.TempDir(), "s.json")
	c := &clock{t: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
	s := newSessions(time.Hour, 24*time.Hour, c.now, file, nil)
	id := s.create(access.PlexAccount{ID: 7, Name: "john"})

	c.add(2 * time.Hour) // idle for longer than the limit while "down"
	again := newSessions(time.Hour, 24*time.Hour, c.now, file, nil)
	if _, ok := again.get(id); ok || again.len() != 0 {
		t.Errorf("an idle-out session came back (%d)", again.len())
	}
}

func TestAnUnreadableFileMeansNoSessionsNotACrash(t *testing.T) {
	for name, content := range map[string]string{"not json": "{{{", "wrong shape": `{"sessions": 7}`, "empty": ""} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "s.json")
			os.WriteFile(file, []byte(content), 0o600)
			s := newSessions(time.Hour, time.Hour, time.Now, file, nil)
			if s.len() != 0 {
				t.Errorf("%d sessions from a bad file", s.len())
			}
			if s.create(access.PlexAccount{ID: 1}) == "" { // and signing in still works, rewriting the file
				t.Error("couldn't sign in")
			}
			if again := newSessions(time.Hour, time.Hour, time.Now, file, nil); again.len() != 1 {
				t.Errorf("the rewritten file has %d sessions", again.len())
			}
		})
	}
}

func TestBeingUnableToSaveDoesNotStopSignIn(t *testing.T) {
	file := filepath.Join(t.TempDir(), "no-such-dir", "s.json")
	s := newSessions(time.Hour, time.Hour, time.Now, file, nil)
	id := s.create(access.PlexAccount{ID: 1})
	if _, ok := s.get(id); !ok {
		t.Error("a session that couldn't be saved should still work until the restart")
	}
}

func TestTheLastUsedTimeIsSavedAtMostOnceAMinute(t *testing.T) {
	file := filepath.Join(t.TempDir(), "s.json")
	c := &clock{t: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
	s := newSessions(12*time.Hour, 7*24*time.Hour, c.now, file, nil)
	id := s.create(access.PlexAccount{ID: 7})
	first, _ := os.ReadFile(file)

	c.add(10 * time.Second)
	s.get(id)
	if now, _ := os.ReadFile(file); string(now) != string(first) {
		t.Error("the file was rewritten for a visit seconds after the last save")
	}
	c.add(2 * time.Minute)
	s.get(id)
	if now, _ := os.ReadFile(file); string(now) == string(first) {
		t.Error("the file never got the newer last-used time")
	}
}

func TestNoTempFilesAreLeftBehind(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.sessions.json")
	s := newSessions(time.Hour, time.Hour, time.Now, file, nil)
	s.create(access.PlexAccount{ID: 1})
	s.create(access.PlexAccount{ID: 2})
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "x.sessions.json" {
		t.Errorf("files in the directory: %v", entries)
	}
}
