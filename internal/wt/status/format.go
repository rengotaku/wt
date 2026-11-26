package status

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// TSVHeader is the column order of WriteTSV. Scripts may rely on it.
var TSVHeader = []string{"REPO", "WORKTREE", "BRANCH", "STATE", "PORTS", "AUTOSTART", "URL"}

// WriteTSV writes the default, machine-oriented output: "#"-prefixed
// key=value lines for the daemon and proxy, then a tab-separated table with
// one worktree per line. No color, no alignment, no truncation; empty values
// are "-".
func WriteTSV(out io.Writer, rep *Report) error {
	daemonLine := "# daemon=stopped"
	if rep.Daemon.Running {
		daemonLine = "# daemon=running pid=" + pidString(rep.Daemon.PID)
	}
	proxyState := "disabled"
	if rep.Proxy.Enabled {
		proxyState = "enabled"
	}
	if _, err := fmt.Fprintf(out, "%s\n# proxy=%s addr=%s\n%s\n",
		daemonLine, proxyState, rep.Proxy.Addr, strings.Join(TSVHeader, "\t")); err != nil {
		return err
	}
	for i := range rep.Worktrees {
		w := &rep.Worktrees[i]
		fields := []string{w.Repo, w.Worktree, w.Branch, w.State, w.Ports, onOff(w.AutoStart), w.URL}
		for j, f := range fields {
			fields[j] = tsvField(f)
		}
		if _, err := fmt.Fprintln(out, strings.Join(fields, "\t")); err != nil {
			return err
		}
	}
	return nil
}

// WriteJSON writes the report as indented JSON.
func WriteJSON(out io.Writer, rep *Report) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func pidString(pid int) string {
	if pid == 0 {
		return "unknown"
	}
	return strconv.Itoa(pid)
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// tsvField keeps one value on one cell: tabs and newlines would break the
// table, and an empty cell is written as "-".
func tsvField(s string) string {
	s = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(s)
	if s == "" {
		return "-"
	}
	return s
}
