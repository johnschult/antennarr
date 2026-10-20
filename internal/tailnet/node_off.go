//go:build !tsnet

package tailnet

// NewNode is nil in a build without the Tailscale node: the page says so, and the setting does nothing.
var NewNode Starter
