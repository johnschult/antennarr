package api

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/johnschult/yagi/internal/discovery"
)

var notNameChars = regexp.MustCompile(`[^a-z0-9]+`)

// suggestName makes a device name from what the tuner calls itself and the end of its DeviceID: "HDHomeRun FLEX
// 4K" with ID 10ABCC87 is "flex-4k-87". The device name shows in addresses and logs, so it stays short and plain.
func suggestName(friendly, id string) string {
	base := strings.TrimPrefix(strings.ToLower(friendly), "hdhomerun")
	base = strings.Trim(notNameChars.ReplaceAllString(base, "-"), "-")
	if base == "" {
		base = "tuner"
	}
	if len(id) >= 2 {
		base += "-" + strings.ToLower(id[len(id)-2:])
	}
	return base
}

// nameFound asks each tuner who it is (over HTTP, a moment each, all at once) and fills in the label and name to
// offer. A tuner that doesn't answer keeps a plain name made from its DeviceID.
func (a *API) nameFound(ctx context.Context, found []Found) {
	client := &http.Client{Timeout: 2 * time.Second}
	var wg sync.WaitGroup
	for i := range found {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var friendly string
			if a.Describe != nil {
				friendly = a.Describe(ctx, found[i].Address)
			} else if d, err := discovery.Probe(ctx, client, found[i].Address); err == nil {
				friendly = d.FriendlyName
			}
			found[i].Label = friendly
			found[i].Name = suggestName(friendly, found[i].ID)
		}()
	}
	wg.Wait()
}
