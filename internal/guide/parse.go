// Package guide reads the programme guide SiliconDust publishes for a tuner
// (XMLTV) so the dashboard can show what's on a channel, with its logo.
package guide

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// Programme is one show on one channel.
type Programme struct {
	Title    string    `json:"title"`
	SubTitle string    `json:"subTitle,omitempty"`
	Start    time.Time `json:"start"`
	Stop     time.Time `json:"stop"`
}

// Channel is what the guide knows about one channel, found by the number the
// tuner's lineup gives it (11.1).
type Channel struct {
	Number     string
	Name       string
	Icon       string // an https URL from the feed; never from a request
	Programmes []Programme
}

const timeLayout = "20060102150405 -0700"

// Parse reads an XMLTV document into channels keyed by channel number. It
// streams, so a large guide is never held as a tree, and it keeps only what
// has not finished by `now`. Programmes for channels it doesn't know, or with
// times it can't read, are skipped rather than failing the whole guide.
func Parse(r io.Reader, now time.Time) (map[string]*Channel, error) {
	dec := xml.NewDecoder(r)
	byID := map[string]*Channel{}
	progs := map[string][]Programme{}
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
				Start    string `xml:"start,attr"`
				Stop     string `xml:"stop,attr"`
				Channel  string `xml:"channel,attr"`
				Title    string `xml:"title"`
				SubTitle string `xml:"sub-title"`
			}
			if err := dec.DecodeElement(&p, &el); err != nil {
				return nil, fmt.Errorf("the guide has a broken programme: %w", err)
			}
			start, err1 := time.Parse(timeLayout, p.Start)
			stop, err2 := time.Parse(timeLayout, p.Stop)
			if err1 != nil || err2 != nil || !stop.After(start) || !stop.After(now) || strings.TrimSpace(p.Title) == "" {
				continue
			}
			progs[p.Channel] = append(progs[p.Channel], Programme{
				Title: strings.TrimSpace(p.Title), SubTitle: strings.TrimSpace(p.SubTitle), Start: start.UTC(), Stop: stop.UTC(),
			})
		}
	}
	if !sawRoot {
		return nil, errors.New("that isn't a guide (no <tv> element)")
	}

	out := make(map[string]*Channel, len(byID))
	for id, ch := range byID {
		ps := progs[id]
		sort.Slice(ps, func(i, j int) bool { return ps[i].Start.Before(ps[j].Start) })
		ch.Programmes = ps
		out[ch.Number] = ch
	}
	if len(out) == 0 {
		return nil, errors.New("the guide has no channels")
	}
	return out, nil
}

// Now returns what is on at `at`, or nil in a gap in the guide.
func (c *Channel) Now(at time.Time) *Programme {
	for i := range c.Programmes {
		p := &c.Programmes[i]
		if !p.Start.After(at) && p.Stop.After(at) {
			return p
		}
	}
	return nil
}
