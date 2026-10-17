package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// Probe asks the HDHomeRun HTTP API at baseURL who it is. It's also how we
// verify that a remembered address still belongs to the tuner we expect.
func Probe(ctx context.Context, client *http.Client, baseURL string) (Device, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/discover.json", nil)
	if err != nil {
		return Device{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Device{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Device{}, fmt.Errorf("%s: %s", baseURL, resp.Status)
	}
	var d struct {
		DeviceID   string
		TunerCount int
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return Device{}, fmt.Errorf("%s: not an HDHomeRun: %w", baseURL, err)
	}
	if d.DeviceID == "" {
		return Device{}, fmt.Errorf("%s: no DeviceID in discover.json", baseURL)
	}
	return Device{ID: strings.ToUpper(d.DeviceID), BaseURL: NormalizeBase(baseURL), TunerCount: d.TunerCount}, nil
}

// ScanHosts probes every host (host or host:port) over HTTP, 64 at a time, and
// returns the ones that answer like an HDHomeRun. If wantID is set it stops at
// the first match.
func ScanHosts(ctx context.Context, hosts []string, wantID string) []Device {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	client := &http.Client{Timeout: 800 * time.Millisecond}
	var (
		mu    sync.Mutex
		found []Device
		wg    sync.WaitGroup
		sem   = make(chan struct{}, 64)
	)
	for _, h := range hosts {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(h string) {
			defer wg.Done()
			defer func() { <-sem }()
			d, err := Probe(ctx, client, "http://"+h)
			if err != nil {
				return
			}
			mu.Lock()
			found = append(found, d)
			mu.Unlock()
			if wantID != "" && d.ID == strings.ToUpper(wantID) {
				cancel()
			}
		}(h)
	}
	wg.Wait()
	return found
}

// Scan probes every address in prefix on port 80.
func Scan(ctx context.Context, prefix netip.Prefix, wantID string) []Device {
	return ScanHosts(ctx, hostsIn(prefix), wantID)
}

// PrefixAround returns the /24 containing ip.
func PrefixAround(ip net.IP) (netip.Prefix, bool) {
	a, ok := netip.AddrFromSlice(ip.To4())
	if !ok {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(a, 24).Masked(), true
}

// hostsIn lists usable host addresses in prefix (not network/broadcast),
// capped at a /16's worth so a typo can't scan the world.
func hostsIn(prefix netip.Prefix) []string {
	const max = 65534
	var out []string
	for a := prefix.Masked().Addr().Next(); prefix.Contains(a) && len(out) < max; a = a.Next() {
		if next := a.Next(); !prefix.Contains(next) { // broadcast address
			break
		}
		out = append(out, a.String())
	}
	return out
}
