package server

import (
	"testing"

	"github.com/johnschult/antennarr/internal/access"
	"github.com/johnschult/antennarr/internal/history"
	"github.com/johnschult/antennarr/internal/tuner"
)

func inUse(res, ch string, via bool, ip string) tuner.Slot {
	return tuner.Slot{Resource: res, InUse: true, Channel: ch, ChannelName: "N" + ch, ViaAntennarr: via, TargetIP: ip}
}

func TestObservationsPairViewersByChannelAndLeaveTheRestLocalOrRemote(t *testing.T) {
	state := tuner.State{Online: true, Slots: []tuner.Slot{
		{Resource: "tuner0"}, // idle
		inUse("tuner1", "2.1", true, "100.64.0.9"),
		inUse("tuner2", "5.1", true, "100.64.0.9"), // through us, but no stream says whose
		inUse("tuner3", "69.1", false, "192.168.68.115"),
	}}
	streams := []access.Active{
		{Device: "antenna", Viewer: "Alex", Channel: "2.1"},
		{Device: "other", Viewer: "Sam", Channel: "5.1"}, // another device's stream is not this one's
	}
	got := observe("antenna", state, streams, func(ch string) string { return "Show " + ch })
	if len(got) != 3 {
		t.Fatalf("%d observations: %+v", len(got), got)
	}
	want := []history.Observation{
		{Device: "antenna", Tuner: "tuner1", Channel: "2.1", ChannelName: "N2.1", Kind: history.KindViewer, Viewer: "Alex", Title: "Show 2.1"},
		{Device: "antenna", Tuner: "tuner2", Channel: "5.1", ChannelName: "N5.1", Kind: history.KindRemote, Title: "Show 5.1"},
		{Device: "antenna", Tuner: "tuner3", Channel: "69.1", ChannelName: "N69.1", Kind: history.KindLocal, Client: "192.168.68.115", Title: "Show 69.1"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestTwoViewersOnTheSameChannelAreEachPairedOnce(t *testing.T) {
	state := tuner.State{Online: true, Slots: []tuner.Slot{inUse("tuner0", "2.1", true, "x"), inUse("tuner1", "2.1", true, "x")}}
	streams := []access.Active{{Device: "antenna", Viewer: "Alex", Channel: "2.1"}, {Device: "antenna", Viewer: "Sam", Channel: "2.1"}}
	got := observe("antenna", state, streams, func(string) string { return "" })
	if len(got) != 2 || got[0].Viewer != "Alex" || got[1].Viewer != "Sam" {
		t.Fatalf("got %+v", got)
	}
}

func TestAnOfflineTunerHasNothingInUse(t *testing.T) {
	state := tuner.State{Online: false, Slots: []tuner.Slot{inUse("tuner0", "2.1", false, "x")}}
	if got := observe("antenna", state, nil, func(string) string { return "" }); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestTheHistoryFileIsBesideTheSettingsFileAndOnlyThenKept(t *testing.T) {
	if got := historyFile("/etc/antennarr/antennarr.json"); got != "/etc/antennarr/antennarr.history.jsonl" {
		t.Errorf("got %q", got)
	}
	if got := historyFile(""); got != "" {
		t.Errorf("with no settings file: %q", got)
	}
}
