package ports

import (
	"fmt"
	"io"
	"strings"
)

// List writes a human-readable table of port allocations and live status.
func List(out io.Writer) error {
	rows, err := Status()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "worktree がありません")
		return nil
	}

	const (
		hRepo = "REPO"
		hWt   = "WORKTREE"
		hPort = "PORTS"
		hLive = "稼働"
	)
	wRepo, wWt, wPort := len(hRepo), len(hWt), len(hPort)
	for _, r := range rows {
		wRepo = max(wRepo, len(r.Repo))
		wWt = max(wWt, len(r.WtName))
		wPort = max(wPort, len(rangeCell(r.PortBase)))
	}

	_, _ = fmt.Fprintf(out, "%-*s  %-*s  %-*s  %s\n", wRepo, hRepo, wWt, hWt, wPort, hPort, hLive)
	for _, r := range rows {
		_, _ = fmt.Fprintf(out, "%-*s  %-*s  %-*s  %s\n",
			wRepo, r.Repo, wWt, r.WtName, wPort, rangeCell(r.PortBase), statusCell(&r))
	}
	return nil
}

// statusCell renders the 稼働 column: live listeners, "idle", "—", or a "stale"
// marker for a ghost row whose worktree directory is gone but whose port block
// is still reserved (reclaim with `wt ports prune`).
func statusCell(r *Row) string {
	if !r.Exists && r.PortBase != 0 {
		return "stale (dir無 → wt ports prune)"
	}
	return liveCell(r.Ports)
}

func rangeCell(base int) string {
	if base == 0 {
		return "—"
	}
	return RangeString(base)
}

// liveCell renders the up ports as "9000 air(123), 9001 vite(456)" or "idle"
// when nothing is up, "—" when the worktree is unallocated. A headless worker
// (running, binds no port by design) gets a trailing "*"; a service that should
// LISTEN but isn't (running yet no socket) gets a trailing "!" to flag it as
// unhealthy rather than reachable.
func liveCell(states []PortState) string {
	if len(states) == 0 {
		return "—"
	}
	var up []string
	for _, s := range states {
		if !s.Listening && !s.Running {
			continue
		}
		label := fmt.Sprintf("%d", s.Port)
		switch {
		case s.Proc != "" && s.PID != 0:
			label += fmt.Sprintf(" %s(%d)", s.Proc, s.PID)
		case s.Proc != "":
			label += " " + s.Proc
		}
		// Running without a LISTEN socket: "*" if headless by design, "!" if it
		// was expected to listen (build failed / crashed before binding).
		if !s.Listening {
			if s.Unhealthy() {
				label += "!"
			} else {
				label += "*"
			}
		}
		up = append(up, label)
	}
	if len(up) == 0 {
		return "idle"
	}
	return strings.Join(up, ", ")
}
