//go:build tsnet

package tailnet

import "testing"

func TestBuiltIsTrueWithTheTag(t *testing.T) {
	if !Built() {
		t.Error("the Tailscale build says it hasn't got the Tailscale node")
	}
}
