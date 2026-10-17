package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

var built = fstest.MapFS{
	"index.html":              {Data: []byte(`<!doctype html><div id="root"></div>`)},
	"favicon.svg":             {Data: []byte(`<svg/>`)},
	"assets/index-abc123.js":  {Data: []byte(`console.log("hi")`)},
	"assets/index-abc123.css": {Data: []byte(`body{}`)},
}

func get(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestServesIndexAtTheRoot(t *testing.T) {
	h := handler("/antennarr/", built)
	for _, target := range []string{"/antennarr/", "/antennarr/index.html"} {
		rec := get(h, "GET", target)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `id="root"`) {
			t.Errorf("%s: %d %q", target, rec.Code, rec.Body)
		}
		if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
			t.Errorf("%s: Content-Type = %q", target, rec.Header().Get("Content-Type"))
		}
		if rec.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: the page must always be revalidated, got %q", target, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestAppRoutesFallBackToIndex(t *testing.T) {
	h := handler("/antennarr/", built)
	for _, target := range []string{"/antennarr/viewers", "/antennarr/settings", "/antennarr/deep/link/here", "/antennarr/assets/"} {
		rec := get(h, "GET", target)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `id="root"`) {
			t.Errorf("%s: %d %q (the app must handle its own routes)", target, rec.Code, rec.Body)
		}
	}
}

func TestAssetsAreTypedAndCachedForever(t *testing.T) {
	h := handler("/antennarr/", built)
	cases := map[string]string{
		"/antennarr/assets/index-abc123.js":  "javascript",
		"/antennarr/assets/index-abc123.css": "text/css",
		"/antennarr/favicon.svg":             "image/svg+xml",
	}
	for target, wantType := range cases {
		rec := get(h, "GET", target)
		if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Type"), wantType) {
			t.Errorf("%s: %d type %q, want %q", target, rec.Code, rec.Header().Get("Content-Type"), wantType)
		}
	}
	if cc := get(h, "GET", "/antennarr/assets/index-abc123.js").Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("hashed assets should be cached forever, got %q", cc)
	}
	if cc := get(h, "GET", "/antennarr/favicon.svg").Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("unhashed files must be revalidated, got %q", cc)
	}
}

func TestMissingAssetIs404NotHTML(t *testing.T) {
	h := handler("/antennarr/", built)
	for _, target := range []string{"/antennarr/assets/gone.js", "/antennarr/nope.css", "/antennarr/x.png"} {
		rec := get(h, "GET", target)
		if rec.Code != 404 || strings.Contains(rec.Body.String(), "root") {
			t.Errorf("%s: %d %q", target, rec.Code, rec.Body)
		}
	}
}

func TestCannotEscapeTheUIDirectory(t *testing.T) {
	h := handler("/antennarr/", built)
	for _, target := range []string{"/antennarr/../etc/passwd", "/antennarr/assets/../../secret.txt", "/antennarr/..%2f..%2fsecret"} {
		rec := get(h, "GET", target)
		if strings.Contains(rec.Body.String(), "secret") || rec.Code == 500 {
			t.Errorf("%s: %d %q", target, rec.Code, rec.Body)
		}
	}
}

func TestOnlyReadsAreAllowed(t *testing.T) {
	h := handler("/antennarr/", built)
	if rec := get(h, "HEAD", "/antennarr/"); rec.Code != 200 || rec.Body.Len() != 0 {
		t.Errorf("HEAD: %d with %d body bytes", rec.Code, rec.Body.Len())
	}
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		rec := get(h, m, "/antennarr/")
		if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD" {
			t.Errorf("%s: %d Allow=%q", m, rec.Code, rec.Header().Get("Allow"))
		}
	}
}

func TestSecurityHeadersAreOnEverything(t *testing.T) {
	h := handler("/antennarr/", built)
	for _, target := range []string{"/antennarr/", "/antennarr/assets/index-abc123.js", "/antennarr/missing.js", "/antennarr/viewers"} {
		hdr := get(h, "GET", target).Header()
		csp := hdr.Get("Content-Security-Policy")
		for _, want := range []string{"default-src 'self'", "frame-ancestors 'none'", "connect-src 'self'", "base-uri 'none'"} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: CSP %q lacks %q", target, csp, want)
			}
		}
		if hdr.Get("X-Frame-Options") != "DENY" || hdr.Get("X-Content-Type-Options") != "nosniff" || hdr.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: headers = %v", target, hdr)
		}
	}
}

func TestExplainsWhenTheUIWasNotBuilt(t *testing.T) {
	empty := fstest.MapFS{".gitkeep": {}}
	h := handler("/antennarr/", empty)
	for _, target := range []string{"/antennarr/", "/antennarr/viewers"} {
		rec := get(h, "GET", target)
		body, _ := io.ReadAll(rec.Body)
		if rec.Code != 503 || !strings.Contains(string(body), "npm run build") || !strings.Contains(string(body), "/antennarr/api/") {
			t.Errorf("%s: %d %q", target, rec.Code, body)
		}
	}
	if rec := get(h, "GET", "/antennarr/missing.js"); rec.Code != 404 {
		t.Errorf("missing asset with no build: %d", rec.Code)
	}
}

// Whatever state the checkout is in, the embedded handler must work: 200 if
// someone has built the UI here, 503 with instructions if not.
func TestEmbeddedHandler(t *testing.T) {
	rec := get(Handler("/antennarr/"), "GET", "/antennarr/")
	if rec.Code != 200 && rec.Code != 503 {
		t.Errorf("status %d", rec.Code)
	}
	if rec.Code == 200 && !strings.Contains(rec.Body.String(), "<div id=\"root\">") {
		t.Errorf("built UI has no root element: %q", rec.Body)
	}
}
