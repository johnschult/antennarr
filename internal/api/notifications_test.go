package api

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/johnschult/antennarr/internal/access"
)

const channels = "/antennarr/api/notifications/channels"

// fakeTester stands in for the notifier: it records what it was asked to send a test to.
type fakeTester struct {
	mu  sync.Mutex
	got []access.NotifyChannel
	err error
}

func (f *fakeTester) Test(_ context.Context, ch access.NotifyChannel) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, ch)
	return f.err
}

func (f *fakeTester) last(t *testing.T) access.NotifyChannel {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.got) == 0 {
		t.Fatal("nothing was tested")
	}
	return f.got[len(f.got)-1]
}

func (f *fakeTester) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.got) }

func notifyEnv(t *testing.T) (*env, *fakeTester) {
	t.Helper()
	e := newEnv(t, johnAndMom, true)
	ft := &fakeTester{}
	e.api.Notify = ft
	return e, ft
}

func notificationsOf(t *testing.T, e *env) map[string]any {
	t.Helper()
	_, v := e.call(t, "GET", "/antennarr/api/state", "")
	return obj(t, obj(t, v)["settings"])["notifications"].(map[string]any)
}

func addNtfy(t *testing.T, e *env, name, token string) string {
	t.Helper()
	body := `{"name":"` + name + `","type":"ntfy","url":"https://ntfy.sh","topic":"antennarr","token":"` + token + `"}`
	code, v := e.call(t, "POST", channels, body)
	if code != 201 {
		t.Fatalf("add: %d %v", code, v)
	}
	return obj(t, v)["id"].(string)
}

func saved(t *testing.T, e *env) access.Settings {
	t.Helper()
	s, _, err := access.LoadSettings(e.path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// ---- what the browser is told ----

func TestStateHasNoChannelsAndTheDefaultEvents(t *testing.T) {
	e, _ := notifyEnv(t)
	n := notificationsOf(t, e)
	if len(n["channels"].([]any)) != 0 {
		t.Errorf("channels = %v", n["channels"])
	}
	ev := obj(t, n["events"])
	if ev["firmware"] != true || ev["tuner"] != true || ev["viewer"] != true || ev["unknown"] != false {
		t.Errorf("events = %v", ev)
	}
}

func TestChannelsAreShownWithoutTheirSecrets(t *testing.T) {
	e, _ := notifyEnv(t)
	addNtfy(t, e, "Phone", "tk_SECRET_TOKEN")
	e.call(t, "POST", channels, `{"name":"Pushover","type":"pushover","token":"SECRET_APP","user":"SECRET_USER"}`)
	e.call(t, "POST", channels, `{"name":"Discord","type":"webhook","url":"https://discord.com/api/webhooks/123/SECRET_PATH?x=SECRET_QUERY","token":"SECRET_BEARER"}`)

	_, raw := e.call(t, "GET", "/antennarr/api/state", "")
	b, _ := json.Marshal(raw)
	for _, s := range []string{"SECRET_TOKEN", "SECRET_APP", "SECRET_USER", "SECRET_PATH", "SECRET_QUERY", "SECRET_BEARER"} {
		if strings.Contains(string(b), s) {
			t.Errorf("the state document repeats %q", s)
		}
	}
	_, settings := e.call(t, "GET", "/antennarr/api/settings", "")
	b, _ = json.Marshal(settings)
	for _, s := range []string{"SECRET_TOKEN", "SECRET_APP", "SECRET_USER", "SECRET_PATH", "SECRET_BEARER"} {
		if strings.Contains(string(b), s) {
			t.Errorf("the settings document repeats %q", s)
		}
	}

	chs := notificationsOf(t, e)["channels"].([]any)
	if len(chs) != 3 {
		t.Fatalf("channels = %v", chs)
	}
	ntfy, po, hook := obj(t, chs[0]), obj(t, chs[1]), obj(t, chs[2])
	if ntfy["url"] != "https://ntfy.sh" || ntfy["topic"] != "antennarr" || ntfy["hasToken"] != true || ntfy["hasUser"] != false {
		t.Errorf("ntfy = %v", ntfy)
	}
	if po["hasToken"] != true || po["hasUser"] != true || po["url"] != nil {
		t.Errorf("pushover = %v", po)
	}
	if hook["host"] != "discord.com" || hook["url"] != nil || hook["hasToken"] != true {
		t.Errorf("webhook = %v", hook)
	}
}

// ---- adding ----

func TestAddingAChannelSavesItWithItsSecretsAndAnswersWithoutThem(t *testing.T) {
	e, _ := notifyEnv(t)
	code, v := e.call(t, "POST", channels, `{"name":"  Phone ","type":"ntfy","url":"https://ntfy.sh","topic":"antennarr","token":"tk_SECRET"}`)
	if code != 201 {
		t.Fatalf("%d %v", code, v)
	}
	doc := obj(t, v)
	if !strings.HasPrefix(doc["id"].(string), "c") || doc["name"] != "Phone" || doc["hasToken"] != true {
		t.Errorf("reply = %v", doc)
	}
	if b, _ := json.Marshal(v); strings.Contains(string(b), "tk_SECRET") {
		t.Errorf("the reply repeats the token: %s", b)
	}

	got := saved(t, e).NotifyChannels()
	if len(got) != 1 || got[0].Token != "tk_SECRET" || got[0].ID != doc["id"] {
		t.Errorf("saved = %+v", got)
	}
	info, _ := os.Stat(e.path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("the file holds a secret and is %v", info.Mode().Perm())
	}
	// and it is live: the policy the notifier reads from has it too
	if len(e.pol.Settings().NotifyChannels()) != 1 {
		t.Error("the new channel is not in the running settings")
	}
}

func TestAChannelThatIsNotValidIsRefusedWithTheReasonAndNeverTheSecret(t *testing.T) {
	e, _ := notifyEnv(t)
	for name, body := range map[string]string{
		"no name":        `{"name":"","type":"ntfy","url":"https://ntfy.sh","topic":"t"}`,
		"no type":        `{"name":"A","url":"https://ntfy.sh","topic":"t"}`,
		"unknown type":   `{"name":"A","type":"email","topic":"t"}`,
		"no topic":       `{"name":"A","type":"ntfy","url":"https://ntfy.sh","token":"SECRET"}`,
		"a password":     `{"name":"A","type":"ntfy","url":"https://me:SECRET@ntfy.sh","topic":"t"}`,
		"not http":       `{"name":"A","type":"webhook","url":"ftp://example.com/SECRET"}`,
		"pushover half":  `{"name":"A","type":"pushover","token":"SECRET"}`,
		"extra property": `{"name":"A","type":"ntfy","url":"https://ntfy.sh","topic":"t","tokn":"SECRET"}`,
	} {
		code, v := e.call(t, "POST", channels, body)
		if code != 400 || errText(v) == "" {
			t.Errorf("%s: %d %v", name, code, v)
		}
		if strings.Contains(errText(v), "SECRET") {
			t.Errorf("%s: the error repeats a secret: %s", name, errText(v))
		}
	}
	if n := len(saved(t, e).NotifyChannels()); n != 0 {
		t.Errorf("%d channels were saved from bad requests", n)
	}
}

func TestTwoChannelsCannotShareAName(t *testing.T) {
	e, _ := notifyEnv(t)
	addNtfy(t, e, "Phone", "")
	code, v := e.call(t, "POST", channels, `{"name":"phone","type":"ntfy","url":"https://ntfy.sh","topic":"other"}`)
	if code != 400 || !strings.Contains(errText(v), "called") {
		t.Errorf("%d %v", code, v)
	}
}

func TestThereAreAtMostEightChannels(t *testing.T) {
	e, _ := notifyEnv(t)
	for i := 0; i < 8; i++ {
		addNtfy(t, e, "Phone "+string(rune('A'+i)), "")
	}
	if code, v := e.call(t, "POST", channels, `{"name":"Ninth","type":"ntfy","url":"https://ntfy.sh","topic":"t"}`); code != 400 {
		t.Errorf("%d %v", code, v)
	}
}

func TestNothingCanBeAddedWithoutASettingsFile(t *testing.T) {
	e := newEnv(t, johnAndMom, false)
	e.api.Notify = &fakeTester{}
	if code, _ := e.call(t, "POST", channels, `{"name":"A","type":"ntfy","url":"https://ntfy.sh","topic":"t"}`); code != 409 {
		t.Errorf("code %d, want 409", code)
	}
}

// ---- changing ----

func TestChangingAChannelKeepsASecretYouLeaveEmpty(t *testing.T) {
	e, _ := notifyEnv(t)
	id := addNtfy(t, e, "Phone", "tk_KEEP")

	code, v := e.call(t, "PUT", channels+"/"+id, `{"name":"My phone","type":"ntfy","url":"https://ntfy.example.com","topic":"newtopic"}`)
	if code != 200 || obj(t, v)["name"] != "My phone" || obj(t, v)["hasToken"] != true {
		t.Fatalf("%d %v", code, v)
	}
	got := saved(t, e).NotifyChannels()[0]
	if got.Token != "tk_KEEP" || got.URL != "https://ntfy.example.com" || got.Topic != "newtopic" || got.ID != id {
		t.Errorf("saved = %+v", got)
	}

	e.call(t, "PUT", channels+"/"+id, `{"name":"My phone","type":"ntfy","url":"https://ntfy.example.com","topic":"newtopic","token":"tk_NEW"}`)
	if got := saved(t, e).NotifyChannels()[0]; got.Token != "tk_NEW" {
		t.Errorf("a new token was not saved: %+v", got)
	}
	e.call(t, "PUT", channels+"/"+id, `{"name":"My phone","type":"ntfy","url":"https://ntfy.example.com","topic":"newtopic","clearToken":true}`)
	if got := saved(t, e).NotifyChannels()[0]; got.Token != "" {
		t.Errorf("the token was not cleared: %+v", got)
	}
}

func TestAWebhookAddressLeftEmptyIsKept(t *testing.T) {
	e, _ := notifyEnv(t)
	_, v := e.call(t, "POST", channels, `{"name":"Hook","type":"webhook","url":"https://hooks.example.com/SECRET_PATH"}`)
	id := obj(t, v)["id"].(string)
	code, _ := e.call(t, "PUT", channels+"/"+id, `{"name":"Renamed","type":"webhook"}`)
	if code != 200 {
		t.Fatalf("code %d", code)
	}
	if got := saved(t, e).NotifyChannels()[0]; got.URL != "https://hooks.example.com/SECRET_PATH" || got.Name != "Renamed" {
		t.Errorf("saved = %+v", got)
	}
}

func TestChangingTheTypeDoesNotCarryTheSecretsToAnotherService(t *testing.T) {
	e, _ := notifyEnv(t)
	id := addNtfy(t, e, "Alerts", "tk_FOR_NTFY")
	code, v := e.call(t, "PUT", channels+"/"+id, `{"name":"Alerts","type":"pushover","token":"app","user":"usr"}`)
	if code != 200 {
		t.Fatalf("%d %v", code, v)
	}
	if got := saved(t, e).NotifyChannels()[0]; got.Token != "app" || got.URL != "" || got.Topic != "" {
		t.Errorf("saved = %+v", got)
	}
	// a switch that gives no new token has nothing to keep, so it is refused rather than reusing the old one
	id2 := addNtfy(t, e, "Second", "tk_OTHER")
	if code, v := e.call(t, "PUT", channels+"/"+id2, `{"name":"Second","type":"pushover","user":"usr"}`); code != 400 {
		t.Errorf("%d %v", code, v)
	}
	if got, _ := saved(t, e).NotifyChannel(id2); got.Type != access.ChannelNtfy || got.Token != "tk_OTHER" {
		t.Errorf("a refused change altered the channel: %+v", got)
	}
}

func TestChangingOrRemovingAChannelThatIsNotThereIs404(t *testing.T) {
	e, _ := notifyEnv(t)
	if code, _ := e.call(t, "PUT", channels+"/cnope", `{"name":"A","type":"ntfy","url":"https://ntfy.sh","topic":"t"}`); code != 404 {
		t.Errorf("PUT: %d", code)
	}
	if code, _ := e.call(t, "DELETE", channels+"/cnope", ""); code != 404 {
		t.Errorf("DELETE: %d", code)
	}
}

func TestRemovingAChannel(t *testing.T) {
	e, _ := notifyEnv(t)
	a, b := addNtfy(t, e, "A", ""), addNtfy(t, e, "B", "")
	if code, _ := e.call(t, "DELETE", channels+"/"+a, ""); code != 204 {
		t.Fatalf("code %d", code)
	}
	got := saved(t, e).NotifyChannels()
	if len(got) != 1 || got[0].ID != b {
		t.Errorf("saved = %+v", got)
	}
	if code, _ := e.call(t, "DELETE", channels+"/"+a, ""); code != 404 {
		t.Errorf("a second removal: %d", code)
	}
}

// ---- the event switches ----

func TestEventsAreSwitchedThroughTheSettings(t *testing.T) {
	e, _ := notifyEnv(t)
	code, v := e.call(t, "PATCH", "/antennarr/api/settings", `{"notifyEvents":{"firmware":false,"unknown":true}}`)
	if code != 200 {
		t.Fatalf("%d %v", code, v)
	}
	ev := obj(t, notificationsOf(t, e)["events"])
	if ev["firmware"] != false || ev["tuner"] != true || ev["viewer"] != true || ev["unknown"] != true {
		t.Errorf("events = %v", ev)
	}
	// only what is sent changes
	e.call(t, "PATCH", "/antennarr/api/settings", `{"notifyEvents":{"tuner":false}}`)
	ev = obj(t, notificationsOf(t, e)["events"])
	if ev["firmware"] != false || ev["tuner"] != false || ev["unknown"] != true {
		t.Errorf("after a second change: %v", ev)
	}
	if code, _ := e.call(t, "PATCH", "/antennarr/api/settings", `{"notifyEvents":{"firmwre":false}}`); code != 400 {
		t.Errorf("a typo was accepted: %d", code)
	}
	// and they survive in the file
	if s := saved(t, e); s.NotifyEnabled(access.NotifyFirmware) || !s.NotifyEnabled(access.NotifyUnknown) {
		t.Errorf("file = %+v", s.Notifications)
	}
}

func TestWatchingIsOffUntilSwitchedOn(t *testing.T) {
	e, _ := notifyEnv(t)
	if ev := obj(t, notificationsOf(t, e)["events"]); ev["watching"] != false {
		t.Errorf("by default: %v", ev)
	}
	if code, v := e.call(t, "PATCH", "/antennarr/api/settings", `{"notifyEvents":{"watching":true}}`); code != 200 {
		t.Fatalf("%d %v", code, v)
	}
	if ev := obj(t, notificationsOf(t, e)["events"]); ev["watching"] != true || ev["viewer"] != true {
		t.Errorf("after: %v", ev)
	}
	if !saved(t, e).NotifyEnabled(access.NotifyWatching) {
		t.Error("not saved")
	}
}

func TestSwitchingEventsDoesNotDisturbTheChannels(t *testing.T) {
	e, _ := notifyEnv(t)
	addNtfy(t, e, "Phone", "tk_KEEP")
	e.call(t, "PATCH", "/antennarr/api/settings", `{"notifyEvents":{"viewer":false}}`)
	if got := saved(t, e).NotifyChannels(); len(got) != 1 || got[0].Token != "tk_KEEP" {
		t.Errorf("saved = %+v", got)
	}
}

// ---- the test button ----

const testURL = "/antennarr/api/notifications/test"

func TestTestingASavedChannelUsesItsSavedSecrets(t *testing.T) {
	e, ft := notifyEnv(t)
	id := addNtfy(t, e, "Phone", "tk_SAVED")
	code, v := e.call(t, "POST", testURL, `{"id":"`+id+`"}`)
	if code != 200 || obj(t, v)["ok"] != true {
		t.Fatalf("%d %v", code, v)
	}
	if got := ft.last(t); got.ID != id || got.Token != "tk_SAVED" || got.Topic != "antennarr" {
		t.Errorf("tested %+v", got)
	}
}

func TestTestingAChannelThatIsNotSavedYet(t *testing.T) {
	e, ft := notifyEnv(t)
	code, v := e.call(t, "POST", testURL, `{"channel":{"name":"New","type":"pushover","token":"app","user":"usr"}}`)
	if code != 200 {
		t.Fatalf("%d %v", code, v)
	}
	if got := ft.last(t); got.Type != access.ChannelPushover || got.Token != "app" || got.User != "usr" {
		t.Errorf("tested %+v", got)
	}
	if n := len(saved(t, e).NotifyChannels()); n != 0 {
		t.Errorf("a test saved %d channels", n)
	}
}

func TestTestingChangesToASavedChannelKeepsItsSecretAndSavesNothing(t *testing.T) {
	e, ft := notifyEnv(t)
	id := addNtfy(t, e, "Phone", "tk_SAVED")
	code, _ := e.call(t, "POST", testURL, `{"id":"`+id+`","channel":{"name":"Phone","type":"ntfy","url":"https://ntfy.example.com","topic":"edited"}}`)
	if code != 200 {
		t.Fatalf("code %d", code)
	}
	if got := ft.last(t); got.ID != id || got.Topic != "edited" || got.URL != "https://ntfy.example.com" || got.Token != "tk_SAVED" {
		t.Errorf("tested %+v", got)
	}
	if got := saved(t, e).NotifyChannels()[0]; got.Topic != "antennarr" {
		t.Errorf("the test saved the edit: %+v", got)
	}
}

func TestATestThatCannotBeSentExplainsWhyWithoutTheSecret(t *testing.T) {
	e, ft := notifyEnv(t)
	id := addNtfy(t, e, "Phone", "tk_SECRET")
	ft.err = errors.New("ntfy.sh answered 401 Unauthorized: bad token")
	code, v := e.call(t, "POST", testURL, `{"id":"`+id+`"}`)
	if code != 502 || !strings.Contains(errText(v), "401") || !strings.Contains(errText(v), "Phone") {
		t.Fatalf("%d %v", code, v)
	}
	if strings.Contains(errText(v), "tk_SECRET") {
		t.Errorf("the error repeats the token: %s", errText(v))
	}
}

func TestATestOfSomethingThatIsNotValidIsRefusedBeforeAnythingIsSent(t *testing.T) {
	e, ft := notifyEnv(t)
	for name, body := range map[string]string{
		"nothing to test": `{}`,
		"a bad draft":     `{"channel":{"name":"A","type":"ntfy","url":"ntfy.sh","topic":"t"}}`,
		"an unknown id":   `{"id":"cnope"}`,
		"an extra field":  `{"id":"x","surprise":1}`,
	} {
		code, v := e.call(t, "POST", testURL, body)
		if code != 400 && code != 404 {
			t.Errorf("%s: %d %v", name, code, v)
		}
	}
	if ft.count() != 0 {
		t.Errorf("%d tests were sent for requests that were not valid", ft.count())
	}
}

func TestTestsToTheSameChannelAreSpacedOut(t *testing.T) {
	e, ft := notifyEnv(t)
	id := addNtfy(t, e, "Phone", "")
	if code, _ := e.call(t, "POST", testURL, `{"id":"`+id+`"}`); code != 200 {
		t.Fatalf("first: %d", code)
	}
	if code, v := e.call(t, "POST", testURL, `{"id":"`+id+`"}`); code != 429 || !strings.Contains(errText(v), "seconds") {
		t.Errorf("second: %d %v", code, v)
	}
	if ft.count() != 1 {
		t.Errorf("%d tests sent, want 1", ft.count())
	}
	other := addNtfy(t, e, "Tablet", "")
	if code, _ := e.call(t, "POST", testURL, `{"id":"`+other+`"}`); code != 200 {
		t.Errorf("another channel was held back: %d", code)
	}
}

func TestTestingWithNoNotifierSaysSo(t *testing.T) {
	e := newEnv(t, johnAndMom, true) // no Notify
	if code, v := e.call(t, "POST", testURL, `{"channel":{"name":"A","type":"ntfy","url":"https://ntfy.sh","topic":"t"}}`); code != 409 || !strings.Contains(errText(v), "not available") {
		t.Errorf("%d %v", code, v)
	}
}

func TestNotificationRoutesRefuseTheWrongMethodAndOutsiders(t *testing.T) {
	e, _ := notifyEnv(t)
	if code, _ := e.call(t, "GET", testURL, ""); code != 405 {
		t.Errorf("GET test: %d", code)
	}
	if code, _ := e.call(t, "GET", channels, ""); code != 405 {
		t.Errorf("GET channels: %d", code)
	}
	for _, path := range []string{testURL, channels} {
		if code, _ := e.call(t, "POST", path, `{}`, "Origin", "https://evil.example"); code != 403 {
			t.Errorf("a cross-site POST to %s: %d", path, code)
		}
	}
}

func TestFirmwareMessagesNeedTheFirmwareCheck(t *testing.T) {
	off := false
	settings := johnAndMom
	settings.FirmwareCheck = &off
	e := newEnv(t, settings, true)
	e.api.Notify = &fakeTester{}
	patch := func(body string) (int, any) { return e.call(t, "PATCH", "/antennarr/api/settings", body) }

	if code, v := patch(`{"notifyEvents":{"firmware":true}}`); code != 400 || !strings.Contains(errText(v), "Check for new tuner firmware") {
		t.Errorf("with the check off: %d %v", code, v)
	}
	if saved(t, e).Notifications != nil {
		t.Errorf("a refused change was saved: %+v", saved(t, e).Notifications)
	}
	// the other events, and switching firmware off, are not held up by it
	if code, v := patch(`{"notifyEvents":{"tuner":false,"firmware":false}}`); code != 200 {
		t.Errorf("other events: %d %v", code, v)
	}
	// turning the check on in the same request is fine
	if code, v := patch(`{"firmwareCheck":true,"notifyEvents":{"firmware":true}}`); code != 200 {
		t.Errorf("both together: %d %v", code, v)
	}
	if !saved(t, e).NotifyEnabled(access.NotifyFirmware) {
		t.Error("the firmware switch was not saved")
	}
	// and once on, turning the check off leaves the switch as it was, to take effect again with the check
	if code, v := patch(`{"firmwareCheck":false}`); code != 200 {
		t.Fatalf("%d %v", code, v)
	}
	if !saved(t, e).NotifyEnabled(access.NotifyFirmware) {
		t.Error("turning the check off changed the firmware switch")
	}
	if code, _ := patch(`{"notifyEvents":{"firmware":true}}`); code != 400 {
		t.Errorf("re-sending it with the check off: %d", code)
	}
}

func TestAChannelAtAnAddressNoServerIsAtIsRefusedWithTheReason(t *testing.T) {
	e, ft := notifyEnv(t)
	for name, body := range map[string]string{
		"the cloud metadata service": `{"name":"A","type":"webhook","url":"http://169.254.169.254/latest/meta-data/"}`,
		"link-local IPv6":            `{"name":"A","type":"ntfy","url":"http://[fe80::1]:8080","topic":"t"}`,
		"the unspecified address":    `{"name":"A","type":"webhook","url":"http://0.0.0.0:8080/x"}`,
	} {
		code, v := e.call(t, "POST", channels, body)
		if code != 400 || !strings.Contains(errText(v), "link-local") {
			t.Errorf("adding %s: %d %v", name, code, v)
		}
	}
	// a test of one is refused before anything is sent
	code, v := e.call(t, "POST", testURL, `{"channel":{"name":"A","type":"webhook","url":"http://169.254.169.254/latest/meta-data/"}}`)
	if code != 400 || !strings.Contains(errText(v), "link-local") {
		t.Errorf("testing one: %d %v", code, v)
	}
	if ft.count() != 0 {
		t.Errorf("%d tests were sent", ft.count())
	}
	// while this machine and the LAN are fine, since a ntfy server there is the usual case
	for _, url := range []string{"http://127.0.0.1:8080", "http://192.168.1.20:2586", "http://100.64.0.9"} {
		if code, v := e.call(t, "POST", channels, `{"name":"`+url+`","type":"webhook","url":"`+url+`"}`); code != 201 {
			t.Errorf("%s: %d %v", url, code, v)
		}
	}
}
