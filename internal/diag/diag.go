// Package diag is diagnostics mode: a switch that is off whenever Yagi starts, and that turns itself off. While it
// is on, Yagi writes down more about each request a media server makes (how long it took, what the tuner
// answered, how many bytes went out and how the stream ended) and, when something fails, checks the tuner at that
// moment. It is for finding out why "channel not available" happens now and then, which is hard to catch from
// outside because by the time anyone looks, it has passed.
//
// Nothing is kept anywhere but the log's ring in memory, so it can't grow without limit, and it never records
// the tuner's token or anything a person typed.
package diag

import (
	"sync"
	"time"
)

const (
	// DefaultFor is how long diagnostics mode stays on when asked for without a time.
	DefaultFor = time.Hour
	// MaxFor is the longest it can be turned on for. It is for catching a failure, not for leaving on.
	MaxFor = 24 * time.Hour
)

// Mode is the switch. The zero value is off. A nil *Mode is off too, so code that may or may not have one
// doesn't need to check.
type Mode struct {
	// Now is the clock, for tests; time.Now when nil.
	Now func() time.Time
	// OnChange is called after the mode is turned on or off, by a person or by running out, with the lock released.
	OnChange func(on bool)

	mu    sync.Mutex
	until time.Time
	timer *time.Timer
}

func (m *Mode) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

// Active says whether diagnostics mode is on right now.
func (m *Mode) Active() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now().Before(m.until)
}

// Until is when it turns itself off; the zero time when it is off.
func (m *Mode) Until() time.Time {
	if m == nil {
		return time.Time{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.now().Before(m.until) {
		return time.Time{}
	}
	return m.until
}

// Enable turns it on for d (DefaultFor when d is not positive, never more than MaxFor), starting from now: turning
// it on again while it is on sets the time again rather than adding to it.
func (m *Mode) Enable(d time.Duration) {
	if d <= 0 {
		d = DefaultFor
	}
	if d > MaxFor {
		d = MaxFor
	}
	m.mu.Lock()
	m.until = m.now().Add(d)
	if m.timer != nil {
		m.timer.Stop()
	}
	m.timer = time.AfterFunc(d, m.expired)
	m.mu.Unlock()
	if m.OnChange != nil {
		m.OnChange(true)
	}
}

// Disable turns it off.
func (m *Mode) Disable() {
	m.mu.Lock()
	was := m.now().Before(m.until)
	m.until = time.Time{}
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	m.mu.Unlock()
	if was && m.OnChange != nil {
		m.OnChange(false)
	}
}

// expired runs when the time is up, so the page and the log hear that it is over.
func (m *Mode) expired() {
	m.mu.Lock()
	over := !m.now().Before(m.until) && !m.until.IsZero()
	if over {
		m.until, m.timer = time.Time{}, nil
	}
	m.mu.Unlock()
	if over && m.OnChange != nil {
		m.OnChange(false)
	}
}
