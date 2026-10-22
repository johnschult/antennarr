package api

import (
	"strings"
	"testing"

	"github.com/johnschult/yagi/internal/access"
)

func TestAPublicAddressCannotBeAddedAsAViewer(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	code, v := e.call(t, "POST", "/api/viewers", `{"name":"Dad","addresses":["10.8.0.6","203.0.113.9"],"limit":1}`)
	if code != 400 || !strings.Contains(obj(t, v)["error"].(string), "203.0.113.9 is a public internet address") || !strings.Contains(obj(t, v)["error"].(string), "Tailscale or a VPN") {
		t.Errorf("a public address = %d %v", code, v)
	}
	if _, body := e.call(t, "GET", "/api/viewers", ""); strings.Contains(toJSON(body), "Dad") {
		t.Error("the viewer was added anyway")
	}
	for name, addrs := range map[string]string{
		"a LAN address":        `["192.168.1.40"]`,
		"a VPN range":          `["10.9.0.0/24"]`,
		"a Tailscale address":  `["100.64.0.9"]`,
		"a Tailscale login":    `["tailscale:dad@example.com"]`,
		"a Tailscale device":   `["tailscale:laptop.tailnet.ts.net"]`,
		"a unique-local range": `["fd12:3456::/48"]`,
	} {
		body := `{"name":"V` + strings.ReplaceAll(name, " ", "") + `","addresses":` + addrs + `,"limit":1}`
		if code, v := e.call(t, "POST", "/api/viewers", body); code != 201 {
			t.Errorf("%s = %d %v", name, code, v)
		}
	}
}

func TestEditingAViewerCannotPutAPublicAddressOnThem(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	if code, v := e.call(t, "PUT", "/api/viewers/Mom", `{"name":"Mom","addresses":["198.51.100.4"],"limit":1}`); code != 400 || !strings.Contains(obj(t, v)["error"].(string), "public internet address") {
		t.Errorf("= %d %v", code, v)
	}
	if code, _ := e.call(t, "PUT", "/api/viewers/Mom", `{"name":"Mom","addresses":["10.8.0.9"],"limit":1}`); code != 200 {
		t.Errorf("a private address = %d", code)
	}
}

func TestAPublicAddressCannotBeAnAdmin(t *testing.T) {
	e := newEnv(t, johnAndMom, true)
	if code, v := e.call(t, "PATCH", "/api/settings", `{"admin":["192.168.1.0/24","203.0.113.0/24"]}`); code != 400 || !strings.Contains(obj(t, v)["error"].(string), "203.0.113.0/24 is a public internet address") {
		t.Errorf("= %d %v", code, v)
	}
	if code, _ := e.call(t, "PATCH", "/api/settings", `{"admin":["192.168.1.0/24","tailscale:me@example.com"]}`); code != 200 {
		t.Errorf("a private range and a login = %d", code)
	}
}

func TestAViewerAlreadyListedWithAPublicAddressSaysSo(t *testing.T) {
	s := johnAndMom
	s.Viewers = append(append([]access.Viewer(nil), s.Viewers...), access.Viewer{Name: "Old", Addresses: []string{"203.0.113.9", "10.8.0.8"}, Limit: 1})
	e := newEnv(t, s, true)
	_, v := e.call(t, "GET", "/api/viewers", "")
	var old map[string]any
	for _, x := range v.([]any) {
		if m := obj(t, x); m["name"] == "Old" {
			old = m
		}
	}
	if old == nil || toJSON(old["publicAddresses"]) != `["203.0.113.9"]` {
		t.Errorf("Old = %v", old)
	}
	for _, x := range v.([]any) {
		if m := obj(t, x); m["name"] == "John" {
			if _, has := m["publicAddresses"]; has {
				t.Errorf("a viewer with none has the field: %v", m)
			}
		}
	}
}
