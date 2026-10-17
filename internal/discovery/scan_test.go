package discovery

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func hdhr(id string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/discover.json" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"FriendlyName":"Fake","DeviceID":"`+id+`","TunerCount":4}`)
	}))
}

func hostOf(s *httptest.Server) string { return strings.TrimPrefix(s.URL, "http://") }

var client = &http.Client{Timeout: time.Second}

func TestProbe(t *testing.T) {
	srv := hdhr("10abcc87") // lower case on the wire
	defer srv.Close()

	d, err := Probe(context.Background(), client, srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "10ABCC87" || d.TunerCount != 4 || d.BaseURL != srv.URL {
		t.Errorf("device = %+v", d)
	}
}

func TestProbeRejectsNonTuners(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"html":       func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "<html>router login</html>") },
		"500":        func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 500) },
		"no id":      func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"FriendlyName":"x"}`) },
		"json array": func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `[]`) },
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			if _, err := Probe(context.Background(), client, srv.URL); err == nil {
				t.Error("probe accepted something that isn't an HDHomeRun")
			}
		})
	}
}

func TestProbeUnreachable(t *testing.T) {
	srv := hdhr("10ABCC87")
	url := srv.URL
	srv.Close()
	if _, err := Probe(context.Background(), client, url); err == nil {
		t.Error("expected an error for a closed server")
	}
}

func TestScanHostsFindsOnlyTuners(t *testing.T) {
	a, b := hdhr("10ABCC87"), hdhr("1111AAA2")
	notTuner := httptest.NewServer(http.NotFoundHandler())
	defer a.Close()
	defer b.Close()
	defer notTuner.Close()

	got := ScanHosts(context.Background(), []string{hostOf(a), hostOf(notTuner), "127.0.0.1:1", hostOf(b)}, "")
	ids := map[string]bool{}
	for _, d := range got {
		ids[d.ID] = true
	}
	if len(got) != 2 || !ids["10ABCC87"] || !ids["1111AAA2"] {
		t.Errorf("got %+v", got)
	}
}

func TestScanHostsStopsEarlyOnWantedID(t *testing.T) {
	want := hdhr("10ABCC87")
	defer want.Close()
	hosts := []string{hostOf(want)}
	for i := 0; i < 2000; i++ { // a long tail of dead ports to skip
		hosts = append(hosts, "127.0.0.1:1")
	}
	start := time.Now()
	got := ScanHosts(context.Background(), hosts, "10abcc87")
	if len(got) == 0 || got[0].ID != "10ABCC87" {
		t.Fatalf("got %+v", got)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("kept scanning after finding the wanted tuner")
	}
}

func TestScanHostsHonoursContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := ScanHosts(ctx, []string{"127.0.0.1:1", "127.0.0.1:2"}, ""); len(got) != 0 {
		t.Errorf("got %+v from a cancelled scan", got)
	}
}

func TestHostsIn(t *testing.T) {
	cases := []struct {
		prefix      string
		n           int
		first, last string
	}{
		{"192.168.1.0/24", 254, "192.168.1.1", "192.168.1.254"},
		{"192.168.1.77/24", 254, "192.168.1.1", "192.168.1.254"}, // host bits are masked off
		{"10.0.0.0/30", 2, "10.0.0.1", "10.0.0.2"},
		{"10.0.0.5/32", 0, "", ""},
	}
	for _, c := range cases {
		got := hostsIn(netip.MustParsePrefix(c.prefix))
		if len(got) != c.n {
			t.Errorf("%s: %d hosts, want %d", c.prefix, len(got), c.n)
			continue
		}
		if c.n > 0 && (got[0] != c.first || got[len(got)-1] != c.last) {
			t.Errorf("%s: %s..%s, want %s..%s", c.prefix, got[0], got[len(got)-1], c.first, c.last)
		}
	}
}

func TestHostsInIsCapped(t *testing.T) {
	if n := len(hostsIn(netip.MustParsePrefix("10.0.0.0/8"))); n > 65534 {
		t.Errorf("a /8 produced %d hosts", n)
	}
}

func TestPrefixAround(t *testing.T) {
	p, ok := PrefixAround(net.ParseIP("192.168.68.143"))
	if !ok || p.String() != "192.168.68.0/24" {
		t.Errorf("got %v ok=%v", p, ok)
	}
	if _, ok := PrefixAround(net.ParseIP("fe80::1")); ok {
		t.Error("IPv6 should not produce a /24")
	}
}
