package banner

import (
	"bytes"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestWhatToDrawDependsOnTheTerminal(t *testing.T) {
	for name, c := range map[string]struct {
		tty  bool
		env  map[string]string
		want Mode
	}{
		"a log file or docker logs get nothing": {false, map[string]string{"COLORTERM": "truecolor"}, None},
		"a dumb terminal gets nothing":          {true, map[string]string{"TERM": "dumb"}, None},
		"NO_COLOR keeps the letters, not color": {true, map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor", "NO_COLOR": "1"}, Plain},
		"true color":                            {true, map[string]string{"TERM": "xterm", "COLORTERM": "truecolor"}, TrueColor},
		"24bit is true color too":               {true, map[string]string{"TERM": "xterm", "COLORTERM": "24bit"}, TrueColor},
		"256 colors":                            {true, map[string]string{"TERM": "xterm-256color"}, Color256},
		"anything else is plain":                {true, map[string]string{"TERM": "vt100"}, Plain},
	} {
		if got := ModeFor(c.tty, env(c.env)); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}

func render(mode Mode) string {
	var b bytes.Buffer
	Print(&b, mode, "2026.10.14")
	return b.String()
}

func TestNothingIsPrintedForNone(t *testing.T) {
	if s := render(None); s != "" {
		t.Errorf("printed %q", s)
	}
}

func TestPlainIsTheLettersAndTheVersionWithNoEscapes(t *testing.T) {
	s := render(Plain)
	if strings.Contains(s, "\x1b") {
		t.Error("plain output has an escape code in it")
	}
	if !strings.Contains(s, "Yagi 2026.10.14") {
		t.Error("the version is missing")
	}
	if !strings.Contains(s, "        YAGIYAGIYAGIYAGIYAGIYAGIYAGIYAGIYAGIY\n") {
		t.Error("the boom is missing")
	}
}

func TestEveryLineEndsItsColor(t *testing.T) {
	for _, mode := range []Mode{TrueColor, Color256} {
		for _, line := range strings.Split(render(mode), "\n") {
			if strings.Contains(line, "\x1b[38;") && !strings.HasSuffix(line, "\x1b[0m") {
				t.Errorf("mode %v: a line leaves its color on: %q", mode, line)
			}
		}
	}
}

func TestShading(t *testing.T) {
	rgb := func(r, c int) string { return strings.TrimSuffix(colorOf(TrueColor, brightness(r, c)), "m") }
	for name, c := range map[string]struct {
		r, c int
		want string
	}{
		"the outer edge of an element is dim":      {1, 13, "\x1b[38;2;19;94;37"},
		"the next cell in is brighter":             {1, 14, "\x1b[38;2;43;213;85"},
		"the top of an element is not dimmed":      {2, 14, "\x1b[38;2;51;253;101"},
		"the boom under a gap is dimmed":           {6, 16, "\x1b[38;2;32;157;63"},
		"the middle of the boom is full":           {6, 12, "\x1b[38;2;51;253;101"},
		"the mast is full inside, dim at its side": {17, 25, "\x1b[38;2;51;253;101"},
	} {
		if got := rgb(c.r, c.c); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}

func TestTheArtIsEvenAndUsesOnlyTheLettersOfTheName(t *testing.T) {
	for r, row := range art {
		if len(row) > 50 {
			t.Errorf("row %d is %d wide", r, len(row))
		}
		if strings.Trim(row, "YAGI ") != "" {
			t.Errorf("row %d has something else in it: %q", r, row)
		}
	}
}
