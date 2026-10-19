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
		out[ch.Number] = ch
	}
	if len(out) == 0 {
		return nil, errors.New("the guide has no channels")
	}
	return out, nil
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
