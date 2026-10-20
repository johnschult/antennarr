//go:build tsnet

package tailnet

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"tailscale.com/tsnet"
)

func init() { NewNode = newTSNetNode }

// tsnetNode is a Tailscale node that lives inside this program, with its own identity (in Config.Dir) and no
// effect on the machine's network or on any Tailscale the machine already runs.
type tsnetNode struct {
	srv    *tsnet.Server
	cancel context.CancelFunc
	done   chan struct{}

	mu     sync.Mutex
	status NodeStatus
}

func newTSNetNode(c Config) Node {
	log := c.Log
	if log == nil {
		log = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	n := &tsnetNode{cancel: cancel, done: make(chan struct{}), status: NodeStatus{State: Starting}}
	n.srv = &tsnet.Server{
		Dir:      c.Dir,
		Hostname: c.Hostname,
		AuthKey:  c.AuthKey,
		// The library's own chatter is for debugging; what a person needs (such as the sign-in link) is read
		// from the node's status below.
		Logf:     func(string, ...any) {},
		UserLogf: func(format string, args ...any) { log.Debug("tailscale: " + strings.TrimSpace(format)) },
	}
	go n.run(ctx, log)
	return n
}

func (n *tsnetNode) set(s NodeStatus) {
	n.mu.Lock()
	n.status = s
	n.mu.Unlock()
}

func (n *tsnetNode) run(ctx context.Context, log *slog.Logger) {
	defer close(n.done)
	if err := n.srv.Start(); err != nil {
		log.Warn("tailscale could not start", "err", err)
		n.set(NodeStatus{State: Failed, Error: err.Error()})
		return
	}
	lc, err := n.srv.LocalClient()
	if err != nil {
		log.Warn("tailscale could not start", "err", err)
		n.set(NodeStatus{State: Failed, Error: err.Error()})
		return
	}
	last := ""
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		st, err := lc.Status(ctx)
		if err == nil {
			ns := NodeStatus{State: Starting}
			switch st.BackendState {
			case "NeedsLogin":
				ns.State, ns.LoginURL = NeedsLogin, st.AuthURL
			case "Running":
				ns.State = Running
				if st.Self != nil {
					ns.DNSName = strings.TrimSuffix(st.Self.DNSName, ".")
				}
				for _, ip := range st.TailscaleIPs {
					ns.Addresses = append(ns.Addresses, ip.String())
				}
			}
			n.set(ns)
			if key := string(ns.State) + ns.LoginURL; key != last {
				last = key
				log.Info("tailscale", "state", ns.State, "name", ns.DNSName)
				if ns.State == NeedsLogin && ns.LoginURL != "" {
					log.Info("tailscale needs signing in: open the link on the Settings page, or set TS_AUTHKEY", "url", ns.LoginURL)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (n *tsnetNode) Status() NodeStatus {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.status
}

func (n *tsnetNode) Close() error {
	n.cancel()
	<-n.done
	return n.srv.Close()
}
