// Package httpx holds the small HTTP helpers shared by the JSON endpoints:
// replying in JSON, reading a request body strictly, and refusing cross-site
// writes.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
)

// MaxBody is the most a request body may be; everything here is tiny.
const MaxBody = 64 << 10

func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteErr(w http.ResponseWriter, code int, msg string) {
	WriteJSON(w, code, map[string]string{"error": msg})
}

// Decode reads a JSON body strictly: no unknown fields, nothing after the
// object, bounded size. It writes the error response itself and returns false
// if the body is unusable.
func Decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			WriteErr(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			WriteErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		}
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		WriteErr(w, http.StatusBadRequest, "invalid JSON: unexpected data after the object")
		return false
	}
	return true
}

// GuardWrites is the CSRF defense for endpoints that trust who is calling (by
// address or by cookie): a web page the owner happens to visit must not be
// able to change anything from their browser. Writes have to be JSON (which a
// cross-origin page can't send without a CORS preflight that we never
// approve), and an Origin header, if present, has to be this very host.
func GuardWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "" {
				if u, err := url.Parse(origin); err != nil || u.Host != r.Host {
					WriteErr(w, http.StatusForbidden, "cross-origin requests are not allowed")
					return
				}
			}
			// Bodyless writes (DELETE) have no type to check.
			if r.ContentLength != 0 {
				mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil || mt != "application/json" {
					WriteErr(w, http.StatusUnsupportedMediaType, "send application/json")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// CrossOriginForm is whether a form post comes from a page on another site. A page that is not the browser's own
// can't know a secret in the address, but the origin is checked as well, as the admin API does. A browser sends
// "Origin: null" when the page has no referrer policy to speak of; it is accepted only if the browser also says the
// request is same-origin. A request with no Origin header (not a browser, or an old one) is not cross-origin.
func CrossOriginForm(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	if origin == "null" && r.Header.Get("Sec-Fetch-Site") == "same-origin" {
		return false
	}
	u, err := url.Parse(origin)
	return err != nil || u.Host != r.Host
}
