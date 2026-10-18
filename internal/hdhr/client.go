package hdhr

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Timeouts, the same as libhdhomerun's: a tuner answers a get or set at once, and the reply to the end of
// an upgrade can take a while because the tuner is checking and writing the image.
const (
	DialTimeout    = 2500 * time.Millisecond
	ReplyTimeout   = 2500 * time.Millisecond
	UpgradeTimeout = 40 * time.Second
)

// Error is a tuner saying no: the text it sent back with a get or set it didn't like.
type Error struct{ Message string }

func (e *Error) Error() string { return "the tuner said: " + e.Message }

// Conn is one open control connection to a tuner. It is not for use from several goroutines at once:
// the protocol answers one request at a time.
type Conn struct {
	mu      sync.Mutex
	conn    net.Conn
	endSent bool // an upgrade's end marker has gone out
}

// EndSent says whether an upgrade got as far as sending the end marker. After that the tuner may be
// installing the image whatever else happened, so a failure from then on is not "nothing was changed".
func (c *Conn) EndSent() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.endSent
}

// redial is how long to wait before the one retry of a connection: libhdhomerun tries twice, and a first
// attempt to a tuner on the local network does fail now and then (a neighbour not yet known to the
// router, or a local-network permission being granted) and work a moment later.
const redial = 300 * time.Millisecond

// Dial opens a control connection to host:port. The host is the tuner's address; use ControlPort for the
// port unless a test is talking to something of its own. A refused or failed first attempt is tried once
// more, as the official tool does.
func Dial(ctx context.Context, addr string) (*Conn, error) {
	d := net.Dialer{Timeout: DialTimeout}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil && ctx.Err() == nil {
		select {
		case <-time.After(redial):
		case <-ctx.Done():
		}
		c, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("couldn't reach the tuner's control port: %w", ctxErr(ctx, err))
	}
	return &Conn{conn: c}, nil
}

// Close ends the connection.
func (c *Conn) Close() error { return c.conn.Close() }

// watch closes the connection if ctx ends first, so a read or write that is waiting gives up; the
// returned func stops watching.
func (c *Conn) watch(ctx context.Context) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			c.conn.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}

func (c *Conn) send(typ uint16, payload []byte) error {
	c.conn.SetWriteDeadline(time.Now().Add(ReplyTimeout))
	_, err := c.conn.Write(Encode(typ, payload))
	return err
}

func (c *Conn) recv(want uint16, timeout time.Duration) ([]byte, error) {
	c.conn.SetReadDeadline(time.Now().Add(timeout))
	typ, payload, err := ReadFrame(c.conn)
	if err != nil {
		return nil, err
	}
	if typ != want {
		return nil, fmt.Errorf("hdhr: unexpected reply type %#x", typ)
	}
	return payload, nil
}

// Get reads a value from the tuner, such as /sys/version.
func (c *Conn) Get(ctx context.Context, name string) (string, error) {
	return c.getSet(ctx, name, nil)
}

// Set changes a value on the tuner and returns what it now holds.
func (c *Conn) Set(ctx context.Context, name, value string) (string, error) {
	return c.getSet(ctx, name, &value)
}

func (c *Conn) getSet(ctx context.Context, name string, value *string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.watch(ctx)()

	payload := AppendTLV(nil, TagGetSetName, cstr(name))
	if value != nil {
		payload = AppendTLV(payload, TagGetSetValue, cstr(*value))
	}
	if len(payload) > MaxPayload {
		return "", errors.New("hdhr: request too long")
	}
	if err := c.send(TypeGetSetReq, payload); err != nil {
		return "", ctxErr(ctx, err)
	}
	reply, err := c.recv(TypeGetSetRpy, ReplyTimeout)
	if err != nil {
		return "", ctxErr(ctx, err)
	}
	tlvs, err := ParseTLVs(reply)
	if err != nil {
		return "", err
	}
	for _, t := range tlvs {
		switch t.Tag {
		case TagGetSetValue:
			return fromCstr(t.Value), nil
		case TagError:
			return "", &Error{Message: fromCstr(t.Value)}
		}
	}
	return "", errors.New("hdhr: the reply had neither a value nor an error")
}

// ctxErr says the context ended when that is why a call failed, which is more useful than "use of closed
// network connection".
func ctxErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
