package hdhr

import (
	"reflect"
	"testing"
)

func TestParseFeatures(t *testing.T) {
	got := ParseFeatures("channelmap: us-bcast us-cable kr-bcast\nmodulation: atsc3 8vsb qam256\nauto-modulation: auto auto6t\nsomething: new\n\nnot a line\n")
	want := Features{ChannelMaps: []string{"us-bcast", "us-cable", "kr-bcast"}, Modulations: []string{"atsc3", "8vsb", "qam256"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if got := ParseFeatures(""); len(got.ChannelMaps) != 0 || len(got.Modulations) != 0 {
		t.Fatalf("empty features gave %+v", got)
	}
}
