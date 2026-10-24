package diag

import (
	"testing"
	"time"
)

func TestOffByDefaultAndWhenNil(t *testing.T) {
	var none *Mode
	if none.Active() || !none.Until().IsZero() {
		t.Error("a nil mode is on")
	}
	if (&Mode{}).Active() {
		t.Error("a new mode is on")
	}
}

func TestEnableLastsThenStops(t *testing.T) {
	now := time.Now()
	var changes []bool
	m := &Mode{Now: func() time.Time { return now }, OnChange: func(on bool) { changes = append(changes, on) }}
	m.Enable(0)
	if !m.Active() || !m.Until().Equal(now.Add(DefaultFor)) {
		t.Errorf("active=%v until=%v", m.Active(), m.Until())
	}
	now = now.Add(DefaultFor - time.Second)
	if !m.Active() {
		t.Error("stopped early")
	}
	now = now.Add(2 * time.Second)
	if m.Active() || !m.Until().IsZero() {
		t.Error("did not stop")
	}
	if len(changes) != 1 || !changes[0] {
		t.Errorf("changes = %v", changes)
	}
}

func TestEnableIsCappedAndRestartsTheTime(t *testing.T) {
	now := time.Now()
	m := &Mode{Now: func() time.Time { return now }}
	m.Enable(1000 * time.Hour)
	if !m.Until().Equal(now.Add(MaxFor)) {
		t.Errorf("until = %v", m.Until())
	}
	now = now.Add(time.Hour)
	m.Enable(time.Minute)
	if !m.Until().Equal(now.Add(time.Minute)) {
		t.Errorf("until = %v", m.Until())
	}
	m.Disable()
}

func TestDisableSaysSoOnlyWhenItWasOn(t *testing.T) {
	var changes []bool
	m := &Mode{OnChange: func(on bool) { changes = append(changes, on) }}
	m.Disable()
	m.Enable(time.Hour)
	m.Disable()
	m.Disable()
	if len(changes) != 2 || !changes[0] || changes[1] {
		t.Errorf("changes = %v", changes)
	}
}

func TestRunningOutIsAnnounced(t *testing.T) {
	done := make(chan bool, 2)
	m := &Mode{OnChange: func(on bool) { done <- on }}
	m.Enable(20 * time.Millisecond)
	<-done // on
	select {
	case on := <-done:
		if on {
			t.Error("announced on")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("running out was never announced")
	}
	if m.Active() {
		t.Error("still on")
	}
}
