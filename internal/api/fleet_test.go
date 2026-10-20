package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/johnschult/yagi/internal/access"
	"github.com/johnschult/yagi/internal/discovery"
)

// fakeFleet records what the API asks of the fleet.
type fakeFleet struct {
	mu      sync.Mutex
	devs    []Device
	added   []access.Device
	removed []string
	addErr  error
	moved   map[string]string
	moveErr error
}

func (f *fakeFleet) Devices() []Device { f.mu.Lock(); defer f.mu.Unlock(); return f.devs }
func (f *fakeFleet) AddDevice(_ context.Context, d access.Device) (access.Device, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added = append(f.added, d)
	if f.addErr != nil {
		return d, f.addErr
	}
	return d, nil
}
func (f *fakeFleet) RemoveDevice(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, name)
	return nil
}

func (f *fakeFleet) SetListen(name, listen string) (access.Device, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.moveErr != nil {
		return access.Device{}, f.moveErr
	}
	if f.moved == nil {
		f.moved = map[string]string{}
	}
	f.moved[name] = listen
	return access.Device{Name: name, Listen: listen}, nil
}

func withFleet(t *testing.T, e *env) *fakeFleet {
	t.Helper()
	f := &fakeFleet{devs: e.api.devices()}
	e.api.Fleet = f
	return f
}

func TestAddingATunerPassesWhatWasGiven(t *testing.T) {
	e := newEnv(t, access.DefaultSettings(), true)
	f := withFleet(t, e)
	// The fake adds nothing to its list, so show the existing device back.
	code, v := e.call(t, "POST", "/api/devices", `{"tuner":" 192.168.1.50 ","id":"10abcc87","name":"","listen":""}`)
	if code != http.StatusInternalServerError { // the fake doesn't list the new tuner, so it can't be shown
		t.Fatalf("code = %d %v", code, v)
	}
	if len(f.added) != 1 || f.added[0].Tuner != "192.168.1.50" || f.added[0].ID != "10ABCC87" {
		t.Errorf("fleet was asked to add %+v, want the address trimmed and the ID upper-cased", f.added)
	}
}

func TestAddingATunerReportsRefusals(t *testing.T) {
	e := newEnv(t, access.DefaultSettings(), true)
	f := withFleet(t, e)
	f.addErr = NewError(http.StatusConflict, "that tuner is already added")
	code, v := e.call(t, "POST", "/api/devices", `{"tuner":"x"}`)
	if code != http.StatusConflict || errText(v) != "that tuner is already added" {
		t.Errorf("= %d %v, want the fleet's own refusal", code, v)
	}
	if code, _ := e.call(t, "POST", "/api/devices", `{"tuner":"x","bogus":1}`); code != http.StatusBadRequest {
		t.Errorf("unknown field = %d, want 400", code)
	}
	if code, _ := e.call(t, "PUT", "/api/devices", `{}`); code != http.StatusMethodNotAllowed {
		t.Errorf("PUT = %d, want 405", code)
	}
}

func TestRemovingATuner(t *testing.T) {
	e := newEnv(t, access.DefaultSettings(), true)
	f := withFleet(t, e)
	if code, _ := e.call(t, "DELETE", "/api/devices/antenna", ""); code != http.StatusNoContent {
		t.Fatalf("= %d, want 204", code)
	}
	if len(f.removed) != 1 || f.removed[0] != "antenna" {
		t.Errorf("removed %v", f.removed)
	}
	if code, _ := e.call(t, "DELETE", "/api/devices/nothing", ""); code != http.StatusNotFound {
		t.Errorf("unknown tuner = %d, want 404", code)
	}
}

func TestAddingAndRemovingNeedAFleet(t *testing.T) {
	e := newEnv(t, access.DefaultSettings(), true) // a fixed set of tuners
	if code, _ := e.call(t, "POST", "/api/devices", `{"tuner":"x"}`); code != http.StatusConflict {
		t.Errorf("add = %d, want 409", code)
	}
	if code, _ := e.call(t, "DELETE", "/api/devices/antenna", ""); code != http.StatusConflict {
		t.Errorf("remove = %d, want 409", code)
	}
}

func TestDiscoverListsTunersAndMarksTheOnesAlreadyAdded(t *testing.T) {
	e := newEnv(t, access.DefaultSettings(), true)
	e.api.Discover = func(context.Context, time.Duration) ([]discovery.Device, error) {
		return []discovery.Device{
			{ID: "10ABCC87", BaseURL: "http://192.168.1.50", TunerCount: 4},
			{ID: "10AAAA01", BaseURL: "http://192.168.1.51", TunerCount: 2},
		}, nil
	}
	code, v := e.call(t, "POST", "/api/discover", "")
	if code != http.StatusOK {
		t.Fatalf("= %d %v", code, v)
	}
	list := obj(t, v)["tuners"].([]any)
	if len(list) != 2 {
		t.Fatalf("tuners = %v", list)
	}
	first, second := obj(t, list[0]), obj(t, list[1])
	if first["id"] != "10AAAA01" || first["added"] != "" || first["tuners"] != float64(2) {
		t.Errorf("first = %v", first)
	}
	if second["id"] != "10ABCC87" || second["added"] != "antenna" || second["address"] != "http://192.168.1.50" {
		t.Errorf("second = %v, want the tuner Yagi already fronts marked with its name", second)
	}
}

func TestDiscoverFailureAndOneSearchAtATime(t *testing.T) {
	e := newEnv(t, access.DefaultSettings(), true)
	e.api.Discover = func(context.Context, time.Duration) ([]discovery.Device, error) {
		return nil, errors.New("no route")
	}
	if code, _ := e.call(t, "POST", "/api/discover", ""); code != http.StatusBadGateway {
		t.Errorf("failed search = %d, want 502", code)
	}

	release := make(chan struct{})
	started := make(chan struct{})
	e.api.Discover = func(context.Context, time.Duration) ([]discovery.Device, error) {
		close(started)
		<-release
		return nil, nil
	}
	done := make(chan int)
	go func() { c, _ := e.call(t, "POST", "/api/discover", ""); done <- c }()
	<-started
	if code, _ := e.call(t, "POST", "/api/discover", ""); code != http.StatusTooManyRequests {
		t.Errorf("second search while one runs = %d, want 429", code)
	}
	close(release)
	if c := <-done; c != http.StatusOK {
		t.Errorf("first search = %d", c)
	}
}

func TestChangingATunersPortGoesToTheFleet(t *testing.T) {
	e := newEnv(t, access.DefaultSettings(), true)
	f := withFleet(t, e)
	code, v := e.call(t, "PATCH", "/api/devices/antenna", `{"listen":":5010","ownerReserve":1}`)
	if code != http.StatusOK {
		t.Fatalf("= %d %v", code, v)
	}
	if f.moved["antenna"] != ":5010" {
		t.Errorf("fleet was asked to move %v, want antenna to :5010", f.moved)
	}

	f.moveErr = NewError(http.StatusConflict, "can't listen on :80: in use")
	code, v = e.call(t, "PATCH", "/api/devices/antenna", `{"listen":":80","ownerReserve":0}`)
	if code != http.StatusConflict || errText(v) != "can't listen on :80: in use" {
		t.Errorf("= %d %v, want the fleet's refusal", code, v)
	}
	if res, _ := e.api.Pol.Limits("antenna"); res != 1 {
		t.Errorf("owner reserve = %d: other fields in a refused request must not be applied", res)
	}
}

func TestChangingThePortNeedsAFleet(t *testing.T) {
	e := newEnv(t, access.DefaultSettings(), true)
	if code, _ := e.call(t, "PATCH", "/api/devices/antenna", `{"listen":":5010"}`); code != http.StatusConflict {
		t.Errorf("without a fleet = %d, want 409", code)
	}
}
