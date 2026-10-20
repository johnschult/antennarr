package access

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// IdentityPrefix starts an address entry that names a Tailscale identity instead of an IP address:
// "tailscale:alice@example.com" is anyone signed in to the tailnet as that login, and
// "tailscale:laptop.tail1234.ts.net" is that one device. It is matched only for connections that came in over
// Yagi's own Tailscale node, which says who is on the other end.
const IdentityPrefix = "tailscale:"

// Identity is who the Tailscale node says is on the other end of a connection.
type Identity struct {
	Login string // the tailnet login, such as alice@example.com; empty for a tagged device
	Node  string // the device's full tailnet name, such as laptop.tail1234.ts.net
}

type identityKey struct{}

// WithIdentity attaches who a request came from. Only the server's tailnet listeners do this, from what the
// node reports; nothing a client sends can set it.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	id.Login, id.Node = strings.ToLower(id.Login), strings.ToLower(strings.TrimSuffix(id.Node, "."))
	return context.WithValue(ctx, identityKey{}, id)
}

// IdentityOf is who a request came from over the tailnet, if the node said.
func IdentityOf(r *http.Request) (Identity, bool) {
	id, ok := r.Context().Value(identityKey{}).(Identity)
	return id, ok
}

// matches reports whether the identity is the one an entry (without its prefix, lower-cased) names.
func (id Identity) matches(entry string) bool {
	if strings.Contains(entry, "@") {
		return id.Login != "" && id.Login == entry
	}
	return id.Node != "" && id.Node == entry
}

// parseIdentity reads an address entry that names a Tailscale identity. ok is false when the entry is not one.
func parseIdentity(s string) (entry string, ok bool, err error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(strings.ToLower(s), IdentityPrefix) {
		return "", false, nil
	}
	// A device is reported without the root dot, so an entry written with one means the same.
	v := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s[len(IdentityPrefix):])), ".")
	switch {
	case v == "":
		return "", true, fmt.Errorf("%q: say who, such as tailscale:alice@example.com", s)
	case strings.ContainsAny(v, " \t\n/,"):
		return "", true, fmt.Errorf("%q: not a Tailscale login or device name", s)
	case strings.Contains(v, "@"):
		if at := strings.Index(v, "@"); at == 0 || at == len(v)-1 || strings.Count(v, "@") != 1 {
			return "", true, fmt.Errorf("%q: not a Tailscale login (expected name@example.com)", s)
		}
	case !strings.Contains(v, ".") || len(v) > 253:
		return "", true, fmt.Errorf("%q: a device is named by its full tailnet name, such as laptop.tail1234.ts.net", s)
	}
	return v, true, nil
}
