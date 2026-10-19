package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/access"
)

// changelog serves a real excerpt of SiliconDust's page and counts requests.
func changelog(t *testing.T) (url string, hits *atomic.Int32) {
	t.Helper()
	body, err := os.ReadFile("../firmware/testdata/changelog.html")
	if err != nil {
		t.Fatal(err)
	}
	hits = &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, hits
}

type fwState struct {
	State          string `json:"state"`
	Current        string `json:"current"`
	Latest         string `json:"latest"`
	DeviceReported string `json:"deviceReported"`
	Error          string `json:"error"`
	CheckedAt      string `json:"checkedAt"`
	Updates        []struct {
		Version string `json:"version"`
	} `json:"updates"`
}

// firmware reads the first device's firmware status and event texts from the API.
func (r *rig) firmware(t *testing.T) (fwState, []string) {
	t.Helper()
	_, body := r.get(t, "antenna", "/yagi/api/state")
	var st struct {
		Devices []struct {
			Firmware fwState `json:"firmware"`
			Tuner    struct {
				Events []struct{ Text string } `json:"events"`
			} `json:"tuner"`
		} `json:"devices"`
	}
	if err := json.Unmarshal([]byte(body), &st); err != nil || len(st.Devices) == 0 {
		t.Fatalf("state: %v\n%s", err, body)
	}
	var texts []string
	for _, e := range st.Devices[0].Tuner.Events {
		texts = append(texts, e.Text)
	}
	return st.Devices[0].Firmware, texts
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func count(texts []string, sub string) int {
	n := 0
	for _, s := range texts {
		if strings.Contains(s, sub) {
			n++
		}
	}
	return n
}

func oneDevice(a *fakeTuner) []access.Device {
	return []access.Device{{Name: "antenna", Listen: ":5004", Tuner: a.name, StreamPort: a.streamPort}}
}

func TestANewerFirmwareIsShownAndAnnouncedOnce(t *testing.T) {
	url, _ := changelog(t)
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	a.setFirmware("20250815", "")
	r := startWith(t, access.DefaultSettings(), oneDevice(a), url, a)

	waitFor(t, "the update to be found", func() bool { st, _ := r.firmware(t); return st.State == "update-available" })
	st, _ := r.firmware(t)
	if st.Current != "20250815" || st.Latest != "20260326" || len(st.Updates) != 2 || st.CheckedAt == "" {
		t.Errorf("status = %+v", st)
	}

	// The tuner is polled every 50ms here; the announcement must not repeat.
	waitFor(t, "the announcement", func() bool { _, ev := r.firmware(t); return count(ev, "Firmware 20260326 is available") > 0 })
	time.Sleep(400 * time.Millisecond)
	_, events := r.firmware(t)
	if n := count(events, "Firmware 20260326 is available"); n != 1 {
		t.Errorf("announced %d times, want once: %q", n, events)
	}
	if n := count(events, "(it is on 20250815)"); n != 1 {
		t.Errorf("the announcement should say what the tuner is on: %q", events)
	}
}

func TestATunerOnTheLatestFirmwareSaysNothing(t *testing.T) {
	url, hits := changelog(t)
	a := newFakeTuner(t, "10ABCC87", "antenna-box") // reports 20260326, the newest release
	r := startWith(t, access.DefaultSettings(), oneDevice(a), url, a)

	waitFor(t, "the changelog to be read", func() bool { st, _ := r.firmware(t); return st.State == "up-to-date" })
	st, events := r.firmware(t)
	if st.Latest != "20260326" || st.CheckedAt == "" || len(st.Updates) != 0 || hits.Load() < 1 {
		t.Errorf("status = %+v hits=%d", st, hits.Load())
	}
	if n := count(events, "Firmware"); n != 0 {
		t.Errorf("up to date, but announced: %q", events)
	}
}

func TestNothingIsFetchedWhenTheCheckIsOff(t *testing.T) {
	url, hits := changelog(t)
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	a.setFirmware("20250815", "")
	off := false
	r := startWith(t, access.Settings{FirmwareCheck: &off, OwnerReserve: 1}, oneDevice(a), url, a)

	waitFor(t, "the state", func() bool { st, _ := r.firmware(t); return st.State == "disabled" })
	time.Sleep(300 * time.Millisecond)
	if hits.Load() != 0 {
		t.Errorf("%d requests to the changelog while the check is off", hits.Load())
	}
	if _, events := r.firmware(t); count(events, "Firmware") != 0 {
		t.Errorf("announced while off: %q", events)
	}
}

func TestTheTunersOwnReportStillShowsWhenTheCheckIsOff(t *testing.T) {
	url, hits := changelog(t)
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	a.setFirmware("20260326", "20270101") // the tuner says there is a newer one: no outside request needed
	off := false
	r := startWith(t, access.Settings{FirmwareCheck: &off, OwnerReserve: 1}, oneDevice(a), url, a)

	waitFor(t, "the tuner's report", func() bool { st, _ := r.firmware(t); return st.State == "update-available" })
	st, _ := r.firmware(t)
	if st.DeviceReported != "20270101" || st.Latest != "20270101" {
		t.Errorf("status = %+v", st)
	}
	waitFor(t, "the announcement", func() bool { _, ev := r.firmware(t); return count(ev, "as reported by the tuner") == 1 })
	if hits.Load() != 0 {
		t.Errorf("%d requests while off", hits.Load())
	}
}

func TestSwitchingTheCheckOnWhileRunningLooksStraightAway(t *testing.T) {
	url, hits := changelog(t)
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	a.setFirmware("20250815", "")
	off := false
	s := access.Settings{FirmwareCheck: &off, OwnerReserve: 1}
	r := startWith(t, s, oneDevice(a), url, a)
	time.Sleep(200 * time.Millisecond)
	if hits.Load() != 0 {
		t.Fatal("looked while off")
	}

	on := true
	s.FirmwareCheck = &on
	if err := r.srv.Reload(s); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the first look", func() bool { return hits.Load() >= 1 })
	waitFor(t, "the update to be found", func() bool { st, _ := r.firmware(t); return st.State == "update-available" })
}

func TestTheAnnouncementIsRepeatedForTheNextRelease(t *testing.T) {
	url, _ := changelog(t)
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	a.setFirmware("20250815", "")
	r := startWith(t, access.DefaultSettings(), oneDevice(a), url, a)
	waitFor(t, "the first announcement", func() bool { _, ev := r.firmware(t); return count(ev, "Firmware 20260326 is available") == 1 })

	a.setFirmware("20260326", "") // the owner installed it
	waitFor(t, "up to date", func() bool { st, _ := r.firmware(t); return st.State == "up-to-date" })

	a.setFirmware("20250815", "") // (a stand-in for a later release being out)
	waitFor(t, "a second announcement", func() bool { _, ev := r.firmware(t); return count(ev, "Firmware 20260326 is available") == 2 })
}

func TestAnUnreachableChangelogDoesNotAffectAnythingElse(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	a.setFirmware("20250815", "")
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", 500) }))
	defer broken.Close()
	r := startWith(t, access.DefaultSettings(), oneDevice(a), broken.URL, a)

	waitFor(t, "the failure to be reported", func() bool { st, _ := r.firmware(t); return st.State == "unknown" && strings.Contains(st.Error, "500") })
	if code, _ := r.get(t, "antenna", "/yagi/api/state"); code != 200 {
		t.Errorf("state: %d", code)
	}
	if _, events := r.firmware(t); count(events, "Firmware") != 0 {
		t.Errorf("announced without knowing: %q", events)
	}
}

func TestAnnouncementsUseTheNameTheOwnerGave(t *testing.T) {
	url, _ := changelog(t)
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	a.setFirmware("20250815", "")
	r := startWith(t, access.Settings{Labels: map[string]string{"antenna": "Living room antenna"}}, oneDevice(a), url, a)
	waitFor(t, "the announcement", func() bool {
		_, ev := r.firmware(t)
		return count(ev, "Firmware 20260326 is available for Living room antenna (it is on 20250815)") == 1
	})
}
