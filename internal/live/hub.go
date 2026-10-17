// Package live tells interested parties (the web UI's event stream) that
// something changed, without them having to poll.
package live

import "sync"

// Hub fans a "something changed" signal out to subscribers. Signals coalesce:
// a subscriber that is slow sees one signal for any number of changes, and a
// slow subscriber never blocks the sender.
type Hub struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

func NewHub() *Hub { return &Hub{subs: map[chan struct{}]struct{}{}} }

// Subscribe returns a channel that receives a value after changes, and a func
// that ends the subscription.
func (h *Hub) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// Notify signals every subscriber. Safe to call on a nil Hub.
func (h *Hub) Notify() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- struct{}{}:
		default: // already has a pending signal
		}
	}
}

// Subscribers is how many listeners are attached (for tests and diagnostics).
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}
