package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTheOldAdminAddressRedirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+legacyPrefix, redirectLegacy)
	for _, c := range []struct{ from, to string }{
		{"/antennarr/", "/yagi/"},
		{"/antennarr/viewers?x=1", "/yagi/viewers?x=1"},
		{"/antennarr/api/state", "/yagi/api/state"},
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
