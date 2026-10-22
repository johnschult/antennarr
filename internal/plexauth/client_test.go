package plexauth

import (
	"context"
	"fmt"
	"net/netip"
	"net/url"
	"reflect"
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

func TestResourcesListsTheServersAndWhereTheyAre(t *testing.T) {
	f := newFakePlexTV(t)
	f.setResources("tok-john",
		server("Home", "mid-home", true,
			conn("192.168.1.5", 32400, true, false),
			conn("203.0.113.50", 32400, false, false),
			conn("2001:db8::5", 32400, false, false),
			conn("::ffff:198.51.100.9", 32400, false, false),
		),
		server("Mom's", "mid-mom", false, conn("198.51.100.7", 32400, false, false)),
	)
	got, err := clientFor(f).Resources(context.Background(), "tok-john")
	if err != nil {
		t.Fatal(err)
	}
	want := []Server{
		{Name: "Home", MachineID: "mid-home", Owned: true, Connections: []Connection{
			{Addr: netip.MustParseAddr("192.168.1.5"), Port: 32400, Local: true},
			{Addr: netip.MustParseAddr("203.0.113.50"), Port: 32400},
			{Addr: netip.MustParseAddr("2001:db8::5"), Port: 32400},
			{Addr: netip.MustParseAddr("198.51.100.9"), Port: 32400}, // an IPv4 address in IPv6 clothes is the IPv4 address
		}},
		{Name: "Mom's", MachineID: "mid-mom", Connections: []Connection{{Addr: netip.MustParseAddr("198.51.100.7"), Port: 32400}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("servers = %+v\nwant     %+v", got, want)
	}
	if f.count("GET /api/v2/resources") != 1 {
		t.Error("expected one request")
	}
}

func TestResourcesDropsWhatIsNotAnAddressOfTheServer(t *testing.T) {
	f := newFakePlexTV(t)
	f.setResources("tok",
		server("Home", "mid-home", true,
			conn("203.0.113.50", 32400, false, true),           // a relay is Plex's address, not the server's
			conn("abc123.plex.direct", 32400, false, false),    // a host name is not an address to allow
			conn("203.0.113.51", 0, false, false),              // no port
			conn("203.0.113.52", 70000, false, false),          // not a port
			conn("not an address at all", 32400, false, false), // junk
			conn("203.0.113.53", 32400, false, false),          // the one that is
		),
		server("Relay only", "mid-relay", true, conn("203.0.113.60", 32400, false, true)), // nothing left, so not listed
	)
	got, err := clientFor(f).Resources(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Connections) != 1 || got[0].Connections[0].Addr.String() != "203.0.113.53" {
		t.Errorf("servers = %+v", got)
	}
}

func TestResourcesIgnoresWhatIsNotAServer(t *testing.T) {
	f := newFakePlexTV(t)
	player := func(provides string) map[string]any {
		return map[string]any{"name": "Living room", "product": "Plex for Android", "clientIdentifier": "mid-" + provides, "provides": provides,
			"connections": []map[string]any{conn("192.168.1.9", 32400, true, false)}}
	}
	f.setResources("tok",
		player("client,player,pubsub-player"),
		player("player"),
		player("servers"), // not the word "server"
		player(""),
		server("Home", "mid-home", true, conn("203.0.113.50", 32400, false, false)),
		map[string]any{"name": "No identifier", "provides": "server", "connections": []map[string]any{conn("203.0.113.70", 32400, false, false)}},
		map[string]any{"name": "Both", "provides": "server, player", "clientIdentifier": "mid-both", "connections": []map[string]any{conn("203.0.113.71", 32400, false, false)}},
	)
	got, err := clientFor(f).Resources(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range got {
		ids = append(ids, s.MachineID)
	}
	if !reflect.DeepEqual(ids, []string{"mid-home", "mid-both"}) {
		t.Errorf("servers = %v, want only the ones that provide a server", ids)
	}
}

func TestResourcesNamesAreCleanedAndCapped(t *testing.T) {
	f := newFakePlexTV(t)
	var many []map[string]any
	for i := 0; i < 60; i++ {
		many = append(many, server(fmt.Sprintf("s%d", i), fmt.Sprintf("mid%d", i), true, conn("203.0.113.1", 32400, false, false)))
	}
	f.setResources("big", many...)
	var conns []map[string]any
	for i := 0; i < 30; i++ {
		conns = append(conns, conn(fmt.Sprintf("203.0.113.%d", i+1), 32400, false, false))
	}
	f.setResources("wide", server("  Basement \x1b[31m\n", "mid-wide", true, conns...), server("", "mid-unnamed", true, conn("203.0.113.99", 32400, false, false)))

	got, err := clientFor(f).Resources(context.Background(), "big")
	if err != nil || len(got) != maxServers {
		t.Errorf("60 servers: %d kept (%v), want %d", len(got), err, maxServers)
	}
	got, err = clientFor(f).Resources(context.Background(), "wide")
	if err != nil || len(got) != 2 {
		t.Fatalf("wide: %+v %v", got, err)
	}
	if got[0].Name != "Basement [31m" || len(got[0].Connections) != maxConnections {
		t.Errorf("name %q with %d connections, want it cleaned and %d connections", got[0].Name, len(got[0].Connections), maxConnections)
	}
	if got[1].Name != "Plex Media Server" {
		t.Errorf("a server with no name is called %q", got[1].Name)
	}
}

func TestResourcesReadsNoMoreThanItShould(t *testing.T) {
	f := newFakePlexTV(t)
	f.setResources("tok", server("Home", "mid-home", true, conn("203.0.113.50", 32400, false, false)))
	f.setPad(true)
	if got, err := clientFor(f).Resources(context.Background(), "tok"); err == nil {
		t.Errorf("an answer far over the limit was accepted: %+v", got)
	}
}

func TestResourcesSendsTheTokenOnlyAsAHeaderAndPassesNoSecretOn(t *testing.T) {
	f := newFakePlexTV(t)
	f.setResources("tok-very-secret", server("Home", "mid-home", true, conn("203.0.113.50", 32400, false, false)))
	got, err := clientFor(f).Resources(context.Background(), "tok-very-secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range f.rawQueries() {
		if strings.Contains(q, "tok-very-secret") || strings.Contains(strings.ToLower(q), "token") {
			t.Errorf("the token is in a URL: %q", q)
		}
	}
	// plex.tv sends each server's own access token and its public address too. None of it is kept.
	if s := fmt.Sprintf("%+v", got); strings.Contains(s, "server-access-token") || strings.Contains(s, "tok-very-secret") {
		t.Errorf("a token came back in the result: %s", s)
	}
}

func TestResourcesFailures(t *testing.T) {
	f := newFakePlexTV(t)
	c := clientFor(f)
	if _, err := c.Resources(context.Background(), "unknown-token"); err != ErrBadToken {
		t.Errorf("unknown token: %v", err)
	}
	f.setResources("tok")
	if got, err := c.Resources(context.Background(), "tok"); err != nil || len(got) != 0 {
		t.Errorf("an account with no servers: %+v %v", got, err)
	}
	f.setFail(true)
	if _, err := c.Resources(context.Background(), "tok"); err == nil || err == ErrBadToken || !strings.Contains(err.Error(), "500") {
		t.Errorf("server error: %v", err)
	}
	f.setFail(false)
	url := f.URL
	f.Close()
	if _, err := (&Client{Base: url, ClientID: "yagi-testclient"}).Resources(context.Background(), "tok"); err == nil || !strings.Contains(err.Error(), "could not reach plex.tv") {
		t.Errorf("unreachable: %v", err)
	}
}
