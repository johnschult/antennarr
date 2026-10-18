package hdhr

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDialTriesAgainOnceAfterAFailedFirstAttempt(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close() // nothing is listening: the first attempt is refused
	go func() {
		time.Sleep(redial / 3)
		l2, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		defer l2.Close()
		c, _ := l2.Accept()
		if c != nil {
			time.Sleep(200 * time.Millisecond)
			c.Close()
		}
	}()
	c, err := Dial(context.Background(), addr)
	if err != nil {
		t.Fatalf("the retry should have found it: %v", err)
	}
	c.Close()
}

func TestDialGivesUpAfterTheRetry(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	start := time.Now()
	_, err := Dial(context.Background(), addr)
	if err == nil || !strings.Contains(err.Error(), "couldn't reach") {
		t.Fatalf("error = %v", err)
	}
	if d := time.Since(start); d < redial || d > 3*time.Second {
		t.Errorf("took %v: it should try twice, with a pause, and no more", d)
	}
}

func TestDialStopsWhenAskedAndSaysSo(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Dial(ctx, addr); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want the context's", err)
	}
}
