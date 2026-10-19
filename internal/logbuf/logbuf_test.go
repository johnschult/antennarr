package logbuf

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func logger(b *Buffer, out *bytes.Buffer, level slog.Level) *slog.Logger {
	return slog.New(Tee(slog.NewTextHandler(out, &slog.HandlerOptions{Level: level}), b))
}

func TestALineIsKeptWithItsAttributesAndStillGoesWhereItWent(t *testing.T) {
	b := New(10)
	var out bytes.Buffer
	l := logger(b, &out, slog.LevelInfo)
	l.Info("tuner online", "device", "antenna", "id", "10ABCC87", "n", 4)
	r := b.Find(Query{})
	if len(r.Entries) != 1 {
		t.Fatalf("%v", r.Entries)
	}
	e := r.Entries[0]
	if e.Level != "info" || e.Message != "tuner online" || e.Seq != 1 || e.Time.IsZero() {
		t.Errorf("%+v", e)
	}
	if len(e.Attrs) != 3 || e.Attrs[0] != (Attr{"device", "antenna"}) || e.Attrs[2] != (Attr{"n", "4"}) {
		t.Errorf("attrs = %v", e.Attrs)
	}
	if !strings.Contains(out.String(), "tuner online") || !strings.Contains(out.String(), "device=antenna") {
		t.Errorf("the original output lost it: %q", out.String())
	}
}

func TestOnlyWhatTheLoggerWouldHaveWrittenIsKept(t *testing.T) {
	b := New(10)
	l := logger(b, &bytes.Buffer{}, slog.LevelInfo)
	l.Debug("noise")
	l.Warn("careful")
	if r := b.Find(Query{}); len(r.Entries) != 1 || r.Entries[0].Level != "warn" {
		t.Errorf("%v", r.Entries)
	}
}

func TestTheOldestAreForgottenWhenItIsFullAndTheCountKeepsGoing(t *testing.T) {
	b := New(3)
	l := logger(b, &bytes.Buffer{}, slog.LevelInfo)
	for _, m := range []string{"a", "b", "c", "d", "e"} {
		l.Info(m)
	}
	r := b.Find(Query{})
	if len(r.Entries) != 3 || r.Entries[0].Message != "c" || r.Entries[2].Message != "e" || r.Entries[2].Seq != 5 || r.Latest != 5 || r.Kept != 3 || r.Capacity != 3 {
		t.Errorf("%+v", r)
	}
}

func TestAskingForWhatCameAfterAPointReturnsOnlyThat(t *testing.T) {
	b := New(10)
	l := logger(b, &bytes.Buffer{}, slog.LevelInfo)
	l.Info("one")
	l.Info("two")
	seen := b.Find(Query{}).Latest
	if r := b.Find(Query{Since: seen}); len(r.Entries) != 0 || r.Latest != seen {
		t.Errorf("nothing new: %+v", r)
	}
	l.Info("three")
	if r := b.Find(Query{Since: seen}); len(r.Entries) != 1 || r.Entries[0].Message != "three" {
		t.Errorf("%+v", r.Entries)
	}
	b.Clear()
	l.Info("four")
	if r := b.Find(Query{Since: seen + 1}); len(r.Entries) != 1 || r.Entries[0].Seq != seen+2 {
		t.Errorf("after a clear the count carries on: %+v", r.Entries)
	}
}

func TestLevelTextAndLimitNarrowWhatIsReturned(t *testing.T) {
	b := New(20)
	l := logger(b, &bytes.Buffer{}, slog.LevelDebug)
	l.Debug("d")
	l.Info("tuner online", "device", "Antenna")
	l.Warn("no viewers configured")
	l.Error("couldn't read the guide", "err", errors.New("boom"))
	if r := b.Find(Query{Level: "warn"}); len(r.Entries) != 2 {
		t.Errorf("warn and up: %v", r.Entries)
	}
	if r := b.Find(Query{Level: "error"}); len(r.Entries) != 1 || r.Entries[0].Message != "couldn't read the guide" {
		t.Errorf("error: %v", r.Entries)
	}
	if r := b.Find(Query{Text: "ANTENNA"}); len(r.Entries) != 1 || r.Entries[0].Message != "tuner online" {
		t.Errorf("text in an attribute, any case: %v", r.Entries)
	}
	if r := b.Find(Query{Text: "boom"}); len(r.Entries) != 1 {
		t.Errorf("text in an error: %v", r.Entries)
	}
	if r := b.Find(Query{Limit: 2}); len(r.Entries) != 2 || r.Entries[1].Level != "error" {
		t.Errorf("the newest two: %v", r.Entries)
	}
	if r := b.Find(Query{Level: "error", Text: "tuner"}); len(r.Entries) != 0 {
		t.Errorf("both must hold: %v", r.Entries)
	}
}

func TestGroupsAndLoggerAttributesAreKept(t *testing.T) {
	b := New(10)
	l := logger(b, &bytes.Buffer{}, slog.LevelInfo).With("device", "antenna").WithGroup("req").With("id", 7)
	l.Info("hello", "path", "/x", slog.Group("peer", "ip", "10.0.0.1"))
	a := b.Find(Query{}).Entries[0].Attrs
	got := map[string]string{}
	for _, x := range a {
		got[x.Key] = x.Value
	}
	want := map[string]string{"device": "antenna", "req.id": "7", "req.path": "/x", "req.peer.ip": "10.0.0.1"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
}

func TestSecretsAreHiddenInAttributesAndInText(t *testing.T) {
	b := New(10)
	l := logger(b, &bytes.Buffer{}, slog.LevelInfo)
	l.Info("fetching https://api.hdhomerun.com/api/xmltv?DeviceAuth=SECRETAUTH&x=1", "token", "SECRETTOKEN", "url", "http://h/p?token=SECRETQ&a=b", "note", "Authorization: Bearer SECRETBEARER", "lockkey", "12345", "device", "antenna")
	var all strings.Builder
	e := b.Find(Query{}).Entries[0]
	all.WriteString(e.Message)
	for _, a := range e.Attrs {
		all.WriteString(a.Key + "=" + a.Value + " ")
	}
	for _, secret := range []string{"SECRETAUTH", "SECRETTOKEN", "SECRETQ", "SECRETBEARER", "12345"} {
		if strings.Contains(all.String(), secret) {
			t.Errorf("%s was kept: %s", secret, all.String())
		}
	}
	if !strings.Contains(all.String(), "device=antenna") || !strings.Contains(e.Message, "x=1") {
		t.Errorf("hid too much: %s", all.String())
	}
}

func TestVeryLongLinesAreCut(t *testing.T) {
	b := New(2)
	logger(b, &bytes.Buffer{}, slog.LevelInfo).Info(strings.Repeat("m", 5000), "v", strings.Repeat("v", 5000))
	e := b.Find(Query{}).Entries[0]
	if len(e.Message) > maxMessage+4 || len(e.Attrs[0].Value) > maxValue+4 {
		t.Errorf("%d %d", len(e.Message), len(e.Attrs[0].Value))
	}
}

func TestValid(t *testing.T) {
	for _, l := range []string{"debug", "info", "warn", "error"} {
		if !Valid(l) {
			t.Error(l)
		}
	}
	if Valid("trace") || Valid("") {
		t.Error("bad level")
	}
}
