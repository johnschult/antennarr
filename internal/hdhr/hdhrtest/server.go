// Package hdhrtest is a fake HDHomeRun's control side, for tests (and the demo): it answers the discover
// and the control protocol, remembers the values it is told, collects an upgrade, and records every frame
// it is sent byte for byte, so a test can compare what a client sends with what another client sends. It
// can also misbehave on purpose.
package hdhrtest

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/johnschult/antennarr/internal/hdhr"
)

// Frame is one request the server received.
type Frame struct {
	Conn    int    // which connection, counting from 1
	Type    uint16 // the frame type
	Payload []byte
	Raw     []byte // the frame as it came, CRC and all
}

// Options is how the fake tuner behaves. The zero value is a well-behaved FLEX with 20250815 firmware.
type Options struct {
	DeviceID string // 8 hex digits; "10ABCC87" if empty
	Version  string // /sys/version; "20250815" if empty
	Model    string // /sys/model; "HDFX-4K" if empty
	Tuners   int    // how many tuners; 4 if zero

	// RejectUpgrade, if set, is the error the tuner answers the end of an upgrade with.
	RejectUpgrade string
	// DropBeforeEnd closes the connection after this many upgrade chunks, before the end marker.
	DropBeforeEnd int
	// DropAfterEnd closes the connection on the end marker, with no reply, as a tuner that restarts at once.
	DropAfterEnd bool
	// NoReply never answers the end marker (and keeps the connection open).
	NoReply bool
	// RebootFor is how long the tuner is unreachable after it accepts an upgrade (it closes every connection
	// in that time). The zero value is no time at all. A very long one is a tuner that never comes back.
	RebootFor time.Duration
	// VersionAfter is what /sys/version says after the upgrade. Empty means unchanged: a tuner that took the
	// image and ignored it.
	VersionAfter string
	// Unknown is a value name the tuner answers with an error for, besides the ones it doesn't have.
	Unknown map[string]bool
}

// Server is the fake tuner.
type Server struct {
	opts Options
	tcp  net.Listener
	udp  net.PacketConn
	wg   sync.WaitGroup

	mu       sync.Mutex
	values   map[string]string
	frames   []Frame
	conns    int
	image    []byte // the last complete upgrade image
	upgraded bool
	closed   bool
	downTill time.Time // the tuner is restarting until then
}

// New starts a fake tuner on addr ("127.0.0.1:0" for any free port; the official tool needs 65001). It
// listens for TCP control and for the UDP discover on the same port.
func New(addr string, o Options) (*Server, error) {
	tcp, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	udp, err := net.ListenPacket("udp", tcp.Addr().String())
	if err != nil {
		tcp.Close()
		return nil, err
	}
	if o.DeviceID == "" {
		o.DeviceID = "10ABCC87"
	}
	if o.Version == "" {
		o.Version = "20250815"
	}
	if o.Model == "" {
		o.Model = "HDFX-4K"
	}
	if o.Tuners == 0 {
		o.Tuners = 4
	}
	s := &Server{opts: o, tcp: tcp, udp: udp, values: map[string]string{"/sys/version": o.Version, "/sys/model": o.Model, "/sys/hwmodel": o.Model}}
	for i := 0; i < o.Tuners; i++ {
		s.values[fmt.Sprintf("/tuner%d/channel", i)] = "none"
		s.values[fmt.Sprintf("/tuner%d/lockkey", i)] = "none"
	}
	s.wg.Add(2)
	go s.serveTCP()
	go s.serveUDP()
	return s, nil
}

// Addr is where the server is listening (TCP and UDP).
func (s *Server) Addr() string { return s.tcp.Addr().String() }

// Close stops the server.
func (s *Server) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.tcp.Close()
	s.udp.Close()
	s.wg.Wait()
}

// Frames are the control requests received so far, in order (discover datagrams are not among them).
func (s *Server) Frames() []Frame {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Frame(nil), s.frames...)
}

// Forget clears the recording.
func (s *Server) Forget() {
	s.mu.Lock()
	s.frames = nil
	s.mu.Unlock()
}

// Image is the last firmware image the tuner was sent in full and told to install, or nil.
func (s *Server) Image() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.image...)
}

// Value is what the tuner holds for a name.
func (s *Server) Value(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[name]
}

// SetValue changes what the tuner holds, as if it had changed by itself (a new version after a restart).
func (s *Server) SetValue(name, value string) {
	s.mu.Lock()
	s.values[name] = value
	s.mu.Unlock()
}

func (s *Server) serveUDP() {
	defer s.wg.Done()
	buf := make([]byte, 2048)
	for {
		n, from, err := s.udp.ReadFrom(buf)
		if err != nil {
			return
		}
		typ, _, err := hdhr.Decode(buf[:n])
		if err != nil || typ != hdhr.TypeDiscoverReq {
			continue
		}
		var id uint32
		fmt.Sscanf(s.opts.DeviceID, "%X", &id)
		p := hdhr.AppendTLV(nil, hdhr.TagDeviceType, binary.BigEndian.AppendUint32(nil, 1))
		p = hdhr.AppendTLV(p, hdhr.TagDeviceID, binary.BigEndian.AppendUint32(nil, id))
		p = hdhr.AppendTLV(p, hdhr.TagTunerCount, []byte{byte(s.opts.Tuners)})
		p = hdhr.AppendTLV(p, hdhr.TagBaseURL, []byte("http://"+s.Addr()))
		s.udp.WriteTo(hdhr.Encode(hdhr.TypeDiscoverRpy, p), from)
	}
}

func (s *Server) serveTCP() {
	defer s.wg.Done()
	for {
		c, err := s.tcp.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns++
		id := s.conns
		down := time.Now().Before(s.downTill)
		s.mu.Unlock()
		if down { // restarting: nothing answers
			c.Close()
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer c.Close()
			s.handle(c, id)
		}()
	}
}

// recorder keeps the bytes of the frame being read, so the record holds the frame as it came.
type recorder struct {
	r io.Reader
	b bytes.Buffer
}

func (r *recorder) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.b.Write(p[:n])
	return n, err
}

func (s *Server) handle(c net.Conn, id int) {
	var chunks int
	var image []byte
	for {
		rec := &recorder{r: c}
		typ, payload, err := hdhr.ReadFrame(rec)
		if err != nil {
			return
		}
		s.mu.Lock()
		s.frames = append(s.frames, Frame{Conn: id, Type: typ, Payload: append([]byte(nil), payload...), Raw: append([]byte(nil), rec.b.Bytes()...)})
		s.mu.Unlock()

		switch typ {
		case hdhr.TypeGetSetReq:
			c.Write(hdhr.Encode(hdhr.TypeGetSetRpy, s.getSet(payload)))
		case hdhr.TypeUpgradeReq:
			if len(payload) < 4 {
				return
			}
			off := binary.BigEndian.Uint32(payload)
			if off != 0xFFFFFFFF {
				if int(off) != len(image) { // a gap or a repeat: the tuner would discard the lot
					return
				}
				image = append(image, payload[4:]...)
				chunks++
				if s.opts.DropBeforeEnd > 0 && chunks >= s.opts.DropBeforeEnd {
					return
				}
				continue
			}
			switch {
			case s.opts.DropAfterEnd:
				s.finish(image)
				return
			case s.opts.NoReply:
				s.finish(image)
				time.Sleep(time.Minute)
				return
			case s.opts.RejectUpgrade != "":
				c.Write(hdhr.Encode(hdhr.TypeUpgradeRpy, hdhr.AppendTLV(nil, hdhr.TagError, append([]byte(s.opts.RejectUpgrade), 0))))
			default:
				s.finish(image)
				c.Write(hdhr.Encode(hdhr.TypeUpgradeRpy, nil))
			}
		}
	}
}

func (s *Server) finish(image []byte) {
	s.mu.Lock()
	s.image = image
	s.upgraded = true
	if s.opts.RebootFor > 0 {
		s.downTill = time.Now().Add(s.opts.RebootFor)
	}
	if s.opts.VersionAfter != "" {
		s.values["/sys/version"] = s.opts.VersionAfter
	}
	s.mu.Unlock()
}

// Upgraded says whether an image was accepted.
func (s *Server) Upgraded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upgraded
}

func (s *Server) getSet(payload []byte) []byte {
	tlvs, err := hdhr.ParseTLVs(payload)
	if err != nil {
		return hdhr.AppendTLV(nil, hdhr.TagError, []byte("ERROR: bad request\x00"))
	}
	var name string
	var value *string
	for _, t := range tlvs {
		switch t.Tag {
		case hdhr.TagGetSetName:
			name = string(bytes.TrimRight(t.Value, "\x00"))
		case hdhr.TagGetSetValue:
			v := string(bytes.TrimRight(t.Value, "\x00"))
			value = &v
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, known := s.values[name]
	if !known || s.opts.Unknown[name] {
		return hdhr.AppendTLV(nil, hdhr.TagError, []byte("ERROR: invalid get/set\x00"))
	}
	if value != nil {
		s.values[name] = *value
		cur = *value
	}
	return hdhr.AppendTLV(nil, hdhr.TagGetSetValue, append([]byte(cur), 0))
}
