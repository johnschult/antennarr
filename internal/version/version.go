// Package version says which Antennarr this is.
//
// Releases are numbered by calendar, YYYY.MM.PATCH (2026.10.0 is the first
// release of October 2026, 2026.10.1 a fix to it), and tagged vYYYY.MM.PATCH.
// The number is put in at build time:
//
//	go build -ldflags "-X github.com/johnschult/antennarr/internal/version.Version=2026.10.0"
package version

import (
	"regexp"
	"runtime/debug"
)

// Repo is where the project lives.
const Repo = "https://github.com/johnschult/antennarr"

// Version is set by the build. It stays "dev" for a build from a checkout.
var Version = "dev"

var calver = regexp.MustCompile(`^\d{4}\.(0?[1-9]|1[0-2])\.\d+$`)

// Get is the version to show: the release number, or "dev" (with the commit it
// was built from, when Go recorded one).
func Get() string {
	if Version != "" && Version != "dev" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return "dev+" + s.Value[:7]
			}
		}
	}
	return "dev"
}

// IsRelease says whether v is a release number (as opposed to a dev build).
func IsRelease(v string) bool { return calver.MatchString(v) }

// URL is the page for this version on GitHub: its release notes for a release,
// the project page otherwise.
func URL(v string) string {
	if IsRelease(v) {
		return Repo + "/releases/tag/v" + v
	}
	return Repo
}

// DocsURL is the documentation for this version on GitHub: the pages as they
// were at its release tag, so what you read matches what is running. A dev
// build gets the latest pages.
func DocsURL(v string) string {
	if IsRelease(v) {
		return Repo + "/blob/v" + v + "/docs/README.md"
	}
	return Repo + "/blob/main/docs/README.md"
}
