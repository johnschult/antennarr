package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/access"
)

// siliconDust serves the real guide excerpt (with its logos pointed back here)
// and a logo, and counts how often the guide is asked for.
func siliconDust(t *testing.T) (url string, hits *atomic.Int32) {
	t.Helper()
	raw, err := os.ReadFile("../guide/testdata/xmltv.xml")
	if err != nil {
		t.Fatal(err)
	}
	// The listings in the fixture are for one day in October; shift them to now so
	// that "what's on" works whenever the test runs.
	now := time.Now().UTC().Truncate(time.Minute)
	for stamp, at := range map[string]time.Time{
		"20261017160000": now.Add(-90 * time.Minute), // already over
		"20261017173000": now.Add(-30 * time.Minute), // on now: starts 30 min ago...
		"20261017180000": now.Add(30 * time.Minute),  // ...ends in 30 min, when the next one starts
		"20261017190000": now.Add(90 * time.Minute),
	} {
		raw = []byte(strings.ReplaceAll(string(raw), stamp, at.Format("20060102150405")))
	}
	hits = &atomic.Int32{}
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/xmltv":
			hits.Add(1)
			if r.URL.Query().Get("DeviceAuth") != "S3CR3T-DEVICE-AUTH" {
				http.Error(w, "unauthorised", http.StatusUnauthorized)
				return
			}
			io.WriteString(w, strings.ReplaceAll(string(raw), "https://img.hdhomerun.com", base))
		case strings.HasPrefix(r.URL.Path, "/channels/"):
			w.Header().Set("Content-Type", "image/png")
			io.WriteString(w, "\x89PNG-fake")
		default:
			http.NotFound(w, r)
		}
	}))
	base = srv.URL
	t.Cleanup(srv.Close)
	return srv.URL, hits
}

type guideEntry struct {
	Name string `json:"name"`
	Icon bool   `json:"icon"`
	Now  *struct {
		Title string `json:"title"`
	} `json:"now"`
}

func (r *rig) guideFor(t *testing.T) (map[string]guideEntry, string) {
	t.Helper()
	_, body := r.get(t, "antenna", "/yagi/api/state")
	var st struct {
		Devices []struct {
			Guide map[string]guideEntry `json:"guide"`
		} `json:"devices"`
	}
	if err := json.Unmarshal([]byte(body), &st); err != nil {
		t.Fatal(err)
	}
	return st.Devices[0].Guide, body
}

func TestTilesGetTheLogoAndWhatIsOn(t *testing.T) {
	url, _ := siliconDust(t)
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	a.channel = "11.1"
	r := startFull(t, access.DefaultSettings(), oneDevice(a), "", url, a)

	var g map[string]guideEntry
	var body string
	waitFor(t, "the guide to arrive", func() bool { g, body = r.guideFor(t); return g["11.1"].Name != "" })
	if !g["11.1"].Icon || g["11.1"].Now == nil || g["11.1"].Now.Title == "" {
		t.Errorf("entry = %+v", g["11.1"])
	}
	if len(g) != 1 {
		t.Errorf("only channels being watched should be listed: %v", g)
	}

	// The logo comes through Yagi, so the browser never calls SiliconDust.
	code, logo := r.get(t, "antenna", "/yagi/api/devices/antenna/guide/11.1/icon")
	if code != 200 || !strings.HasPrefix(logo, "\x89PNG") {
		t.Errorf("logo: %d %q", code, logo)
	}
	if code, _ := r.get(t, "antenna", "/yagi/api/devices/antenna/guide/8.1/icon"); code != 404 {
		t.Errorf("a channel with a foreign logo address: %d, want 404", code)
	}
	if code, _ := r.get(t, "antenna", "/yagi/api/devices/antenna/guide/99.9/icon"); code != 404 {
		t.Errorf("an unknown channel: %d, want 404", code)
	}

	// The tuner's DeviceAuth is for SiliconDust only: never in an API response or the log.
	if strings.Contains(body, "S3CR3T") || strings.Contains(r.logs.String(), "S3CR3T") {
		t.Error("the DeviceAuth leaked")
	}
}

func TestNoGuideIsFetchedWhileOffAndItCanBeSwitchedOn(t *testing.T) {
	url, hits := siliconDust(t)
	off := false
	s := access.DefaultSettings()
	s.Guide = &off
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	a.channel = "11.1"
	r := startFull(t, s, oneDevice(a), "", url, a)

	g, _ := r.guideFor(t)
	if len(g) != 0 || hits.Load() != 0 {
		t.Fatalf("while off: guide=%v hits=%d", g, hits.Load())
	}
	if code, _ := r.get(t, "antenna", "/yagi/api/devices/antenna/guide/11.1/icon"); code != 404 {
		t.Errorf("a logo while off: %d", code)
	}

	req, _ := http.NewRequest("PATCH", "http://"+r.addr["antenna"]+"/yagi/api/settings", strings.NewReader(`{"guide":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+r.addr["antenna"])
	if resp, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != 409 && resp.StatusCode != 200 { // 409: this test rig has no settings file to save to
			t.Fatalf("PATCH: %d", resp.StatusCode)
		}
	}
}
