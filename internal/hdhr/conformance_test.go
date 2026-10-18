package hdhr_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/hdhr"
	"github.com/johnschult/antennarr/internal/hdhr/hdhrtest"
)

// This is the check that matters most. Silicondust's own hdhomerun_config is the oracle: it and this
// package are run against the same fake tuner, which records every byte it is sent, and what they send
// must be identical. Our own tests can't show that we have understood the protocol, since the fake is
// written from the same understanding; the official tool can.
//
// It needs the official tool built (https://github.com/Silicondust/libhdhomerun, `make`) and port 65001
// free on this machine, because the tool only ever talks to 65001. It is skipped otherwise, so it never
// runs in CI. Run it with:
//
//	HDHOMERUN_CONFIG=/path/to/hdhomerun_config go test -run Conformance -v ./internal/hdhr
func official(t *testing.T) string {
	t.Helper()
	path := os.Getenv("HDHOMERUN_CONFIG")
	if path == "" {
		t.Skip("HDHOMERUN_CONFIG is not set: no official hdhomerun_config to compare with")
	}
	return path
}

func startOnControlPort(t *testing.T, o hdhrtest.Options) *hdhrtest.Server {
	t.Helper()
	s, err := hdhrtest.New("127.0.0.1:"+hdhr.ControlPort, o)
	if err != nil {
		t.Skipf("port %s is not free here: %v", hdhr.ControlPort, err)
	}
	t.Cleanup(s.Close)
	return s
}

// describe is a frame in words, for a failure message.
func describe(f hdhrtest.Frame) string {
	switch f.Type {
	case hdhr.TypeGetSetReq:
		tlvs, _ := hdhr.ParseTLVs(f.Payload)
		var parts []string
		for _, tl := range tlvs {
			parts = append(parts, fmt.Sprintf("%#x=%q", tl.Tag, tl.Value))
		}
		return "getset " + strings.Join(parts, " ")
	case hdhr.TypeUpgradeReq:
		off := binary.BigEndian.Uint32(f.Payload)
		if off == 0xFFFFFFFF {
			return "upgrade END"
		}
		return fmt.Sprintf("upgrade offset=%d len=%d", off, len(f.Payload)-4)
	}
	return fmt.Sprintf("type %#x", f.Type)
}

func dump(frames []hdhrtest.Frame) string {
	var b strings.Builder
	for i, f := range frames {
		fmt.Fprintf(&b, "  %d. conn %d: %s\n", i+1, f.Conn, describe(f))
	}
	return b.String()
}

// same fails unless the two sequences are the same frames, byte for byte (the connection they came on
// doesn't matter).
func same(t *testing.T, what string, official, ours []hdhrtest.Frame) {
	t.Helper()
	if len(official) != len(ours) {
		t.Fatalf("%s: the official tool sent %d frames and we sent %d\nofficial:\n%sours:\n%s", what, len(official), len(ours), dump(official), dump(ours))
	}
	for i := range official {
		if !bytes.Equal(official[i].Raw, ours[i].Raw) {
			t.Fatalf("%s: frame %d differs\nofficial: %s\n          %x\nours:     %s\n          %x", what, i+1, describe(official[i]), official[i].Raw, describe(ours[i]), ours[i].Raw)
		}
	}
	t.Logf("%s: %d frames, identical to the official tool's\n%s", what, len(official), dump(ours))
}

func runOfficial(t *testing.T, tool string, args ...string) string {
	t.Helper()
	out, err := exec.Command(tool, append([]string{"127.0.0.1"}, args...)...).CombinedOutput()
	t.Logf("hdhomerun_config %s -> %q (%v)", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	return string(out)
}

func TestConformanceGetAndSet(t *testing.T) {
	tool := official(t)
	srv := startOnControlPort(t, hdhrtest.Options{})
	ctx := context.Background()

	for _, tc := range []struct {
		what string
		args []string
		do   func(*hdhr.Conn) error
	}{
		{"get /sys/version", []string{"get", "/sys/version"}, func(c *hdhr.Conn) error { _, err := c.Get(ctx, "/sys/version"); return err }},
		{"get /sys/model", []string{"get", "/sys/model"}, func(c *hdhr.Conn) error { _, err := c.Get(ctx, "/sys/model"); return err }},
		{"set /tuner0/channel none", []string{"set", "/tuner0/channel", "none"}, func(c *hdhr.Conn) error { _, err := c.Set(ctx, "/tuner0/channel", "none"); return err }},
		{"set /tuner1/lockkey force", []string{"set", "/tuner1/lockkey", "force"}, func(c *hdhr.Conn) error { _, err := c.Set(ctx, "/tuner1/lockkey", "force"); return err }},
	} {
		srv.Forget()
		runOfficial(t, tool, tc.args...)
		theirs := srv.Frames()

		srv.Forget()
		c, err := hdhr.Dial(ctx, srv.Addr())
		if err != nil {
			t.Fatal(err)
		}
		if err := tc.do(c); err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		c.Close()

		// The official tool also asks for the model once it has connected, so it sends more than the command: compare
		// the command itself, which is the last request it makes.
		if len(theirs) == 0 {
			t.Fatalf("%s: the official tool sent nothing", tc.what)
		}
		same(t, tc.what, theirs[len(theirs)-1:], srv.Frames())
	}
}

// A long value uses a two-byte length, which is the part of the framing most easily got wrong.
func TestConformanceLongValue(t *testing.T) {
	tool := official(t)
	srv := startOnControlPort(t, hdhrtest.Options{})
	long := strings.Repeat("x", 300)
	srv.SetValue("/sys/long", "")

	srv.Forget()
	runOfficial(t, tool, "set", "/sys/long", long)
	theirs := srv.Frames()
	srv.Forget()
	c, err := hdhr.Dial(context.Background(), srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Set(context.Background(), "/sys/long", long)
	if len(theirs) == 0 {
		t.Fatal("the official tool sent nothing")
	}
	same(t, "set with a 300 byte value", theirs[len(theirs)-1:], srv.Frames())
}

func TestConformanceUpgrade(t *testing.T) {
	tool := official(t)
	srv := startOnControlPort(t, hdhrtest.Options{})

	// Not a multiple of the 1 KB chunk, so the last frame is short, as the real image's is.
	image := make([]byte, 2500)
	for i := range image {
		image[i] = byte(i * 7)
	}
	file := t.TempDir() + "/fw.bin"
	if err := os.WriteFile(file, image, 0o600); err != nil {
		t.Fatal(err)
	}

	srv.Forget()
	start := time.Now()
	runOfficial(t, tool, "upgrade", file) // it waits about ten seconds for the "restart"
	t.Logf("the official upgrade took %v", time.Since(start).Round(time.Second))
	theirs := srv.Frames()
	if !bytes.Equal(srv.Image(), image) {
		t.Fatalf("the tuner did not get the whole image from the official tool (%d of %d bytes)", len(srv.Image()), len(image))
	}

	// The official tool sends a model query first and polls the version afterwards; compare the upgrade itself:
	// from freeing the tuners to the end marker.
	span := func(frames []hdhrtest.Frame) []hdhrtest.Frame {
		from, to := -1, -1
		for i, f := range frames {
			if from < 0 && f.Type == hdhr.TypeGetSetReq && bytes.Contains(f.Payload, []byte("/tuner0/lockkey")) {
				from = i
			}
			if f.Type == hdhr.TypeUpgradeReq && len(f.Payload) == 4 && binary.BigEndian.Uint32(f.Payload) == 0xFFFFFFFF {
				to = i
			}
		}
		if from < 0 || to < from {
			t.Fatalf("no upgrade in what was sent:\n%s", dump(frames))
		}
		return frames[from : to+1]
	}

	srv.Forget()
	c, err := hdhr.Dial(context.Background(), srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var sent int
	if err := c.Upgrade(context.Background(), image, func(s, total int) { sent = s }); err != nil {
		t.Fatalf("our upgrade: %v", err)
	}
	if sent != len(image) {
		t.Errorf("progress ended at %d of %d", sent, len(image))
	}
	if !bytes.Equal(srv.Image(), image) {
		t.Fatalf("the tuner did not get the whole image from us (%d of %d bytes)", len(srv.Image()), len(image))
	}
	same(t, "a 2500 byte upgrade", span(theirs), span(srv.Frames()))
}
