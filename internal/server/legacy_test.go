package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/johnschult/yagi/internal/access"
)

func TestTheOldAdminAddressRedirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+legacyPrefix, redirectLegacy)
	for _, c := range []struct{ from, to string }{
		{"/antennarr/", "/"},
		{"/antennarr/viewers?x=1", "/viewers?x=1"},
		{"/antennarr/api/state", "/api/state"},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", c.from, nil))
		if rec.Code != http.StatusPermanentRedirect || rec.Header().Get("Location") != c.to {
			t.Errorf("%s = %d %q, want 308 to %s", c.from, rec.Code, rec.Header().Get("Location"), c.to)
		}
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/antennarr/api/devices", nil))
	if rec.Code == http.StatusPermanentRedirect {
		t.Errorf("a POST was redirected; only GETs should be")
	}
}

func TestTheOldYagiPrefixIsSentOn(t *testing.T) {
	a := newFakeTuner(t, "10ABCC87", "antenna-box")
	devs := []access.Device{{Name: "antenna", Listen: ":5004", Tuner: a.name, StreamPort: a.streamPort}}
	r := start(t, access.DefaultSettings(), devs, a)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for from, to := range map[string]string{
		"/yagi/":                  "/",
		"/yagi/viewers":           "/viewers",
		"/yagi/claim?token=abc_-": "/claim?token=abc_-",
		"/antennarr/settings":     "/settings",
	} {
		resp, err := client.Get("http://" + r.admin + from)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusPermanentRedirect || resp.Header.Get("Location") != to {
			t.Errorf("%s: %d -> %q, want %q", from, resp.StatusCode, resp.Header.Get("Location"), to)
		}
	}
	// The tuner's own paths are still the tuner's, and a page of the UI is not.
	resp, err := client.Get("http://" + r.admin + "/viewers")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 503 {
		t.Errorf("/viewers = %d, want the page", resp.StatusCode)
	}
}
