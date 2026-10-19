// Package guide reads the program guide SiliconDust publishes for a tuner
// (XMLTV) so the dashboard can show what's on a channel, with its logo.
package guide

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Program is one show on one channel.
type Program struct {
	Title       string    `json:"title"`
	SubTitle    string    `json:"subTitle,omitempty"`
	Description string    `json:"description,omitempty"`
	Category    string    `json:"category,omitempty"` // Series, News, Movie...
	Episode     string    `json:"episode,omitempty"`  // as shown on screen: S01E32
	Start       time.Time `json:"start"`
	Stop        time.Time `json:"stop"`
	// Poster names the program's picture, for asking for it with Service.Poster: the part of its address
	// that identifies it. Empty when the guide has none. The address itself stays here, on the server.
	Poster string `json:"poster,omitempty"`
	icon   string // an https URL from the feed; never from a request
}

// Channel is what the guide knows about one channel, found by the number the
// tuner's lineup gives it (11.1).
type Channel struct {
	Number   string
	Name     string
	Icon     string // an https URL from the feed; never from a request
	Programs []Program
	About    About
}

// About is what the guide says about a channel as a whole: the network it carries, the languages its programs
// are in, and what it mostly shows. All of it comes from the guide's own words and nothing is inferred; whatever
// the guide doesn't say is left empty.
type About struct {
	Network    string   `json:"network,omitempty"`
	Languages  []string `json:"languages,omitempty"`  // language codes as the guide gives them (en, es), most programs first
	Categories []string `json:"categories,omitempty"` // what its programs are, most common first
}

const timeLayout = "20060102150405 -0700"

// Parse reads an XMLTV document into channels keyed by channel number. It
// streams, so a large guide is never held as a tree, and it keeps only what
// has not finished by `now`. Programs for channels it doesn't know, or with
// times it can't read, are skipped rather than failing the whole guide.
func Parse(r io.Reader, now time.Time) (map[string]*Channel, error) {
	dec := xml.NewDecoder(r)
	byID := map[string]*Channel{}
	progs := map[string][]Program{}
	tally := map[string]*tallies{}
	sawRoot := false

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("the guide isn't readable XML: %w", err)
		}
		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch el.Name.Local {
		case "tv":
			sawRoot = true
		case "channel":
			var c struct {
				ID    string   `xml:"id,attr"`
				Names []string `xml:"display-name"`
				LCN   string   `xml:"lcn"`
				Icon  struct {
					Src string `xml:"src,attr"`
				} `xml:"icon"`
			}
			if err := dec.DecodeElement(&c, &el); err != nil {
				return nil, fmt.Errorf("the guide has a broken channel: %w", err)
			}
			num := strings.TrimSpace(c.LCN)
			if num == "" || c.ID == "" {
				continue
			}
			ch := &Channel{Number: num, Icon: strings.TrimSpace(c.Icon.Src)}
			if len(c.Names) > 0 {
				ch.Name = strings.TrimSpace(c.Names[0])
			}
			ch.About.Network = networkOf(c.Names, num)
			byID[c.ID] = ch
		case "programme":
			var p struct {
				Start    string   `xml:"start,attr"`
				Stop     string   `xml:"stop,attr"`
				Channel  string   `xml:"channel,attr"`
				Title    string   `xml:"title"`
				SubTitle string   `xml:"sub-title"`
				Desc     string   `xml:"desc"`
				Category []string `xml:"category"`
				Language string   `xml:"language"`
				Icons    []struct {
					Src    string `xml:"src,attr"`
					Width  int    `xml:"width,attr"`
					Height int    `xml:"height,attr"`
				} `xml:"icon"`
				Episodes []struct {
					System string `xml:"system,attr"`
					Text   string `xml:",chardata"`
				} `xml:"episode-num"`
			}
			if err := dec.DecodeElement(&p, &el); err != nil {
				return nil, fmt.Errorf("the guide has a broken program: %w", err)
			}
			start, err1 := time.Parse(timeLayout, p.Start)
			stop, err2 := time.Parse(timeLayout, p.Stop)
			if err1 != nil || err2 != nil || !stop.After(start) || !stop.After(now) || strings.TrimSpace(p.Title) == "" {
				continue
			}
			prog := Program{
				Title: strings.TrimSpace(p.Title), SubTitle: strings.TrimSpace(p.SubTitle), Start: start.UTC(), Stop: stop.UTC(),
				Description: limit(strings.Join(strings.Fields(p.Desc), " "), maxDescription),
			}
			if len(p.Category) > 0 {
				prog.Category = limit(strings.TrimSpace(p.Category[0]), 40)
			}
			for _, e := range p.Episodes {
				if e.System == "onscreen" {
					prog.Episode = limit(strings.TrimSpace(e.Text), 20)
				}
			}
			// The picture to show is the first one that is wider than tall (a still, which suits a wide
			// tile), else the first there is.
			for _, ic := range p.Icons {
				if ic.Src == "" {
					continue
				}
				if prog.icon == "" {
					prog.icon = ic.Src
				}
				if ic.Width > ic.Height {
					prog.icon = ic.Src
					break
				}
			}
			prog.Poster = posterToken(prog.icon)
			progs[p.Channel] = append(progs[p.Channel], prog)
			t := tally[p.Channel]
			if t == nil {
				t = &tallies{langs: map[string]int{}, cats: map[string]int{}, shown: map[string]string{}}
				tally[p.Channel] = t
			}
			t.programs++
			if l := strings.ToLower(strings.TrimSpace(p.Language)); l != "" && len(l) <= 8 {
				t.langs[l]++
			}
			seen := map[string]bool{}
			for _, c := range p.Category {
				if c = strings.TrimSpace(c); c != "" && len(c) <= 40 && !seen[strings.ToLower(c)] {
					k := strings.ToLower(c)
					seen[k] = true
					t.cats[k]++
					if _, ok := t.shown[k]; !ok {
						t.shown[k] = c
					}
				}
			}
		}
	}
	if !sawRoot {
		return nil, errors.New("that isn't a guide (no <tv> element)")
	}

	out := make(map[string]*Channel, len(byID))
	for id, ch := range byID {
		ps := progs[id]
		sort.Slice(ps, func(i, j int) bool { return ps[i].Start.Before(ps[j].Start) })
		ch.Programs = ps
		if t := tally[id]; t != nil {
			ch.About.Languages, ch.About.Categories = t.languages(), t.categories()
		}
		out[ch.Number] = ch
	}
	if len(out) == 0 {
		return nil, errors.New("the guide has no channels")
	}
	return out, nil
}

// tallies counts what a channel's programs say about themselves.
type tallies struct {
	programs int
	langs    map[string]int
	cats     map[string]int    // by lowercase name, so News and news are one
	shown    map[string]string // the name as first written
}

// networkOf is the network a channel carries, when the guide gives one. A channel's names run: call sign, then the
// number and call sign, the number and name, the number alone, the name, and, only when it differs from the name,
// the network (ABC for WSB-HD). It is read as the name after the number's own entry, never by position alone.
func networkOf(names []string, number string) string {
	for i, n := range names {
		if strings.TrimSpace(n) == number && i+2 < len(names) {
			name, network := strings.TrimSpace(names[i+1]), strings.TrimSpace(names[i+2])
			if network != "" && !strings.EqualFold(name, network) {
				return limit(network, 60)
			}
			return ""
		}
	}
	return ""
}

// ranked is the keys of counts, most first, then by name, keeping those that pass keep.
func ranked(counts map[string]int, keep func(k string, n int) bool) []string {
	var out []string
	for k, n := range counts {
		if keep(k, n) {
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if counts[out[i]] != counts[out[j]] {
			return counts[out[i]] > counts[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

// languages are those of at least a tenth of the programs, so a stray program doesn't make a channel bilingual.
func (t *tallies) languages() []string {
	return ranked(t.langs, func(_ string, n int) bool { return n*10 >= t.programs })
}

// categories are the five most common, leaving out "Series", which says how a show is made, not what it is about.
func (t *tallies) categories() []string {
	c := ranked(t.cats, func(k string, _ int) bool { return !strings.EqualFold(k, "series") })
	if len(c) > 5 {
		c = c[:5]
	}
	for i, k := range c {
		c[i] = t.shown[k]
	}
	return c
}

// Now returns what is on at `at`, or nil in a gap in the guide.
func (c *Channel) Now(at time.Time) *Program {
	for i := range c.Programs {
		p := &c.Programs[i]
		if !p.Start.After(at) && p.Stop.After(at) {
			return p
		}
	}
	return nil
}

// maxDescription is the longest description kept: it rides along in every state update, and a tile only
// shows a couple of lines of it.
const maxDescription = 400

// limit cuts s to at most n bytes without splitting a character, marking the cut.
func limit(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return strings.TrimSpace(s) + "…"
}

// posterRe is what a picture's name may be made of: letters, digits, dash and underscore.
var posterRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// posterToken is the part of a picture's address that identifies it (C184032ENGD02 for
// https://img.hdhomerun.com/titles/C184032ENGD02.jpg), or "" if there isn't one that is plain enough to
// put in a request. It is only ever used to find the program again, never to build an address.
func posterToken(icon string) string {
	if icon == "" {
		return ""
	}
	u, err := url.Parse(icon)
	if err != nil {
		return ""
	}
	name := path.Base(u.Path)
	name = strings.TrimSuffix(name, path.Ext(name))
	if !posterRe.MatchString(name) {
		return ""
	}
	return name
}
