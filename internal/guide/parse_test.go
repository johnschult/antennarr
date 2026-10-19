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

// aboutGuide is a guide with the words a channel as a whole is read from: its names, and its programs' languages
// and categories.
const aboutGuide = `<?xml version="1.0"?><tv>
<channel id="a"><display-name>WSBDT</display-name><display-name>2.1 WSBDT</display-name><display-name>2.1 WSB-HD</display-name><display-name>2.1</display-name><display-name>WSB-HD</display-name><display-name>ABC</display-name><lcn>2.1</lcn></channel>
<channel id="b"><display-name>WSBDT2</display-name><display-name>2.2 WSBDT2</display-name><display-name>2.2 BOUNCE</display-name><display-name>2.2</display-name><display-name>BOUNCE</display-name><lcn>2.2</lcn></channel>
<channel id="c"><display-name>X</display-name><display-name>26.1 X</display-name><display-name>26.1 VENT</display-name><display-name>26.1</display-name><display-name>VENT</display-name><display-name>vent</display-name><lcn>26.1</lcn></channel>
<channel id="d"><display-name>Bare</display-name><lcn>9.1</lcn></channel>
%s
</tv>`

func aboutProg(ch, lang string, cats ...string) string {
	c := ""
	for _, x := range cats {
		c += "<category>" + x + "</category>"
	}
	l := ""
	if lang != "" {
		l = "<language>" + lang + "</language>"
	}
	return `<programme start="20990101000000 +0000" stop="20990101010000 +0000" channel="` + ch + `"><title>T</title>` + c + l + `</programme>`
}

func TestAChannelSaysWhatItCarriesInWhatLanguagesAndWhatItShows(t *testing.T) {
	var progs string
	for i := 0; i < 8; i++ {
		progs += aboutProg("a", "en", "Series", "News")
	}
	progs += aboutProg("a", "en", "Talk", "news") // the same category in another case counts as one
	progs += aboutProg("a", "es", "Talk")         // 1 of 10: under a tenth is not enough... exactly a tenth is
	chs, err := Parse(strings.NewReader(strings.Replace(aboutGuide, "%s", progs, 1)), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a := chs["2.1"].About
	if a.Network != "ABC" {
		t.Errorf("network = %q", a.Network)
	}
	if len(a.Languages) != 2 || a.Languages[0] != "en" || a.Languages[1] != "es" {
		t.Errorf("languages = %v, want en then es (es is a tenth of the programs)", a.Languages)
	}
	if len(a.Categories) != 2 || a.Categories[0] != "News" || a.Categories[1] != "Talk" {
		t.Errorf("categories = %v, want News then Talk, and never Series", a.Categories)
	}
}

func TestAStrayProgramDoesNotMakeAChannelBilingual(t *testing.T) {
	var progs string
	for i := 0; i < 19; i++ {
		progs += aboutProg("a", "en", "News")
	}
	progs += aboutProg("a", "es", "News")
	chs, _ := Parse(strings.NewReader(strings.Replace(aboutGuide, "%s", progs, 1)), time.Now())
	if l := chs["2.1"].About.Languages; len(l) != 1 || l[0] != "en" {
		t.Errorf("languages = %v", l)
	}
}

func TestNothingIsMadeUpForWhatTheGuideDoesNotSay(t *testing.T) {
	progs := aboutProg("b", "", "") + aboutProg("d", "")
	chs, _ := Parse(strings.NewReader(strings.Replace(aboutGuide, "%s", progs, 1)), time.Now())
	for _, n := range []string{"2.2", "9.1"} {
		a := chs[n].About
		if a.Network != "" || len(a.Languages) != 0 || len(a.Categories) != 0 {
			t.Errorf("%s = %+v, want nothing", n, a)
		}
	}
}

func TestANetworkIsOnlyWhatDiffersFromTheName(t *testing.T) {
	chs, _ := Parse(strings.NewReader(strings.Replace(aboutGuide, "%s", "", 1)), time.Now())
	if n := chs["2.2"].About.Network; n != "" {
		t.Errorf("BOUNCE has no network of its own: %q", n)
	}
	if n := chs["26.1"].About.Network; n != "" {
		t.Errorf("a network that is the name in another case is not a network: %q", n)
	}
	if n := chs["2.1"].About.Network; n != "ABC" {
		t.Errorf("%q", n)
	}
}

func TestAboutIsFoundByDeviceAndChannel(t *testing.T) {
	s := NewService(Config{})
	s.devices = map[string]func() string{"antenna": func() string { return "auth" }}
	chs, _ := Parse(strings.NewReader(strings.Replace(aboutGuide, "%s", aboutProg("a", "en", "News"), 1)), time.Now())
	s.feeds = map[string]*feed{"auth": {channels: chs}}
	if a, ok := s.About("antenna", "2.1"); !ok || a.Network != "ABC" {
		t.Errorf("%+v %v", a, ok)
	}
	if _, ok := s.About("antenna", "99.9"); ok {
		t.Error("an unknown channel")
	}
	if _, ok := s.About("nope", "2.1"); ok {
		t.Error("an unknown device")
	}
}
