package plexauth

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"
)

func clientFor(f *fakePlexTV) *Client { return &Client{Base: f.URL, ClientID: "yagi-testclient"} }

func TestNewPINAsksForAStrongPINAndIdentifiesItself(t *testing.T) {
	f := newFakePlexTV(t)
	pin, err := clientFor(f).NewPIN(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pin.ID != 1001 || !strings.HasPrefix(pin.Code, "code1001-") || pin.ExpiresAt.Before(time.Now().Add(25*time.Minute)) {
		t.Errorf("pin = %+v", pin)
	}
	if f.count("POST /api/v2/pins") != 1 {
		t.Error("expected one request")
	}
	if ids := f.ids(); len(ids) != 1 || !ids["yagi-testclient"] {
		t.Errorf("client identifiers seen by plex.tv: %v", ids)
	}
}

func TestNewPINFailures(t *testing.T) {
	f := newFakePlexTV(t)
	f.setFail(true)
	if _, err := clientFor(f).NewPIN(context.Background()); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("server error: %v", err)
	}

	url := f.URL
	f.Close()
	if _, err := (&Client{Base: url, ClientID: "yagi-testclient"}).NewPIN(context.Background()); err == nil || !strings.Contains(err.Error(), "could not reach plex.tv") {
		t.Errorf("unreachable: %v", err)
	}
}

func TestCheckPIN(t *testing.T) {
	f := newFakePlexTV(t)
	c := clientFor(f)
	pin, _ := c.NewPIN(context.Background())

	if tok, err := c.CheckPIN(context.Background(), pin.ID, pin.Code); err != nil || tok != "" {
		t.Errorf("before anyone signs in: %q %v", tok, err)
	}
	f.signIn(pin.ID, "tok-abc", fakeUser{ID: 7, Username: "john"})
	if tok, err := c.CheckPIN(context.Background(), pin.ID, pin.Code); err != nil || tok != "tok-abc" {
		t.Errorf("after sign-in: %q %v", tok, err)
	}
	f.expire(pin.ID)
	if _, err := c.CheckPIN(context.Background(), pin.ID, pin.Code); err != ErrExpired {
		t.Errorf("expired: %v", err)
	}
	if _, err := c.CheckPIN(context.Background(), 424242, "x"); err != ErrExpired {
		t.Errorf("unknown: %v", err)
	}
	f.setFail(true)
	if _, err := c.CheckPIN(context.Background(), pin.ID, pin.Code); err == nil || err == ErrExpired {
		t.Errorf("a server error is not 'expired': %v", err)
	}
}

func TestUser(t *testing.T) {
	f := newFakePlexTV(t)
	c := clientFor(f)
	for token, tc := range map[string]struct {
		in   fakeUser
		want string
	}{
		"t1": {fakeUser{ID: 1, Username: "john", Title: "John S"}, "john"},
		"t2": {fakeUser{ID: 2, Title: "Mom & Dad"}, "Mom & Dad"}, // no username: the title
		"t3": {fakeUser{ID: 3}, "Plex user"},
		"t4": {fakeUser{ID: 4, Username: "  spaced  \x1b[31m\n"}, "spaced  [31m"},
		"t5": {fakeUser{ID: 5, Username: strings.Repeat("é", 80)}, strings.Repeat("é", 60)},
	} {
		f.addUser(token, tc.in)
		u, err := c.User(context.Background(), token)
		if err != nil || u.ID != tc.in.ID || u.Name != tc.want {
			t.Errorf("%s: %+v %v, want name %q", token, u, err, tc.want)
		}
	}
}

func TestUserFailures(t *testing.T) {
	f := newFakePlexTV(t)
	c := clientFor(f)
	if _, err := c.User(context.Background(), "unknown-token"); err != ErrBadToken {
		t.Errorf("unknown token: %v", err)
	}
	f.addUser("zero", fakeUser{ID: 0, Username: "x"})
	if _, err := c.User(context.Background(), "zero"); err == nil {
		t.Error("an account with no ID was accepted")
	}
	f.setFail(true)
	if _, err := c.User(context.Background(), "unknown-token"); err == nil || err == ErrBadToken {
		t.Errorf("server error: %v", err)
	}
}

func TestAuthURL(t *testing.T) {
	c := &Client{ClientID: "yagi-abc123"}
	link := c.AuthURL(PIN{ID: 5, Code: "the-code"})
	if !strings.HasPrefix(link, "https://app.plex.tv/auth#?") {
		t.Fatalf("link = %s", link)
	}
	q, err := url.ParseQuery(strings.TrimPrefix(link, "https://app.plex.tv/auth#?"))
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("clientID") != "yagi-abc123" || q.Get("code") != "the-code" || q.Get("context[device][product]") != "Yagi" {
		t.Errorf("query = %v", q)
	}
	if strings.Contains(link, "[") {
		t.Errorf("brackets must be encoded in the link: %s", link)
	}
	if got := (&Client{ClientID: "x", AuthBase: "http://localhost:9/auth"}).AuthURL(PIN{Code: "c"}); !strings.HasPrefix(got, "http://localhost:9/auth#?") {
		t.Errorf("custom base: %s", got)
	}
}

func TestClientDefaults(t *testing.T) {
	c := &Client{}
	if c.base() != DefaultBase || c.httpClient().Timeout == 0 {
		t.Error("defaults")
	}
	if (&Client{Base: "http://x/"}).base() != "http://x" {
		t.Error("trailing slash")
	}
}
