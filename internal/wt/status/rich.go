package status

import (
	"fmt"
	"io"
	"strings"
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiPurple = "\x1b[35m"
)

var richHeader = []string{"worktree", "branch", "state", "ports", "autostart", "url"}

// WriteRich writes a human-oriented view: a daemon/proxy summary, then one
// boxed table per repository. color adds ANSI colors; pass false when the
// output is not a terminal.
func WriteRich(out io.Writer, rep *Report, color bool) error {
	p := painter{on: color}
	var b strings.Builder

	daemon := p.paint(ansiRed, "stopped")
	if rep.Daemon.Running {
		daemon = p.paint(ansiGreen, "running") + " (pid " + pidString(rep.Daemon.PID) + ")"
	}
	proxy := p.paint(ansiDim, "disabled")
	if rep.Proxy.Enabled {
		proxy = rep.Proxy.Addr
	}
	fmt.Fprintf(&b, "%s %s   %s %s\n", p.paint(ansiBold, "wt daemon:"), daemon, p.paint(ansiBold, "proxy:"), proxy)

	for _, group := range groupByRepo(rep.Worktrees) {
		b.WriteString("\n" + p.paint(ansiBold, group[0].Repo) + "\n")
		writeTable(&b, group, &p)
	}
	_, err := io.WriteString(out, b.String())
	return err
}

func groupByRepo(wts []Worktree) [][]Worktree {
	var groups [][]Worktree
	for i := range wts {
		if len(groups) == 0 || groups[len(groups)-1][0].Repo != wts[i].Repo {
			groups = append(groups, nil)
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], wts[i])
	}
	return groups
}

type cell struct {
	text  string // plain text, used for width
	color string // ANSI color applied after padding
}

func writeTable(b *strings.Builder, wts []Worktree, p *painter) {
	rows := make([][]cell, 0, len(wts))
	for i := range wts {
		w := &wts[i]
		auto := ""
		if w.AutoStart {
			auto = "on"
		}
		rows = append(rows, []cell{
			{text: w.Worktree},
			{text: dash(w.Branch)},
			stateCell(w.State),
			{text: w.Ports},
			{text: auto},
			{text: w.URL},
		})
	}

	widths := make([]int, len(richHeader))
	for i, h := range richHeader {
		widths[i] = displayWidth(h)
	}
	for _, r := range rows {
		for i, c := range r {
			widths[i] = max(widths[i], displayWidth(c.text))
		}
	}

	rule := func(l, m, r string) {
		b.WriteString("  " + l)
		for i, w := range widths {
			b.WriteString(strings.Repeat("─", w+2))
			if i < len(widths)-1 {
				b.WriteString(m)
			}
		}
		b.WriteString(r + "\n")
	}
	line := func(cells []cell) {
		b.WriteString("  │")
		for i, c := range cells {
			pad := strings.Repeat(" ", widths[i]-displayWidth(c.text))
			b.WriteString(" " + p.paint(c.color, c.text) + pad + " │")
		}
		b.WriteString("\n")
	}

	rule("┌", "┬", "┐")
	header := make([]cell, len(richHeader))
	for i, h := range richHeader {
		header[i] = cell{text: h, color: ansiBold}
	}
	line(header)
	rule("├", "┼", "┤")
	for _, r := range rows {
		line(r)
	}
	rule("└", "┴", "┘")
}

func stateCell(state string) cell {
	switch state {
	case StateRunning:
		return cell{text: "● running", color: ansiGreen}
	case StateUnhealthy:
		return cell{text: "● unhealthy", color: ansiRed}
	case StateIdle:
		return cell{text: "○ idle", color: ansiYellow}
	case StateStale:
		return cell{text: "✗ stale", color: ansiPurple}
	default:
		return cell{text: state, color: ansiDim}
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

type painter struct{ on bool }

func (p painter) paint(code, s string) string {
	if !p.on || code == "" || s == "" {
		return s
	}
	return code + s + ansiReset
}

// displayWidth counts terminal columns: East Asian wide characters take two.
func displayWidth(s string) int {
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
