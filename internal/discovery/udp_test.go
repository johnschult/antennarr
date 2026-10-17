package discovery

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"net"
	"strings"
	"testing"
	"time"
)

// tag encodes one tag/length/value, using the two-byte length form when needed.
func tag(t byte, v []byte) []byte {
	out := []byte{t}
	if len(v) < 0x80 {
		out = append(out, byte(len(v)))
	} else {
		out = append(out, byte(len(v)&0x7F)|0x80, byte(len(v)>>7))
	}
	return append(out, v...)
}

func u32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }

func reply(devType, id uint32, count byte, baseURL string) []byte {
	var p []byte
	p = append(p, tag(tagDeviceType, u32(devType))...)
	p = append(p, tag(tagDeviceID, u32(id))...)
	p = append(p, tag(tagTunerCount, []byte{count})...)
	if baseURL != "" {
		p = append(p, tag(tagBaseURL, []byte(baseURL))...)
	}
	return frame(typeDiscoverReply, p)
}

func TestDiscoverRequestBytes(t *testing.T) {
	got := discoverRequest()
	wantBody := []byte{
		0x00, 0x02, // discover request
		0x00, 0x0C, // 12 bytes of tags
		0x01, 0x04, 0xFF, 0xFF, 0xFF, 0xFF, // any device type
		0x02, 0x04, 0xFF, 0xFF, 0xFF, 0xFF, // any device id
	}
	if !bytes.HasPrefix(got, wantBody) || len(got) != len(wantBody)+4 {
		t.Fatalf("request = % x", got)
	}
	if c := binary.LittleEndian.Uint32(got[len(wantBody):]); c != crc32.ChecksumIEEE(wantBody) {
		t.Errorf("CRC = %08x, want %08x", c, crc32.ChecksumIEEE(wantBody))
	}
}

func TestParseReply(t *testing.T) {
	d, ok, err := parseReply(reply(deviceTypeTuner, 0x10ABCC87, 4, "http://192.168.1.132"))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if d.ID != "10ABCC87" || d.TunerCount != 4 || d.BaseURL != "http://192.168.1.132" {
		t.Errorf("device = %+v", d)
	}
}

func TestParseReplyLongTagUsesTwoByteLength(t *testing.T) {
	long := "http://" + strings.Repeat("a", 200) + ".example"
	d, ok, err := parseReply(reply(deviceTypeTuner, 0x10ABCC87, 2, long))
	if err != nil || !ok || d.BaseURL != long {
		t.Fatalf("ok=%v err=%v baseURL len=%d", ok, err, len(d.BaseURL))
	}
}

func TestParseReplyNonTunerIsNotOK(t *testing.T) {
	const storage = 5
	_, ok, err := parseReply(reply(storage, 0x1234ABCD, 0, "http://dvr"))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("a storage engine reply was accepted as a tuner")
	}
}

func TestParseReplyRejectsBadPackets(t *testing.T) {
	good := reply(deviceTypeTuner, 0x10ABCC87, 4, "http://x")
	flipped := append([]byte(nil), good...)
	flipped[6] ^= 0xFF // corrupt a payload byte; CRC no longer matches

	wrongType := frame(0x0004, tag(tagDeviceID, u32(1)))
	noID := frame(typeDiscoverReply, tag(tagTunerCount, []byte{4}))
	overrun := frame(typeDiscoverReply, []byte{tagBaseURL, 50, 'x'})

	cases := map[string][]byte{
		"empty":          nil,
		"short":          good[:5],
		"truncated":      good[:len(good)-3],
		"bad crc":        flipped,
		"wrong type":     wrongType,
		"no device id":   noID,
		"tag overruns":   overrun,
		"request packet": discoverRequest(),
	}
	for name, b := range cases {
		if _, _, err := parseReply(b); err == nil {
			t.Errorf("%s: parsed without error", name)
		}
	}
}

// fakeTuner answers discovery requests on a loopback UDP socket. answer gets
// the 1-based request count and returns the packets to send back.
func fakeTuner(t *testing.T, answer func(n int) [][]byte) string {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		buf := make([]byte, 2048)
		for n := 1; ; n++ {
			size, from, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			if !bytes.Equal(buf[:size], discoverRequest()) {
				continue
			}
			for _, pkt := range answer(n) {
				_, _ = conn.WriteTo(pkt, from)
			}
		}
	}()
	return conn.LocalAddr().String()
}

func TestQueryCollectsTunersOnly(t *testing.T) {
	addr := fakeTuner(t, func(int) [][]byte {
		return [][]byte{
			[]byte("garbage"),
			reply(5, 0x1234ABCD, 0, "http://dvr"), // storage engine, not a tuner
			reply(deviceTypeTuner, 0x10ABCC87, 4, "http://192.168.1.132"),
			reply(deviceTypeTuner, 0x10ABCC87, 4, "http://192.168.1.132"), // duplicate
		}
	})
	got, err := Query(context.Background(), 300*time.Millisecond, []string{addr})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "10ABCC87" || got[0].TunerCount != 4 || got[0].BaseURL != "http://192.168.1.132" {
		t.Errorf("got %+v", got)
	}
}

func TestQueryFallsBackToSenderAddressWithoutBaseURL(t *testing.T) {
	addr := fakeTuner(t, func(int) [][]byte {
		return [][]byte{reply(deviceTypeTuner, 0x10ABCC87, 4, "")}
	})
	got, err := Query(context.Background(), 300*time.Millisecond, []string{addr})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if got[0].BaseURL != "http://127.0.0.1" {
		t.Errorf("BaseURL = %q", got[0].BaseURL)
	}
}

func TestQueryResendsAfterLostPacket(t *testing.T) {
	addr := fakeTuner(t, func(n int) [][]byte {
		if n == 1 {
			return nil // first request "lost"
		}
		return [][]byte{reply(deviceTypeTuner, 0x10ABCC87, 4, "http://t")}
	})
	got, err := Query(context.Background(), 600*time.Millisecond, []string{addr})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestQueryNobodyAnswers(t *testing.T) {
	addr := fakeTuner(t, func(int) [][]byte { return nil })
	start := time.Now()
	got, err := Query(context.Background(), 200*time.Millisecond, []string{addr})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if d := time.Since(start); d < 150*time.Millisecond || d > time.Second {
		t.Errorf("returned after %v, want about the 200ms wait", d)
	}
}

func TestQueryHonoursContext(t *testing.T) {
	addr := fakeTuner(t, func(int) [][]byte { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _ = Query(ctx, 5*time.Second, []string{addr})
	if d := time.Since(start); d > time.Second {
		t.Errorf("ignored context for %v", d)
	}
}

func TestQueryBadTarget(t *testing.T) {
	if _, err := Query(context.Background(), 50*time.Millisecond, []string{"not a host:port:x"}); err == nil {
		t.Error("expected an error for an unresolvable target")
	}
}

func TestNormalizeBase(t *testing.T) {
	cases := map[string]string{
		"http://192.168.68.143:80":    "http://192.168.68.143", // what a real FLEX 4K reports
		"http://192.168.68.143":       "http://192.168.68.143",
		"http://192.168.68.143/":      "http://192.168.68.143",
		"http://192.168.68.143:80/":   "http://192.168.68.143",
		"http://192.168.68.143:5004":  "http://192.168.68.143:5004", // a real port is kept
		"https://tuner.example:443":   "https://tuner.example",
		"https://tuner.example:8443":  "https://tuner.example:8443",
		"http://[fd00::1]:80":         "http://[fd00::1]",
		"http://tuner.example.ts.net": "http://tuner.example.ts.net",
		"":                            "",
		"not a url ::":                "not a url ::",
	}
	for in, want := range cases {
		if got := NormalizeBase(in); got != want {
			t.Errorf("NormalizeBase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDedupe(t *testing.T) {
	got := dedupe([]string{"a", "b", "a", "c", "b"})
	if strings.Join(got, ",") != "a,b,c" {
		t.Errorf("dedupe = %v", got)
	}
}

func TestDirectedBroadcastsAreIPv4HostPorts(t *testing.T) {
	for _, s := range directedBroadcasts() {
		host, port, err := net.SplitHostPort(s)
		if err != nil || port != Port || net.ParseIP(host).To4() == nil {
			t.Errorf("bad broadcast target %q", s)
		}
	}
}
