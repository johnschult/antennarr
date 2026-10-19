package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/johnschult/yagi/internal/access"
)

// Tester sends a test message to a channel (the notify.Notifier).
type Tester interface {
	Test(ctx context.Context, ch access.NotifyChannel) error
}

// testGap is how soon after a test to the same channel another is refused, so the
// button can't be used to hammer someone's server.
const testGap = 3 * time.Second

// NotificationsDoc is the notification settings as the browser sees them. The
// secrets are deliberately not here: a channel says whether it has a token or a
// user key, not what it is, and a webhook's address (which often has a secret in
// it) is shown as its host only. A secret is written, never read back.
type NotificationsDoc struct {
	Channels []ChannelDoc `json:"channels"`
	Events   EventsDoc    `json:"events"`
}

// ChannelDoc is one channel without its secrets.
type ChannelDoc struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	URL   string `json:"url,omitempty"`   // the ntfy server; a webhook shows only Host
	Host  string `json:"host,omitempty"`  // the webhook's host
	Topic string `json:"topic,omitempty"` // ntfy
	// HasToken: a ntfy or webhook token, or the Pushover application token, is saved.
	HasToken bool `json:"hasToken"`
	// HasUser: the Pushover user key is saved.
	HasUser bool `json:"hasUser"`
}

// EventsDoc says which kinds of event are switched on.
type EventsDoc struct {
	Firmware bool `json:"firmware"`
	Tuner    bool `json:"tuner"`
	Viewer   bool `json:"viewer"`
	Unknown  bool `json:"unknown"`
	Watching bool `json:"watching"`
}

func notificationsDoc(set access.Settings) NotificationsDoc {
	d := NotificationsDoc{
		Channels: []ChannelDoc{},
		Events: EventsDoc{
			Firmware: set.NotifyEnabled(access.NotifyFirmware), Tuner: set.NotifyEnabled(access.NotifyTuner),
			Viewer: set.NotifyEnabled(access.NotifyViewer), Unknown: set.NotifyEnabled(access.NotifyUnknown),
			Watching: set.NotifyEnabled(access.NotifyWatching),
		},
	}
	for _, c := range set.NotifyChannels() {
		d.Channels = append(d.Channels, channelDoc(c))
	}
	return d
}

func channelDoc(c access.NotifyChannel) ChannelDoc {
	d := ChannelDoc{ID: c.ID, Name: c.Name, Type: c.Type, Topic: c.Topic, HasToken: c.Token != "", HasUser: c.User != ""}
	switch c.Type {
	case access.ChannelNtfy:
		d.URL = c.URL
	case access.ChannelWebhook:
		if u, err := url.Parse(c.URL); err == nil {
			d.Host = u.Host
		}
	}
	return d
}

// channelInput is what the browser sends to add or change a channel. A secret that
// is left empty keeps the one already saved.
type channelInput struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	URL   string `json:"url"`
	Topic string `json:"topic"`
	Token string `json:"token"`
	User  string `json:"user"`
	// ClearToken removes the saved token, since an empty Token means "keep it".
	ClearToken bool `json:"clearToken"`
}

// merge applies the input to a saved channel (or to nothing, for a new one).
// Secrets the input leaves empty are kept, but not when the type changes: a
// token for one service must not follow the channel to another.
func (in channelInput) merge(saved access.NotifyChannel) access.NotifyChannel {
	out := access.NotifyChannel{ID: saved.ID, Name: strings.TrimSpace(in.Name), Type: in.Type, URL: strings.TrimSpace(in.URL), Topic: strings.TrimSpace(in.Topic), Token: strings.TrimSpace(in.Token), User: strings.TrimSpace(in.User)}
	if saved.Type != in.Type {
		return out
	}
	if out.Token == "" && !in.ClearToken {
		out.Token = saved.Token
	}
	if out.User == "" {
		out.User = saved.User
	}
	// A webhook's address is only ever shown as its host, so leaving it empty keeps it.
	if out.Type == access.ChannelWebhook && out.URL == "" {
		out.URL = saved.URL
	}
	return out
}

func newChannelID(taken func(string) bool) string {
	for {
		b := make([]byte, 5)
		if _, err := rand.Read(b); err != nil {
			panic(err) // no randomness is not something to carry on without
		}
		if id := "c" + hex.EncodeToString(b); !taken(id) {
			return id
		}
	}
}

func channelIndex(chs []access.NotifyChannel, id string) int {
	for i, c := range chs {
		if c.ID == id {
			return i
		}
	}
	return -1
}

// edit changes the notification settings through the store, making the section if there is none.
func (a *API) editNotifications(fn func(n *access.Notifications) error) error {
	_, err := a.Store.Edit(func(s *access.Settings) error {
		if s.Notifications == nil {
			s.Notifications = &access.Notifications{}
		}
		return fn(s.Notifications)
	})
	return err
}

func (a *API) addChannel(w http.ResponseWriter, r *http.Request) {
	var in channelInput
	if !decode(w, r, &in) {
		return
	}
	var added access.NotifyChannel
	err := a.editNotifications(func(n *access.Notifications) error {
		c := in.merge(access.NotifyChannel{})
		c.ID = newChannelID(func(id string) bool { return channelIndex(n.Channels, id) >= 0 })
		if err := c.Validate(); err != nil {
			return &httpErr{http.StatusBadRequest, err.Error()}
		}
		n.Channels = append(n.Channels, c)
		added = c
		return nil
	})
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, channelDoc(added))
}

func (a *API) replaceChannel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in channelInput
	if !decode(w, r, &in) {
		return
	}
	var saved access.NotifyChannel
	err := a.editNotifications(func(n *access.Notifications) error {
		i := channelIndex(n.Channels, id)
		if i < 0 {
			return &httpErr{http.StatusNotFound, "no such notification channel"}
		}
		c := in.merge(n.Channels[i])
		if err := c.Validate(); err != nil {
			return &httpErr{http.StatusBadRequest, err.Error()}
		}
		n.Channels[i] = c
		saved = c
		return nil
	})
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, channelDoc(saved))
}

func (a *API) deleteChannel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := a.editNotifications(func(n *access.Notifications) error {
		i := channelIndex(n.Channels, id)
		if i < 0 {
			return &httpErr{http.StatusNotFound, "no such notification channel"}
		}
		n.Channels = append(n.Channels[:i:i], n.Channels[i+1:]...)
		return nil
	})
	if err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testInput is a channel to send a test to: a saved one by ID, or one that isn't
// saved yet, or a saved one with changes that aren't saved yet.
type testInput struct {
	ID      string        `json:"id"`
	Channel *channelInput `json:"channel"`
}

// testChannel sends one message to a channel, so the owner can see it works before
// relying on it. It says why when it doesn't.
func (a *API) testChannel(w http.ResponseWriter, r *http.Request) {
	var in testInput
	if !decode(w, r, &in) {
		return
	}
	if a.Notify == nil {
		writeErr(w, http.StatusConflict, "notifications are not available")
		return
	}
	var saved access.NotifyChannel
	if in.ID != "" {
		var ok bool
		if saved, ok = a.Pol.Settings().NotifyChannel(in.ID); !ok {
			writeErr(w, http.StatusNotFound, "no such notification channel")
			return
		}
	}
	ch := saved
	switch {
	case in.Channel != nil:
		ch = in.Channel.merge(saved)
	case in.ID == "":
		writeErr(w, http.StatusBadRequest, "say which channel to test")
		return
	}
	key := in.ID
	if key == "" {
		key = "draft"
		ch.ID = key
	}
	if err := ch.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if !a.allowTest(key) {
		writeErr(w, http.StatusTooManyRequests, "a test was just sent; give it a few seconds")
		return
	}
	if err := a.Notify.Test(r.Context(), ch); err != nil {
		a.Log.Info("notification test failed", "channel", ch.Name, "type", ch.Type, "err", err)
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("the test to %q didn't go through: %v", ch.Name, err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// allowTest lets a test through unless one to the same channel was sent a moment ago.
func (a *API) allowTest(key string) bool {
	a.testMu.Lock()
	defer a.testMu.Unlock()
	now := time.Now()
	if t, ok := a.tested[key]; ok && now.Sub(t) < testGap {
		return false
	}
	if a.tested == nil {
		a.tested = map[string]time.Time{}
	}
	a.tested[key] = now
	return true
}

// notifyEventsPatch switches kinds of event on or off; only those present change.
type notifyEventsPatch struct {
	Firmware *bool `json:"firmware"`
	Tuner    *bool `json:"tuner"`
	Viewer   *bool `json:"viewer"`
	Unknown  *bool `json:"unknown"`
	Watching *bool `json:"watching"`
}

func (p notifyEventsPatch) apply(n *access.Notifications) {
	set := func(dst **bool, v *bool) {
		if v != nil {
			b := *v
			*dst = &b
		}
	}
	set(&n.Events.Firmware, p.Firmware)
	set(&n.Events.Tuner, p.Tuner)
	set(&n.Events.Viewer, p.Viewer)
	set(&n.Events.Unknown, p.Unknown)
	set(&n.Events.Watching, p.Watching)
}
