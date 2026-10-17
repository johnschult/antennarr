package live

import (
	"sync"
	"testing"
	"time"
)

func received(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	case <-time.After(50 * time.Millisecond):
		return false
	}
}

func TestNotifyReachesEverySubscriber(t *testing.T) {
	h := NewHub()
	a, cancelA := h.Subscribe()
	b, cancelB := h.Subscribe()
	defer cancelA()
	defer cancelB()

	h.Notify()
	if !received(a) || !received(b) {
		t.Error("a subscriber missed the signal")
	}
}

func TestSignalsCoalesce(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe()
	defer cancel()

	for i := 0; i < 100; i++ {
		h.Notify() // nobody is reading: must not block
	}
	if !received(ch) {
		t.Fatal("no signal")
	}
	if received(ch) {
		t.Error("100 changes produced more than one pending signal")
	}
}

func TestCancelStopsDelivery(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe()
	if h.Subscribers() != 1 {
		t.Fatalf("subscribers = %d", h.Subscribers())
	}
	cancel()
	cancel() // idempotent
	if h.Subscribers() != 0 {
		t.Errorf("subscribers = %d after cancel", h.Subscribers())
	}
	h.Notify()
	if received(ch) {
		t.Error("signal delivered after cancel")
	}
}

func TestNilHubIsSafe(t *testing.T) {
	var h *Hub
	h.Notify() // must not panic
}

func TestConcurrentUse(t *testing.T) {
	h := NewHub()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, cancel := h.Subscribe()
			h.Notify()
			cancel()
		}()
		go func() { defer wg.Done(); h.Notify() }()
	}
	wg.Wait()
	if h.Subscribers() != 0 {
		t.Errorf("leaked %d subscribers", h.Subscribers())
	}
}
