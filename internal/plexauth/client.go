// Package plexauth lets the admin pages be opened by signing in with a Plex
// account. The server does every call to plex.tv: the browser only ever sees
// a link to Plex's own sign-in page and, afterwards, a session cookie. The
// Plex token that comes back is used once, to learn who signed in, and thrown
// away.
package plexauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBase     = "https://plex.tv"
	DefaultAuthBase = "https://app.plex.tv/auth"
	Product         = "Yagi"
)

var (
	// ErrExpired means the PIN is no longer valid: it ran out, or plex.tv forgot it.
	ErrExpired = errors.New("the Plex sign-in expired")
	// ErrBadToken means plex.tv refused the token it had just issued.
	ErrBadToken = errors.New("plex.tv did not accept the sign-in")
)

// PIN is a sign-in in progress, as plex.tv describes it.
type PIN struct {
	ID        int64
	Code      string
	ExpiresAt time.Time
}

// User is who signed in. Only what's needed is kept: no email, no token.
type User struct {
	ID   int64
	Name string
}

// Client talks to plex.tv.
type Client struct {
	Base     string // default DefaultBase
	AuthBase string // default DefaultAuthBase
	// ClientID identifies this Yagi to plex.tv and ties a PIN to it. It
	// is not a secret that protects anything (the sign-in link has to carry it
	// to plex.tv's own page), but there is no reason to show it anywhere else.
	ClientID string
	HTTP     *http.Client
}

func (c *Client) base() string {
	if c.Base != "" {
		return strings.TrimRight(c.Base, "/")
	}
	return DefaultBase
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

const maxResponse = 1 << 20

func (c *Client) do(ctx context.Context, method, path string, q url.Values, token string, out any) (int, error) {
	u := c.base() + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Client-Identifier", c.ClientID)
	req.Header.Set("X-Plex-Product", Product)
	req.Header.Set("X-Plex-Device-Name", Product)
	if token != "" {
		req.Header.Set("X-Plex-Token", token)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return 0, fmt.Errorf("could not reach plex.tv: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		if out != nil {
			if err := json.Unmarshal(body, out); err != nil {
				return resp.StatusCode, fmt.Errorf("plex.tv sent something unexpected: %w", err)
			}
		}
	}
	return resp.StatusCode, nil
}

type pinDoc struct {
	ID        int64   `json:"id"`
	Code      string  `json:"code"`
	AuthToken *string `json:"authToken"`
	ExpiresAt string  `json:"expiresAt"`
}

func (d pinDoc) expires() time.Time {
	t, _ := time.Parse(time.RFC3339, d.ExpiresAt)
	return t
}

// NewPIN starts a sign-in. "strong" asks for the long code that goes in the
// link, rather than the 4 characters a person would type at plex.tv/link.
func (c *Client) NewPIN(ctx context.Context) (PIN, error) {
	var d pinDoc
	code, err := c.do(ctx, http.MethodPost, "/api/v2/pins", url.Values{"strong": {"true"}}, "", &d)
	if err != nil {
		return PIN{}, err
	}
	if (code != http.StatusOK && code != http.StatusCreated) || d.ID == 0 || d.Code == "" {
		return PIN{}, fmt.Errorf("plex.tv would not start a sign-in (status %d)", code)
	}
	return PIN{ID: d.ID, Code: d.Code, ExpiresAt: d.expires()}, nil
}

// CheckPIN asks whether the person has signed in yet. The token is empty while
// they haven't.
func (c *Client) CheckPIN(ctx context.Context, id int64, code string) (token string, err error) {
	var d pinDoc
	status, err := c.do(ctx, http.MethodGet, "/api/v2/pins/"+strconv.FormatInt(id, 10), url.Values{"code": {code}}, "", &d)
	switch {
	case err != nil:
		return "", err
	case status == http.StatusNotFound || status == http.StatusGone:
		return "", ErrExpired
	case status != http.StatusOK:
		return "", fmt.Errorf("plex.tv answered %d when checking the sign-in", status)
	}
	if d.AuthToken == nil {
		return "", nil
	}
	return *d.AuthToken, nil
}

// User learns who a token belongs to.
func (c *Client) User(ctx context.Context, token string) (User, error) {
	var d struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Title    string `json:"title"`
	}
	status, err := c.do(ctx, http.MethodGet, "/api/v2/user", nil, token, &d)
	switch {
	case err != nil:
		return User{}, err
	case status == http.StatusUnauthorized:
		return User{}, ErrBadToken
	case status != http.StatusOK || d.ID <= 0:
		return User{}, fmt.Errorf("plex.tv answered %d when asked who signed in", status)
	}
	name := d.Username
	if name == "" {
		name = d.Title
	}
	if name == "" {
		name = "Plex user"
	}
	return User{ID: d.ID, Name: cleanName(name)}, nil
}

// Server is a Plex Media Server the signed-in account owns or has been shared. Only what is needed to allow its
// addresses is kept: not the server's own access token, which plex.tv also sends, and not its public address.
type Server struct {
	// Name is what the server is called, cleaned like a user name.
	Name string
	// MachineID is the server's own identifier, which does not change when its address does.
	MachineID string
	// Owned is whether the account owns it, rather than being shared on it.
	Owned bool
	// Connections are the addresses it can be reached at, without the relayed ones: a relay is Plex's address, not
	// the server's, so allowing it would allow everyone on that relay.
	Connections []Connection
}

// Connection is one address a server answers on.
type Connection struct {
	// Addr is the IP address. A connection that is a host name instead is left out, because the allowlist is of
	// addresses.
	Addr netip.Addr
	Port int
	// Local is whether plex.tv says it is a private-network address, as against one reached over the internet.
	Local bool
}

// The most of each that is kept, so a strange answer can't make an unbounded list.
const (
	maxServers     = 50
	maxConnections = 20
)

// Resources lists the servers a token's account owns or is shared. The token is sent as a header and nowhere else,
// and not kept: the caller has it only as long as it takes to ask.
func (c *Client) Resources(ctx context.Context, token string) ([]Server, error) {
	var docs []struct {
		Name             string `json:"name"`
		ClientIdentifier string `json:"clientIdentifier"`
		Provides         string `json:"provides"`
		Owned            bool   `json:"owned"`
		Connections      []struct {
			Address string `json:"address"`
			Port    int    `json:"port"`
			Local   bool   `json:"local"`
			Relay   bool   `json:"relay"`
		} `json:"connections"`
	}
	status, err := c.do(ctx, http.MethodGet, "/api/v2/resources", url.Values{"includeHttps": {"1"}, "includeRelay": {"0"}}, token, &docs)
	switch {
	case err != nil:
		return nil, err
	case status == http.StatusUnauthorized:
		return nil, ErrBadToken
	case status != http.StatusOK:
		return nil, fmt.Errorf("plex.tv answered %d when asked for the account's servers", status)
	}
	var out []Server
	for _, d := range docs {
		if !provides(d.Provides, "server") || d.ClientIdentifier == "" {
			continue // a player or a phone, not a server
		}
		srv := Server{Name: cleanName(d.Name), MachineID: cleanName(d.ClientIdentifier), Owned: d.Owned}
		if srv.Name == "" {
			srv.Name = "Plex Media Server"
		}
		for _, cn := range d.Connections {
			addr, err := netip.ParseAddr(strings.TrimSpace(cn.Address))
			if cn.Relay || err != nil || cn.Port < 1 || cn.Port > 65535 {
				continue
			}
			if len(srv.Connections) < maxConnections {
				srv.Connections = append(srv.Connections, Connection{Addr: addr.Unmap(), Port: cn.Port, Local: cn.Local})
			}
		}
		if len(srv.Connections) == 0 {
			continue
		}
		out = append(out, srv)
		if len(out) == maxServers {
			break
		}
	}
	return out, nil
}

// provides is whether a resource's comma-separated "provides" list has the word in it.
func provides(list, word string) bool {
	for _, p := range strings.Split(list, ",") {
		if strings.TrimSpace(p) == word {
			return true
		}
	}
	return false
}

// cleanName keeps a name safe to store and show: no control characters, not
// too long.
func cleanName(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if r >= ' ' && r != 0x7f {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if r := []rune(out); len(r) > 60 {
		out = string(r[:60])
	}
	return out
}

// AuthURL is where to send the person to sign in: Plex's own page, which
// brings them back nowhere; Yagi finds out by asking plex.tv about the PIN.
func (c *Client) AuthURL(p PIN) string {
	base := c.AuthBase
	if base == "" {
		base = DefaultAuthBase
	}
	q := url.Values{}
	q.Set("clientID", c.ClientID)
	q.Set("code", p.Code)
	q.Set("context[device][product]", Product)
	return base + "#?" + q.Encode()
}
