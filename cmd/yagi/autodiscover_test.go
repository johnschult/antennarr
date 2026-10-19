package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/access"
	"github.com/johnschult/yagi/internal/discovery"
)

func TestWhenToLookForTuners(t *testing.T) {
	withDevice := access.DefaultSettings()
	withDevice.Devices = []access.Device{{Name: "antenna", Listen: ":5004", Tuner: "192.168.1.50"}}

	for name, c := range map[string]struct {
		s    access.Settings
		set  map[string]bool
		want bool
	}{
		"a clean start":                {access.DefaultSettings(), nil, true},
		"-listen alone picks the port": {access.DefaultSettings(), map[string]bool{"listen": true}, true},
		"-config alone":                {access.DefaultSettings(), map[string]bool{"config": true}, true},
		"a tuner address was given":    {access.DefaultSettings(), map[string]bool{"tuner": true}, false},
		"a DeviceID was given":         {access.DefaultSettings(), map[string]bool{"tuner-id": true}, false},
		"-advertise without a tuner":   {access.DefaultSettings(), map[string]bool{"advertise": true}, false},
		"-stream-port without a tuner": {access.DefaultSettings(), map[string]bool{"stream-port": true}, false},
		"devices are in the settings":  {withDevice, nil, false},
	} {
		if got := wantsAutoDiscovery(c.s, c.set); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}

func fakeDiscover(results ...[]discovery.Device) (discoverFunc, *atomic.Int32) {
	var calls atomic.Int32
	return func(ctx context.Context, wait time.Duration) ([]discovery.Device, error) {
		n := int(calls.Add(1))
		if n > len(results) {
			n = len(results)
		}
		return results[n-1], nil
	}, &calls
}

func TestOneTunerBecomesTheDeviceCalledTuner(t *testing.T) {
	disc, _ := fakeDiscover([]discovery.Device{{ID: "10ABCC87", BaseURL: "http://192.168.1.132", TunerCount: 4}})
	devs, err := findTuners(context.Background(), ":5004", disc, time.Millisecond, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 {
		t.Fatalf("%d devices", len(devs))
	}
	d := devs[0]
	if d.Name != "tuner" || d.Listen != ":5004" || d.Tuner != "http://192.168.1.132" || d.ID != "10ABCC87" {
		t.Errorf("%+v", d)
	}
	if err := d.Validate(); err != nil {
		t.Errorf("the device it made is not valid: %v", err)
	}
}

// Several tuners each get a name from their DeviceID and a port of their own,
// the same ones every time whatever order they answered in.
func TestSeveralTunersGetTheirOwnNamesAndPorts(t *testing.T) {
	a := discovery.Device{ID: "1111AAA2", BaseURL: "http://192.168.1.11"}
	b := discovery.Device{ID: "10ABCC87", BaseURL: "http://192.168.1.10"}
	for _, order := range [][]discovery.Device{{a, b}, {b, a}} {
		disc, _ := fakeDiscover(order)
		devs, err := findTuners(context.Background(), "127.0.0.1:6000", disc, time.Millisecond, quietLog())
		if err != nil {
			t.Fatal(err)
		}
		got := []string{devs[0].Name + "@" + devs[0].Listen, devs[1].Name + "@" + devs[1].Listen}
		want := []string{"tuner-10abcc87@127.0.0.1:6000", "tuner-1111aaa2@127.0.0.1:6001"}
		if got[0] != want[0] || got[1] != want[1] {
			t.Errorf("%v, want %v", got, want)
		}
		if err := (access.Settings{Devices: devs}).Validate(); err != nil {
			t.Errorf("the settings it makes are not valid: %v", err)
		}
	}
}

// A tuner that isn't there yet is waited for, not a reason to stop.
func TestKeepsLookingUntilATunerAnswers(t *testing.T) {
	disc, calls := fakeDiscover(nil, nil, []discovery.Device{{ID: "10ABCC87", BaseURL: "http://192.168.1.132"}})
	devs, err := findTuners(context.Background(), ":5004", disc, time.Millisecond, quietLog())
	if err != nil || len(devs) != 1 {
		t.Fatalf("%v %v", devs, err)
	}
	if calls.Load() != 3 {
		t.Errorf("looked %d times, want 3", calls.Load())
	}
}

func TestAnErrorFromTheLookupIsRetried(t *testing.T) {
	var calls atomic.Int32
	disc := func(ctx context.Context, wait time.Duration) ([]discovery.Device, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("network unreachable")
		}
		return []discovery.Device{{ID: "10ABCC87", BaseURL: "http://192.168.1.132"}}, nil
	}
	devs, err := findTuners(context.Background(), ":5004", disc, time.Millisecond, quietLog())
	if err != nil || len(devs) != 1 {
		t.Fatalf("%v %v", devs, err)
	}
}

func TestStopsLookingWhenAskedTo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	disc := func(ctx context.Context, wait time.Duration) ([]discovery.Device, error) {
		cancel() // asked to stop during the first look
		return nil, nil
	}
	done := make(chan error, 1)
	go func() {
		_, err := findTuners(ctx, ":5004", disc, time.Hour, quietLog())
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("still looking after being asked to stop")
	}
}

func TestBadListenIsReportedBeforeLooking(t *testing.T) {
	disc, calls := fakeDiscover(nil)
	for _, listen := range []string{"5004", ":0", ":99999", ":abc", ""} {
		if _, err := findTuners(context.Background(), listen, disc, time.Millisecond, quietLog()); err == nil || !strings.Contains(err.Error(), "-listen") {
			t.Errorf("%q: err = %v", listen, err)
		}
	}
	if calls.Load() != 0 {
		t.Error("looked for tuners with a listen address that can't work")
	}
}

func TestNoRoomForEveryTunerAbovePort(t *testing.T) {
	disc, _ := fakeDiscover([]discovery.Device{{ID: "10ABCC87", BaseURL: "http://a"}, {ID: "10ABCC88", BaseURL: "http://b"}})
	if _, err := findTuners(context.Background(), ":65535", disc, time.Millisecond, quietLog()); err == nil {
		t.Error("two tuners at port 65535 should not fit")
	}
}

// What was found is what gets saved, and a settings file that already has
// devices is not looked into again.
func TestFoundTunersSurviveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "yagi.json")
	s, save, err := loadOrCreateSettings(path, false, quietLog())
	if err != nil || save != path {
		t.Fatal(save, err)
	}
	disc, _ := fakeDiscover([]discovery.Device{{ID: "10ABCC87", BaseURL: "http://192.168.1.132"}})
	devs, err := findTuners(context.Background(), ":5004", disc, time.Millisecond, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	s.Devices = devs
	if err := access.SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}

	again, _, err := access.LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if wantsAutoDiscovery(again, nil) {
		t.Error("looked for tuners again though the settings file names one")
	}
	got, err := devicesFor(again, defaults, nil)
	if err != nil || len(got) != 1 || got[0].ID != "10ABCC87" || got[0].Name != "tuner" {
		t.Errorf("%+v, %v", got, err)
	}
}
