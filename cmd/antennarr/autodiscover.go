package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/johnschult/antennarr/internal/access"
	"github.com/johnschult/antennarr/internal/discovery"
)

const (
	discoverWait  = 3 * time.Second // how long to listen for answers in one try
	discoverRetry = 5 * time.Second // pause between tries when nothing answered
)

// discoverFunc looks for tuners on the network (discovery.Discover, or a stand-in
// in tests).
type discoverFunc func(ctx context.Context, wait time.Duration) ([]discovery.Device, error)

// wantsAutoDiscovery reports whether Antennarr should look for tuners itself:
// no devices are declared in the settings, and no flag names a tuner.
// -listen alone is fine (it says which port to use); the other device flags
// describe a tuner that hasn't been named, which is a mistake worth reporting.
func wantsAutoDiscovery(s access.Settings, set map[string]bool) bool {
	if len(s.Devices) > 0 {
		return false
	}
	for _, n := range flagNames {
		if n != "listen" && set[n] {
			return false
		}
	}
	return true
}

// findTuners looks for HDHomeRun tuners until at least one answers, and returns
// a device for each. It keeps looking, saying so in the log, until ctx ends: a
// tuner that isn't plugged in yet shouldn't stop Antennarr from starting
// correctly once it is.
//
// A single tuner is the device "tuner" on listen; several are "tuner-<id>", each
// on the next port up, since every device needs a port of its own.
func findTuners(ctx context.Context, listen string, discover discoverFunc, retry time.Duration, log *slog.Logger) ([]access.Device, error) {
	host, port, err := splitListen(listen)
	if err != nil {
		return nil, err
	}
	for attempt := 1; ; attempt++ {
		found, err := discover(ctx, discoverWait)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			log.Warn("looking for tuners failed", "err", err)
		}
		if len(found) > 0 {
			return devicesFromFound(found, host, port)
		}
		if attempt == 1 {
			log.Info("no tuner given, so looking for HDHomeRun tuners on the network. If yours is on another network, start with -tuner ADDRESS. In Docker, use host networking")
		} else if attempt%6 == 0 { // about every 30 seconds
			log.Info("still looking for an HDHomeRun tuner")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retry):
		}
	}
}

func splitListen(listen string) (host string, port int, err error) {
	h, p, err := net.SplitHostPort(listen)
	if err != nil {
		return "", 0, fmt.Errorf("-listen %q must look like \":5004\" or \"127.0.0.1:5004\"", listen)
	}
	n, err := strconv.Atoi(p)
	if err != nil || n < 1 || n > 65535 {
		return "", 0, fmt.Errorf("-listen %q has a bad port", listen)
	}
	return h, n, nil
}

func devicesFromFound(found []discovery.Device, host string, port int) ([]access.Device, error) {
	found = append([]discovery.Device(nil), found...)
	// A stable order, so the same tuners get the same names and ports each time.
	sort.Slice(found, func(i, j int) bool { return found[i].ID < found[j].ID })
	if port+len(found)-1 > 65535 {
		return nil, fmt.Errorf("-listen port %d leaves no room for %d tuners", port, len(found))
	}
	out := make([]access.Device, len(found))
	for i, d := range found {
		name := "tuner"
		if len(found) > 1 {
			name = "tuner-" + strings.ToLower(d.ID)
		}
		out[i] = access.Device{
			Name:   name,
			Listen: net.JoinHostPort(host, strconv.Itoa(port+i)),
			Tuner:  d.BaseURL,
			ID:     d.ID,
		}
		if err := out[i].Validate(); err != nil {
			return nil, fmt.Errorf("tuner %s: %w", d.ID, err)
		}
	}
	return out, nil
}
