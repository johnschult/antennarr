// Package tunerinfo reads what a tuner says about itself: its model, firmware and addresses, for the Device
// panel. It only ever reads, and only the handful of values named here: nothing a person types reaches
// the tuner.
package tunerinfo

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/johnschult/yagi/internal/hdhr"
)

// Info is what the tuner reported. A field that couldn't be read is empty, and Problems says why in
// words, so the panel shows what it has and doesn't guess the rest.
type Info struct {
	Model         string    `json:"model,omitempty"`         // its firmware family, such as hdhomerun_dvr_atsc3
	HardwareModel string    `json:"hardwareModel,omitempty"` // such as HDFX-4K
	Version       string    `json:"version,omitempty"`
	ChannelMaps   []string  `json:"channelMaps,omitempty"`
	Modulations   []string  `json:"modulations,omitempty"`
	DeviceID      string    `json:"deviceId,omitempty"`
	DRM           string    `json:"drm,omitempty"` // the ATSC 3.0 DRM version
	MAC           string    `json:"mac,omitempty"`
	IP            string    `json:"ip,omitempty"`
	Subnet        string    `json:"subnet,omitempty"`
	ReadAt        time.Time `json:"readAt"`
	Problems      []string  `json:"problems,omitempty"`
}

const (
	// systemPath is the tuner's own System Menu page. Some of what it shows (the DRM version, the MAC
	// address, the subnet mask) is not available over the control protocol.
	systemPath  = "/system.html"
	maxPage     = 64 << 10
	maxField    = 120
	maxFeatures = 2000
	cacheFor    = 30 * time.Second
)

// Reader reads tuners, remembering each answer for a short time so opening the panel (or several people
// opening it) doesn't keep asking the tuner.
type Reader struct {
	// Dial opens a control connection to a tuner's host. Required.
	Dial func(ctx context.Context, host string) (*hdhr.Conn, error)
	// Client fetches the System Menu page; http.DefaultClient's behavior is not used because the page
	// must not be followed anywhere else.
	Client *http.Client
	Now    func() time.Time

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	at   time.Time
	info Info
}

func (r *Reader) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Reader) client() *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// Read asks the tuner at baseURL about itself. key identifies the device for the short memory; refresh
// skips that memory.
func (r *Reader) Read(ctx context.Context, key, baseURL string, refresh bool) (Info, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Hostname() == "" {
		return Info{}, errors.New("Yagi doesn't know where the tuner is")
	}
	r.mu.Lock()
	if c, ok := r.cache[key]; ok && !refresh && r.now().Sub(c.at) < cacheFor {
		r.mu.Unlock()
		return c.info, nil
	}
	r.mu.Unlock()

	info := Info{ReadAt: r.now()}
	r.readControl(ctx, u.Hostname(), &info)
	r.readPage(ctx, baseURL, &info)
	if info.Model == "" && info.HardwareModel == "" && info.Version == "" && info.DRM == "" && info.MAC == "" {
		return Info{}, fmt.Errorf("couldn't read anything from the tuner: %s", strings.Join(info.Problems, "; "))
	}

	r.mu.Lock()
	if r.cache == nil {
		r.cache = map[string]cached{}
	}
	r.cache[key] = cached{at: info.ReadAt, info: info}
	r.mu.Unlock()
	return info, nil
}

// readControl reads the values the control protocol has. Each is its own request, so one the tuner
// doesn't know leaves the rest alone.
func (r *Reader) readControl(ctx context.Context, host string, info *Info) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	c, err := r.Dial(ctx, host)
	if err != nil {
		info.Problems = append(info.Problems, "the tuner's control port didn't answer")
		return
	}
	defer c.Close()
	get := func(name string, into *string) {
		v, err := c.Get(ctx, name)
		if err != nil {
			info.Problems = append(info.Problems, fmt.Sprintf("couldn't read %s", name))
			return
		}
		*into = clean(v, maxField)
	}
	get("/sys/model", &info.Model)
	get("/sys/hwmodel", &info.HardwareModel)
	get("/sys/version", &info.Version)
	// The feature list is several lines, so it is read as it comes and cut down to words by ParseFeatures.
	if v, err := c.Get(ctx, "/sys/features"); err != nil {
		info.Problems = append(info.Problems, "couldn't read /sys/features")
	} else if len(v) <= maxFeatures {
		f := hdhr.ParseFeatures(v)
		info.ChannelMaps, info.Modulations = f.ChannelMaps, f.Modulations
	}
}

// rows picks the label and value out of a System Menu table row.
var rowRe = regexp.MustCompile(`(?is)<tr[^>]*>\s*<td[^>]*>(.*?)</td>\s*<td[^>]*>(.*?)</td>\s*</tr>`)

var tagRe = regexp.MustCompile(`<[^>]*>`)

// readPage reads the System Menu page. If the page isn't there, or looks different from what is expected,
// those fields are left empty and Problems says so: the layout is the tuner's, not ours.
func (r *Reader) readPage(ctx context.Context, baseURL string, info *Info) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+systemPath, nil)
	if err != nil {
		return
	}
	resp, err := r.client().Do(req)
	if err != nil {
		info.Problems = append(info.Problems, "the tuner's System Menu page didn't answer")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		info.Problems = append(info.Problems, "the tuner has no System Menu page to read")
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPage))
	if err != nil {
		info.Problems = append(info.Problems, "the tuner's System Menu page was cut off")
		return
	}
	found := ParsePage(string(body))
	if len(found) == 0 {
		info.Problems = append(info.Problems, "the tuner's System Menu page isn't laid out as expected")
		return
	}
	info.DRM = found["ATSC3-DRM"]
	info.DeviceID = found["Device ID"]
	info.MAC = found["MAC Address"]
	info.Subnet = found["Subnet Mask"]
	if ip := found["IP Address"]; net.ParseIP(ip) != nil {
		info.IP = ip
	}
}

// ParsePage reads the two-column rows of the System Menu page as label and value, with tags removed and
// text unescaped. Long values are dropped.
func ParsePage(page string) map[string]string {
	out := map[string]string{}
	for _, m := range rowRe.FindAllStringSubmatch(page, -1) {
		label := clean(tagRe.ReplaceAllString(m[1], ""), maxField)
		value := clean(tagRe.ReplaceAllString(m[2], ""), maxField)
		if label != "" && value != "" {
			out[label] = value
		}
	}
	return out
}

// clean makes a value from the tuner safe to show: unescaped, trimmed, one line, and not long.
func clean(s string, max int) string {
	s = strings.Join(strings.Fields(html.UnescapeString(s)), " ")
	if len(s) > max {
		return ""
	}
	return s
}
