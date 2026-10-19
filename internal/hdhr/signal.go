package hdhr

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrTunerBusy is what testing a signal returns when the tuner it was going to use turns out not to be free:
// something has it tuned or locked. Nothing was changed.
var ErrTunerBusy = errors.New("that tuner is in use")

// SetKeyed is Set with a lock key: a tuner that is locked to a key only takes changes that carry it.
func (c *Conn) SetKeyed(ctx context.Context, name, value string, key uint32) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.watch(ctx)()
	payload := AppendTLV(nil, TagGetSetName, cstr(name))
	payload = AppendTLV(payload, TagGetSetValue, cstr(value))
	if key != 0 {
		payload = AppendTLV(payload, TagLockKey, binary.BigEndian.AppendUint32(nil, key))
	}
	return c.exchange(ctx, payload)
}

// Signal is what a tuner reported while tuned to a channel.
type Signal struct {
	Tuner    int    `json:"tuner"`
	Channel  string `json:"channel"`
	Locked   bool   `json:"locked"`         // whether the tuner got a lock on it
	Lock     string `json:"lock,omitempty"` // what it locked on to (atsc3, 8vsb, qam256)
	Strength int    `json:"strength"`       // signal strength, percent
	Quality  int    `json:"quality"`        // signal-to-noise quality, percent
	Symbol   int    `json:"symbol"`         // symbol quality, percent
}

// TestOptions are the timings of a signal test, which tests make short.
type TestOptions struct {
	// Wait is how long to wait for a lock; Settle is how long to let a locked signal settle before reading
	// it again; Poll is how often to ask. The defaults are 8s, 1s and 500ms.
	Wait, Settle, Poll time.Duration
	// Key makes the lock key; the default is a random one.
	Key func() uint32
}

func (o TestOptions) withDefaults() TestOptions {
	if o.Wait == 0 {
		o.Wait = 8 * time.Second
	}
	if o.Settle == 0 {
		o.Settle = time.Second
	}
	if o.Poll == 0 {
		o.Poll = 500 * time.Millisecond
	}
	if o.Key == nil {
		o.Key = func() uint32 {
			var b [4]byte
			for {
				if _, err := rand.Read(b[:]); err != nil {
					return uint32(time.Now().UnixNano())
				}
				if k := binary.BigEndian.Uint32(b[:]); k != 0 {
					return k
				}
			}
		}
	}
	return o
}

var vchannelRe = regexp.MustCompile(`^[0-9]{1,5}(\.[0-9]{1,5})?$`)

// TestSignal tunes a free tuner to a channel for a few seconds and reads how well it receives it, the way a
// person would with the tuner's own pages, and lets the tuner go again.
//
// It only ever uses a tuner that is free: one that has no lock, no channel and no target. It locks it with
// a key of its own before tuning, so nothing else can retune it while it is looked at, and it gives the lock
// and the channel back however the test ends (including when ctx ends first), so a test that fails does
// not leave a tuner held. A tuner that turns out to be busy is left alone and ErrTunerBusy is returned.
func (c *Conn) TestSignal(ctx context.Context, tuner int, vchannel string, o TestOptions) (Signal, error) {
	o = o.withDefaults()
	if tuner < 0 || tuner > 15 {
		return Signal{}, errors.New("hdhr: no such tuner")
	}
	if !vchannelRe.MatchString(vchannel) {
		return Signal{}, errors.New("hdhr: that isn't a channel number")
	}
	name := func(v string) string { return fmt.Sprintf("/tuner%d/%s", tuner, v) }

	// A free tuner has nothing on it. Anything else is somebody's.
	for _, v := range []string{"lockkey", "channel", "target"} {
		got, err := c.Get(ctx, name(v))
		if err != nil {
			return Signal{}, fmt.Errorf("couldn't read the tuner's %s: %w", v, err)
		}
		if got != "none" {
			return Signal{}, ErrTunerBusy
		}
	}

	// Take it with a key of our own. A tuner that is locked by someone else refuses with an error; one that takes
	// the lock says nothing more than yes (a real one answers with the address that holds the lock, not the key
	// that was set, so the answer is not compared with the key). It is ours from here, whatever else goes wrong.
	key := o.Key()
	var refused *Error
	if _, err := c.Set(ctx, name("lockkey"), strconv.FormatUint(uint64(key), 10)); err != nil {
		if errors.As(err, &refused) {
			return Signal{}, ErrTunerBusy
		}
		return Signal{}, fmt.Errorf("couldn't take the tuner: %w", err)
	}
	// Whatever happens next, give it back: the channel first, then the lock.
	defer func() {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		// Best effort: if the tuner does not answer there is nothing more to be done from here.
		_, _ = c.SetKeyed(rctx, name("channel"), "none", key)
		_, _ = c.SetKeyed(rctx, name("lockkey"), "none", key)
	}()

	if _, err := c.SetKeyed(ctx, name("vchannel"), vchannel, key); err != nil {
		if errors.As(err, &refused) {
			return Signal{}, fmt.Errorf("the tuner wouldn't tune to it: %s", strings.TrimPrefix(refused.Message, "ERROR: "))
		}
		return Signal{}, fmt.Errorf("couldn't tune the tuner: %w", err)
	}

	read := func() (Signal, error) {
		s, err := c.Get(ctx, name("status"))
		if err != nil {
			return Signal{}, err
		}
		sig := ParseStatus(s)
		sig.Tuner, sig.Channel = tuner, vchannel
		return sig, nil
	}
	var last Signal
	deadline := time.Now().Add(o.Wait)
	for {
		sig, err := read()
		if err != nil {
			return Signal{}, fmt.Errorf("couldn't read the tuner's signal: %w", err)
		}
		last = sig
		if sig.Locked {
			// let it settle, and read it once more: the first reading after a lock is often still climbing
			if !sleep(ctx, o.Settle) {
				return last, ctx.Err()
			}
			if again, err := read(); err == nil && again.Locked {
				last = again
			}
			return last, nil
		}
		if time.Now().Add(o.Poll).After(deadline) {
			return last, nil // never locked: what it did read is the answer
		}
		if !sleep(ctx, o.Poll) {
			return last, ctx.Err()
		}
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// ParseStatus reads a tuner's status line ("ch=8vsb:575000000 lock=8vsb ss=84 snq=100 seq=100 bps=0 pps=0").
// A field it doesn't find is zero, and a tuner that says lock=none, or doesn't say, is not locked.
func ParseStatus(s string) Signal {
	var sig Signal
	for _, f := range strings.Fields(s) {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			continue
		}
		n, _ := strconv.Atoi(v)
		n = min(max(n, 0), 100)
		switch k {
		case "lock":
			if v != "none" && v != "" {
				sig.Lock, sig.Locked = v, true
			}
		case "ss":
			sig.Strength = n
		case "snq":
			sig.Quality = n
		case "seq":
			sig.Symbol = n
		}
	}
	return sig
}
