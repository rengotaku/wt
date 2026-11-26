package out

import (
	"io"
	"strings"
)

// ANSI codes used by the rich view.
const (
	Reset  = "\x1b[0m"
	Bold   = "\x1b[1m"
	Dim    = "\x1b[2m"
	Red    = "\x1b[31m"
	Green  = "\x1b[32m"
	Yellow = "\x1b[33m"
	Purple = "\x1b[35m"
)

// stateColors colors well-known state values in the rich view.
var stateColors = map[string]string{
	"running":   Green,
	"unhealthy": Red,
	"idle":      Yellow,
	"stale":     Purple,
	"stopped":   Red,
	"on":        Green,
}

// WriteRich writes t as a boxed table under an optional bold title. Meta
// lines become a "key: value" line above the table. color adds ANSI colors.
func WriteRich(w io.Writer, t *Table, title string, color bool) error {
	p := painter{on: color}
	var b strings.Builder
	if title != "" {
		b.WriteString(p.paint(Bold, title) + "\n")
	}
	if len(t.Meta) > 0 {
		parts := make([]string, 0, len(t.Meta))
		for _, m := range t.Meta {
			parts = append(parts, p.paint(Bold, m[0]+":")+" "+p.paint(stateColors[m[1]], Cell(m[1])))
		}
		b.WriteString(strings.Join(parts, "   ") + "\n")
	}

	widths := make([]int, len(t.Header))
	for i, h := range t.Header {
		widths[i] = DisplayWidth(h)
	}
	for _, r := range t.Rows {
		for i := range widths {
			if i < len(r) {
				widths[i] = max(widths[i], DisplayWidth(Cell(r[i])))
			}
		}
	}
	rule := func(l, m, r string) {
		b.WriteString(l)
		for i, wd := range widths {
			b.WriteString(strings.Repeat("─", wd+2))
			if i < len(widths)-1 {
				b.WriteString(m)
			}
		}
		b.WriteString(r + "\n")
	}
	line := func(cells []string, header bool) {
		b.WriteString("│")
		for i := range widths {
			c := ""
			if i < len(cells) {
				c = Cell(cells[i])
			}
			color := stateColors[c]
			if header {
				color = Bold
			}
			b.WriteString(" " + p.paint(color, c) + strings.Repeat(" ", widths[i]-DisplayWidth(c)) + " │")
		}
		b.WriteString("\n")
	}
	rule("┌", "┬", "┐")
	line(t.Header, true)
	rule("├", "┼", "┤")
	for _, r := range t.Rows {
		line(r, false)
	}
	rule("└", "┴", "┘")
	_, err := io.WriteString(w, b.String())
	return err
}

type painter struct{ on bool }

func (p painter) paint(code, s string) string {
	if !p.on || code == "" || s == "" {
		return s
	}
	return code + s + Reset
}

// DisplayWidth counts terminal columns: East Asian wide characters take two.
func DisplayWidth(s string) int {
	w := 0
	for _, r := range s {
		if isWide(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F,
		r >= 0x2E80 && r <= 0xA4CF,
		r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE4F,
		r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F300 && r <= 0x1FAFF:
		return true
	}
	return false
}
