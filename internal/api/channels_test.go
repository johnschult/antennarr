package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/lineup"
	"github.com/johnschult/antennarr/internal/lineup/linetest"
)

// toServer sends every request to srv, whatever address it was made to: the monitor's tuner is a different
// fake, and the lineup is a second one standing in for the same tuner.
type toServer struct{ to *url.URL }

func (t toServer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host = t.to.Scheme, t.to.Host
	return http.DefaultTransport.RoundTrip(r)
}

const chanPath = "/antennarr/api/devices/antenna/channels"
const scanPath = "/antennarr/api/devices/antenna/channel-scan"

func channelsEnv(t *testing.T, tunerStatus string) (*env, *linetest.Server) {
	t.Helper()
	e := newEnvWith(t, johnAndMom, true, tunerStatus)
	srv := linetest.New([]linetest.Channel{
		{Number: "2.1", Name: "WSB-HD", HD: true},
		{Number: "2.2", Name: "Laff", State: "favorite"},
		{Number: "10.3", Name: "Old", State: "disabled"},
		{Number: "703", Name: "Locked", Unsubscribed: true},
	}, 400*time.Millisecond)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	e.api.Lineup = &lineup.Client{HTTP: &http.Client{Transport: toServer{u}, Timeout: 5 * time.Second}}
	return e, srv
}

func posts(s *linetest.Server) int {
	n := 0
	for _, r := range s.Requests() {
		if strings.HasPrefix(r, "POST") {
			n++
		}
	}
	return n
}

func TestChannelsAreListedWithTheirStateAndTheScan(t *testing.T) {
	e, _ := channelsEnv(t, idleTuner)
	code, v := e.call(t, "GET", chanPath, "")
	d := obj(t, v)
	chans, _ := d["channels"].([]any)
	if code != 200 || len(chans) != 4 {
		t.Fatalf("%d %v", code, v)
	}
	states := map[string]string{}
	for _, c := range chans {
		m := obj(t, c)
		states[m["number"].(string)] = m["state"].(string)
	}
	if states["2.1"] != "enabled" || states["2.2"] != "favorite" || states["10.3"] != "disabled" {
		t.Errorf("states = %v", states)
	}
	scan := obj(t, d["scan"])
	if scan["possible"] != true || scan["inProgress"] != false || scan["source"] != "Antenna" {
		t.Errorf("scan = %v", scan)
	}
	if _, leaked := obj(t, chans[0])["URL"]; leaked {
		t.Error("the tuner's stream address was passed on")
	}
}

func TestAChannelCanBeDisabledAndEnabledAgain(t *testing.T) {
	e, srv := channelsEnv(t, idleTuner)
	code, v := e.call(t, "PUT", chanPath+"/2.1", `{"state":"disabled"}`)
	if code != 200 || obj(t, v)["state"] != "disabled" || srv.State("2.1") != "disabled" {
		t.Fatalf("%d %v (the tuner has %q)", code, v, srv.State("2.1"))
	}
	code, v = e.call(t, "PUT", chanPath+"/2.1", `{"state":"favorite"}`)
	if code != 200 || srv.State("2.1") != "favorite" {
		t.Fatalf("%d %v", code, v)
	}
	if code, _ = e.call(t, "PUT", chanPath+"/2.1", `{"state":"enabled"}`); code != 200 || srv.State("2.1") != "enabled" {
		t.Fatalf("back to enabled: %d %q", code, srv.State("2.1"))
	}
}

func TestChannelRefusalsSendNothingToTheTuner(t *testing.T) {
	e, srv := channelsEnv(t, idleTuner)
	cases := []struct {
		name, number, body string
		want               int
	}{
		{"a state that isn't one", "2.1", `{"state":"gone"}`, 400},
		{"no state", "2.1", `{}`, 400},
		{"a symbol for a state", "2.1", `{"state":"x"}`, 400},
		{"a number that isn't one", "2.1&scan=start", `{"state":"disabled"}`, 400},
		{"a channel the tuner doesn't have", "99.9", `{"state":"disabled"}`, 404},
		{"a channel the tuner can't play", "703", `{"state":"disabled"}`, 409},
	}
	for _, c := range cases {
		srv.Forget()
		code, v := e.call(t, "PUT", chanPath+"/"+url.PathEscape(c.number), c.body)
		if code != c.want {
			t.Errorf("%s: %d %v, want %d", c.name, code, v, c.want)
		}
		if posts(srv) != 0 {
			t.Errorf("%s: a request was sent to the tuner: %v", c.name, srv.Requests())
		}
	}
}

func TestATunerThatDoesNotTakeAChangeIsSaidNotBelieved(t *testing.T) {
	e, srv := channelsEnv(t, idleTuner)
	srv.IgnoreChanges = true
	code, v := e.call(t, "PUT", chanPath+"/2.1", `{"state":"disabled"}`)
	if code != 502 || !strings.Contains(errText(v), "didn't change it") {
		t.Errorf("%d %v", code, v)
	}
}

func TestChannelsOfAnOfflineTunerAreNotAsked(t *testing.T) {
	e, srv := channelsEnv(t, idleTuner)
	e.tun.Close()
	for i := 0; i < 4; i++ {
		e.mon.Poll(context.Background())
	}
	srv.Forget()
	for _, c := range [][3]string{{"GET", chanPath, ""}, {"PUT", chanPath + "/2.1", `{"state":"disabled"}`}, {"POST", scanPath, `{"interrupt":true}`}} {
		code, v := e.call(t, c[0], c[1], c[2])
		if code != 409 || !strings.Contains(errText(v), "offline") {
			t.Errorf("%s: %d %v", c[0], code, v)
		}
	}
	if len(srv.Requests()) != 0 {
		t.Errorf("asked an offline tuner: %v", srv.Requests())
	}
}

func TestUnknownTunerAndNoLineupClient(t *testing.T) {
	e, _ := channelsEnv(t, idleTuner)
	if code, _ := e.call(t, "GET", "/antennarr/api/devices/nope/channels", ""); code != 404 {
		t.Errorf("unknown tuner: %d", code)
	}
	e.api.Lineup = nil
	if code, _ := e.call(t, "GET", chanPath, ""); code != 409 {
		t.Errorf("unavailable: %d", code)
	}
}

func TestChannelsAreForAdminsOnly(t *testing.T) {
	e, srv := channelsEnv(t, idleTuner)
	for _, c := range [][2]string{{"GET", chanPath}, {"PUT", chanPath + "/2.1"}, {"POST", scanPath}, {"DELETE", scanPath}} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(c[0], c[1], strings.NewReader(`{"state":"disabled"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "100.67.117.32:1" // John is a viewer, not an admin
		e.pol.AdminOnly(e.api.Handler()).ServeHTTP(rec, req)
		if rec.Code != 403 {
			t.Errorf("%s %s: %d", c[0], c[1], rec.Code)
		}
	}
	if len(srv.Requests()) != 0 {
		t.Errorf("the tuner was asked on a viewer's behalf: %v", srv.Requests())
	}
}

func TestAScanCanBeStartedAndStopped(t *testing.T) {
	e, srv := channelsEnv(t, idleTuner)
	code, v := e.call(t, "POST", scanPath, `{"source":"Cable"}`)
	if code != 202 || obj(t, v)["source"] != "Cable" || !srv.Scanning() {
		t.Fatalf("%d %v scanning=%v", code, v, srv.Scanning())
	}
	_, g := e.call(t, "GET", chanPath, "")
	if sc := obj(t, obj(t, g)["scan"]); sc["inProgress"] != true || sc["source"] != "Cable" {
		t.Errorf("scan = %v", sc)
	}
	if code, v := e.call(t, "POST", scanPath, `{}`); code != 409 || !strings.Contains(errText(v), "already running") {
		t.Errorf("a second: %d %v", code, v)
	}
	if code, _ := e.call(t, "DELETE", scanPath, ""); code != 204 || srv.Scanning() {
		t.Errorf("stop: %d scanning=%v", code, srv.Scanning())
	}
}

func TestAScanUsesTheTunersCurrentSourceWhenNoneIsGiven(t *testing.T) {
	e, srv := channelsEnv(t, idleTuner)
	if code, _ := e.call(t, "POST", scanPath, `{}`); code != 202 {
		t.Fatalf("%d", code)
	}
	found := false
	for _, r := range srv.Requests() {
		if strings.Contains(r, "scan=start&source=Antenna") {
			found = true
		}
	}
	if !found {
		t.Errorf("requests = %v", srv.Requests())
	}
}

func TestAScanOfWhatIsInUseNeedsTheInterruptFlag(t *testing.T) {
	e, srv := channelsEnv(t, defaultBusyStatus)
	code, v := e.call(t, "POST", scanPath, `{}`)
	if code != 409 || !strings.Contains(errText(v), "2 of the tuner's tuners are in use") || !strings.Contains(errText(v), "interrupt") {
		t.Fatalf("%d %v", code, v)
	}
	if posts(srv) != 0 {
		t.Errorf("a refused scan was sent: %v", srv.Requests())
	}
	if code, v := e.call(t, "POST", scanPath, `{"interrupt":true}`); code != 202 {
		t.Fatalf("%d %v", code, v)
	}
}

func TestScanRefusals(t *testing.T) {
	e, srv := channelsEnv(t, idleTuner)
	if code, v := e.call(t, "POST", scanPath, `{"source":"Satellite"}`); code != 400 {
		t.Errorf("an unknown source: %d %v", code, v)
	}
	srv.CannotScan = true
	if code, v := e.call(t, "POST", scanPath, `{}`); code != 409 || !strings.Contains(errText(v), "can't scan") {
		t.Errorf("when it can't: %d %v", code, v)
	}
	if posts(srv) != 0 {
		t.Errorf("a refused scan was sent: %v", srv.Requests())
	}
}
