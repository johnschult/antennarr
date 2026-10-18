//go:build demo

package server

import (
	"context"
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
// through Antennarr and one local device, plus three made-up viewers. The
// channel logos are plain badges, not any network's marks. The address it
// listens on is written to $DEMO_DIR/addr.
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
	var mu sync.Mutex
	remote := false // Alex is watching 2.1 through Antennarr

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
			fmt.Fprintf(w, `[{"GuideNumber":"2.1","GuideName":"WSB-HD","URL":"http://127.0.0.1:%s/auto/v2.1"},{"GuideNumber":"69.1","GuideName":"WUPA-HD","URL":"http://127.0.0.1:%s/auto/v69.1"}]`, sport, sport)
		case "/status.json":
			mu.Lock()
			rm := remote
			mu.Unlock()
			t2 := `{"Resource":"tuner2"}`
			if rm {
				t2 = fmt.Sprintf(`{"Resource":"tuner2","VctNumber":"2.1","VctName":"WSB-HD","Frequency":599000000,"SignalStrengthPercent":100,"SignalQualityPercent":100,"SymbolQualityPercent":100,"NetworkRate":%d,"TargetIP":"127.0.0.1"}`, wobble(9500000, 0))
			}
			fmt.Fprintf(w, `[{"Resource":"tuner0"},{"Resource":"tuner1"},%s,{"Resource":"tuner3","VctNumber":"69.1","VctName":"WUPA-HD","Frequency":527000000,"SignalStrengthPercent":100,"SignalQualityPercent":100,"SymbolQualityPercent":100,"NetworkRate":%d,"TargetIP":"192.168.68.115"}]`, t2, wobble(6700000, 1.7))
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
			xml := `<?xml version="1.0" encoding="utf-8"?><tv>
<channel id="a"><display-name>WSB-HD</display-name><lcn>2.1</lcn><icon src="https://img.hdhomerun.com/channels/wsb.png"/></channel>
<channel id="b"><display-name>WUPA-HD</display-name><lcn>69.1</lcn><icon src="https://img.hdhomerun.com/channels/wupa.png"/></channel>
<programme start="` + stamp(-40*time.Minute) + `" stop="` + stamp(80*time.Minute) + `" channel="a"><title>College Football</title></programme>
<programme start="` + stamp(-40*time.Minute) + `" stop="` + stamp(80*time.Minute) + `" channel="b"><title>College Football</title></programme>
</tv>`
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
	// Alex watches 2.1 through Antennarr.
	go func() {
		time.Sleep(time.Second)
		resp, err := http.Get(base2 + "/auto/v2.1")
		if err != nil {
			return
		}
		io.Copy(io.Discard, resp.Body)
	}()
	<-ctx.Done()
}
