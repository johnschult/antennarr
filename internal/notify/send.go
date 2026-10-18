package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/johnschult/antennarr/internal/access"
)

const (
	// DefaultPushoverURL is where Pushover takes messages.
	DefaultPushoverURL = "https://api.pushover.net/1/messages.json"
	userAgent          = "Antennarr (notification)"
	sendTimeout        = 10 * time.Second
	maxReply           = 200 // how much of a reply to quote in an error
)

// NewClient is the HTTP client notifications go out through. It never follows a
// redirect: a notification goes where it was set up to go and nowhere else.
func NewClient() *http.Client {
	return &http.Client{
		Timeout:       sendTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// send delivers one event to one channel.
func (n *Notifier) send(ctx context.Context, ch access.NotifyChannel, ev Event) error {
	switch ch.Type {
	case access.ChannelNtfy:
		body, _ := json.Marshal(map[string]any{
			"topic":    ch.Topic,
			"title":    ev.Title,
			"message":  ev.Message,
			"tags":     []string{tag(ev.Kind)},
			"priority": priority(ev.Kind),
		})
		return n.post(ctx, ch.URL, "application/json", body, ch.Token)
	case access.ChannelPushover:
		form := url.Values{"token": {ch.Token}, "user": {ch.User}, "title": {ev.Title}, "message": {ev.Message}}
		return n.post(ctx, n.pushoverURL, "application/x-www-form-urlencoded", []byte(form.Encode()), "")
	case access.ChannelWebhook:
		body, _ := json.Marshal(map[string]string{
			"event":   ev.Kind,
			"title":   ev.Title,
			"message": ev.Message,
			"device":  ev.Device,
			"time":    ev.Time.UTC().Format(time.RFC3339),
		})
		return n.post(ctx, ch.URL, "application/json", body, ch.Token)
	}
	return fmt.Errorf("unknown channel type %q", ch.Type)
}

func tag(kind string) string {
	switch kind {
	case access.NotifyFirmware:
		return "package"
	case access.NotifyTuner:
		return "tv"
	case access.NotifyUnknown:
		return "no_entry"
	case testKind:
		return "white_check_mark"
	}
	return "warning"
}

// priority is ntfy's 1 to 5. A tuner going offline matters more than a firmware release.
func priority(kind string) int {
	if kind == access.NotifyTuner {
		return 4
	}
	return 3
}

// post sends a body and says what went wrong, if anything. The errors name the
// host but never the address in full, nor anything in it: webhook addresses often
// contain a secret, and Go's own errors quote the whole URL.
func (n *Notifier) post(ctx context.Context, target, contentType string, body []byte, bearer string) error {
	u, err := url.Parse(target)
	if err != nil || u.Hostname() == "" {
		return errors.New("the address is not valid")
	}
	host := u.Hostname()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return errors.New("the address is not valid")
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", userAgent)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't reach %s: %s", host, reason(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return nil
	}
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	msg := fmt.Sprintf("%s answered %s", host, resp.Status)
	if s := tidy(string(snippet)); s != "" {
		msg += ": " + s
	}
	return errors.New(msg)
}

// reason is why a request failed, without the URL that Go's errors repeat.
func reason(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "it took too long to answer"
	case errors.Is(err, context.Canceled):
		return "the request was cancelled"
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	if ue != nil && ue.Timeout() {
		return "it took too long to answer"
	}
	return err.Error()
}

// tidy makes a reply fit on one line.
func tidy(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxReply {
		s = s[:maxReply] + "…"
	}
	return s
}
