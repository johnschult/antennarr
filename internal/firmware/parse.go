// Package firmware tells you when SiliconDust has released firmware for your
// tuners. It reads their public changelog page, works out which releases apply
// to each model, and says so honestly when it can't tell.
package firmware

import (
	"errors"
	"html"
	"io"
	"regexp"
	"strings"
)

// Note is one line of a release's changes.
type Note struct {
	// Scope is the models the line says it is for, as written, e.g. "ATSC3
	// models" or "HDHR5-2DT/4DT". Empty means the line doesn't say, which in
	// practice means it applies to everything.
	Scope string `json:"scope,omitempty"`
	Text  string `json:"text"`
}

// Release is one firmware release from the changelog.
type Release struct {
	Version string `json:"version"` // YYYYMMDD, which sorts as text
	Notes   []Note `json:"notes"`
}

var (
	releaseRe = regexp.MustCompile(`(?is)<h4[^>]*>\s*Release\s+(\d{8})\s*:?\s*</h4>`)
	liRe      = regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li>`)
	tagRe     = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRe   = regexp.MustCompile(`\s+`)
	scopeRe   = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9 /,&.\-]{0,39}):\s+(\S.*)$`)
)

// maxPage bounds how much of the page is read; the real one is about 60 KB.
const maxPage = 2 << 20

// Parse reads the changelog page. Releases come back in page order, which is
// newest first. It fails, rather than guessing, if no releases can be found,
// since that means the page's layout has changed.
func Parse(r io.Reader) ([]Release, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxPage))
	if err != nil {
		return nil, err
	}
	page := string(raw)

	heads := releaseRe.FindAllStringSubmatchIndex(page, -1)
	if len(heads) == 0 {
		return nil, errors.New("no firmware releases found on the page (has its layout changed?)")
	}
	out := make([]Release, 0, len(heads))
	for i, h := range heads {
		end := len(page)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		rel := Release{Version: page[h[2]:h[3]]}
		for _, m := range liRe.FindAllStringSubmatch(page[h[1]:end], -1) {
			if n, ok := parseNote(m[1]); ok {
				rel.Notes = append(rel.Notes, n)
			}
		}
		out = append(out, rel)
	}
	return out, nil
}

// parseNote cleans one list item and splits off a model scope if it has one.
func parseNote(markup string) (Note, bool) {
	text := spaceRe.ReplaceAllString(html.UnescapeString(tagRe.ReplaceAllString(markup, "")), " ")
	text = strings.TrimSpace(text)
	if text == "" {
		return Note{}, false
	}
	if m := scopeRe.FindStringSubmatch(text); m != nil && looksLikeScope(m[1]) {
		return Note{Scope: strings.TrimSpace(m[1]), Text: m[2]}, true
	}
	return Note{Text: text}, true
}

// looksLikeScope separates "ATSC3 models" and "HDHR5-2DT/4DT" from an ordinary
// sentence that happens to contain a colon, or a topic like "Security".
func looksLikeScope(s string) bool {
	if strings.Contains(strings.ToLower(s), "model") {
		return true
	}
	for _, tok := range scopeTokens(s) {
		if tok != strings.ToUpper(tok) { // "Security", "Fix" ... are words, not model names
			return false
		}
	}
	return len(scopeTokens(s)) > 0
}

// scopeTokens splits a scope into its words: "HDFX/HDVR/HHDD models" gives
// HDFX, HDVR, HHDD, models.
func scopeTokens(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == '/' || r == ',' || r == '&' || r == ' ' })
}
