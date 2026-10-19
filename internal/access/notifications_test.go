package access

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ptr(b bool) *bool { return &b }

func ntfy(id string) NotifyChannel {
	return NotifyChannel{ID: id, Name: "Phone " + id, Type: ChannelNtfy, URL: "https://ntfy.sh", Topic: "yagi-" + id}
}

func TestNotificationChannelsValidate(t *testing.T) {
	good := []NotifyChannel{
		ntfy("a"),
		{ID: "home", Name: "Home ntfy", Type: ChannelNtfy, URL: "http://192.168.1.20:8080", Topic: "tv", Token: "tk_abc"},
		{ID: "po", Name: "Pushover", Type: ChannelPushover, Token: "app-token", User: "user-key"},
		{ID: "hook", Name: "Discord", Type: ChannelWebhook, URL: "https://discord.com/api/webhooks/1/secret-part"},
		{ID: "hook2", Name: "Home Assistant", Type: ChannelWebhook, URL: "http://homeassistant.local:8123/api/webhook/xyz", Token: "bearer"},
	}
	for _, c := range good {
		if err := (Settings{Notifications: &Notifications{Channels: []NotifyChannel{c}}}).Validate(); err != nil {
			t.Errorf("%s: rejected: %v", c.Name, err)
		}
	}

	with := func(f func(*NotifyChannel)) NotifyChannel {
		c := ntfy("a")
		f(&c)
		return c
	}
	bad := map[string]NotifyChannel{
		"no id":                 with(func(c *NotifyChannel) { c.ID = "" }),
		"id with a slash":       with(func(c *NotifyChannel) { c.ID = "a/b" }),
		"no name":               with(func(c *NotifyChannel) { c.Name = "  " }),
		"unknown type":          with(func(c *NotifyChannel) { c.Type = "email" }),
		"ntfy without a server": with(func(c *NotifyChannel) { c.URL = "" }),
		"ntfy ftp server":       with(func(c *NotifyChannel) { c.URL = "ftp://ntfy.sh" }),
		"ntfy bare host":        with(func(c *NotifyChannel) { c.URL = "ntfy.sh" }),
		"ntfy password in url":  with(func(c *NotifyChannel) { c.URL = "https://me:hunter2@ntfy.sh" }),
		"ntfy topic with slash": with(func(c *NotifyChannel) { c.Topic = "a/b" }),
		"ntfy no topic":         with(func(c *NotifyChannel) { c.Topic = "" }),
		"ntfy with a user key":  with(func(c *NotifyChannel) { c.User = "u" }),
		"pushover no user":      {ID: "p", Name: "P", Type: ChannelPushover, Token: "t"},
		"pushover no token":     {ID: "p", Name: "P", Type: ChannelPushover, User: "u"},
		"pushover with a url":   {ID: "p", Name: "P", Type: ChannelPushover, Token: "t", User: "u", URL: "https://example.com"},
		"webhook no url":        {ID: "w", Name: "W", Type: ChannelWebhook},
		"webhook with a topic":  {ID: "w", Name: "W", Type: ChannelWebhook, URL: "https://example.com", Topic: "x"},
		"control character":     with(func(c *NotifyChannel) { c.Token = "a\nb" }),
		"very long token":       with(func(c *NotifyChannel) { c.Token = strings.Repeat("x", 301) }),
	}
	for name, c := range bad {
		if err := (Settings{Notifications: &Notifications{Channels: []NotifyChannel{c}}}).Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	dup := Notifications{Channels: []NotifyChannel{ntfy("a"), ntfy("a")}}
	if err := (Settings{Notifications: &dup}).Validate(); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("duplicate id: %v", err)
	}
	same := ntfy("b")
	same.Name = " phone a "
	if err := (Settings{Notifications: &Notifications{Channels: []NotifyChannel{ntfy("a"), same}}}).Validate(); err == nil {
		t.Error("two channels with one name were accepted")
	}
	var many []NotifyChannel
	for i := 0; i < maxChannels+1; i++ {
		many = append(many, ntfy(string(rune('a'+i))))
	}
	if err := (Settings{Notifications: &Notifications{Channels: many}}).Validate(); err == nil {
		t.Error("too many channels were accepted")
	}
}

func TestNotificationErrorsNeverQuoteSecrets(t *testing.T) {
	secrets := []string{"hunter2-password", "tok_SECRET", "uk_SECRET"}
	cases := []NotifyChannel{
		{ID: "a", Name: "A", Type: ChannelNtfy, URL: "https://me:hunter2-password@ntfy.sh", Topic: "t", Token: "tok_SECRET"},
		{ID: "b", Name: "B", Type: ChannelNtfy, URL: "https://ntfy.sh", Topic: "bad topic", Token: "tok_SECRET"},
		{ID: "c", Name: "C", Type: ChannelPushover, Token: "tok_SECRET", User: "uk_SECRET", URL: "https://x"},
		{ID: "d", Name: "D", Type: "carrier-pigeon", Token: "tok_SECRET", User: "uk_SECRET"},
		{ID: "e", Name: "E", Type: ChannelWebhook, URL: "https://hooks.example/tok_SECRET", Topic: "x"},
	}
	for _, c := range cases {
		err := c.Validate()
		if err == nil {
			t.Errorf("%s: accepted", c.Name)
			continue
		}
		for _, s := range secrets {
			if strings.Contains(err.Error(), s) {
				t.Errorf("%s: the error repeats a secret: %v", c.Name, err)
			}
		}
	}
}

func TestNotificationEventDefaults(t *testing.T) {
	var s Settings
	for kind, want := range map[string]bool{NotifyFirmware: true, NotifyTuner: true, NotifyViewer: true, NotifyUnknown: false, "nonsense": false} {
		if got := s.NotifyEnabled(kind); got != want {
			t.Errorf("default for %s = %v, want %v", kind, got, want)
		}
	}
	s.Notifications = &Notifications{Events: NotifyEvents{Firmware: ptr(false), Unknown: ptr(true)}}
	for kind, want := range map[string]bool{NotifyFirmware: false, NotifyTuner: true, NotifyViewer: true, NotifyUnknown: true} {
		if got := s.NotifyEnabled(kind); got != want {
			t.Errorf("set: %s = %v, want %v", kind, got, want)
		}
	}
}

func TestNotificationsCloneIsDeep(t *testing.T) {
	s := Settings{Notifications: &Notifications{Channels: []NotifyChannel{ntfy("a")}, Events: NotifyEvents{Tuner: ptr(true)}}}
	c := s.Clone()
	c.Notifications.Channels[0].Token = "changed"
	c.Notifications.Channels = append(c.Notifications.Channels, ntfy("b"))
	*c.Notifications.Events.Tuner = false
	if s.Notifications.Channels[0].Token != "" || len(s.Notifications.Channels) != 1 || !*s.Notifications.Events.Tuner {
		t.Errorf("changing the copy changed the original: %+v", s.Notifications)
	}
	if got := s.NotifyChannels(); len(got) != 1 {
		t.Fatalf("NotifyChannels = %v", got)
	}
	s.NotifyChannels()[0].Name = "x" // a copy
	if s.Notifications.Channels[0].Name == "x" {
		t.Error("NotifyChannels shares memory with the settings")
	}
	if _, ok := s.NotifyChannel("a"); !ok {
		t.Error("channel a not found")
	}
	if _, ok := s.NotifyChannel("zzz"); ok {
		t.Error("found a channel that is not there")
	}
}

func TestNotificationsSurviveTheFileAndStayPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "yagi.json")
	in := Settings{OwnerReserve: 1, Notifications: &Notifications{
		Channels: []NotifyChannel{{ID: "po", Name: "Pushover", Type: ChannelPushover, Token: "app-token", User: "user-key"}, ntfy("a")},
		Events:   NotifyEvents{Unknown: ptr(true)},
	}}
	if err := SaveSettings(path, in); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the file holds secrets and must be 0600: %v %v", info, err)
	}
	out, _, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := out.NotifyChannels(); len(got) != 2 || got[0].Token != "app-token" || got[0].User != "user-key" {
		t.Errorf("round trip lost the channels: %+v", got)
	}
	if !out.NotifyEnabled(NotifyUnknown) || !out.NotifyEnabled(NotifyFirmware) {
		t.Errorf("round trip lost the events: %+v", out.Notifications.Events)
	}

	// A file with none of it, as every existing install has, loads as before and writes none of it back.
	plain := filepath.Join(t.TempDir(), "plain.json")
	if err := SaveSettings(plain, Settings{OwnerReserve: 1}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(plain)
	if strings.Contains(string(b), "notifications") {
		t.Errorf("an install without notifications gained a key:\n%s", b)
	}
}

func TestNotificationsRejectUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "yagi.json")
	for name, body := range map[string]string{
		"a typo in a channel": `{"viewers":[],"admin":[],"notifications":{"channels":[{"id":"a","name":"A","type":"ntfy","url":"https://ntfy.sh","topic":"t","tokn":"x"}]}}`,
		"a typo in events":    `{"viewers":[],"admin":[],"notifications":{"events":{"firmwre":true}}}`,
		"a typo at the top":   `{"viewers":[],"admin":[],"notifcations":{}}`,
	} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadSettings(path); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestNotifyAddrAllowed(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1":       true, // this machine: ntfy next to Yagi
		"::1":             true,
		"192.168.1.20":    true, // a LAN server
		"10.0.0.5":        true,
		"172.16.0.9":      true,
		"100.64.0.9":      true, // Tailscale
		"fd7a:115c::1":    true,
		"203.0.113.9":     true, // a public server
		"2001:db8::1":     true,
		"::ffff:10.0.0.5": true, // an IPv4 address written the IPv6 way
		// not allowed: nothing is ever there, or it is a cloud machine's keys
		"169.254.169.254":        false, // the cloud metadata service
		"169.254.0.1":            false,
		"fe80::1":                false,
		"fe80::1%en0":            false,
		"::ffff:169.254.169.254": false, // the same address, written the IPv6 way
		"0.0.0.0":                false,
		"::":                     false,
		"224.0.0.1":              false,
		"ff02::1":                false,
		"255.255.255.255":        false,
	} {
		ip, err := netip.ParseAddr(addr)
		if err != nil {
			t.Fatalf("%s: %v", addr, err)
		}
		if got := NotifyAddrAllowed(ip); got != want {
			t.Errorf("NotifyAddrAllowed(%s) = %v, want %v", addr, got, want)
		}
	}
	if NotifyAddrAllowed(netip.Addr{}) {
		t.Error("the zero address was allowed")
	}
}

func TestChannelAddressesWrittenOutAreCheckedWhenSaved(t *testing.T) {
	try := func(typ, raw string) error {
		c := NotifyChannel{ID: "a", Name: "A", Type: typ, URL: raw, Topic: "t"}
		if typ == ChannelWebhook {
			c.Topic = ""
		}
		return c.Validate()
	}
	for _, raw := range []string{
		"http://169.254.169.254/latest/meta-data", "http://169.254.169.254:80", "https://[fe80::1]/x", "http://0.0.0.0:8080",
		"http://224.0.0.1/x", "http://255.255.255.255/x", "http://[::ffff:169.254.169.254]/x",
	} {
		for _, typ := range []string{ChannelNtfy, ChannelWebhook} {
			if err := try(typ, raw); err == nil {
				t.Errorf("%s %s was accepted", typ, raw)
			}
		}
	}
	for _, raw := range []string{
		"http://127.0.0.1:8080", "http://[::1]:8080/x", "http://192.168.1.20", "http://10.0.0.5:2586", "http://100.64.0.9",
		"https://ntfy.sh", "https://ntfy.example.com/prefix", "http://homeassistant.local:8123/api/webhook/x", "http://localhost:8080",
	} {
		for _, typ := range []string{ChannelNtfy, ChannelWebhook} {
			if err := try(typ, raw); err != nil {
				t.Errorf("%s %s was refused: %v", typ, raw, err)
			}
		}
	}
	if err := try(ChannelNtfy, "http://169.254.169.254"); err == nil || !strings.Contains(err.Error(), "link-local") {
		t.Errorf("the reason should say why: %v", err)
	}
}
