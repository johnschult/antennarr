package hdhr

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
)

const (
	// chunkSize is how much of the image goes in each frame, as libhdhomerun does it.
	chunkSize = 1024
	// endMarker, in place of an offset, says the whole image has been sent and the tuner can use it.
	endMarker = 0xFFFFFFFF
)

// ErrNoReply is what Upgrade returns when the image was sent and the end marker acknowledged by nothing:
// the connection ended or timed out. A tuner that is already restarting does that, so it is not a
// failure by itself; the caller finds out by asking the tuner what it is running afterwards.
var ErrNoReply = errors.New("the tuner sent no reply after the firmware")

// Upgrade sends a firmware image and tells the tuner to install it. It does exactly what
// hdhomerun_config does, in the same order:
//
//  1. free the tuners (take the lock from whatever has it, and stop them), as a tuner that is in use
//     won't take an upgrade. This cuts every stream on the tuner. Errors are ignored: a tuner with one
//     tuner has no /tuner1.
//  2. read /sys/version (libhdhomerun does, for an old tuner that needs a slower upload),
//  3. send the image in 1 KB frames, each with its byte offset and no reply,
//  4. send the end marker and wait up to 40 seconds for the tuner's answer.
//
// progress, if set, is called with the bytes sent so far and the total. A nil error means the tuner
// accepted the image: it is still to restart and install it, which takes about a minute. The tuner does no
// checking of an image on the client's behalf before the end marker, so nothing here can make a bad image
// safe: that is for whoever chooses the image.
func (c *Conn) Upgrade(ctx context.Context, image []byte, progress func(sent, total int)) error {
	if len(image) == 0 {
		return errors.New("hdhr: that firmware image is empty")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.watch(ctx)()

	for _, tuner := range []string{"/tuner0", "/tuner1"} {
		c.rawSet(tuner+"/lockkey", "force")
		c.rawSet(tuner+"/channel", "none")
	}
	c.rawGet("/sys/version")

	for off := 0; off < len(image); off += chunkSize {
		end := min(off+chunkSize, len(image))
		payload := binary.BigEndian.AppendUint32(make([]byte, 0, 4+end-off), uint32(off))
		payload = append(payload, image[off:end]...)
		if err := c.send(TypeUpgradeReq, payload); err != nil {
			return ctxErr(ctx, err)
		}
		if progress != nil {
			progress(end, len(image))
		}
	}

	if err := c.send(TypeUpgradeReq, binary.BigEndian.AppendUint32(nil, endMarker)); err != nil {
		return ctxErr(ctx, err)
	}
	c.endSent = true
	reply, err := c.recv(TypeUpgradeRpy, UpgradeTimeout)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var ne net.Error
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || (errors.As(err, &ne) && ne.Timeout()) || errors.Is(err, os.ErrDeadlineExceeded) {
			return ErrNoReply
		}
		return err
	}
	tlvs, err := ParseTLVs(reply)
	if err != nil {
		return err
	}
	for _, t := range tlvs {
		if t.Tag == TagError {
			return &Error{Message: fromCstr(t.Value)}
		}
	}
	return nil
}

// rawGet and rawSet are for the steps that ignore the answer. The lock is already held.
func (c *Conn) rawGet(name string) {
	if c.send(TypeGetSetReq, AppendTLV(nil, TagGetSetName, cstr(name))) == nil {
		_, _ = c.recv(TypeGetSetRpy, ReplyTimeout) // the reply is not used: this only sends the request libhdhomerun sends
	}
}

func (c *Conn) rawSet(name, value string) {
	p := AppendTLV(nil, TagGetSetName, cstr(name))
	p = AppendTLV(p, TagGetSetValue, cstr(value))
	if c.send(TypeGetSetReq, p) == nil {
		_, _ = c.recv(TypeGetSetRpy, ReplyTimeout) // the reply is not used: this only sends the request libhdhomerun sends
	}
}

// Percent is how far through an upload is, 0 to 100, for showing.
func Percent(sent, total int) int {
	if total <= 0 {
		return 0
	}
	return min(100, sent*100/total)
}
