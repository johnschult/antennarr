package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/guide"
	"github.com/johnschult/yagi/internal/hdhr"
	"github.com/johnschult/yagi/internal/hdhr/hdhrtest"
	"github.com/johnschult/yagi/internal/lineup"
	"github.com/johnschult/yagi/internal/lineup/linetest"
)

// toServer sends every request to srv, whatever address it was made to: the monitor's tuner is a different
// fake, and the lineup is a second one standing in for the same tuner.
type toServer struct{ to *url.URL }

func (t toServer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host = t.to.Scheme, t.to.Host
	return http.DefaultTransport.RoundTrip(r)
}

const chanPath = "/api/devices/antenna/channels"
const scanPath = "/api/devices/antenna/channel-scan"

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
	if code, _ := e.call(t, "GET", "/api/devices/nope/channels", ""); code != 404 {
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

// ---- testing a signal ----

func testEnv(t *testing.T, tunerStatus string, o hdhrtest.Options) (*env, *linetest.Server, *hdhrtest.Server) {
	t.Helper()
	e, lt := channelsEnv(t, tunerStatus)
	ctrl, err := hdhrtest.New("127.0.0.1:0", o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctrl.Close)
	e.api.Control = func(ctx context.Context, _ string) (*hdhr.Conn, error) { return hdhr.Dial(ctx, ctrl.Addr()) }
	e.api.SignalTest = hdhr.TestOptions{Wait: 500 * time.Millisecond, Settle: 10 * time.Millisecond, Poll: 20 * time.Millisecond}
	return e, lt, ctrl
}

func testPath(number string) string { return chanPath + "/" + number + "/test" }

// threeBusy is a status with tuners 0, 1 and 2 in use and tuner 3 free.
const threeBusy = `[{"Resource":"tuner0","VctNumber":"2.1","TargetIP":"192.168.1.9"},{"Resource":"tuner1","VctNumber":"5.1","TargetIP":"192.168.1.9"},{"Resource":"tuner2","VctNumber":"7.1","TargetIP":"192.168.1.9"},{"Resource":"tuner3"}]`
const allBusy = `[{"Resource":"tuner0","VctNumber":"2.1","TargetIP":"192.168.1.9"},{"Resource":"tuner1","VctNumber":"5.1","TargetIP":"192.168.1.9"},{"Resource":"tuner2","VctNumber":"7.1","TargetIP":"192.168.1.9"},{"Resource":"tuner3","VctNumber":"9.1","TargetIP":"192.168.1.9"}]`

func TestASignalIsTestedOnTheHighestFreeTunerAndGivenBack(t *testing.T) {
	e, _, ctrl := testEnv(t, idleTuner, hdhrtest.Options{Signals: map[string]hdhrtest.Signal{"2.1": {Lock: "atsc3", Strength: 88, Quality: 96, Symbol: 100}}})
	code, v := e.call(t, "POST", testPath("2.1"), "")
	m := obj(t, v)
	if code != 200 || m["locked"] != true || m["strength"] != float64(88) || m["quality"] != float64(96) || m["symbol"] != float64(100) || m["tuner"] != float64(3) || m["channel"] != "2.1" {
		t.Fatalf("%d %v", code, v)
	}
	if tunes := ctrl.Tunes(); len(tunes) != 1 || tunes[0] != "tuner3 2.1" {
		t.Errorf("tunes = %v: a new stream takes the lowest free tuner, so the test should take the highest", tunes)
	}
	if ctrl.Held(3) {
		t.Error("the tuner was left held")
	}
}

func TestATunerThatHasNoSignalIsAnAnswer(t *testing.T) {
	e, _, ctrl := testEnv(t, idleTuner, hdhrtest.Options{Signals: map[string]hdhrtest.Signal{"2.1": {Strength: 9}}})
	code, v := e.call(t, "POST", testPath("2.1"), "")
	if m := obj(t, v); code != 200 || m["locked"] != false || m["strength"] != float64(9) {
		t.Fatalf("%d %v", code, v)
	}
	if ctrl.Held(3) {
		t.Error("the tuner was left held")
	}
}

func TestTheTestSkipsATunerThatIsBusyAtTheTuner(t *testing.T) {
	// the page's status says 3 is free; the tuner itself says somebody has it
	e, _, ctrl := testEnv(t, idleTuner, hdhrtest.Options{Busy: map[int]string{3: "locked"}})
	code, v := e.call(t, "POST", testPath("2.1"), "")
	if code != 200 || obj(t, v)["tuner"] != float64(2) {
		t.Fatalf("%d %v", code, v)
	}
	if tunes := ctrl.Tunes(); len(tunes) != 1 || tunes[0] != "tuner2 2.1" {
		t.Errorf("tunes = %v", tunes)
	}
}

func TestATestNeedsAFreeTunerAndNeverInterruptsAnything(t *testing.T) {
	e, _, ctrl := testEnv(t, allBusy, hdhrtest.Options{})
	code, v := e.call(t, "POST", testPath("2.1"), "")
	if code != 409 || !strings.Contains(errText(v), "never interrupts") {
		t.Fatalf("%d %v", code, v)
	}
	if len(ctrl.Frames()) != 0 {
		t.Errorf("the tuner was written to: %d frames", len(ctrl.Frames()))
	}
	// and with only one free, only that one is used
	e2, _, ctrl2 := testEnv(t, threeBusy, hdhrtest.Options{})
	if code, _ := e2.call(t, "POST", testPath("2.1"), ""); code != 200 {
		t.Fatalf("one free: %d", code)
	}
	if tunes := ctrl2.Tunes(); len(tunes) != 1 || tunes[0] != "tuner3 2.1" {
		t.Errorf("tunes = %v", tunes)
	}
}

func TestEveryTunerTurningOutBusyIsSaid(t *testing.T) {
	e, _, ctrl := testEnv(t, idleTuner, hdhrtest.Options{Busy: map[int]string{0: "tuned", 1: "locked", 2: "streaming", 3: "locked"}})
	code, v := e.call(t, "POST", testPath("2.1"), "")
	if code != 409 || !strings.Contains(errText(v), "turned out to be in use") {
		t.Fatalf("%d %v", code, v)
	}
	if len(ctrl.Tunes()) != 0 {
		t.Errorf("a busy tuner was tuned: %v", ctrl.Tunes())
	}
}

func TestSignalTestRefusalsTouchNothing(t *testing.T) {
	e, lt, ctrl := testEnv(t, idleTuner, hdhrtest.Options{})
	for name, c := range map[string]struct {
		number string
		want   int
	}{
		"not a number":              {"2.1&x=1", 400},
		"a channel the tuner lacks": {"99.9", 404},
		"a channel it can't play":   {"703", 409},
	} {
		lt.Forget()
		code, v := e.call(t, "POST", testPath(url.PathEscape(c.number)), "")
		if code != c.want {
			t.Errorf("%s: %d %v, want %d", name, code, v, c.want)
		}
		if len(ctrl.Frames()) != 0 {
			t.Errorf("%s: the tuner was written to", name)
		}
	}
	// while it scans
	lt.Forget()
	e.call(t, "POST", scanPath, `{}`)
	if code, v := e.call(t, "POST", testPath("2.1"), ""); code != 409 || !strings.Contains(errText(v), "scanning") {
		t.Errorf("while scanning: %d %v", code, v)
	}
	if len(ctrl.Frames()) != 0 {
		t.Error("the tuner was written to while it scanned")
	}
}

func TestOnlyOneSignalTestRunsAtATime(t *testing.T) {
	e, _, ctrl := testEnv(t, idleTuner, hdhrtest.Options{LockAfter: 400 * time.Millisecond})
	done := make(chan int, 1)
	go func() { code, _ := e.call(t, "POST", testPath("2.1"), ""); done <- code }()
	deadline := time.Now().Add(3 * time.Second)
	for len(ctrl.Tunes()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	code, v := e.call(t, "POST", testPath("2.2"), "")
	if code != 409 || !strings.Contains(errText(v), "already running") {
		t.Errorf("a second: %d %v", code, v)
	}
	if first := <-done; first != 200 {
		t.Errorf("the first: %d", first)
	}
	if code, _ := e.call(t, "POST", testPath("2.2"), ""); code != 200 {
		t.Errorf("after the first finished: %d", code)
	}
}

func TestNoSignalTestsWhileTheTunerRestartsOrIsOffline(t *testing.T) {
	e, _, ctrl := testEnv(t, idleTuner, hdhrtest.Options{})
	e.tun.Close()
	for i := 0; i < 4; i++ {
		e.mon.Poll(context.Background())
	}
	if code, v := e.call(t, "POST", testPath("2.1"), ""); code != 409 || !strings.Contains(errText(v), "offline") {
		t.Errorf("%d %v", code, v)
	}
	if len(ctrl.Frames()) != 0 {
		t.Error("an offline tuner was written to")
	}
}

func TestSignalTestIsForAdminsOnlyAndNeedsAControlPort(t *testing.T) {
	e, _, ctrl := testEnv(t, idleTuner, hdhrtest.Options{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", testPath("2.1"), nil)
	req.RemoteAddr = "100.67.117.32:1" // John is a viewer, not an admin
	e.pol.AdminOnly(e.api.Handler()).ServeHTTP(rec, req)
	if rec.Code != 403 || len(ctrl.Frames()) != 0 {
		t.Errorf("a viewer: %d, %d frames", rec.Code, len(ctrl.Frames()))
	}
	e.api.Control = nil
	if code, _ := e.call(t, "POST", testPath("2.1"), ""); code != 409 {
		t.Errorf("unavailable: %d", code)
	}
}

// guideService has read a small guide in which 2.2 (Laff) is an English comedy channel on a network.
func guideService(t *testing.T, enabled func() bool) *guide.Service {
	t.Helper()
	xml := `<?xml version="1.0"?><tv><channel id="a"><display-name>X</display-name><display-name>2.2 X</display-name><display-name>2.2 Laff</display-name><display-name>2.2</display-name><display-name>Laff</display-name><display-name>ION</display-name><lcn>2.2</lcn></channel>
<programme start="20990101000000 +0000" stop="20990101010000 +0000" channel="a"><title>T</title><category>Sitcom</category><category>Comedy</category><language>en</language></programme></tv>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(xml)) }))
	t.Cleanup(srv.Close)
	svc := guide.NewService(guide.Config{URL: srv.URL, Enabled: enabled})
	svc.Watch("antenna", func() string { return "auth" })
	svc.Refresh(context.Background())
	return svc
}

func TestChannelsCarryWhatTheGuideSaysAboutThem(t *testing.T) {
	e, _ := channelsEnv(t, idleTuner)
	e.api.Guide = guideService(t, nil)
	_, v := e.call(t, "GET", chanPath, "")
	by := map[string]map[string]any{}
	for _, c := range obj(t, v)["channels"].([]any) {
		m := obj(t, c)
		by[m["number"].(string)] = m
	}
	about := obj(t, by["2.2"]["about"])
	if about["network"] != "ION" || about["languages"].([]any)[0] != "en" || len(about["categories"].([]any)) != 2 {
		t.Errorf("about = %v", about)
	}
	if by["2.2"]["state"] != "favorite" {
		t.Errorf("the tuner's own fields are lost: %v", by["2.2"])
	}
	// a channel the guide doesn't have has no about at all, rather than an empty one
	if _, has := by["2.1"]["about"]; has {
		t.Errorf("about for a channel the guide doesn't have: %v", by["2.1"]["about"])
	}
}

func TestChannelsHaveNoAboutWithoutAGuideOrWhileItIsOff(t *testing.T) {
	e, _ := channelsEnv(t, idleTuner)
	_, v := e.call(t, "GET", chanPath, "")
	if _, has := obj(t, obj(t, v)["channels"].([]any)[1])["about"]; has {
		t.Error("about without a guide")
	}
	e.api.Guide = guideService(t, nil)
	e.call(t, "PATCH", "/api/settings", `{"guide":false}`)
	_, v = e.call(t, "GET", chanPath, "")
	for _, c := range obj(t, v)["channels"].([]any) {
		if _, has := obj(t, c)["about"]; has {
			t.Error("about while the guide is off")
		}
	}
}
