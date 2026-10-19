package hdhr_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/hdhr"
)

// A look at a real tuner, for checking the client against hardware. It only ever reads (Get): there is
// no Set and no Upgrade in it. Skipped unless HDHR_TUNER is set to the tuner's address:
//
//	HDHR_TUNER=192.168.1.50 go test -run LiveReadOnly -v ./internal/hdhr
//
// and, to compare with the official tool's answers too, HDHOMERUN_CONFIG=/path/to/hdhomerun_config.
func TestLiveReadOnly(t *testing.T) {
	host := os.Getenv("HDHR_TUNER")
	if host == "" {
		t.Skip("HDHR_TUNER is not set: no real tuner to look at")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Straight to the tuner's IP, with no discover first, as the official tool does one.
	c, err := hdhr.Dial(ctx, net.JoinHostPort(host, hdhr.ControlPort))
	if err != nil {
		t.Fatalf("a direct connection to the control port: %v", err)
	}
	defer c.Close()

	tool := os.Getenv("HDHOMERUN_CONFIG")
	for _, name := range []string{"/sys/version", "/sys/model", "/sys/hwmodel", "/sys/features", "/tuner0/status"} {
		ours, err := c.Get(ctx, name)
		if err != nil {
			t.Logf("%-16s ours: error: %v", name, err)
			continue
		}
		line := ours
		if tool != "" {
			out, _ := exec.Command(tool, host, "get", name).Output()
			theirs := strings.TrimSpace(string(out))
			if name != "/tuner0/status" && theirs != strings.TrimSpace(ours) { // the tool prints the value and a newline // a tuner's status changes from moment to moment
				t.Errorf("%s: the official tool says %q and we say %q", name, theirs, strings.TrimSpace(ours))
			}
			line += "   | official: " + strings.ReplaceAll(theirs, "\n", " ")
		}
		t.Logf("%-16s %s", name, strings.ReplaceAll(line, "\n", " "))
	}
}
