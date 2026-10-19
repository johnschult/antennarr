package firmware

import (
	"regexp"
	"strings"
)

// Device is what a tuner says about itself in discover.json.
type Device struct {
	Model            string // ModelNumber, e.g. "HDFX-4K"
	FirmwareName     string // e.g. "hdhomerun_dvr_atsc3"
	Version          string // FirmwareVersion, e.g. "20260326"
	UpgradeAvailable string // set by the tuner itself when it knows of a newer version
}

// applies says whether a note is for a given tuner.
type applies int

const (
	no    applies = iota // the note is for other models
	maybe                // the scope names something we don't recognize
	yes                  // for this model, or for all of them
)

// profile is what we can tell about a tuner from its model number and
// firmware name: words that a changelog scope might use to mean it.
type profile struct {
	model   string
	aliases map[string]bool
	known   bool // the model number is one we recognize
}

// families maps the start of a model number to the names SiliconDust uses for
// that family in the changelog.
var families = []struct {
	prefix string
	names  []string
}{
	{"hdfx", []string{"hdfx", "flex", "dvr", "record"}},
	{"hdvr", []string{"hdvr", "scribe", "dvr", "record"}},
	{"hhdd", []string{"hhdd", "scribe", "dvr", "record"}},
	{"hdhr5", []string{"hdhr5"}},
	{"hdhr4", []string{"hdhr4"}},
	{"hdhr3", []string{"hdhr3", "legacy"}},
	{"hdtc", []string{"hdtc", "tc"}},
	{"hdhr-", []string{"hdhr"}},
	{"hdhr", []string{"hdhr"}},
}

func newProfile(d Device) profile {
	p := profile{model: strings.ToLower(strings.TrimSpace(d.Model)), aliases: map[string]bool{}}
	add := func(ws ...string) {
		for _, w := range ws {
			if w != "" {
				p.aliases[w] = true
			}
		}
	}
	add(p.model)
	add(strings.Split(p.model, "-")...)
	for _, f := range families {
		if strings.HasPrefix(p.model, f.prefix) {
			add(f.names...)
			p.known = true
			break
		}
	}
	if strings.Contains(p.model, "4k") {
		add("4k")
	}
	// "hdhomerun_dvr_atsc3" says it is a DVR with ATSC 3.0.
	for _, part := range strings.Split(strings.ToLower(d.FirmwareName), "_") {
		if part == "" || part == "hdhomerun" {
			continue
		}
		add(part)
		if strings.HasPrefix(part, "atsc") {
			add("atsc")
		}
	}
	return p
}

var (
	// words that qualify a scope but aren't a model name
	ignoredWords = map[string]bool{"model": true, "models": true, "only": true, "series": true, "and": true}
	// names that appear in scopes and mean particular hardware: if a scope uses
	// only these and none is ours, the note is for someone else.
	knownWords = map[string]bool{
		"atsc": true, "atsc3": true, "dvr": true, "record": true, "flex": true, "scribe": true, "servio": true,
		"tc": true, "cc": true, "dc": true, "eu": true, "us": true, "dt": true, "dvb": true, "dvbt": true, "dvbc": true,
		"tech": true, "dev": true, "legacy": true, "cablecard": true, "4k": true, "hdhr": true,
	}
	// words that pick out a subset of whatever else a scope names
	narrowingWords = map[string]bool{"tech": true, "dev": true}
	modelWordRe    = regexp.MustCompile(`^(hdhr|hdtc|hdfx|hdvr|hhdd)[a-z0-9-]*$|^\d[a-z]{1,3}$`)
)

func isKnown(tok string) bool { return knownWords[tok] || modelWordRe.MatchString(tok) }

// matches reports whether a scope word means this tuner.
func (p profile) matches(tok string) bool {
	if p.aliases[tok] {
		return true
	}
	// "2DT" and "4K" name part of a model number.
	return len(tok) >= 2 && strings.Contains(p.model, tok)
}

func (p profile) appliesTo(n Note) applies {
	if n.Scope == "" {
		return yes
	}
	var toks []string
	for _, t := range scopeTokens(strings.ToLower(n.Scope)) {
		if !ignoredWords[t] {
			toks = append(toks, t)
		}
	}
	if len(toks) == 0 {
		return yes
	}
	// "ATSC3 TECH/DEV models" means the TECH and DEV models among the ATSC3
	// ones, not every ATSC3 model: these words narrow a scope, so one of them
	// has to match as well.
	var narrowing []string
	for _, t := range toks {
		if narrowingWords[t] {
			narrowing = append(narrowing, t)
		}
	}
	if len(narrowing) > 0 {
		matched := false
		for _, t := range narrowing {
			matched = matched || p.matches(t)
		}
		if !matched {
			if p.known {
				return no
			}
			return maybe
		}
	}
	allKnown := true
	for _, t := range toks {
		switch {
		case t == "all":
			return yes
		case p.matches(t):
			return yes
		case !isKnown(t):
			allKnown = false
		}
	}
	if allKnown && p.known {
		return no
	}
	return maybe
}
