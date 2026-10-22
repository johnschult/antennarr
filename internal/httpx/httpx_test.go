package httpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSONAndErr(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, 201, map[string]int{"a": 1})
	if rec.Code != 201 || rec.Header().Get("Content-Type") != "application/json" || rec.Header().Get("Cache-Control") != "no-store" || strings.TrimSpace(rec.Body.String()) != `{"a":1}` {
		t.Errorf("%d %v %q", rec.Code, rec.Header(), rec.Body)
	}
	rec = httptest.NewRecorder()
	WriteErr(rec, 409, "nope")
	if rec.Code != 409 || strings.TrimSpace(rec.Body.String()) != `{"error":"nope"}` {
		t.Errorf("%d %q", rec.Code, rec.Body)
	}
}

func decode(body string) (code int, ok bool, got struct{ A int }) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	ok = Decode(rec, req, &got)
	return rec.Code, ok, got
}

func TestDecodeIsStrict(t *testing.T) {
	if code, ok, got := decode(`{"A":5}`); !ok || got.A != 5 || code != 200 {
		t.Errorf("good body: %d %v %+v", code, ok, got)
	}
	for name, body := range map[string]string{
		"malformed": `{"A":`, "unknown field": `{"B":1}`, "wrong type": `{"A":"x"}`, "trailing": `{"A":1} {}`, "empty": ``,
	} {
		if code, ok, _ := decode(body); ok || code != 400 {
			t.Errorf("%s: ok=%v code=%d", name, ok, code)
		}
	}
	if code, ok, _ := decode(`{"A":1,"pad":"` + strings.Repeat("x", MaxBody) + `"}`); ok || code != 413 {
		t.Errorf("oversized: ok=%v code=%d", ok, code)
	}
}

func TestGuardWrites(t *testing.T) {
	h := GuardWrites(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	do := func(method, ct, origin, body string) int {
		req := httptest.NewRequest(method, "http://yagi.local/x", strings.NewReader(body))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for _, c := range []struct {
		name                     string
		method, ct, origin, body string
		want                     int
	}{
		{"read needs nothing", "GET", "", "", "", 200},
		{"read ignores origin", "GET", "", "https://evil.example", "", 200},
		{"json write", "POST", "application/json", "", `{}`, 200},
		{"json with charset", "POST", "application/json; charset=utf-8", "", `{}`, 200},
		{"same origin", "POST", "application/json", "http://yagi.local", `{}`, 200},
		{"text/plain write", "POST", "text/plain", "", `{}`, 415},
		{"form write", "POST", "application/x-www-form-urlencoded", "", `a=b`, 415},
		{"no type, with body", "POST", "", "", `{}`, 415},
		{"cross-origin write", "POST", "application/json", "https://evil.example", `{}`, 403},
		{"opaque origin", "POST", "application/json", "null", `{}`, 403},
		{"bodyless delete", "DELETE", "", "", "", 200},
		{"cross-origin bodyless delete", "DELETE", "", "https://evil.example", "", 403},
	} {
		if got := do(c.method, c.ct, c.origin, c.body); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

func TestCrossOriginForm(t *testing.T) {
	for name, c := range map[string]struct {
		origin, fetchSite, host string
		want                    bool
	}{
		"no origin (not a browser)":         {"", "", "yagi:9244", false},
		"this very host":                    {"http://yagi:9244", "", "yagi:9244", false},
		"another port is another site":      {"http://yagi:5004", "", "yagi:9244", true},
		"another host":                      {"http://evil.example", "cross-site", "yagi:9244", true},
		"null, and the browser says so":     {"null", "same-origin", "yagi:9244", false},
		"null from somewhere else":          {"null", "cross-site", "yagi:9244", true},
		"null with nothing to vouch for it": {"null", "", "yagi:9244", true},
		"not an origin at all":              {"::::", "", "yagi:9244", true},
	} {
		r := httptest.NewRequest("POST", "http://"+c.host+"/x", nil)
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.fetchSite != "" {
			r.Header.Set("Sec-Fetch-Site", c.fetchSite)
		}
		if got := CrossOriginForm(r); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}
