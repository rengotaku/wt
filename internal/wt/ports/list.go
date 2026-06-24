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
			wRepo, r.Repo, wWt, r.WtName, wPort, rangeCell(r.PortBase), liveCell(r.Ports))
	}
	return nil
}

func rangeCell(base int) string {
	if base == 0 {
		return "—"
	}
	return RangeString(base)
}

// liveCell renders the up ports as "9000 air(123), 9001 vite(456)" or "idle"
// when nothing is up, "—" when the worktree is unallocated. A service that is
// running but binds no port (a headless worker) is shown with a trailing "*" so
// it is not mistaken for something reachable on that port.
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
		// Running without a LISTEN socket = headless service, no port to reach.
		if !s.Listening {
			label += "*"
		}
		up = append(up, label)
	}
	if len(up) == 0 {
		return "idle"
	}
	return strings.Join(up, ", ")
}
