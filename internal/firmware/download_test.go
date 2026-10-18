package firmware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// image is something the size of a real firmware image, which is what the checks look at.
func image(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i * 31)
	}
	return b
}

// fwServer serves one file, as SiliconDust's does: a binary, at <family>_firmware_<release>.bin.
func fwServer(t *testing.T, path string, status int, contentType string, body []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "Antennarr") {
			t.Errorf("user agent = %q", ua)
		}
		switch contentType {
		case "":
			w.Header()["Content-Type"] = nil // none at all: without this Go sniffs one, and binary data sniffs as octet-stream
		default:
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

const goodPath = "/hdhomerun_dvr_atsc3_firmware_20260326.bin"

func TestFetchGetsTheImageAndItsChecksum(t *testing.T) {
	data := image(2_231_056) // the FLEX 4K's size
	srv, _ := fwServer(t, goodPath, 200, "application/octet-stream", data)
	img, err := Downloader{Base: srv.URL}.Fetch(context.Background(), "hdhomerun_dvr_atsc3", "20260326")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if !bytes.Equal(img.Data, data) || img.SHA256 != hex.EncodeToString(sum[:]) || img.Family != "hdhomerun_dvr_atsc3" || img.Release != "20260326" {
		t.Errorf("image = %d bytes, %s", len(img.Data), img.SHA256)
	}
}

func TestTheAddressIsBuiltFromWhatTheTunerAndTheChangelogSay(t *testing.T) {
	u, err := (Downloader{}).URL("hdhomerun_dvr_atsc3", "20260326")
	if err != nil || u != "https://download.silicondust.com/hdhomerun/hdhomerun_dvr_atsc3_firmware_20260326.bin" {
		t.Errorf("%q %v", u, err)
	}
	if u, _ := (Downloader{Base: "http://localhost:9/x/"}).URL("hdhomerun5_4dt", "20250815"); u != "http://localhost:9/x/hdhomerun5_4dt_firmware_20250815.bin" {
		t.Errorf("a base with a trailing slash: %q", u)
	}
}

func TestNothingThatIsNotAFamilyOrAReleaseGetsIntoTheAddress(t *testing.T) {
	d := Downloader{Base: "http://127.0.0.1:1"} // nothing is listening: if a request were made it would fail differently
	for _, family := range []string{"", "x", "../../etc/passwd", "hdhomerun_dvr_atsc3/../x", "a b c", "HDHOMERUN", "hdhomerun_dvr_atsc3?x=1", "hdhomerun_dvr_atsc3#", "hdhomerun%2e", strings.Repeat("a", 65), "-bad", "bad\n"} {
		if _, err := d.URL(family, "20260326"); err == nil {
			t.Errorf("family %q was accepted", family)
		}
		if _, err := d.Fetch(context.Background(), family, "20260326"); err == nil || !strings.Contains(err.Error(), "not a firmware family") {
			t.Errorf("Fetch family %q: %v", family, err)
		}
	}
	for _, release := range []string{"", "2026", "202603261", "2026032a", "../x", "20260326/../x", "20260326.bin", " 20260326"} {
		if _, err := d.URL("hdhomerun_dvr_atsc3", release); err == nil {
			t.Errorf("release %q was accepted", release)
		}
	}
}

func TestAFileThatIsNotThereIsSaidToBeNotThere(t *testing.T) {
	srv, hits := fwServer(t, goodPath, 200, "application/octet-stream", image(MinImageSize))
	_, err := Downloader{Base: srv.URL}.Fetch(context.Background(), "hdhomerun_dvr_atsc3", "20991231")
	if err != ErrNotFound {
		t.Errorf("error = %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("%d requests: nothing else should be tried", hits.Load())
	}
}

func TestAnythingButAWholeBinaryFileIsRefused(t *testing.T) {
	good := image(MinImageSize + 100)
	for name, tc := range map[string]struct {
		status int
		ctype  string
		body   []byte
		want   string
	}{
		"a server error":       {500, "application/octet-stream", good, "500"},
		"forbidden":            {403, "text/plain", []byte("no"), "403"},
		"an error page as 200": {200, "text/html; charset=utf-8", append([]byte("<html>"), good...), "not a firmware file"},
		"no content type":      {200, "", good, "not a firmware file"},
		"too small":            {200, "application/octet-stream", image(1000), "not a size"},
		"empty":                {200, "application/octet-stream", nil, "not a size"},
		"too big":              {200, "application/octet-stream", image(MaxImageSize + 1), "not a size"},
	} {
		srv, _ := fwServer(t, goodPath, tc.status, tc.ctype, tc.body)
		_, err := Downloader{Base: srv.URL}.Fetch(context.Background(), "hdhomerun_dvr_atsc3", "20260326")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestADownloadThatIsCutShortIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", "2231056")
		w.Write(image(1_000_000)) // promises 2.2 MB, sends 1 MB, and ends
	}))
	defer srv.Close()
	_, err := Downloader{Base: srv.URL}.Fetch(context.Background(), "hdhomerun_dvr_atsc3", "20260326")
	if err == nil || !strings.Contains(err.Error(), "cut short") {
		t.Errorf("error = %v", err)
	}
}

func TestARedirectIsNotFollowed(t *testing.T) {
	var elsewhere atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(image(MinImageSize + 1))
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/evil.bin", http.StatusFound)
	}))
	defer redirect.Close()
	_, err := Downloader{Base: redirect.URL}.Fetch(context.Background(), "hdhomerun_dvr_atsc3", "20260326")
	if err == nil || elsewhere.Load() != 0 {
		t.Errorf("error = %v, and %d requests went to where it redirected", err, elsewhere.Load())
	}
}

func TestErrorsNeverRepeatTheAddress(t *testing.T) {
	// The address could, in a test or a mirror, carry something that should not be in a log or on the page.
	d := Downloader{Base: "http://user:SECRETPASS@127.0.0.1:1/SECRETPATH"}
	_, err := d.Fetch(context.Background(), "hdhomerun_dvr_atsc3", "20260326")
	if err == nil {
		t.Fatal("no error")
	}
	if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("the error repeats the address: %v", err)
	}
}

func TestTheDownloadStopsWhenAsked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(image(1000))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Downloader{Base: srv.URL}.Fetch(ctx, "hdhomerun_dvr_atsc3", "20260326")
	if err == nil || time.Since(start) > 3*time.Second {
		t.Errorf("error = %v after %v", err, time.Since(start))
	}
}

func TestInstallableOnlyForANewerRelease(t *testing.T) {
	update := Status{State: StateUpdateAvailable, Current: "20250815", Latest: "20260326"}
	if rel, ok, why := Installable(update, "hdhomerun_dvr_atsc3"); !ok || rel != "20260326" || why != "" {
		t.Errorf("%q %v %q", rel, ok, why)
	}
	for name, tc := range map[string]struct {
		s      Status
		family string
	}{
		"up to date":           {Status{State: StateUpToDate, Current: "20260326", Latest: "20260326"}, "hdhomerun_dvr_atsc3"},
		"only a possible one":  {Status{State: StatePossibleUpdate, Current: "20250815", Latest: "20260326"}, "hdhomerun_dvr_atsc3"},
		"unknown":              {Status{State: StateUnknown, Current: "20250815"}, "hdhomerun_dvr_atsc3"},
		"the check is off":     {Status{State: StateDisabled, Current: "20250815"}, "hdhomerun_dvr_atsc3"},
		"no family known":      {update, ""},
		"a bad family":         {update, "../x"},
		"a downgrade":          {Status{State: StateUpdateAvailable, Current: "20260326", Latest: "20250815"}, "hdhomerun_dvr_atsc3"},
		"the same release":     {Status{State: StateUpdateAvailable, Current: "20260326", Latest: "20260326"}, "hdhomerun_dvr_atsc3"},
		"a version not a date": {Status{State: StateUpdateAvailable, Current: "20250815", Latest: "beta"}, "hdhomerun_dvr_atsc3"},
		"no current version":   {Status{State: StateUpdateAvailable, Current: "", Latest: "20260326"}, "hdhomerun_dvr_atsc3"},
	} {
		if rel, ok, why := Installable(tc.s, tc.family); ok || rel != "" || why == "" {
			t.Errorf("%s: %q %v %q", name, rel, ok, why)
		}
	}
}
