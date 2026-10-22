package access

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// ViewerPlex says where an enrolled viewer's addresses came from: the Plex account that signed in on an invite
// page, and the server of theirs they picked. A viewer added by hand has none.
type ViewerPlex struct {
	AccountID   int64  `json:"accountId"`
	AccountName string `json:"accountName"`
	ServerID    string `json:"serverId"`
	ServerName  string `json:"serverName"`
}

// Invite is a link the owner sent to a person, so they can add themselves as a viewer by signing in with Plex. The
// link's token is never stored, only a hash of it, so the settings file can't be used to enroll anybody.
type Invite struct {
	// ID names the invite on the admin page and in the API. It is not a secret and not part of the link.
	ID string `json:"id"`
	// TokenHash is the hex SHA-256 of the token in the link.
	TokenHash string `json:"tokenHash"`
	// Name is what the viewer will be called.
	Name string `json:"name"`
	// Limit and Limits are what the viewer gets, as for any viewer.
	Limit  int            `json:"limit"`
	Limits map[string]int `json:"limits,omitempty"`
	// Created is when the owner made it.
	Created time.Time `json:"created"`
	// Expires is when an invite that hasn't been used stops working. Once it is used it doesn't expire: the person
	// opens the same link again to bring their addresses up to date, until the owner revokes it.
	Expires time.Time `json:"expires"`
	// ViewerName is the viewer made from the invite, once someone has used it.
	ViewerName string `json:"viewerName,omitempty"`
}

const (
	// MaxInvites is the most invites kept at once, used or not.
	MaxInvites = 50
	// InviteLife is how long an invite that nobody has used is good for.
	InviteLife = 7 * 24 * time.Hour
)

var (
	inviteIDRe   = regexp.MustCompile(`^[a-f0-9]{12}$`)
	tokenHashRe  = regexp.MustCompile(`^[a-f0-9]{64}$`)
	plexServerRe = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
)

// NewInvite makes an invite and the token for its link. The token is returned once, to give to the person; only its
// hash is kept in the invite.
func NewInvite(name string, limit int, limits map[string]int, now time.Time) (token string, inv Invite) {
	id := make([]byte, 6)
	tok := make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		panic("access: no randomness: " + err.Error()) // nothing safe to do without it
	}
	if _, err := rand.Read(tok); err != nil {
		panic("access: no randomness: " + err.Error())
	}
	token = base64.RawURLEncoding.EncodeToString(tok)
	if len(limits) == 0 {
		limits = nil
	}
	return token, Invite{
		ID: hex.EncodeToString(id), TokenHash: HashInviteToken(token), Name: strings.TrimSpace(name),
		Limit: limit, Limits: limits, Created: now.UTC(), Expires: now.Add(InviteLife).UTC(),
	}
}

// HashInviteToken is what is kept of a token.
func HashInviteToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Used is whether anyone has enrolled with the invite.
func (i Invite) Used() bool { return i.ViewerName != "" }

// Live is whether the invite works at this moment: a used one until it is revoked, an unused one until it expires.
func (i Invite) Live(now time.Time) bool { return i.Used() || now.Before(i.Expires) }

// InviteForToken finds the invite a link's token belongs to. It looks at every invite and compares in constant
// time, so how long it takes says nothing about which one was close.
func (s Settings) InviteForToken(token string) (Invite, bool) {
	want := []byte(HashInviteToken(token))
	var found Invite
	ok := 0
	for _, inv := range s.Invites {
		if subtle.ConstantTimeCompare(want, []byte(inv.TokenHash)) == 1 {
			found, ok = inv, 1
		}
	}
	return found, ok == 1
}

// InviteByID finds an invite by its ID.
func (s Settings) InviteByID(id string) (Invite, bool) {
	for _, inv := range s.Invites {
		if inv.ID == id {
			return inv, true
		}
	}
	return Invite{}, false
}

// EnrollmentEnabled is whether people may enroll themselves through an invite link. It needs Sign in with Plex to be
// on as well, because it is the same switch for whether Yagi may talk to plex.tv at all.
func (s Settings) EnrollmentEnabled() bool {
	return s.PlexEnabled() && s.Plex.Enrollment
}

func cloneInvites(in []Invite) []Invite {
	if in == nil {
		return nil
	}
	out := make([]Invite, len(in))
	for i, inv := range in {
		out[i] = inv
		if inv.Limits != nil {
			out[i].Limits = make(map[string]int, len(inv.Limits))
			for k, n := range inv.Limits {
				out[i].Limits[k] = n
			}
		}
	}
	return out
}

func (p *ViewerPlex) validate(viewer string) error {
	if p == nil {
		return nil
	}
	if p.AccountID <= 0 {
		return fmt.Errorf("viewer %q: the Plex account has no ID", viewer)
	}
	if len(p.AccountName) > maxLabelLen || strings.ContainsFunc(p.AccountName, unicode.IsControl) {
		return fmt.Errorf("viewer %q: bad Plex account name", viewer)
	}
	if !plexServerRe.MatchString(p.ServerID) {
		return fmt.Errorf("viewer %q: bad Plex server ID", viewer)
	}
	if len(p.ServerName) > maxLabelLen || strings.ContainsFunc(p.ServerName, unicode.IsControl) {
		return fmt.Errorf("viewer %q: bad Plex server name", viewer)
	}
	return nil
}

func validateInvites(invites []Invite) error {
	if len(invites) > MaxInvites {
		return fmt.Errorf("invites: at most %d", MaxInvites)
	}
	ids, hashes, names := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, inv := range invites {
		if !inviteIDRe.MatchString(inv.ID) {
			return errors.New("invites: an invite has no valid ID")
		}
		if ids[inv.ID] {
			return fmt.Errorf("invites: ID %s is used twice", inv.ID)
		}
		ids[inv.ID] = true
		if !tokenHashRe.MatchString(inv.TokenHash) {
			return fmt.Errorf("invites: invite %s has no valid token hash", inv.ID)
		}
		if hashes[inv.TokenHash] {
			return fmt.Errorf("invites: invite %s has the same token as another", inv.ID)
		}
		hashes[inv.TokenHash] = true
		name := strings.TrimSpace(inv.Name)
		if name == "" {
			return fmt.Errorf("invites: invite %s needs a name", inv.ID)
		}
		if err := ValidateLabel(name); err != nil {
			return fmt.Errorf("invites: invite %s name: %w", inv.ID, err)
		}
		if names[strings.ToLower(name)] {
			return fmt.Errorf("invites: there are two invites for %q", name)
		}
		names[strings.ToLower(name)] = true
		if inv.Limit < 0 {
			return fmt.Errorf("invites: invite %s: limit cannot be negative", inv.ID)
		}
		for dev, n := range inv.Limits {
			if n < 0 {
				return fmt.Errorf("invites: invite %s: limit for %q cannot be negative", inv.ID, dev)
			}
		}
		if inv.ViewerName == "" && inv.Expires.IsZero() {
			return fmt.Errorf("invites: invite %s has no expiry", inv.ID)
		}
	}
	return nil
}

// PublicAddr is whether ip is an address on the public internet: one that, unlike 192.168.1.5, belongs to one
// place. Only those are worth allowing for a person's server.
func PublicAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsValid() && ip.IsGlobalUnicast() && !privateAddr(ip)
}

// ViewerHolding is the viewer, other than except, whose addresses already cover ip: an address on two viewers would
// be one of them using the other's limit. A viewer that is a Tailscale login or device has no addresses to cover.
func (s Settings) ViewerHolding(ip netip.Addr, except string) (string, bool) {
	c, err := s.parse()
	if err != nil {
		return "", false
	}
	ip = ip.Unmap()
	for _, v := range c.viewers {
		if strings.EqualFold(v.Name, except) {
			continue
		}
		for _, p := range v.prefixes {
			if p.Contains(ip) {
				return v.Name, true
			}
		}
	}
	return "", false
}
