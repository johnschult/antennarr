package server

import (
	"net/http"
	"strings"
)

// legacyPrefix was the admin pages' address before the project was renamed.
// Bookmarks and links to it keep working: a GET is sent on to the new address
// (the rest of the path and the query kept) and anything else is left to 404.
//
// The pages were also under /yagi/ until 2026.10.13; that address is sent on the same way.
const (
	legacyPrefix  = "/antennarr/"
	renamedPrefix = "/yagi/"
)

func redirectLegacy(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/"+strings.TrimPrefix(r.URL.Path, legacyPrefix)+queryOf(r), http.StatusPermanentRedirect)
}

func redirectRenamed(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/"+strings.TrimPrefix(r.URL.Path, renamedPrefix)+queryOf(r), http.StatusPermanentRedirect)
}

func queryOf(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return ""
	}
	return "?" + r.URL.RawQuery
}
