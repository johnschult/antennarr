//go:build demo

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/access"
)

// TestDemo runs Antennarr against a made-up tuner and guide until interrupted,
// for taking the README screenshots (docs/screenshots/retake.sh). Not part of
// the normal tests: it only builds with -tags demo.
//
// The tuner is an HDHomeRun FLEX 4K with one stream from a viewer (Alex)
// through Antennarr and one or two local devices, plus three made-up viewers.
// The address it listens on is written to $DEMO_DIR/addr.
//
// The channels are two with plain badges, not any network's marks, unless
// $DEMO_LOGOS/channels.json lists others: a JSON array of {"number", "name",
// "logo", "title"}, where logo is a file in that folder. retake.sh makes one from
// a real tuner's guide (fetch-logos.py) so the screenshots can show real logos
// without them being kept in the repository.
// The first channel is what Alex watches; the next two are on local devices.

// demoChannel is one channel of the made-up lineup.
type demoChannel struct {
	Number string `json:"number"`
	Name   string `json:"name"`
	Logo   string `json:"logo"`  // a file in the logos folder
	Title  string `json:"title"` // what is on now
}

func demoChannels(logos string) []demoChannel {
	chs := []demoChannel{
		{"2.1", "WSB-HD", "wsb.png", "College Football"},
		{"69.1", "WUPA-HD", "wupa.png", "College Football"},
	}
	if b, err := os.ReadFile(filepath.Join(logos, "channels.json")); err == nil {
		var read []demoChannel
		if json.Unmarshal(b, &read) == nil && len(read) > 0 {
			chs = read
		}
	}
	return chs
}

// wobble varies a bit rate over time, as a real broadcast does.
func wobble(rate float64, phase float64) int {
	t := float64(time.Now().UnixNano()) / 1e9
	return int(rate * (1 + 0.09*math.Sin(t/2.3+phase) + 0.05*math.Sin(t*1.7+phase*3)))
}

func TestDemo(t *testing.T) {
	dir := os.Getenv("DEMO_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	logos := os.Getenv("DEMO_LOGOS")
	if logos == "" {
		logos = "../../docs/screenshots/demo"
	}
	chs := demoChannels(logos)
	var mu sync.Mutex
	remote := false // Alex is watching the first channel through Antennarr

	stream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		remote = true
		mu.Unlock()
		defer func() { mu.Lock(); remote = false; mu.Unlock() }()
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
				w.Write([]byte("TS"))
				w.(http.Flusher).Flush()
			}
		}
	}))
	defer stream.Close()
	_, sport, _ := net.SplitHostPort(strings.TrimPrefix(stream.URL, "http://"))

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/discover.json":
			fmt.Fprint(w, `{"DeviceAuth":"DEMO-AUTH","FriendlyName":"HDHomeRun FLEX 4K","ModelNumber":"HDFX-4K","FirmwareName":"hdhomerun_dvr_atsc3","FirmwareVersion":"20260326","DeviceID":"10ABCC87","TunerCount":4,"BaseURL":"http://x","LineupURL":"http://x/lineup.json"}`)
		case "/lineup.json":
			var items []string
			for _, c := range chs {
				items = append(items, fmt.Sprintf(`{"GuideNumber":%q,"GuideName":%q,"URL":"http://127.0.0.1:%s/auto/v%s"}`, c.Number, c.Name, sport, c.Number))
			}
			fmt.Fprintf(w, "[%s]", strings.Join(items, ","))
		case "/status.json":
			mu.Lock()
			rm := remote
			mu.Unlock()
			busy := func(tuner int, c demoChannel, freq int, ip string, rate float64, phase float64) string {
				return fmt.Sprintf(`{"Resource":"tuner%d","VctNumber":%q,"VctName":%q,"Frequency":%d,"SignalStrengthPercent":100,"SignalQualityPercent":100,"SymbolQualityPercent":100,"NetworkRate":%d,"TargetIP":%q}`, tuner, c.Number, c.Name, freq, wobble(rate, phase), ip)
			}
			slots := []string{`{"Resource":"tuner0"}`, `{"Resource":"tuner1"}`, `{"Resource":"tuner2"}`, `{"Resource":"tuner3"}`}
			if rm {
				slots[2] = busy(2, chs[0], 599000000, "127.0.0.1", 9500000, 0)
			}
			if len(chs) > 1 { // a local device on the second channel
				slots[3] = busy(3, chs[1], 527000000, "192.168.68.115", 6700000, 1.7)
			}
			if len(chs) > 2 { // and another on the third
				slots[0] = busy(0, chs[2], 539000000, "192.168.68.120", 8100000, 3.1)
			}
			fmt.Fprintf(w, "[%s]", strings.Join(slots, ","))
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	now := time.Now().UTC().Truncate(time.Minute)
	stamp := func(d time.Duration) string { return now.Add(d).Format("20060102150405") + " +0000" }
	var base string
	guide := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/xmltv":
			xml := `<?xml version="1.0" encoding="utf-8"?><tv>` + "\n"
			for i, c := range chs {
				xml += fmt.Sprintf(`<channel id="c%d"><display-name>%s</display-name><lcn>%s</lcn><icon src="https://img.hdhomerun.com/channels/%s"/></channel>`+"\n", i, c.Name, c.Number, c.Logo)
			}
			for i, c := range chs {
				title := c.Title
				if title == "" {
					title = "Evening News"
				}
				xml += `<programme start="` + stamp(-40*time.Minute) + `" stop="` + stamp(80*time.Minute) + fmt.Sprintf(`" channel="c%d"><title>%s</title></programme>`, i, title) + "\n"
			}
			xml += `</tv>`
			io.WriteString(w, strings.ReplaceAll(xml, "https://img.hdhomerun.com", base))
		case strings.HasPrefix(r.URL.Path, "/channels/"):
			b, err := os.ReadFile(filepath.Join(logos, filepath.Base(r.URL.Path)))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			w.Write(b)
		default:
			http.NotFound(w, r)
		}
	}))
	defer guide.Close()
	base = guide.URL

	// The usual port, so the pages show the address people will really use;
	// any free one if it is taken.
	ln, err := net.Listen("tcp", "127.0.0.1:5004")
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	s := access.DefaultSettings()
	s.OwnerReserve = 1
	s.Admin = []string{"192.0.2.0/24"}
	s.Viewers = []access.Viewer{
		{Name: "Alex", Addresses: []string{"127.0.0.1"}, Limit: 2},
		{Name: "Sam", Addresses: []string{"100.64.0.9"}, Limit: 1},
		{Name: "Jordan", Addresses: []string{"10.8.0.0/24"}, Limit: 1},
	}
	s.Labels = map[string]string{"antenna": "Living room antenna"}
	dev := access.Device{Name: "antenna", Listen: ln.Addr().String(), Tuner: "demo", ID: "10ABCC87", StreamPort: sport}
	s.Devices = []access.Device{dev}
	cfgPath := filepath.Join(dir, "antennarr.json")
	if err := access.SaveSettings(cfgPath, s); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	srv, err := New(ctx, Config{
		Settings: s, Devices: s.Devices, ConfigPath: cfgPath, Poll: time.Second, Log: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Locate:    func(context.Context, string, string) (string, error) { return api.URL, nil },
		Listeners: map[string]net.Listener{"antenna": ln}, GuideURL: guide.URL + "/api/xmltv", GuideIconBase: guide.URL,
		AdminListen: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Run(ctx)
	base2 := "http://" + ln.Addr().String()
	os.WriteFile(filepath.Join(dir, "addr"), []byte(base2), 0o644)
	// Alex watches the first channel through Antennarr.
	go func() {
		time.Sleep(time.Second)
		resp, err := http.Get(base2 + "/auto/v" + chs[0].Number)
		if err != nil {
			return
		}
		io.Copy(io.Discard, resp.Body)
	}()
	<-ctx.Done()
}
