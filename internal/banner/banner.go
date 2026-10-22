// Package banner draws the Yagi antenna, made of the letters of its name, for a terminal when Yagi is started by hand.
package banner

import (
	"fmt"
	"io"
	"strings"
)

// art is the antenna: a boom with three rows of elements across it, on a mast. Each cell is a letter of YAGI, going
// round, and which one it is does not matter. The shape does.
var art = []string{
	"",
	"             IYAG       GIYA       AGI       IYAG",
	"            YAGIYA     IYAGI     AGIYAG     YAGIY",
	"           AGIYAG     YAGIY     GIYAGI     AGIYAG",
	"          GIYAGI     AGIYA     IYAGIY     GIYAGI",
	"         IYAGIY     GIYAG     YAGIYA     IYAGIY",
	"        YAGIYAGIYAGIYAGIYAGIYAGIYAGIYAGIYAGIY",
	"       AGIYAGIYAGIYAGIYAGIYAGIYAGIYAGIYAGIYAG",
	"      GIYAGIYAGIYAGIYAGIYAGIYAGIYAGIYAGIYAGI",
	"     IYAGIY     GIYAG     YAGIYA     IYAGIY",
	"    YAGIYA     IYAGI     AGIYAG     YAGIY",
	"   AGIYAG     YAGIY     GIYAGI     AGIYA",
	"  GIYAGI     AGIYA     IYAGIY     GIYAG",
	"   AGIY      IYAG       GIYA      YAGI",
	"",
	"",
	"                       IYAGI",
	"                       AGIYA",
	"                       IYAGI",
	"                       AGIYA",
	"                       IYAGI",
	"                       AGIYA",
	"                       IYAGI",
	"                       AGIYA",
	"                         A",
}

// Mode is how much a terminal can be shown.
type Mode int

const (
	// None prints nothing: not a terminal, or one that can't be drawn on.
	None Mode = iota
	// Plain prints the letters without color.
	Plain
	// Color256 uses the 256-color palette.
	Color256
	// TrueColor uses 24-bit color.
	TrueColor
)

// ModeFor says what to draw for a terminal. isTerminal is whether the output is one at all (a log file, `docker logs`
// and systemd are not, and must not get escape codes). getenv reads the environment.
func ModeFor(isTerminal bool, getenv func(string) string) Mode {
	if !isTerminal || getenv("TERM") == "dumb" {
		return None
	}
	if getenv("NO_COLOR") != "" {
		return Plain
	}
	switch strings.ToLower(getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return TrueColor
	}
	if strings.Contains(getenv("TERM"), "256color") {
		return Color256
	}
	return Plain
}

// Print draws the antenna, and under it the name and version. It prints nothing for None.
func Print(w io.Writer, mode Mode, version string) {
	if mode == None {
		return
	}
	var b strings.Builder
	b.WriteString("\n")
	for r, row := range art {
		for c := 0; c < len(row); c++ {
			ch := row[c]
			if ch == ' ' || mode == Plain {
				b.WriteByte(ch)
				continue
			}
			b.WriteString(colorOf(mode, brightness(r, c)))
			b.WriteByte(ch)
		}
		if mode != Plain && strings.TrimSpace(row) != "" {
			b.WriteString("\x1b[0m")
		}
		b.WriteString("\n")
	}
	title := "Yagi " + version
	pad := (50 - len(title)) / 2
	if pad < 0 {
		pad = 0
	}
	b.WriteString("\n" + strings.Repeat(" ", pad))
	if mode == Plain {
		b.WriteString(title)
	} else {
		b.WriteString(colorOf(mode, 1) + title + "\x1b[0m")
	}
	b.WriteString("\n\n")
	_, _ = io.WriteString(w, b.String())
}

// filled is whether the cell at row r, column c has a letter in it. Outside the art is empty.
func filled(r, c int) bool {
	return r >= 0 && r < len(art) && c >= 0 && c < len(art[r]) && art[r][c] != ' '
}

// gap is whether the empty cell at row r, column c is between two parts of the art, with letters to its left and to
// its right on the row. Blank space around the antenna is not.
func gap(r, c int) bool {
	if r < 0 || r >= len(art) || filled(r, c) {
		return false
	}
	row := art[r]
	left := strings.TrimSpace(row[:min(c, len(row))]) != ""
	right := c < len(row) && strings.TrimSpace(row[c:]) != ""
	return left && right
}

// edge is how many steps from the cell, in one direction, the art stops: 1 for a cell at the edge, at most 3. Across
// a row it stops at the first empty cell. Up and down only a gap between two elements counts, so the tops and bottoms
// of the elements and the mast stay bright and the boom is dimmed only where it meets them.
func edge(r, c, dr, dc int) int {
	n := 1
	for n < 3 {
		rr, cc := r+dr*n, c+dc*n
		if filled(rr, cc) {
			n++
			continue
		}
		if dc == 0 && !gap(rr, cc) {
			return 3
		}
		break
	}
	return n
}

var (
	// across is the brightness by distance from the left or right edge of a run, down is by distance from the
	// top or bottom: the sides fall off faster than the ends, which is what makes the elements look round.
	across = [...]float64{0, 0.37, 0.84, 1}
	down   = [...]float64{0, 0.62, 0.9, 1}
)

// brightness is how bright the cell should be, from 0 to 1: the edges of a shape are dim and the middle is full.
func brightness(r, c int) float64 {
	h := across[min(edge(r, c, 0, -1), edge(r, c, 0, 1))]
	v := down[min(edge(r, c, -1, 0), edge(r, c, 1, 0))]
	return min(h, v)
}

// greens are the 256-color palette's greens, dim to bright.
var greens = [...]int{22, 28, 34, 40, 46}

// colorOf is the escape sequence that sets the text to a green of the given brightness.
func colorOf(mode Mode, b float64) string {
	if mode == Color256 {
		return fmt.Sprintf("\x1b[38;5;%dm", greens[int(b*float64(len(greens)-1)+0.5)])
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", int(51*b+0.5), int(253*b+0.5), int(101*b+0.5))
}
