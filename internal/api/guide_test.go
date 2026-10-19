package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/guide"
	"github.com/johnschult/antennarr/internal/netinfo"
)

func TestSettingsReportTheGuideSwitch(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	_, v := e.call(t, "GET", "/antennarr/api/settings", "")
	if obj(t, v)["guide"] != true {
		t.Errorf("default = %v, want on", obj(t, v)["guide"])
	}
	code, v := e.call(t, "PATCH", "/antennarr/api/settings", `{"guide":false}`)
	if code != 200 || obj(t, v)["guide"] != false {
		t.Fatalf("%d %v", code, v)
	}
	if fileSettings(t, e).GuideEnabled() {
		t.Error("the choice was not saved")
	}
	e.call(t, "PATCH", "/antennarr/api/settings", `{"sharedCap":2}`)
	if fileSettings(t, e).GuideEnabled() {
		t.Error("an unrelated patch switched the guide back on")
	}
	if code, _ := e.call(t, "PATCH", "/antennarr/api/settings", `{"guide":"yes"}`); code != 400 {
		t.Errorf("a non-boolean: %d", code)
	}
	if code, v := e.call(t, "PATCH", "/antennarr/api/settings", `{"guide":true}`); code != 200 || obj(t, v)["guide"] != true {
		t.Errorf("switching back on: %d %v", code, v)
	}
}

func TestSwitchingTheGuideOnKicksTheService(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	hits := make(chan struct{}, 4)
	sd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits <- struct{}{}
		body, _ := os.ReadFile("../guide/testdata/xmltv.xml")
		w.Write(body)
	}))
	defer sd.Close()
	svc := guide.NewService(guide.Config{URL: sd.URL, Interval: time.Hour, MinGap: time.Nanosecond,
		Enabled: func() bool { return e.pol.Settings().GuideEnabled() }})
	svc.Watch("antenna", func() string { return "auth" })
	e.api.Guide = svc
	e.call(t, "PATCH", "/antennarr/api/settings", `{"guide":false}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)
	time.Sleep(100 * time.Millisecond)
	select {
	case <-hits:
		t.Fatal("looked while off")
	default:
	}
	e.call(t, "PATCH", "/antennarr/api/settings", `{"guide":true}`)
	select {
	case <-hits:
	case <-time.After(3 * time.Second):
		t.Error("switching the guide on did not make it look")
	}
}

func TestStateHasAnEmptyGuideWhenThereIsNone(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	_, v := e.call(t, "GET", "/antennarr/api/state", "")
	d := obj(t, obj(t, v)["devices"].([]any)[0])
	g, ok := d["guide"].(map[string]any)
	if !ok || len(g) != 0 {
		t.Errorf("guide = %v, want {} (never null, so the UI needn't check)", d["guide"])
	}
}

func TestLogoRouteIsClosedWithoutAGuide(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	path := "/antennarr/api/devices/antenna/guide/11.1/icon"
	if code, _ := e.call(t, "GET", path, ""); code != 404 {
		t.Errorf("no service: %d", code)
	}
	e.api.Guide = guide.NewService(guide.Config{})
	if code, _ := e.call(t, "GET", path, ""); code != 404 {
		t.Errorf("service that knows nothing: %d", code)
	}
	e.call(t, "PATCH", "/antennarr/api/settings", `{"guide":false}`)
	if code, _ := e.call(t, "GET", path, ""); code != 404 {
		t.Errorf("while off: %d", code)
	}
	if code, _ := e.call(t, "POST", path, ""); code != 405 {
		t.Errorf("POST: %d, want 405", code)
	}
}

func TestNetworkListsThisMachinesAddresses(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	e.api.Network = func() []netinfo.Address {
		return []netinfo.Address{{IP: "100.64.0.9", Kind: netinfo.Tailscale, Interface: "tailscale0"}, {IP: "192.168.1.5", Kind: netinfo.LAN, Interface: "eth0"}}
	}
	e.api.Firewall = func() netinfo.Firewall { return netinfo.Firewall{Tool: "ufw", Active: true} }
	code, v := e.call(t, "GET", "/antennarr/api/network", "")
	if fw := obj(t, obj(t, v)["firewall"]); fw["tool"] != "ufw" || fw["active"] != true {
		t.Errorf("firewall = %v", fw)
	}
	addrs, _ := obj(t, v)["addresses"].([]any)
	if code != 200 || len(addrs) != 2 || obj(t, addrs[0])["ip"] != "100.64.0.9" || obj(t, addrs[0])["kind"] != "tailscale" || obj(t, addrs[1])["interface"] != "eth0" {
		t.Fatalf("%d %v", code, v)
	}
}

func TestNetworkIsAnEmptyListWhenThereIsNothingToOffer(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	e.api.Network = func() []netinfo.Address { return nil }
	e.api.Firewall = func() netinfo.Firewall { return netinfo.Firewall{} }
	_, v := e.call(t, "GET", "/antennarr/api/network", "")
	if a, ok := obj(t, v)["addresses"].([]any); !ok || len(a) != 0 {
		t.Errorf("addresses = %v, want [] (never null)", obj(t, v)["addresses"])
	}
	if fw := obj(t, obj(t, v)["firewall"]); fw["tool"] != "" || fw["active"] != false {
		t.Errorf("firewall = %v, want nothing found", fw)
	}
}

func TestNetworkIsReadOnly(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	if code, _ := e.call(t, "POST", "/antennarr/api/network", "{}"); code != 405 {
		t.Errorf("POST: %d, want 405", code)
	}
}

func TestStateCarriesTheVersionAndItsGitHubPage(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	code, v := e.call(t, "GET", "/antennarr/api/state", "")
	st := obj(t, v)
	ver, _ := st["version"].(string)
	url, _ := st["versionUrl"].(string)
	docs, _ := st["docsUrl"].(string)
	if code != 200 || ver == "" || !strings.HasPrefix(url, "https://github.com/johnschult/antennarr") {
		t.Fatalf("version=%q url=%q (%d)", ver, url, code)
	}
	if !strings.HasPrefix(docs, "https://github.com/johnschult/antennarr/blob/") || !strings.HasSuffix(docs, "/docs/README.md") {
		t.Errorf("docsUrl = %q", docs)
	}
}

func TestPictureRouteIsClosedWithoutAGuide(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	path := "/antennarr/api/devices/antenna/guide/11.1/poster/C22744932ENOGRU"
	if code, _ := e.call(t, "GET", path, ""); code != 404 {
		t.Errorf("no service: %d", code)
	}
	e.api.Guide = guide.NewService(guide.Config{})
	if code, _ := e.call(t, "GET", path, ""); code != 404 {
		t.Errorf("service that knows nothing: %d", code)
	}
	e.call(t, "PATCH", "/antennarr/api/settings", `{"guide":false}`)
	if code, _ := e.call(t, "GET", path, ""); code != 404 {
		t.Errorf("while off: %d", code)
	}
	if code, _ := e.call(t, "POST", path, ""); code != 405 {
		t.Errorf("POST: %d, want 405", code)
	}
}
