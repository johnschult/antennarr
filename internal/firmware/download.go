package firmware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// DownloadBase is where SiliconDust serves firmware images. The file for a tuner is
// <family>_firmware_<release>.bin in it, for example hdhomerun_dvr_atsc3_firmware_20260326.bin. That
// naming is what their site does today; it isn't documented, so a file that isn't there is reported as
// that and nothing else is tried.
const DownloadBase = "https://download.silicondust.com/hdhomerun"

// Sizes an image can be. They are real images' sizes with room either side (the FLEX 4K's is 2.2 MB): a
// reply far smaller is an error page and far larger is not firmware, so neither is sent to a tuner.
const (
	MinImageSize = 256 << 10
	MaxImageSize = 16 << 20
)

// familyRe is what a firmware family name looks like (hdhomerun_dvr_atsc3). A release is versionRe: eight digits.
var familyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{2,63}$`)

// Image is a downloaded firmware file.
type Image struct {
	Family  string // e.g. hdhomerun_dvr_atsc3
	Release string // e.g. 20260326
	Data    []byte
	SHA256  string // of Data, in hex, to log and show: SiliconDust publishes no checksum to compare with
}

// Downloader fetches firmware images.
type Downloader struct {
	// Base is where images are; empty means DownloadBase. Tests point it at their own server.
	Base string
	// Client fetches them; nil means one with a five minute limit that does not follow redirects, since a
	// redirect is somewhere the image was not meant to come from.
	Client *http.Client
}

func (d Downloader) client() *http.Client {
	if d.Client != nil {
		return d.Client
	}
	return &http.Client{
		Timeout:       5 * time.Minute,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// ErrNotFound is returned when SiliconDust has no such file.
var ErrNotFound = errors.New("SiliconDust has no firmware file under that name")

// URL is where the image for a family and release is. Both are checked first: they come from the tuner and
// the changelog, but they end up in an address, so only what they should look like is let through.
func (d Downloader) URL(family, release string) (string, error) {
	if !familyRe.MatchString(family) {
		return "", fmt.Errorf("%q is not a firmware family name", family)
	}
	if !versionRe.MatchString(release) {
		return "", fmt.Errorf("%q is not a firmware release", release)
	}
	base := d.Base
	if base == "" {
		base = DownloadBase
	}
	return strings.TrimRight(base, "/") + "/" + family + "_firmware_" + release + ".bin", nil
}

// Fetch downloads the image. Nothing is returned unless it came whole: a 200, the type of a binary file,
// a size within bounds, and as many bytes as the server said it would send.
func (d Downloader) Fetch(ctx context.Context, family, release string) (Image, error) {
	target, err := d.URL(family, release)
	if err != nil {
		return Image{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Image{}, err
	}
	req.Header.Set("User-Agent", "Antennarr (firmware install)")
	resp, err := d.client().Do(req)
	if err != nil {
		// Go's error would repeat the address; say what failed without it.
		if ctx.Err() != nil {
			return Image{}, ctx.Err()
		}
		return Image{}, errors.New("couldn't download the firmware from SiliconDust: " + shortErr(err))
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Image{}, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return Image{}, fmt.Errorf("SiliconDust answered %s for the firmware file", resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(strings.ToLower(ct), "application/octet-stream") {
		return Image{}, fmt.Errorf("what SiliconDust sent is not a firmware file (it says %q)", ct)
	}
	if n := resp.ContentLength; n >= 0 && (n < MinImageSize || n > MaxImageSize) {
		return Image{}, fmt.Errorf("the firmware file is %d bytes, which is not a size a firmware image can be", n)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxImageSize+1))
	if err != nil {
		if ctx.Err() != nil {
			return Image{}, ctx.Err()
		}
		return Image{}, errors.New("the firmware download was cut short: " + shortErr(err))
	}
	if len(data) < MinImageSize || len(data) > MaxImageSize {
		return Image{}, fmt.Errorf("the firmware file is %d bytes, which is not a size a firmware image can be", len(data))
	}
	if resp.ContentLength >= 0 && int64(len(data)) != resp.ContentLength {
		return Image{}, errors.New("the firmware download was cut short")
	}
	sum := sha256.Sum256(data)
	return Image{Family: family, Release: release, Data: data, SHA256: hex.EncodeToString(sum[:])}, nil
}

// shortErr is an error's reason without the address Go puts in front of it.
func shortErr(err error) string {
	var ue interface{ Unwrap() error }
	for errors.As(err, &ue) && ue.Unwrap() != nil {
		err = ue.Unwrap()
	}
	return err.Error()
}

// Installable says whether Antennarr will install a release on a tuner, and which. Only the newest
// release the changelog says certainly applies, on a tuner whose own firmware family is known, and only
// forwards: there is no downgrade, and nothing that isn't SiliconDust's own file for that tuner.
func Installable(s Status, family string) (release string, ok bool, why string) {
	switch {
	case s.State != StateUpdateAvailable:
		return "", false, "there is no newer firmware for this tuner"
	case !familyRe.MatchString(family):
		return "", false, "the tuner has not said which firmware it runs"
	case !versionRe.MatchString(s.Latest) || !versionRe.MatchString(s.Current):
		return "", false, "the firmware versions are not readable"
	case s.Latest <= s.Current: // dates, so text order is date order
		return "", false, "that is not newer than what the tuner runs"
	}
	return s.Latest, true, ""
}
