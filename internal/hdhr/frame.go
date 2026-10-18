// Package hdhr talks to an HDHomeRun over its control protocol (TCP port 65001): reading and setting
// the values it keeps (/sys/version, /tuner0/channel...) and installing firmware. It is the protocol
// that Silicondust's hdhomerun_config tool speaks, written from libhdhomerun's documented packet format
// and checked byte for byte against that tool (see conformance_test.go).
package hdhr

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

// ControlPort is where a tuner listens for control, both over TCP and for the UDP discover.
const ControlPort = "65001"

// Frame types. A reply is the request's type plus one.
const (
	TypeDiscoverReq uint16 = 0x0002
	TypeDiscoverRpy uint16 = 0x0003
	TypeGetSetReq   uint16 = 0x0004
	TypeGetSetRpy   uint16 = 0x0005
	TypeUpgradeReq  uint16 = 0x0006
	TypeUpgradeRpy  uint16 = 0x0007
)

// TLV tags.
const (
	TagDeviceType  byte = 0x01
	TagDeviceID    byte = 0x02
	TagGetSetName  byte = 0x03
	TagGetSetValue byte = 0x04
	TagError       byte = 0x05
	TagTunerCount  byte = 0x10
	TagLockKey     byte = 0x15
	TagBaseURL     byte = 0x2A
)

const (
	// MaxPayload is the most a frame can carry (libhdhomerun's HDHOMERUN_MAX_PAYLOAD_SIZE).
	MaxPayload = 1452
	headerLen  = 4 // type and length
	crcLen     = 4
)

// ErrBadCRC is a frame whose checksum doesn't match: whatever sent it is not speaking this protocol, or
// the stream is out of step.
var ErrBadCRC = errors.New("hdhr: bad frame checksum")

// Encode makes a frame: type and payload length (big-endian), the payload, and a CRC-32 (IEEE) of all of
// that, which unlike everything else in the frame is little-endian.
func Encode(typ uint16, payload []byte) []byte {
	b := make([]byte, 0, headerLen+len(payload)+crcLen)
	b = binary.BigEndian.AppendUint16(b, typ)
	b = binary.BigEndian.AppendUint16(b, uint16(len(payload)))
	b = append(b, payload...)
	return binary.LittleEndian.AppendUint32(b, crc32.ChecksumIEEE(b))
}

// ReadFrame reads one whole frame from a stream, which does not line up with frames: it reads the header,
// then exactly as much as the header says, and checks the checksum.
func ReadFrame(r io.Reader) (typ uint16, payload []byte, err error) {
	var head [headerLen]byte
	if _, err = io.ReadFull(r, head[:]); err != nil {
		return 0, nil, err
	}
	n := int(binary.BigEndian.Uint16(head[2:]))
	rest := make([]byte, n+crcLen)
	if _, err = io.ReadFull(r, rest); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return 0, nil, err
	}
	sum := crc32.NewIEEE()
	sum.Write(head[:])
	sum.Write(rest[:n])
	if sum.Sum32() != binary.LittleEndian.Uint32(rest[n:]) {
		return 0, nil, ErrBadCRC
	}
	return binary.BigEndian.Uint16(head[:2]), rest[:n], nil
}

// Decode reads a frame that is already all in memory (a UDP datagram).
func Decode(b []byte) (typ uint16, payload []byte, err error) {
	if len(b) < headerLen+crcLen {
		return 0, nil, io.ErrUnexpectedEOF
	}
	n := int(binary.BigEndian.Uint16(b[2:]))
	if len(b) < headerLen+n+crcLen {
		return 0, nil, io.ErrUnexpectedEOF
	}
	if crc32.ChecksumIEEE(b[:headerLen+n]) != binary.LittleEndian.Uint32(b[headerLen+n:]) {
		return 0, nil, ErrBadCRC
	}
	return binary.BigEndian.Uint16(b[:2]), b[headerLen : headerLen+n], nil
}

// AppendTLV adds a tag, a length (one byte up to 127, otherwise two) and a value.
func AppendTLV(b []byte, tag byte, value []byte) []byte {
	b = append(b, tag)
	if n := len(value); n <= 127 {
		b = append(b, byte(n))
	} else {
		b = append(b, byte(n)|0x80, byte(n>>7))
	}
	return append(b, value...)
}

// TLV is one tag and its value, as read from a payload.
type TLV struct {
	Tag   byte
	Value []byte
}

// ParseTLVs reads a payload's tags. A tag it doesn't know is returned like any other: the caller picks
// what it wants and skips the rest.
func ParseTLVs(payload []byte) ([]TLV, error) {
	var out []TLV
	for len(payload) > 0 {
		if len(payload) < 2 {
			return nil, fmt.Errorf("hdhr: a tag with no length")
		}
		tag, n, rest := payload[0], int(payload[1]), payload[2:]
		if n&0x80 != 0 {
			if len(rest) < 1 {
				return nil, fmt.Errorf("hdhr: a tag with half a length")
			}
			n = n&0x7F | int(rest[0])<<7
			rest = rest[1:]
		}
		if n > len(rest) {
			return nil, fmt.Errorf("hdhr: tag %#x is %d bytes but only %d are left", tag, n, len(rest))
		}
		out = append(out, TLV{Tag: tag, Value: rest[:n]})
		payload = rest[n:]
	}
	return out, nil
}

// cstr is a value as the protocol writes a string: with a NUL on the end.
func cstr(s string) []byte { return append([]byte(s), 0) }

// fromCstr takes the NUL (and anything after it) off a string value. The protocol's own header says not
// to trust that it is there, so it is not required.
func fromCstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
