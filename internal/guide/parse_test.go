package guide

import (
	"os"
	"strings"
	"testing"
	"time"
)

var at = time.Date(2026, 10, 17, 17, 42, 0, 0, time.UTC)

func load(t *testing.T) map[string]*Channel {
	t.Helper()
	f, err := os.Open("testdata/xmltv.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	chans, err := Parse(f, at)
	if err != nil {
		t.Fatal(err)
	}
	return chans
}

func TestParseKeysChannelsByNumber(t *testing.T) {
	chans := load(t)
	if len(chans) != 2 {
		t.Fatalf("got %d channels, want 2 (the one without a number is skipped): %v", len(chans), chans)
	}
	nbc := chans["11.1"]
	if nbc == nil || nbc.Name != "WXIADT" || nbc.Icon != "https://img.hdhomerun.com/channels/US19587.png" {
		t.Fatalf("11.1 = %+v", nbc)
	}
}

func TestParseKeepsOnlyWhatHasNotFinished(t *testing.T) {
	nbc := load(t)["11.1"]
	var titles []string
	for _, p := range nbc.Programs {
		titles = append(titles, p.Title)
	}
	// "Already Over" ended, "Bad Time" can't be read, "Ghost" is for no known channel.
	if got := strings.Join(titles, "|"); got != "Rosie's Rules|Evening News" {
		t.Errorf("programs = %s", got)
	}
	p := nbc.Programs[0]
	if p.SubTitle != "The Catrina Mystery" || !p.Start.Equal(time.Date(2026, 10, 17, 17, 30, 0, 0, time.UTC)) {
		t.Errorf("first = %+v", p)
	}
}

func TestNow(t *testing.T) {
	nbc := load(t)["11.1"]
	if p := nbc.Now(at); p == nil || p.Title != "Rosie's Rules" {
		t.Fatalf("now = %v", p)
	}
	// Exactly at the boundary the new show has started.
	if p := nbc.Now(time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)); p == nil || p.Title != "Evening News" {
		t.Errorf("at 18:00: %v", p)
	}
	// In a gap, or past the end of the guide: nothing.
	if p := nbc.Now(time.Date(2026, 10, 17, 17, 0, 0, 0, time.UTC)); p != nil {
		t.Errorf("before the guide starts: %v", p)
	}
	if p := nbc.Now(time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)); p != nil {
		t.Errorf("after the guide ends: %v", p)
	}
}

func TestParseRejectsWhatIsNotAGuide(t *testing.T) {
	for name, doc := range map[string]string{
		"not xml":       "<html><body>nope",
		"wrong root":    `<?xml version="1.0"?><rss></rss>`,
		"no channels":   `<tv></tv>`,
		"cut off":       `<tv><channel id="a"><lcn>1.1</lcn>`,
		"empty":         ``,
		"bad programme": `<tv><channel id="a"><lcn>1.1</lcn></channel><programme start="x"`,
	} {
		if _, err := Parse(strings.NewReader(doc), at); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
