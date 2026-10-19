// Package logbuf keeps the most recent log lines in memory so the web page can show them. It wraps the logger
// Yagi already has: everything still goes where it went before (standard error), and a copy of each line,
// with its attributes, goes into a ring that forgets the oldest when it is full. Nothing is written to disk, and
// anything that looks like a secret is hidden before it is kept.
package logbuf

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	maxMessage = 2000
	maxValue   = 500
)

// Attr is one key and value on a line, as text.
type Attr struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Entry is one log line.
type Entry struct {
	Seq     uint64    `json:"seq"` // counts up from 1 for as long as Yagi runs
	Time    time.Time `json:"time"`
	Level   string    `json:"level"` // debug, info, warn or error
	Message string    `json:"message"`
	Attrs   []Attr    `json:"attrs,omitempty"`
}

// Buffer is the ring.
type Buffer struct {
	mu   sync.Mutex
	ring []Entry
	max  int
	next uint64 // the Seq of the next entry
}

// New keeps the last max entries.
func New(max int) *Buffer {
	if max < 1 {
		max = 1
	}
	return &Buffer{max: max, next: 1}
}

func (b *Buffer) add(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e.Seq = b.next
	b.next++
	if len(b.ring) == b.max {
		copy(b.ring, b.ring[1:])
		b.ring = b.ring[:b.max-1]
	}
	b.ring = append(b.ring, e)
}

// Query says what to return: lines after Since (0 for from the oldest kept), at or above Level (empty for all),
// containing Text (without regard to case, in the message or any attribute), and at most Limit of the newest.
type Query struct {
	Since uint64
	Level string
	Text  string
	Limit int
}

// Result is what a query found, oldest first.
type Result struct {
	Entries []Entry
	// Latest is the Seq of the newest line kept, whether or not it matched, to ask for what came after next time.
	Latest uint64
	// Kept is how many lines are held, and Capacity how many can be.
	Kept, Capacity int
}

var rank = map[string]int{"debug": 0, "info": 1, "warn": 2, "error": 3}

// Valid says whether s names a level.
func Valid(s string) bool { _, ok := rank[s]; return ok }

// Find returns the matching lines.
func (b *Buffer) Find(q Query) Result {
	b.mu.Lock()
	defer b.mu.Unlock()
	res := Result{Entries: []Entry{}, Latest: b.next - 1, Kept: len(b.ring), Capacity: b.max}
	min := rank[q.Level]
	text := strings.ToLower(q.Text)
	for _, e := range b.ring {
		if e.Seq <= q.Since || rank[e.Level] < min {
			continue
		}
		if text != "" && !matches(e, text) {
			continue
		}
		res.Entries = append(res.Entries, e)
	}
	if q.Limit > 0 && len(res.Entries) > q.Limit {
		res.Entries = res.Entries[len(res.Entries)-q.Limit:]
	}
	return res
}

func matches(e Entry, text string) bool {
	if strings.Contains(strings.ToLower(e.Message), text) {
		return true
	}
	for _, a := range e.Attrs {
		if strings.Contains(strings.ToLower(a.Key), text) || strings.Contains(strings.ToLower(a.Value), text) {
			return true
		}
	}
	return false
}

// Clear forgets what is kept. The count does not start over, so a page that asks for what came after the last
// line it saw is not confused.
func (b *Buffer) Clear() {
	b.mu.Lock()
	b.ring = nil
	b.mu.Unlock()
}

// Tee is a handler that passes every record to next and keeps a copy in b.
func Tee(next slog.Handler, b *Buffer) slog.Handler { return &handler{next: next, buf: b} }

type handler struct {
	next   slog.Handler
	buf    *Buffer
	attrs  []slog.Attr
	prefix string // the groups so far, as "a.b."
}

func (h *handler) Enabled(ctx context.Context, l slog.Level) bool { return h.next.Enabled(ctx, l) }

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	e := Entry{Time: r.Time, Level: level(r.Level), Message: hide(clip(r.Message, maxMessage))}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	add := func(prefix string, a slog.Attr) { collect(&e.Attrs, prefix, a) }
	for _, a := range h.attrs {
		add("", a)
	}
	r.Attrs(func(a slog.Attr) bool {
		add(h.prefix, a)
		return true
	})
	h.buf.add(e)
	return h.next.Handle(ctx, r)
}

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	c := *h
	c.next = h.next.WithAttrs(as)
	c.attrs = append([]slog.Attr(nil), h.attrs...)
	for _, a := range as {
		c.attrs = append(c.attrs, slog.Attr{Key: h.prefix + a.Key, Value: a.Value})
	}
	return &c
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.next = h.next.WithGroup(name)
	c.prefix = h.prefix + name + "."
	return &c
}

func level(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "error"
	case l >= slog.LevelWarn:
		return "warn"
	case l >= slog.LevelInfo:
		return "info"
	}
	return "debug"
}

// collect adds an attribute, flattening groups into dotted keys.
func collect(out *[]Attr, prefix string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		p := prefix
		if a.Key != "" {
			p += a.Key + "."
		}
		for _, g := range a.Value.Group() {
			collect(out, p, g)
		}
		return
	}
	if a.Key == "" {
		return
	}
	key := prefix + a.Key
	val := fmt.Sprint(a.Value.Any())
	if secretKey.MatchString(key) {
		val = hidden
	}
	*out = append(*out, Attr{Key: key, Value: hide(clip(val, maxValue))})
}

const hidden = "[hidden]"

// secretKey is an attribute name that holds something that must not be shown.
var secretKey = regexp.MustCompile(`(?i)token|secret|passw|apikey|api_key|deviceauth|authorization|cookie|lockkey`)

// secretText is something in a piece of text that looks like a secret being passed: token=…, DeviceAuth=…, a
// bearer token.
var secretText = regexp.MustCompile(`(?i)((?:token|secret|password|passwd|deviceauth|apikey|api_key|key)=)[^&\s"']+|(bearer\s+)[A-Za-z0-9._~+/=-]+`)

func hide(s string) string {
	return secretText.ReplaceAllStringFunc(s, func(m string) string {
		if i := strings.IndexAny(m, "= "); i >= 0 {
			return m[:i+1] + hidden
		}
		return hidden
	})
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + "…"
}
