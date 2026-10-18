package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/access"
)

// ntfyBox is a fake ntfy server that keeps the titles it is sent.
type ntfyBox struct {
	*httptest.Server
	mu     sync.Mutex
	titles []string
}

func newNtfyBox(t *testing.T) *ntfyBox {
	t.Helper()
	n := &ntfyBox{}
	n.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m struct{ Title, Message string }
		json.Unmarshal(b, &m)
		n.mu.Lock()
		n.titles = append(n.titles, m.Title+" | "+m.Message)
		n.mu.Unlock()
	}))
	t.Cleanup(n.Close)
	return n
}

func (n *ntfyBox) got() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.titles...)
}

func (n *ntfyBox) countOf(sub string) int { return count(n.got(), sub) }

func (n *ntfyBox) waitFor(t *testing.T, sub string) {
	t.Helper()
	waitFor(t, "a notification with "+sub, func() bool { return n.countOf(sub) > 0 })
}

// notifying is settings with one ntfy channel pointing at box, saved to a file so the server has somewhere to keep its notes.
func notifying(t *testing.T, box *ntfyBox, viewers ...access.Viewer) (access.Settings, string) {
	t.Helper()
	on := true
	s := access.Settings{
		OwnerReserve: 1, Viewers: viewers,
		Notifications: &access.Notifications{
			Channels: []access.NotifyChannel{{ID: "phone", Name: "Phone", Type: access.ChannelNtfy, URL: box.URL, Topic: "antennarr"}},
			Events:   access.NotifyEvents{Unknown: &on},
		},
	}
	path := filepath.Join(t.TempDir(), "antennarr.json")
	if err := access.SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	return s, path
}

func withConfig(path string, grace time.Duration) func(*Config) {
	return func(c *Config) { c.ConfigPath, c.OfflineGrace = path, grace }
}

func TestAnOfflineTunerIsReportedOnceAndSoIsItsReturn(t *testing.T) {
	box := newNtfyBox(t)
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	s, path := notifying(t, box, access.Viewer{Name: "test", Addresses: []string{"127.0.0.1"}, Limit: 8})
	startConfigured(t, s, oneDevice(a), "", "", withConfig(path, 300*time.Millisecond), a)

	if n := len(box.got()); n != 0 {
		t.Fatalf("starting up said something: %q", box.got())
	}

	// (A blip shorter than the grace period is not reported either; the notifier's own tests cover that with a fake clock,
	// since a real tuner that goes offline is searched for for a couple of seconds before it is polled again.)
	a.setDown(true)
	box.waitFor(t, "is offline")
	if !strings.Contains(box.got()[0], "antenna is offline") {
		t.Errorf("message = %q", box.got())
	}
	time.Sleep(300 * time.Millisecond)
	if n := box.countOf("is offline"); n != 1 {
		t.Errorf("told %d times it was offline", n)
	}

	a.setDown(false)
	box.waitFor(t, "is back online")
	if n := box.countOf("is back online"); n != 1 {
		t.Errorf("told %d times it was back", n)
	}
}

func TestNewFirmwareIsMessagedOnceAndNotAgainAfterARestart(t *testing.T) {
	url, _ := changelog(t)
	box := newNtfyBox(t)
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	a.setFirmware("20250815", "")
	s, path := notifying(t, box, access.Viewer{Name: "test", Addresses: []string{"127.0.0.1"}, Limit: 8})
	on := true
	s.FirmwareCheck = &on

	r := startConfigured(t, s, oneDevice(a), url, "", withConfig(path, time.Minute), a)
	box.waitFor(t, "New firmware for antenna")
	time.Sleep(400 * time.Millisecond) // the tuner is polled every 50ms
	if n := box.countOf("New firmware"); n != 1 {
		t.Fatalf("messaged %d times: %q", n, box.got())
	}
	if !strings.Contains(box.got()[0], "20260326") {
		t.Errorf("the message should name the release: %q", box.got()[0])
	}
	notified := strings.TrimSuffix(path, ".json") + ".notified.json"
	if info, err := os.Stat(notified); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the notified file: %v %v", info, err)
	}

	// Watchtower replaces the container: the tuner still has old firmware, and the owner is not told again.
	r.cancel()
	<-r.done
	r2 := startConfigured(t, s, oneDevice(a), url, "", withConfig(path, time.Minute), a)
	waitFor(t, "the update to be found again", func() bool { st, _ := r2.firmware(t); return st.State == "update-available" })
	time.Sleep(400 * time.Millisecond)
	if n := box.countOf("New firmware"); n != 1 {
		t.Errorf("a restart messaged again: %q", box.got())
	}
}

func TestAnUnknownAddressIsMessagedAndAViewerAtItsLimitToo(t *testing.T) {
	box := newNtfyBox(t)
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	// Only a viewer who isn't us: the test's own requests come from 127.0.0.1 and so are unknown...
	s, path := notifying(t, box, access.Viewer{Name: "Elsewhere", Addresses: []string{"10.9.9.9"}, Limit: 1})
	r := startConfigured(t, s, oneDevice(a), "", "", withConfig(path, time.Minute), a)

	if code, _ := r.get(t, "antenna", "/lineup.json"); code != http.StatusForbidden {
		t.Fatalf("an unknown address got %d", code)
	}
	box.waitFor(t, "An unknown address was refused")
	if !strings.Contains(box.got()[0], "127.0.0.1 tried to use antenna") {
		t.Errorf("message = %q", box.got()[0])
	}
	for i := 0; i < 5; i++ {
		r.get(t, "antenna", "/lineup.json")
	}
	time.Sleep(200 * time.Millisecond)
	if n := box.countOf("unknown address"); n != 1 {
		t.Errorf("the same address was reported %d times", n)
	}
}

func TestAViewerAtItsLimitIsMessaged(t *testing.T) {
	box := newNtfyBox(t)
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	s, path := notifying(t, box, access.Viewer{Name: "Paused", Addresses: []string{"127.0.0.1"}, Limit: 0})
	s.Labels = map[string]string{"antenna": "Living room"}
	if err := access.SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	r := startConfigured(t, s, oneDevice(a), "", "", withConfig(path, time.Minute), a)

	if code, _ := r.get(t, "antenna", "/auto/v5.1"); code != http.StatusServiceUnavailable {
		t.Fatalf("the viewer got %d", code)
	}
	box.waitFor(t, "Paused was refused a tuner")
	if !strings.Contains(box.got()[0], "on Living room") {
		t.Errorf("message = %q", box.got()[0])
	}
	for i := 0; i < 5; i++ {
		r.get(t, "antenna", "/auto/v5.1") // the media server retries
	}
	time.Sleep(200 * time.Millisecond)
	if n := box.countOf("was refused"); n != 1 {
		t.Errorf("reported %d times", n)
	}
}

func TestNothingIsSentOrContactedWithoutAChannel(t *testing.T) {
	box := newNtfyBox(t)
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	s, path := notifying(t, box, access.Viewer{Name: "Paused", Addresses: []string{"127.0.0.1"}, Limit: 0})
	s.Notifications.Channels = nil
	if err := access.SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	r := startConfigured(t, s, oneDevice(a), "", "", withConfig(path, 100*time.Millisecond), a)
	r.get(t, "antenna", "/auto/v5.1")
	a.setDown(true)
	time.Sleep(600 * time.Millisecond)
	if n := len(box.got()); n != 0 {
		t.Errorf("%d messages with no channel", n)
	}
}

// online says whether the first device is seen online.
func (r *rig) online(t *testing.T) bool {
	t.Helper()
	_, body := r.get(t, "antenna", "/antennarr/api/state")
	var st struct {
		Devices []struct{ Tuner struct{ Online bool } }
	}
	if err := json.Unmarshal([]byte(body), &st); err != nil || len(st.Devices) == 0 {
		t.Fatalf("state: %v\n%s", err, body)
	}
	return st.Devices[0].Tuner.Online
}
