// Package ui serves the web UI that is built from /web and embedded in the
// binary, so Antennarr stays a single file.
package ui

import (
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// dist holds the output of `npm run build` in /web. Only .gitkeep is committed,
// so a fresh checkout still compiles (the UI just reports that it isn't built).
//
//go:embed all:dist
var dist embed.FS

// Handler serves the embedded UI at prefix (e.g. "/antennarr/").
func Handler(prefix string) http.Handler {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // dist is embedded at compile time; it can't be missing
	}
	return handler(prefix, root)
}

const notBuilt = `The Antennarr web UI has not been built into this binary.

Build it with:   cd web && npm install && npm run build
then rebuild Antennarr. The JSON API under /antennarr/api/ works without it.
`

// csp is strict because this page can change who may use the tuner: nothing
// loads from anywhere else and nothing can frame it. Styles need
// 'unsafe-inline' for React's style attributes and the toast library.
const csp = "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
	"connect-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

func handler(prefix string, root fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(r.URL.Path, prefix)), "/")
		if name == "" {
			name = "index.html"
		}

		if info, err := fs.Stat(root, name); err == nil && !info.IsDir() {
			serve(w, r, root, name)
			return
		}
		// A missing file with an extension is a broken asset link: say so
		// rather than answering with HTML that the browser will choke on.
		if path.Ext(name) != "" && name != "index.html" {
			http.NotFound(w, r)
			return
		}
		// Anything else is a page of the single-page app (client-side routing).
		if _, err := fs.Stat(root, "index.html"); err != nil {
			h.Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			io.WriteString(w, notBuilt)
			return
		}
		serve(w, r, root, "index.html")
	})
}

func serve(w http.ResponseWriter, r *http.Request, root fs.FS, name string) {
	f, err := root.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		http.Error(w, "cannot serve file", http.StatusInternalServerError)
		return
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if strings.HasPrefix(name, "assets/") {
		// Build output is named by content hash, so it never changes.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, name, time.Time{}, rs)
}
