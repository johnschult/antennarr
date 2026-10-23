//go:build !tsnet

package tailnet

import "testing"

func TestBuiltIsFalseWithoutTheTag(t *testing.T) {
	if Built() {
		t.Error("the plain build says it has the Tailscale node")
	}
}
