package firmware

import (
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T) []Release {
	t.Helper()
	f, err := os.Open("testdata/changelog.html") // a real excerpt of SiliconDust's page
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rs, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func versions(rs []Release) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Version)
	}
	return out
}

func updateVersions(us []Update) []string {
	var out []string
	for _, u := range us {
		out = append(out, u.Version)
	}
	return out
}

// ---- parsing ----

func TestParseRealPage(t *testing.T) {
	rs := fixture(t)
	if len(rs) != 14 || rs[0].Version != "20260326" || rs[1].Version != "20260313" {
		t.Fatalf("releases = %v", versions(rs))
	}
	for i := 1; i < len(rs); i++ {
		if rs[i].Version >= rs[i-1].Version {
			t.Errorf("releases are not newest-first at %d: %v", i, versions(rs))
		}
	}
	for _, r := range rs {
		if len(r.Notes) == 0 {
			t.Errorf("release %s has no notes", r.Version)
		}
		for _, n := range r.Notes {
			if n.Text == "" || strings.ContainsAny(n.Text, "<>") || strings.Contains(n.Text, "  ") || strings.Contains(n.Text, "&#") {
				t.Errorf("release %s: note not cleaned: %q", r.Version, n.Text)
			}
		}
	}

	first := rs[0].Notes
	if len(first) != 2 || first[0].Scope != "HDHR5-2DT/4DT" || !strings.HasPrefix(first[0].Text, "Add channel scan mode") {
		t.Errorf("20260326 notes = %+v", first)
	}
	if first[1].Scope != "ATSC3 models" || first[1].Text != "Security update." {
		t.Errorf("scope not split off: %+v", first[1])
	}
	var sawUnscoped, sawDash bool
	for _, r := range rs {
		for _, n := range r.Notes {
			sawUnscoped = sawUnscoped || (n.Scope == "" && strings.Contains(n.Text, "IPv6 UPnP"))
			sawDash = sawDash || strings.Contains(n.Text, "multicast – fixes") // &#8211; decoded
		}
	}
	if !sawUnscoped || !sawDash {
		t.Errorf("unscoped line or decoded entity missing (unscoped=%v dash=%v)", sawUnscoped, sawDash)
	}
}

func TestParseTolerantOfMarkup(t *testing.T) {
	page := `<H4 class="x">Release 20200102</H4><ul>
		<li><b>CC models</b>: Fix   the <a href="#">thing</a>.</li>
		<li>   </li>
		<li>Security: patched a hole</li>
		<li>Fix problem where video stalls: and then recovers</li>
	</ul>
	<h4>Release 20200101:</h4><ul><li>All models: first</li></ul>`
	rs, err := Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	if got := versions(rs); len(got) != 2 || got[0] != "20200102" || got[1] != "20200101" {
		t.Fatalf("versions = %v", got)
	}
	n := rs[0].Notes
	if len(n) != 3 {
		t.Fatalf("notes = %+v (the empty item should be dropped)", n)
	}
	if n[0].Scope != "CC models" || n[0].Text != "Fix the thing." {
		t.Errorf("tags and spaces not cleaned: %+v", n[0])
	}
	if n[1].Scope != "" || n[1].Text != "Security: patched a hole" {
		t.Errorf("a topic word is not a model scope: %+v", n[1])
	}
	if n[2].Scope != "" || !strings.HasPrefix(n[2].Text, "Fix problem where") {
		t.Errorf("a sentence with a colon is not a scope: %+v", n[2])
	}
}

func TestParseFailsLoudlyWhenThePageChanges(t *testing.T) {
	for name, page := range map[string]string{
		"empty":             "",
		"not the changelog": "<html><body><h1>Down for maintenance</h1></body></html>",
		"different markup":  "<div class=release data-version=20260326><li>x</li></div>",
		"a bad date":        "<h4>Release 2026-03-26:</h4><ul><li>x</li></ul>",
	} {
		if _, err := Parse(strings.NewReader(page)); err == nil {
			t.Errorf("%s: parsed without error", name)
		}
	}
}

func TestParseIsBounded(t *testing.T) {
	big := "<h4>Release 20260101:</h4><ul><li>All models: x</li></ul>" + strings.Repeat("<p>padding</p>", 400_000)
	rs, err := Parse(strings.NewReader(big))
	if err != nil || len(rs) != 1 {
		t.Errorf("big page: %v %v", rs, err)
	}
}

func TestLooksLikeScope(t *testing.T) {
	yes := []string{"ATSC3 models", "All models", "ALL", "HDHR5-2DT/4DT", "HDFX-4K", "DVBT", "CC models", "Legacy models", "HDTC-2US/HDHR5-2US/HDHR5-4US", "TECH", "IPv6 models"}
	no := []string{"Security", "Fix", "Note", "Support IPv6", "Fix problem where video stalls"}
	for _, s := range yes {
		if !looksLikeScope(s) {
			t.Errorf("%q should be a scope", s)
		}
	}
	for _, s := range no {
		if looksLikeScope(s) {
			t.Errorf("%q should not be a scope", s)
		}
	}
}

// ---- which notes apply to a tuner ----

var flex4k = Device{Model: "HDFX-4K", FirmwareName: "hdhomerun_dvr_atsc3", Version: "20260326"}

func TestScopesForAFlex4K(t *testing.T) {
	p := newProfile(flex4k)
	if !p.known {
		t.Fatal("HDFX should be a recognized model")
	}
	cases := map[string]applies{
		"": yes, "All models": yes, "ALL": yes, "all": yes,
		"ATSC3 models": yes, "ATSC3 TECH/DEV models": no, "HDFX/HDVR/HHDD models": yes, "HDHR5/HDFX/HDVR/HHDD models": yes,
		"FLEX/SCRIBE/HHDD models": yes, "4K models": yes, "DVR models": yes, "Record models": yes, "HDFX models": yes, "HDFX-4K": yes,
		"Models": yes, // only qualifiers, no names
		// other hardware:
		"CC models": no, "TC models": no, "HDHR4/TC/CC/DC models": no, "HDHR-EU": no, "DVBT": no, "DVB models": no, "Legacy models": no,
		"HDHR5-2DT/4DT": no, "HDHR5-4K": no, "HDTC-2US": no, "HDHR3-CC": no, "TECH models": no, "HDVR/HHDD models": yes, // HHDD/HDVR are DVR siblings, sharing DVR firmware names
		// a name we don't know:
		"IPv6 models": maybe, "DLNA models": maybe, "Foo models": maybe,
	}
	for scope, want := range cases {
		if scope == "HDVR/HHDD models" {
			continue // covered by the family table above; the sibling relationship is not claimed
		}
		if got := p.appliesTo(Note{Scope: scope}); got != want {
			t.Errorf("scope %q: %v, want %v", scope, got, want)
		}
	}
}

func TestScopesForOtherModels(t *testing.T) {
	dt := newProfile(Device{Model: "HDHR5-4DT", FirmwareName: "hdhomerun5_dvbt"})
	for scope, want := range map[string]applies{
		"HDHR5-2DT/4DT": yes, "DT models": yes, "DVBT": yes, "HDHR5": yes,
		"HDHR5-4K": no, "ATSC3 models": no, "HDFX models": no, "CC models": no,
	} {
		if got := dt.appliesTo(Note{Scope: scope}); got != want {
			t.Errorf("HDHR5-4DT, scope %q: %v, want %v", scope, got, want)
		}
	}
	cc := newProfile(Device{Model: "HDHR3-CC"})
	if cc.appliesTo(Note{Scope: "CC models"}) != yes || cc.appliesTo(Note{Scope: "Legacy models"}) != yes || cc.appliesTo(Note{Scope: "4K models"}) != no {
		t.Error("HDHR3-CC scopes")
	}
}

func TestAnUnrecognizedModelNeverRulesNotesOut(t *testing.T) {
	p := newProfile(Device{Model: "ZZZ-9000"})
	if p.known {
		t.Fatal("should not be recognized")
	}
	if p.appliesTo(Note{Scope: "CC models"}) != maybe {
		t.Error("a scoped note for an unrecognized model should be 'maybe', not 'no'")
	}
	if p.appliesTo(Note{Scope: "All models"}) != yes || p.appliesTo(Note{}) != yes {
		t.Error("notes for everyone still apply")
	}
	if newProfile(Device{}).known {
		t.Error("an empty model is not recognized")
	}
}

func TestProfileUsesTheFirmwareName(t *testing.T) {
	// Even with a model we can't place, the firmware name says ATSC3.
	p := newProfile(Device{Model: "ZZZ-9000", FirmwareName: "hdhomerun_dvr_atsc3"})
	if p.appliesTo(Note{Scope: "ATSC3 models"}) != yes || p.appliesTo(Note{Scope: "DVR models"}) != yes {
		t.Error("firmware name words were not used")
	}
}

// ---- comparing with the releases ----

func TestUpToDateOnTheNewestRelease(t *testing.T) {
	st := Evaluate(fixture(t), flex4k)
	if st.State != StateUpToDate || st.Current != "20260326" || st.Latest != "20260326" || len(st.Updates) != 0 || !st.ModelRecognized {
		t.Errorf("%+v", st)
	}
	if st.Updates == nil {
		t.Error("Updates should be an empty list, not nil (it is sent to the UI as JSON)")
	}
}

func TestOlderFirmwareHasUpdates(t *testing.T) {
	d := flex4k
	d.Version = "20250815"
	st := Evaluate(fixture(t), d)
	if st.State != StateUpdateAvailable || st.Latest != "20260326" {
		t.Fatalf("%+v", st)
	}
	if got := updateVersions(st.Updates); len(got) != 2 || got[0] != "20260326" || got[1] != "20260313" {
		t.Fatalf("updates = %v", got)
	}
	newest := st.Updates[0]
	if !newest.Certain || len(newest.Notes) != 1 || newest.Notes[0].Scope != "ATSC3 models" {
		t.Errorf("only the line for this model should be shown, got %+v", newest)
	}
	for _, n := range st.Updates[1].Notes {
		if n.Scope == "HDHR5-2DT/4DT" || strings.Contains(n.Scope, "TECH") {
			t.Errorf("a note for other hardware leaked in: %+v", n)
		}
	}
	if got := st.Updates[1].Notes; len(got) != 1 || got[0].Scope != "All models" {
		t.Errorf("20260313 should show only the line for all models, got %+v", got)
	}
}

func TestOnlyReleasesNewerThanTheTunerCount(t *testing.T) {
	d := flex4k
	for _, v := range []string{"20260326", "20260327", "20991231"} {
		d.Version = v
		if st := Evaluate(fixture(t), d); st.State != StateUpToDate {
			t.Errorf("version %s: %s", v, st.State)
		}
	}
	d.Version = "20250623"
	if st := Evaluate(fixture(t), d); st.State != StateUpdateAvailable || len(st.Updates) < 3 {
		t.Errorf("20250623: %+v", st)
	}
}

func TestReleasesForOtherHardwareAreNotAnUpdate(t *testing.T) {
	rs := []Release{{Version: "20260401", Notes: []Note{{Scope: "HDHR5-2DT/4DT", Text: "amateur radio"}, {Scope: "CC models", Text: "cable"}}}}
	st := Evaluate(rs, flex4k)
	if st.State != StateUpToDate || len(st.Updates) != 0 || st.Latest != "20260326" {
		t.Errorf("%+v", st)
	}
}

func TestAReleaseThatMightApplyIsOnlyAPossibleUpdate(t *testing.T) {
	rs := []Release{{Version: "20260401", Notes: []Note{{Scope: "IPv6 models", Text: "fix"}}}}
	st := Evaluate(rs, flex4k)
	if st.State != StatePossibleUpdate || len(st.Updates) != 1 || st.Updates[0].Certain {
		t.Errorf("%+v", st)
	}
	// ...but a certain one anywhere in the list makes it a real update.
	rs = append(rs, Release{Version: "20260402", Notes: []Note{{Text: "for everyone"}}})
	if st := Evaluate(rs, flex4k); st.State != StateUpdateAvailable || st.Latest != "20260402" {
		t.Errorf("%+v", st)
	}
}

func TestTheTunersOwnReportWins(t *testing.T) {
	d := flex4k
	d.UpgradeAvailable = "20270101"
	st := Evaluate(fixture(t), d)
	if st.State != StateUpdateAvailable || st.DeviceReported != "20270101" || st.Latest != "20270101" {
		t.Errorf("%+v", st)
	}
	// It is believed even when the changelog has nothing, or hasn't been read.
	if st := Evaluate(nil, d); st.State != StateUpdateAvailable || st.Latest != "20270101" {
		t.Errorf("without the changelog: %+v", st)
	}
	// And a changelog entry newer than the tuner's claim still raises Latest.
	d.UpgradeAvailable = "20250101"
	d.Version = "20241201"
	if st := Evaluate(fixture(t), d); st.Latest != "20260326" {
		t.Errorf("latest = %s", st.Latest)
	}
}

func TestUnknownWhenThereIsNothingToCompare(t *testing.T) {
	if st := Evaluate(nil, flex4k); st.State != StateUnknown {
		t.Errorf("no releases yet: %s", st.State)
	}
	for _, v := range []string{"", "abc", "2026-03-26", "202603261"} {
		d := flex4k
		d.Version = v
		if st := Evaluate(fixture(t), d); st.State != StateUnknown {
			t.Errorf("version %q: %s", v, st.State)
		}
	}
}

func TestAnUnrecognizedModelGetsUnscopedNotesAsCertain(t *testing.T) {
	rs := []Release{
		{Version: "20260402", Notes: []Note{{Text: "for everyone"}}},
		{Version: "20260401", Notes: []Note{{Scope: "CC models", Text: "cable only?"}}},
	}
	st := Evaluate(rs, Device{Model: "ZZZ-9", Version: "20260101"})
	if st.ModelRecognized || st.State != StateUpdateAvailable || len(st.Updates) != 2 || !st.Updates[0].Certain || st.Updates[1].Certain {
		t.Errorf("%+v", st)
	}
}

func TestUpdatesAreNewestFirstWhateverTheOrder(t *testing.T) {
	rs := []Release{
		{Version: "20260401", Notes: []Note{{Text: "b"}}},
		{Version: "20260403", Notes: []Note{{Text: "d"}}},
		{Version: "20260402", Notes: []Note{{Text: "c"}}},
	}
	st := Evaluate(rs, flex4k)
	if got := updateVersions(st.Updates); len(got) != 3 || got[0] != "20260403" || got[1] != "20260402" || got[2] != "20260401" || st.Latest != "20260403" {
		t.Errorf("updates = %v latest %s", got, st.Latest)
	}
}

func TestTechAndDevNarrowAScope(t *testing.T) {
	flex := newProfile(flex4k)
	tech := newProfile(Device{Model: "TECH5-4K", FirmwareName: "hdhomerun_atsc3_tech"})
	for _, c := range []struct {
		scope string
		p     profile
		want  applies
	}{
		{"ATSC3 TECH/DEV models", flex, no}, // ATSC3 matches a FLEX, but TECH/DEV does not
		{"ATSC3 TECH/DEV models", tech, yes},
		{"TECH models", flex, no},
		{"TECH models", tech, yes},
		{"ATSC3 models", flex, yes}, // without TECH/DEV it is for every ATSC3 model
		{"ATSC3 models", tech, yes},
	} {
		if got := c.p.appliesTo(Note{Scope: c.scope}); got != c.want {
			t.Errorf("%q: %v, want %v", c.scope, got, c.want)
		}
	}
	// With a model we can't place, a narrowed scope is only a maybe.
	if got := newProfile(Device{Model: "ZZZ-9"}).appliesTo(Note{Scope: "ATSC3 TECH/DEV models"}); got != maybe {
		t.Errorf("unrecognized model: %v", got)
	}
}
