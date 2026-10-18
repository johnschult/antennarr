package tunerinfo

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/johnschult/antennarr/internal/hdhr"
	"github.com/johnschult/antennarr/internal/hdhr/hdhrtest"
)

func page(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/system.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// rig is a fake tuner's control port and System Menu page, and a Reader pointed at both.
func rig(t *testing.T, body string, status int) (*Reader, *httptest.Server, *hdhrtest.Server) {
	t.Helper()
	ctrl, err := hdhrtest.New("127.0.0.1:0", hdhrtest.Options{Version: "20260326"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctrl.Close)
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/system.html" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(web.Close)
	r := &Reader{Dial: func(ctx context.Context, host string) (*hdhr.Conn, error) { return hdhr.Dial(ctx, ctrl.Addr()) }}
	return r, web, ctrl
}

func TestReadsControlAndPage(t *testing.T) {
	r, web, _ := rig(t, page(t), 200)
	info, err := r.Read(context.Background(), "a", web.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	if info.Model != "hdhomerun_dvr_atsc3" || info.HardwareModel != "HDFX-4K" || info.Version != "20260326" {
		t.Errorf("control values wrong: %+v", info)
	}
	if info.DRM != "SD20230420" || info.MAC != "00:18:DD:00:00:01" || info.IP != "192.168.1.50" || info.Subnet != "255.255.255.0" || info.DeviceID != "10ABCC87" {
		t.Errorf("page values wrong: %+v", info)
	}
	if len(info.ChannelMaps) == 0 || info.Modulations[0] != "atsc3" {
		t.Errorf("features wrong: %+v", info)
	}
	if len(info.Problems) != 0 {
		t.Errorf("problems on a good read: %v", info.Problems)
	}
}

func TestOnlyReadsTheListedNames(t *testing.T) {
	r, web, ctrl := rig(t, page(t), 200)
	if _, err := r.Read(context.Background(), "a", web.URL, false); err != nil {
		t.Fatal(err)
	}
	for _, f := range ctrl.Frames() {
		if f.Type != hdhr.TypeGetSetReq {
			t.Fatalf("sent a frame of type %#x", f.Type)
		}
		tlvs, _ := hdhr.ParseTLVs(f.Payload)
		for _, tl := range tlvs {
			if tl.Tag == hdhr.TagGetSetValue {
				t.Fatalf("a read carried a value to set")
			}
			if tl.Tag == hdhr.TagGetSetName {
				switch strings.TrimRight(string(tl.Value), "\x00") {
				case "/sys/model", "/sys/hwmodel", "/sys/version", "/sys/features":
				default:
					t.Errorf("asked the tuner for %q", tl.Value)
				}
			}
		}
	}
}

func TestPageGoneLeavesControlValuesAndSaysWhy(t *testing.T) {
	r, web, _ := rig(t, "nope", 404)
	info, err := r.Read(context.Background(), "a", web.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	if info.HardwareModel != "HDFX-4K" || info.DRM != "" || info.MAC != "" {
		t.Errorf("got %+v", info)
	}
	if len(info.Problems) != 1 || !strings.Contains(info.Problems[0], "System Menu") {
		t.Errorf("problems = %v", info.Problems)
	}
}

func TestChangedPageLayoutIsSaidNotGuessed(t *testing.T) {
	r, web, _ := rig(t, "<html><body><p>Hardware Model: HDFX-4K</p></body></html>", 200)
	info, _ := r.Read(context.Background(), "a", web.URL, false)
	if info.MAC != "" || info.DRM != "" {
		t.Errorf("guessed from a page that isn't a table: %+v", info)
	}
	if len(info.Problems) != 1 || !strings.Contains(info.Problems[0], "isn't laid out as expected") {
		t.Errorf("problems = %v", info.Problems)
	}
}

func TestNothingReadableIsAnError(t *testing.T) {
	r, web, _ := rig(t, "nope", 500)
	r.Dial = func(context.Context, string) (*hdhr.Conn, error) { return nil, net.ErrClosed }
	if _, err := r.Read(context.Background(), "a", web.URL, false); err == nil {
		t.Fatal("expected an error when neither side answers")
	}
}

func TestRemembersBrieflyAndRefreshReads(t *testing.T) {
	r, web, ctrl := rig(t, page(t), 200)
	now := time.Unix(1000, 0)
	r.Now = func() time.Time { return now }
	read := func(refresh bool) {
		if _, err := r.Read(context.Background(), "a", web.URL, refresh); err != nil {
			t.Fatal(err)
		}
	}
	read(false)
	n := len(ctrl.Frames())
	read(false)
	if len(ctrl.Frames()) != n {
		t.Fatal("asked the tuner again straight away")
	}
	read(true)
	if len(ctrl.Frames()) == n {
		t.Fatal("refresh did not ask again")
	}
	n = len(ctrl.Frames())
	now = now.Add(cacheFor + time.Second)
	read(false)
	if len(ctrl.Frames()) == n {
		t.Fatal("an old answer was reused")
	}
}

func TestPageValuesAreUnescapedAndBounded(t *testing.T) {
	got := ParsePage(`<table><tr><td>Hardware Model</td><td>A &amp; <b>B</b></td></tr><tr><td>MAC Address</td><td>` + strings.Repeat("x", 500) + `</td></tr></table>`)
	if got["Hardware Model"] != "A & B" {
		t.Errorf("got %q", got["Hardware Model"])
	}
	if _, ok := got["MAC Address"]; ok {
		t.Error("kept a 500 character value")
	}
}

func TestBadAddressIsRefused(t *testing.T) {
	r := &Reader{Dial: func(context.Context, string) (*hdhr.Conn, error) { t.Fatal("dialled"); return nil, nil }}
	if _, err := r.Read(context.Background(), "a", "", false); err == nil {
		t.Fatal("expected an error")
	}
}

func TestIPFromPageMustBeAnIP(t *testing.T) {
	r, web, _ := rig(t, strings.Replace(page(t), "192.168.1.50", "<script>x</script>evil", 1), 200)
	info, _ := r.Read(context.Background(), "a", web.URL, false)
	if info.IP != "" {
		t.Errorf("ip = %q", info.IP)
	}
}
