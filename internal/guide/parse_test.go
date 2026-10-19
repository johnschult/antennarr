package guide

import (
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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

func TestAProgramCarriesItsDescriptionCategoryEpisodeAndPicture(t *testing.T) {
	f, _ := os.Open("testdata/xmltv.xml")
	defer f.Close()
	chans, err := Parse(f, at)
	if err != nil {
		t.Fatal(err)
	}
	var rosie Program
	for _, p := range chans["11.1"].Programs {
		if p.Title == "Rosie's Rules" {
			rosie = p
		}
	}
	if rosie.Description != "For the Day of the Dead celebration." || rosie.Category != "Series" || rosie.Episode != "S01E32" {
		t.Errorf("rosie = %+v", rosie)
	}
	if rosie.Poster != "C22744932ENOGRU" || rosie.icon != "https://img.hdhomerun.com/titles/C22744932ENOGRU.jpg" {
		t.Errorf("picture = %q %q", rosie.Poster, rosie.icon)
	}
	for _, p := range chans["11.1"].Programs {
		if p.Title == "Evening News" && (p.Poster != "" || p.Description != "" || p.Category != "") {
			t.Errorf("a program with nothing extra has %+v", p)
		}
	}
}

func parseOne(t *testing.T, programme string) Program {
	t.Helper()
	doc := `<tv><channel id="a"><lcn>1.1</lcn></channel><programme start="20261017160000 +0000" stop="20261017190000 +0000" channel="a"><title>T</title>` + programme + `</programme></tv>`
	chans, err := Parse(strings.NewReader(doc), at)
	if err != nil {
		t.Fatal(err)
	}
	return chans["1.1"].Programs[0]
}

func TestTheWidePictureIsPreferredAndAnOddNameIsNotUsed(t *testing.T) {
	p := parseOne(t, `<icon src="https://img.hdhomerun.com/posters/TALL1.jpg" width="240" height="360"/><icon src="https://img.hdhomerun.com/titles/WIDE1.jpg" width="360" height="270"/>`)
	if p.Poster != "WIDE1" {
		t.Errorf("chose %q", p.Poster)
	}
	if p := parseOne(t, `<icon src="https://img.hdhomerun.com/posters/TALL1.jpg" width="240" height="360"/>`); p.Poster != "TALL1" {
		t.Errorf("a lone tall picture: %q", p.Poster)
	}
	for _, name := range []string{"a b", "%2e%2e", strings.Repeat("a", 65)} {
		if p := parseOne(t, `<icon src="https://img.hdhomerun.com/titles/`+name+`.jpg"/>`); p.Poster != "" {
			t.Errorf("%q gave a token %q", name, p.Poster)
		}
	}
}

func TestOnlyTheOnScreenEpisodeNumberIsKept(t *testing.T) {
	p := parseOne(t, `<episode-num system="dd_progid">EP00003786.2795</episode-num><episode-num system="onscreen">S55E17</episode-num><episode-num system="xmltv_ns">54.16.</episode-num>`)
	if p.Episode != "S55E17" {
		t.Errorf("episode = %q", p.Episode)
	}
	if p := parseOne(t, `<episode-num system="dd_progid">EP00003786.2795</episode-num>`); p.Episode != "" {
		t.Errorf("a program id was taken for an episode: %q", p.Episode)
	}
}

func TestADescriptionIsTidiedAndCapped(t *testing.T) {
	p := parseOne(t, "<desc>  A   long\n  story.  </desc>")
	if p.Description != "A long story." {
		t.Errorf("description = %q", p.Description)
	}
	p = parseOne(t, "<desc>"+strings.Repeat("é", 500)+"</desc>")
	if len(p.Description) > maxDescription+len("…") || !strings.HasSuffix(p.Description, "…") || !utf8.ValidString(p.Description) {
		t.Errorf("a long description: %d bytes, valid=%v", len(p.Description), utf8.ValidString(p.Description))
	}
}
