package access

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// What a notification channel is.
const (
	ChannelNtfy     = "ntfy"
	ChannelPushover = "pushover"
	ChannelWebhook  = "webhook"
)

// The kinds of event a notification can be about, which the owner switches on and off.
const (
	NotifyFirmware = "firmware" // newer tuner firmware was found
	NotifyTuner    = "tuner"    // a tuner went offline, or came back
	NotifyViewer   = "viewer"   // a viewer was refused: at its limit, or over a cap
	NotifyUnknown  = "unknown"  // an address that isn't a viewer tried to connect
	NotifyWatching = "watching" // a viewer started watching
)

const (
	maxChannels    = 8
	maxChannelText = 300 // URL, tokens and keys
)

var (
	channelIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	ntfyTopicRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// NotifyChannel is somewhere to send notifications. Its secrets (Token, User and,
// for a webhook, the URL) are written to the settings file, which is only
// readable by its owner, and are never sent to the browser.
type NotifyChannel struct {
	// ID identifies the channel. It is made once and never changes.
	ID   string `json:"id"`
	Name string `json:"name"`
	// Type is ChannelNtfy, ChannelPushover or ChannelWebhook.
	Type string `json:"type"`
	// URL is the ntfy server (such as https://ntfy.sh) or the webhook's address.
	// A Pushover channel doesn't have one.
	URL string `json:"url,omitempty"`
	// Topic is the ntfy topic.
	Topic string `json:"topic,omitempty"`
	// Token is the ntfy or webhook bearer token (optional), or the Pushover application token.
	Token string `json:"token,omitempty"`
	// User is the Pushover user (or group) key.
	User string `json:"user,omitempty"`
}

// NotifyEvents says which kinds of event are sent. An unset one takes its default:
// firmware, tuner and viewer are on, and unknown addresses are off, since a tuner
// port that faces the internet is scanned all day. A viewer starting to watch is off too: it is only wanted by
// someone who wants to know each time.
type NotifyEvents struct {
	Firmware *bool `json:"firmware,omitempty"`
	Tuner    *bool `json:"tuner,omitempty"`
	Viewer   *bool `json:"viewer,omitempty"`
	Unknown  *bool `json:"unknown,omitempty"`
	Watching *bool `json:"watching,omitempty"`
}

// Notifications is the optional "tell me about it" settings. With no channels
// nothing is sent and nothing is contacted.
type Notifications struct {
	Channels []NotifyChannel `json:"channels,omitempty"`
	Events   NotifyEvents    `json:"events,omitempty"`
}

// NotifyChannels is a copy of the channels.
func (s Settings) NotifyChannels() []NotifyChannel {
	if s.Notifications == nil {
		return nil
	}
	return append([]NotifyChannel(nil), s.Notifications.Channels...)
}

// NotifyEnabled reports whether this kind of event is switched on.
func (s Settings) NotifyEnabled(kind string) bool {
	var ev NotifyEvents
	if s.Notifications != nil {
		ev = s.Notifications.Events
	}
	pick := func(p *bool, def bool) bool {
		if p == nil {
			return def
		}
		return *p
	}
	switch kind {
	case NotifyFirmware:
		return pick(ev.Firmware, true)
	case NotifyTuner:
		return pick(ev.Tuner, true)
	case NotifyViewer:
		return pick(ev.Viewer, true)
	case NotifyUnknown:
		return pick(ev.Unknown, false)
	case NotifyWatching:
		return pick(ev.Watching, false)
	}
	return false
}

// NotifyChannel finds a channel by ID.
func (s Settings) NotifyChannel(id string) (NotifyChannel, bool) {
	if s.Notifications != nil {
		for _, c := range s.Notifications.Channels {
			if c.ID == id {
				return c, true
			}
		}
	}
	return NotifyChannel{}, false
}

func (n *Notifications) clone() *Notifications {
	if n == nil {
		return nil
	}
	out := *n
	out.Channels = append([]NotifyChannel(nil), n.Channels...)
	out.Events = NotifyEvents{Firmware: cloneBool(n.Events.Firmware), Tuner: cloneBool(n.Events.Tuner), Viewer: cloneBool(n.Events.Viewer), Unknown: cloneBool(n.Events.Unknown), Watching: cloneBool(n.Events.Watching)}
	return &out
}

func cloneBool(p *bool) *bool {
	if p == nil {
		return nil
	}
	b := *p
	return &b
}

func (n *Notifications) validate() error {
	if n == nil {
		return nil
	}
	if len(n.Channels) > maxChannels {
		return fmt.Errorf("notifications: at most %d channels", maxChannels)
	}
	seenID := map[string]bool{}
	seenName := map[string]bool{}
	for _, c := range n.Channels {
		if err := c.Validate(); err != nil {
			return fmt.Errorf("notifications: %w", err)
		}
		if seenID[c.ID] {
			return fmt.Errorf("notifications: channel ID %q is used twice", c.ID)
		}
		seenID[c.ID] = true
		key := strings.ToLower(strings.TrimSpace(c.Name))
		if seenName[key] {
			return fmt.Errorf("notifications: there are two channels called %q", c.Name)
		}
		seenName[key] = true
	}
	return nil
}

// Validate reports the first problem with a channel. Its messages never include
// the secrets in it.
func (c NotifyChannel) Validate() error {
	if !channelIDRe.MatchString(c.ID) {
		return errors.New("a channel has no valid ID")
	}
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return errors.New("give the channel a name")
	}
	if err := ValidateLabel(name); err != nil {
		return fmt.Errorf("channel name: %w", err)
	}
	for _, field := range []string{c.URL, c.Topic, c.Token, c.User} {
		if len(field) > maxChannelText || strings.ContainsFunc(field, unicode.IsControl) {
			return fmt.Errorf("channel %q: a field is too long or has control characters", name)
		}
	}
	switch c.Type {
	case ChannelNtfy:
		if err := validChannelURL(c.URL); err != nil {
			return fmt.Errorf("channel %q: the server address %w", name, err)
		}
		if !ntfyTopicRe.MatchString(c.Topic) {
			return fmt.Errorf("channel %q: the topic must be 1 to 64 letters, digits, '_' or '-'", name)
		}
		if c.User != "" {
			return fmt.Errorf("channel %q: a user key is for Pushover", name)
		}
	case ChannelPushover:
		if c.Token == "" || c.User == "" {
			return fmt.Errorf("channel %q: Pushover needs an application token and a user key", name)
		}
		if c.URL != "" || c.Topic != "" {
			return fmt.Errorf("channel %q: a server address and topic are for ntfy", name)
		}
	case ChannelWebhook:
		if err := validChannelURL(c.URL); err != nil {
			return fmt.Errorf("channel %q: the webhook address %w", name, err)
		}
		if c.Topic != "" || c.User != "" {
			return fmt.Errorf("channel %q: a topic and user key are not for a webhook", name)
		}
	default:
		return fmt.Errorf("channel %q: the type must be %s, %s or %s", name, ChannelNtfy, ChannelPushover, ChannelWebhook)
	}
	return nil
}

// NotifyAddrAllowed reports whether Antennarr will send a notification to this
// address. A private or LAN address is allowed on purpose, since a ntfy server on
// your own network is the usual case, and so is this machine. What isn't is an
// address no server is ever at, and the one that holds the keys of a cloud machine:
// link-local (169.254.0.0/16, which includes the cloud metadata service at
// 169.254.169.254, and fe80::/10), the unspecified address, multicast and broadcast.
func NotifyAddrAllowed(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsValid() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast() && !ip.IsUnspecified() &&
		ip != netip.AddrFrom4([4]byte{255, 255, 255, 255})
}

// validChannelURL accepts an http or https address with a host. A password in
// the address is refused: it would end up in logs and error messages. (A token
// in the path or query, which some webhooks use, is fine; it is never logged.)
func validChannelURL(raw string) error {
	if raw == "" {
		return errors.New("is empty")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return errors.New("must be an http:// or https:// address")
	}
	if u.User != nil {
		return errors.New("can't have a username or password in it; use the token field")
	}
	// A name is looked up when the message is sent, and checked then; an address written out is checked now.
	if ip, err := netip.ParseAddr(strings.Trim(u.Hostname(), "[]")); err == nil && !NotifyAddrAllowed(ip) {
		return errors.New("can't be a link-local, multicast or unspecified address")
	}
	return nil
}
