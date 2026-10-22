package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/access"
	"github.com/johnschult/yagi/internal/discovery"
	"github.com/johnschult/yagi/internal/live"
	"github.com/johnschult/yagi/internal/tuner"
)

// ---- test environment ----

type env struct {
	tun  *httptest.Server // the fake tuner's HTTP side
	srv  *httptest.Server
	api  *API
	pol  *access.Policy
	mon  *tuner.Monitor
	hub  *live.Hub
	path string
}

func fakeHDHR(t *testing.T, status string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/discover.json":
			io.WriteString(w, `{"FriendlyName":"Fake FLEX","ModelNumber":"HDFX-4K","DeviceID":"10ABCC87","TunerCount":4,"FirmwareName":"hdhomerun_dvr_atsc3","FirmwareVersion":"20250815"}`)
		case "/status.json":
			io.WriteString(w, status)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

var johnAndMom = access.Settings{
	OwnerReserve: 1,
	Viewers: []access.Viewer{
		{Name: "John", Addresses: []string{"100.67.117.32"}, Limit: 2},
		{Name: "Mom", Addresses: []string{"10.8.0.0/24"}, Limit: 1},
	},
}

// newEnv wires a real monitor, policy, store and hub together. The monitor
// talks to a fake tuner that reports one local stream and one of ours.
func newEnv(t *testing.T, s access.Settings, withFile bool) *env {
	t.Helper()
	return newEnvWith(t, s, withFile, `[
		{"Resource":"tuner0","VctNumber":"5.1","VctName":"WFRV","TargetIP":"192.168.1.20"},
		{"Resource":"tuner1","VctNumber":"2.1","VctName":"WBAY","TargetIP":"192.168.1.77"},
		{"Resource":"tuner2"},{"Resource":"tuner3"}
	]`)
}

// newEnvWith is newEnv with the tuner's status.json of your choosing.
func newEnvWith(t *testing.T, s access.Settings, withFile bool, tunerStatus string) *env {
	t.Helper()
	tun := fakeHDHR(t, tunerStatus)
	hub := live.NewHub()
	mon := tuner.NewMonitor(tuner.Config{
		Name: "antenna", BaseURL: tun.URL, ID: "10ABCC87", OnChange: hub.Notify,
		// A tuner that goes away is not looked for on the network: a test must never reach a real one.
		Find: func(context.Context, string, string) (discovery.Device, error) {
			return discovery.Device{}, errors.New("not found")
		},
		LocalIPs: func() []string { return []string{"192.168.1.20"} },
	})
	pol, err := access.NewPolicy(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	pol.AddDevice("antenna", mon, mon)
	pol.SetOnChange(hub.Notify)

	e := &env{pol: pol, mon: mon, hub: hub, tun: tun}
	if withFile {
		e.path = filepath.Join(t.TempDir(), "yagi.json")
		if err := access.SaveSettings(e.path, s); err != nil {
			t.Fatal(err)
		}
	}
	e.api = &API{Devices: []Device{{Name: "antenna", Listen: ":5004", Mon: mon}}, Pol: pol, Store: access.NewStore(e.path, pol), Hub: hub, KeepAlive: time.Hour}
	e.srv = httptest.NewServer(pol.AdminOnly(e.api.Handler()))
	t.Cleanup(e.srv.Close)
	mon.Poll(context.Background())
	return e
}

// call makes a request and decodes the JSON reply into a generic map or slice.
func (e *env) call(t *testing.T, method, path, body string, hdr ...string) (int, any) {
	t.Helper()
	req, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var v any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("%s %s: reply is not JSON: %s", method, path, raw)
		}
	}
	return resp.StatusCode, v
}

func obj(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("not an object: %#v", v)
	}
	return m
}

func errText(v any) string {
	if m, ok := v.(map[string]any); ok {
		s, _ := m["error"].(string)
		return s
	}
	return ""
}

func viewerNames(t *testing.T, e *env) []string {
	t.Helper()
	_, v := e.call(t, "GET", "/api/viewers", "")
	var names []string
	for _, x := range v.([]any) {
		names = append(names, obj(t, x)["name"].(string))
	}
	return names
}

// ---- state ----

func TestState(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	// One of John's streams is running through us.
	r := httptest.NewRequest("GET", "/auto/v5.1", nil)
	r.RemoteAddr = "100.67.117.32:4000"
	if _, err := e.pol.Device("antenna").Stream(r); err != nil {
		t.Fatal(err)
	}

	code, v := e.call(t, "GET", "/api/state", "")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	st := obj(t, v)

	devices := st["devices"].([]any)
	if len(devices) != 1 {
		t.Fatalf("%d devices", len(devices))
	}
	dev := obj(t, devices[0])
	if dev["name"] != "antenna" || dev["listen"] != ":5004" || dev["ownerReserve"] != float64(1) || dev["sharedCap"] != float64(0) {
		t.Errorf("device = %v", dev)
	}
	tun := obj(t, dev["tuner"])
	if tun["online"] != true || tun["id"] != "10ABCC87" || tun["name"] != "antenna" || tun["friendlyName"] != "Fake FLEX" || len(tun["slots"].([]any)) != 4 {
		t.Errorf("tuner = %v", tun)
	}
	sum := obj(t, dev["summary"])
	want := map[string]float64{"total": 4, "inUse": 2, "remote": 1, "local": 1, "reserved": 1, "free": 2}
	for k, w := range want {
		if sum[k] != w {
			t.Errorf("summary.%s = %v, want %v", k, sum[k], w)
		}
	}
	streams := dev["streams"].([]any)
	if len(streams) != 1 || obj(t, streams[0])["viewer"] != "John" || obj(t, streams[0])["channel"] != "5.1" || obj(t, streams[0])["device"] != "antenna" {
		t.Errorf("streams = %v", streams)
	}
	viewers := st["viewers"].([]any)
	if len(viewers) != 2 || obj(t, viewers[0])["inUse"] != float64(1) || obj(t, viewers[1])["lastSeen"] != nil {
		t.Errorf("viewers = %v", viewers)
	}
	if by := obj(t, obj(t, viewers[0])["inUseByDevice"]); by["antenna"] != float64(1) {
		t.Errorf("inUseByDevice = %v", by)
	}
	set := obj(t, st["settings"])
	if set["mode"] != "enforced" || set["ownerReserve"] != float64(1) || st["editable"] != true {
		t.Errorf("settings = %v editable=%v", set, st["editable"])
	}
}

func TestStateBeforeAnyPollHasEmptyListsNotNulls(t *testing.T) {
	tun := fakeHDHR(t, `[]`)
	mon := tuner.NewMonitor(tuner.Config{BaseURL: tun.URL, ID: "10ABCC87"}) // never polled
	pol, _ := access.NewPolicy(access.DefaultSettings(), nil)
	pol.AddDevice("antenna", mon, mon)
	a := &API{Devices: []Device{{Name: "antenna", Mon: mon}}, Pol: pol, Store: access.NewStore("", pol), Hub: live.NewHub()}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	for _, field := range []string{`"slots":[]`, `"events":[]`, `"streams":[]`, `"viewers":[]`, `"admin":[]`} {
		if !strings.Contains(string(b), field) {
			t.Errorf("missing %s in %s", field, b)
		}
	}
	if strings.Contains(string(b), "null") && !strings.Contains(string(b), `"lastSeen":null`) {
		t.Errorf("unexpected null in %s", b)
	}
}

// ---- viewers ----

func TestViewerCRUD(t *testing.T) {
	e := newEnv(t, johnAndMom, true)

	code, v := e.call(t, "POST", "/api/viewers", `{"name":"Cabin","addresses":["10.9.0.6"],"limit":1}`)
	if code != 201 || obj(t, v)["name"] != "Cabin" {
		t.Fatalf("add: %d %v", code, v)
	}
	if got := viewerNames(t, e); strings.Join(got, ",") != "John,Mom,Cabin" {
		t.Errorf("after add: %v", got)
	}

	code, v = e.call(t, "PUT", "/api/viewers/cabin", `{"name":"Cabin Pi","addresses":["10.9.0.6","10.9.0.7"],"limit":2}`)
	if code != 200 || obj(t, v)["name"] != "Cabin Pi" || obj(t, v)["limit"] != float64(2) {
		t.Fatalf("replace (case-insensitive lookup, renamed): %d %v", code, v)
	}
	if got := viewerNames(t, e); strings.Join(got, ",") != "John,Mom,Cabin Pi" {
		t.Errorf("after rename: %v", got)
	}

	if code, _ := e.call(t, "DELETE", "/api/viewers/Cabin%20Pi", ""); code != 204 {
		t.Errorf("delete: %d", code)
	}
	if got := viewerNames(t, e); strings.Join(got, ",") != "John,Mom" {
		t.Errorf("after delete: %v", got)
	}

	// Everything above went through to the file and to the live policy.
	onDisk, _, err := access.LoadSettings(e.path)
	if err != nil || len(onDisk.Viewers) != 2 {
		t.Errorf("file: %+v (%v)", onDisk, err)
	}
}

func TestViewerErrors(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	cases := []struct {
		name, method, path, body string
		want                     int
		contains                 string
	}{
		{"duplicate name", "POST", "/api/viewers", `{"name":"john","addresses":["10.9.0.9"],"limit":1}`, 409, "already exists"},
		{"bad address", "POST", "/api/viewers", `{"name":"X","addresses":["not-an-ip"],"limit":1}`, 400, "bad address"},
		{"no addresses", "POST", "/api/viewers", `{"name":"X","addresses":[],"limit":1}`, 400, "no addresses"},
		{"missing addresses", "POST", "/api/viewers", `{"name":"X","limit":1}`, 400, "no addresses"},
		{"negative limit", "POST", "/api/viewers", `{"name":"X","addresses":["10.9.0.9"],"limit":-1}`, 400, "negative"},
		{"address taken", "POST", "/api/viewers", `{"name":"X","addresses":["100.67.117.32"],"limit":1}`, 400, "both"},
		{"unknown field", "POST", "/api/viewers", `{"name":"X","addresses":["10.9.0.9"],"limit":1,"admin":true}`, 400, "unknown field"},
		{"not json", "POST", "/api/viewers", `nope`, 400, "invalid JSON"},
		{"trailing data", "POST", "/api/viewers", `{"name":"X","addresses":["10.9.0.9"],"limit":1} {}`, 400, "unexpected data"},
		{"replace unknown", "PUT", "/api/viewers/ghost", `{"name":"ghost","addresses":["10.9.0.9"],"limit":1}`, 404, "no viewer"},
		{"rename onto existing", "PUT", "/api/viewers/Mom", `{"name":"John","addresses":["10.8.0.0/24"],"limit":1}`, 400, "twice"},
		{"delete unknown", "DELETE", "/api/viewers/ghost", ``, 404, "no viewer"},
	}
	for _, c := range cases {
		code, v := e.call(t, c.method, c.path, c.body)
		if code != c.want || !strings.Contains(errText(v), c.contains) {
			t.Errorf("%s: %d %q, want %d containing %q", c.name, code, errText(v), c.want, c.contains)
		}
	}
	if got := viewerNames(t, e); strings.Join(got, ",") != "John,Mom" {
		t.Errorf("failed requests changed the viewers: %v", got)
	}
}

// Removing the last viewer is allowed: with no viewers nobody can use the tuner,
// so it closes rather than opens.
func TestDeletingTheLastViewerClosesTheTuner(t *testing.T) {
	e := newEnv(t, access.Settings{Viewers: []access.Viewer{{Name: "John", Addresses: []string{"100.67.117.32"}, Limit: 1}}}, true)
	if code, _ := e.call(t, "DELETE", "/api/viewers/John", ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if e.pol.Mode() != "no-viewers" || len(viewerNames(t, e)) != 0 {
		t.Errorf("mode = %q, viewers = %v", e.pol.Mode(), viewerNames(t, e))
	}
}

func TestViewerNameIsURLDecoded(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	e.call(t, "POST", "/api/viewers", `{"name":"Mom & Dad","addresses":["10.9.0.2"],"limit":1}`)
	if code, _ := e.call(t, "DELETE", "/api/viewers/"+url.PathEscape("Mom & Dad"), ""); code != 204 {
		t.Errorf("delete by escaped name: %d", code)
	}
}

// ---- settings ----

func TestSettingsPatchChangesOnlyWhatIsSent(t *testing.T) {
	e := newEnv(t, access.Settings{OwnerReserve: 1, SharedCap: 3, Admin: []string{"100.64.0.0/10"},
		Viewers: johnAndMom.Viewers}, true)

	code, v := e.call(t, "PATCH", "/api/settings", `{"sharedCap":2}`)
	set := obj(t, v)
	if code != 200 || set["sharedCap"] != float64(2) || set["ownerReserve"] != float64(1) {
		t.Fatalf("%d %v", code, v)
	}
	if admin := set["admin"].([]any); len(admin) != 1 || admin[0] != "100.64.0.0/10" {
		t.Errorf("a field that wasn't sent changed: admin = %v", admin)
	}

	// Sending zero is a change, not "absent".
	code, v = e.call(t, "PATCH", "/api/settings", `{"sharedCap":0,"ownerReserve":2,"admin":[]}`)
	set = obj(t, v)
	if code != 200 || set["sharedCap"] != float64(0) || set["ownerReserve"] != float64(2) || len(set["admin"].([]any)) != 0 {
		t.Errorf("%d %v", code, v)
	}

	code, v = e.call(t, "GET", "/api/settings", "")
	if code != 200 || obj(t, v)["ownerReserve"] != float64(2) {
		t.Errorf("GET after PATCH: %d %v", code, v)
	}
}

func TestSettingsPatchErrors(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	for name, body := range map[string]string{
		"negative reserve": `{"ownerReserve":-1}`,
		"negative cap":     `{"sharedCap":-5}`,
		"bad admin":        `{"admin":["nope"]}`,
		"unknown field":    `{"viewers":[]}`, // viewers have their own endpoints
		"wrong type":       `{"sharedCap":"two"}`,
	} {
		if code, v := e.call(t, "PATCH", "/api/settings", body); code != 400 {
			t.Errorf("%s: %d %v", name, code, v)
		}
	}
	if set := e.pol.Settings(); set.OwnerReserve != 1 || set.SharedCap != 0 {
		t.Errorf("failed patches changed settings: %+v", set)
	}
}

func TestReadOnlyWithoutConfigFile(t *testing.T) {
	e := newEnv(t, johnAndMom, false)
	_, v := e.call(t, "GET", "/api/state", "")
	if obj(t, v)["editable"] != false {
		t.Error("state should say editable=false")
	}
	for _, c := range [][3]string{
		{"POST", "/api/viewers", `{"name":"X","addresses":["10.9.0.9"],"limit":1}`},
		{"PATCH", "/api/settings", `{"sharedCap":1}`},
		{"DELETE", "/api/viewers/John", ``},
	} {
		code, v := e.call(t, c[0], c[1], c[2])
		if code != 409 || !strings.Contains(errText(v), "-config") {
			t.Errorf("%s %s: %d %v", c[0], c[1], code, v)
		}
	}
	if code, _ := e.call(t, "GET", "/api/viewers", ""); code != 200 {
		t.Errorf("reading must still work: %d", code)
	}
}

func TestSaveFailureIsA500AndChangesNothing(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	e.api.Store = access.NewStore(filepath.Join(e.path+"-dir", "gone", "a.json"), e.pol) // unwritable
	code, v := e.call(t, "PATCH", "/api/settings", `{"sharedCap":3}`)
	if code != 500 || !strings.Contains(errText(v), "could not save") {
		t.Errorf("%d %v", code, v)
	}
	if e.pol.Settings().SharedCap != 0 {
		t.Error("settings changed although saving failed")
	}
}

// ---- guards ----

func TestWritesMustBeJSON(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	body := `{"sharedCap":1}`
	for ct, want := range map[string]int{
		"text/plain":                        415, // what a cross-site <form> or no-cors fetch can send
		"application/x-www-form-urlencoded": 415,
		"":                                  415,
		"application/json":                  200,
		"application/json; charset=utf-8":   200,
	} {
		req, _ := http.NewRequest("PATCH", e.srv.URL+"/api/settings", strings.NewReader(body))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("Content-Type %q: %d, want %d", ct, resp.StatusCode, want)
		}
	}
}

func TestCrossOriginWritesAreRefused(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	for origin, want := range map[string]int{
		"https://evil.example": 403,
		"null":                 403, // sandboxed iframes
		e.srv.URL:              200, // the UI served by this very host
	} {
		code, v := e.call(t, "PATCH", "/api/settings", `{"sharedCap":1}`, "Origin", origin)
		if code != want {
			t.Errorf("Origin %q: %d %v, want %d", origin, code, v, want)
		}
	}
	// And a cross-origin DELETE, which has no body to type-check.
	if code, _ := e.call(t, "DELETE", "/api/viewers/Mom", "", "Origin", "https://evil.example"); code != 403 {
		t.Errorf("cross-origin DELETE: %d", code)
	}
	if len(viewerNames(t, e)) != 2 {
		t.Error("a cross-origin request deleted a viewer")
	}
}

func TestOversizedBodyIsRefused(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	big := `{"name":"X","addresses":["` + strings.Repeat("1", maxBody) + `"],"limit":1}`
	code, v := e.call(t, "POST", "/api/viewers", big)
	if code != 413 {
		t.Errorf("%d %v", code, v)
	}
}

func TestOnlyAdminsReachTheAPI(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	h := e.pol.AdminOnly(e.api.Handler())
	for _, remote := range []string{"203.0.113.9:1", "100.67.117.32:1", "10.8.0.5:1"} { // a stranger and both viewers
		for _, c := range [][2]string{{"GET", "/api/state"}, {"GET", "/api/viewers"}, {"GET", "/api/events"}, {"DELETE", "/api/viewers/John"}} {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(c[0], c[1], nil)
			req.RemoteAddr = remote
			h.ServeHTTP(rec, req)
			if rec.Code != 403 {
				t.Errorf("%s %s %s: %d, want 403", remote, c[0], c[1], rec.Code)
			}
		}
	}
	if len(viewerNames(t, e)) != 2 {
		t.Error("a non-admin changed something")
	}
}

func TestUnknownRoutesAndMethods(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	code, v := e.call(t, "GET", "/api/nope", "")
	if code != 404 || errText(v) == "" {
		t.Errorf("unknown route: %d %v (want a JSON 404)", code, v)
	}
	code, v = e.call(t, "PUT", "/api/state", `{}`)
	if code != 405 || errText(v) == "" {
		t.Errorf("PUT state: %d %v (want a JSON 405)", code, v)
	}

	req, _ := http.NewRequest("POST", e.srv.URL+"/api/viewers/John", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 405 || resp.Header.Get("Allow") != "DELETE, PUT" {
		t.Errorf("POST on a viewer: %d Allow=%q", resp.StatusCode, resp.Header.Get("Allow"))
	}
}

// ---- live events ----

type sseMsg struct {
	event, data string
	comment     bool
}

// openEvents connects to the event stream and parses it into messages.
func openEvents(t *testing.T, e *env) (<-chan sseMsg, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", e.srv.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	t.Cleanup(func() { cancel(); resp.Body.Close() })

	ch := make(chan sseMsg, 64)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		var m sseMsg
		for sc.Scan() {
			switch line := sc.Text(); {
			case line == "":
				if m.event != "" {
					ch <- m
				}
				m = sseMsg{}
			case strings.HasPrefix(line, ":"):
				ch <- sseMsg{comment: true}
			case strings.HasPrefix(line, "event: "):
				m.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				m.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return ch, cancel
}

func nextMsg(t *testing.T, ch <-chan sseMsg, within time.Duration) (sseMsg, bool) {
	t.Helper()
	select {
	case m, ok := <-ch:
		return m, ok
	case <-time.After(within):
		return sseMsg{}, false
	}
}

func streamsIn(t *testing.T, m sseMsg) int {
	t.Helper()
	var st struct {
		Devices []struct{ Streams []any }
	}
	if err := json.Unmarshal([]byte(m.data), &st); err != nil {
		t.Fatalf("bad event data: %v\n%s", err, m.data)
	}
	n := 0
	for _, d := range st.Devices {
		n += len(d.Streams)
	}
	return n
}

func TestEventsSendStateNowAndAfterChanges(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	ch, _ := openEvents(t, e)

	first, ok := nextMsg(t, ch, 2*time.Second)
	if !ok || first.event != "state" || streamsIn(t, first) != 0 {
		t.Fatalf("first event = %+v ok=%v", first, ok)
	}
	if _, ok := nextMsg(t, ch, 300*time.Millisecond); ok {
		t.Error("an idle stream sent something")
	}

	r := httptest.NewRequest("GET", "/auto/v2.1", nil)
	r.RemoteAddr = "10.8.0.5:4000"
	release, err := e.pol.Device("antenna").Stream(r)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := nextMsg(t, ch, 2*time.Second)
	if !ok || streamsIn(t, m) != 1 {
		t.Fatalf("after a stream started: %+v ok=%v", m, ok)
	}

	release()
	m, ok = nextMsg(t, ch, 2*time.Second)
	if !ok || streamsIn(t, m) != 0 {
		t.Fatalf("after the stream ended: %+v ok=%v", m, ok)
	}

	// Edits made through the API reach other open dashboards too.
	e.call(t, "POST", "/api/viewers", `{"name":"Cabin","addresses":["10.9.0.6"],"limit":1}`)
	m, ok = nextMsg(t, ch, 2*time.Second)
	if !ok || !strings.Contains(m.data, `"Cabin"`) {
		t.Fatalf("after adding a viewer: %+v ok=%v", m, ok)
	}
}

func TestEventsCoalesceBursts(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	ch, _ := openEvents(t, e)
	if _, ok := nextMsg(t, ch, 2*time.Second); !ok {
		t.Fatal("no initial event")
	}

	for i := 0; i < 25; i++ {
		e.hub.Notify()
	}
	n := 0
	for {
		if _, ok := nextMsg(t, ch, 400*time.Millisecond); !ok {
			break
		}
		n++
	}
	if n < 1 || n > 2 {
		t.Errorf("25 rapid changes produced %d events, want 1 (or at most 2)", n)
	}
}

func TestEventsKeepAlive(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	e.api.KeepAlive = 50 * time.Millisecond
	ch, _ := openEvents(t, e)
	if _, ok := nextMsg(t, ch, 2*time.Second); !ok {
		t.Fatal("no initial event")
	}
	m, ok := nextMsg(t, ch, 2*time.Second)
	if !ok || !m.comment {
		t.Errorf("expected a keep-alive comment, got %+v ok=%v", m, ok)
	}
}

func TestEventsStopWhenClientLeaves(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	ch, cancel := openEvents(t, e)
	if _, ok := nextMsg(t, ch, 2*time.Second); !ok {
		t.Fatal("no initial event")
	}
	if e.hub.Subscribers() != 1 {
		t.Fatalf("subscribers = %d while connected", e.hub.Subscribers())
	}
	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for e.hub.Subscribers() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if e.hub.Subscribers() != 0 {
		t.Error("the handler kept its subscription after the client left")
	}
}

func TestStateFileIsRealJSONOnDisk(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	e.call(t, "PATCH", "/api/settings", `{"sharedCap":2}`)
	b, err := os.ReadFile(e.path)
	if err != nil {
		t.Fatal(err)
	}
	var s access.Settings
	if err := json.Unmarshal(b, &s); err != nil || s.SharedCap != 2 {
		t.Errorf("file = %s (%v)", b, err)
	}
}
