// Package lineup reads and changes a tuner's channel lineup over its web side, as the tuner's own Channel
// Lineup page does: which channels it has found, which are enabled, disabled or favorites, and scanning for
// more. It only ever asks for the few addresses that page uses, and only changes a channel the tuner says it
// has.
package lineup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// State is what a channel is set to.
type State string

const (
	Enabled  State = "enabled"  // in the lineup that viewers and media servers get
	Favorite State = "favorite" // enabled, and starred
	Disabled State = "disabled" // left out of the lineup
)

// mark is how the tuner's page writes a state in a request: "-" for normal, "+" for a favorite, "x" for disabled.
var mark = map[State]string{Enabled: "-", Favorite: "+", Disabled: "x"}

// Valid says whether s is one of the three states.
func (s State) Valid() bool { _, ok := mark[s]; return ok }

// Channel is one channel the tuner has found.
type Channel struct {
	Number     string `json:"number"`
	Name       string `json:"name"`
	HD         bool   `json:"hd,omitempty"`
	Video      string `json:"video,omitempty"`
	Audio      string `json:"audio,omitempty"`
	DRM        bool   `json:"drm,omitempty"`
	State      State  `json:"state"`
	Subscribed bool   `json:"subscribed"` // false: the tuner has it but can't be told to play it, so it can't be changed
}

// Scan is what the tuner says about scanning for channels.
type Scan struct {
	InProgress bool     `json:"inProgress"`
	Possible   bool     `json:"possible"`
	Source     string   `json:"source,omitempty"`
	Sources    []string `json:"sources"`
	Progress   int      `json:"progress,omitempty"` // percent, while scanning
	Found      int      `json:"found,omitempty"`    // programs found so far
}

// NumberRe is what a channel number looks like (2.1, 11.11, 703): digits, and perhaps a dot and more digits.
var NumberRe = regexp.MustCompile(`^[0-9]{1,5}(\.[0-9]{1,5})?$`)

const (
	maxBody     = 4 << 20
	maxChannels = 5000
	timeout     = 8 * time.Second
)

// Client talks to a tuner's web side.
type Client struct {
	// HTTP is the client used; the default has a timeout and does not follow redirects.
	HTTP *http.Client
}

func (c Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (c Client) do(ctx context.Context, method, base, path string, query url.Values) ([]byte, error) {
	u := strings.TrimRight(base, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, errors.New("the tuner's address isn't usable")
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, errors.New("couldn't reach the tuner")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the tuner answered %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return nil, errors.New("the tuner's answer was too long or cut off")
	}
	return body, nil
}

// raw is a channel as the tuner writes it. A flag the tuner doesn't send is absent, which for Enabled and
// Subscribed means "yes".
type raw struct {
	GuideNumber string `json:"GuideNumber"`
	GuideName   string `json:"GuideName"`
	VideoCodec  string `json:"VideoCodec"`
	AudioCodec  string `json:"AudioCodec"`
	HD          int    `json:"HD"`
	DRM         int    `json:"DRM"`
	Favorite    int    `json:"Favorite"`
	Enabled     *int   `json:"Enabled"`
	Subscribed  *int   `json:"Subscribed"`
}

// Found lists every channel the tuner has found, enabled or not.
func (c Client) Found(ctx context.Context, base string) ([]Channel, error) {
	body, err := c.do(ctx, http.MethodGet, base, "/lineup.json", url.Values{"show": {"found"}})
	if err != nil {
		return nil, err
	}
	var rs []raw
	if err := json.Unmarshal(body, &rs); err != nil {
		return nil, errors.New("that isn't a channel lineup")
	}
	if len(rs) > maxChannels {
		return nil, errors.New("the tuner listed more channels than expected")
	}
	out := make([]Channel, 0, len(rs))
	for _, r := range rs {
		if !NumberRe.MatchString(r.GuideNumber) {
			continue // not something that could be asked about again
		}
		ch := Channel{
			Number: r.GuideNumber, Name: clip(r.GuideName, 60), HD: r.HD == 1, DRM: r.DRM == 1,
			Video: clip(r.VideoCodec, 16), Audio: clip(r.AudioCodec, 16), State: Enabled, Subscribed: r.Subscribed == nil || *r.Subscribed != 0,
		}
		switch {
		case r.Enabled != nil && *r.Enabled == 0:
			ch.State = Disabled
		case r.Favorite == 1:
			ch.State = Favorite
		}
		out = append(out, ch)
	}
	return out, nil
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n]
	}
	return s
}

// status is lineup_status.json.
type status struct {
	ScanInProgress int      `json:"ScanInProgress"`
	ScanPossible   int      `json:"ScanPossible"`
	Source         string   `json:"Source"`
	SourceList     []string `json:"SourceList"`
	Progress       int      `json:"Progress"`
	Found          int      `json:"Found"`
}

// Scan says whether the tuner is scanning, and whether it can.
func (c Client) Scan(ctx context.Context, base string) (Scan, error) {
	body, err := c.do(ctx, http.MethodGet, base, "/lineup_status.json", nil)
	if err != nil {
		return Scan{}, err
	}
	var s status
	if err := json.Unmarshal(body, &s); err != nil {
		return Scan{}, errors.New("that isn't a scan status")
	}
	sc := Scan{InProgress: s.ScanInProgress == 1, Possible: s.ScanPossible == 1, Source: clip(s.Source, 30), Sources: []string{}, Progress: min(max(s.Progress, 0), 100), Found: max(s.Found, 0)}
	for _, src := range s.SourceList {
		if src = clip(src, 30); src != "" && len(sc.Sources) < 10 {
			sc.Sources = append(sc.Sources, src)
		}
	}
	return sc, nil
}

// SetState sets a channel to a state. The number must look like a channel number and the state must be one of
// the three: nothing else is ever put in the request.
func (c Client) SetState(ctx context.Context, base, number string, st State) error {
	if !NumberRe.MatchString(number) {
		return errors.New("that isn't a channel number")
	}
	m, ok := mark[st]
	if !ok {
		return errors.New("that isn't a channel state")
	}
	_, err := c.do(ctx, http.MethodPost, base, "/lineup.post", url.Values{"favorite": {m + number}})
	return err
}

// StartScan begins scanning for channels from a source (Antenna, Cable): the tuners are taken for as long as it runs.
func (c Client) StartScan(ctx context.Context, base, source string) error {
	q := url.Values{"scan": {"start"}}
	if source != "" {
		q.Set("source", source)
	}
	_, err := c.do(ctx, http.MethodPost, base, "/lineup.post", q)
	return err
}

// AbortScan stops a scan.
func (c Client) AbortScan(ctx context.Context, base string) error {
	_, err := c.do(ctx, http.MethodPost, base, "/lineup.post", url.Values{"scan": {"abort"}})
	return err
}
