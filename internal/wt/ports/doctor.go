package ports

import (
	"fmt"
	"io"
	"sort"
)

// DoctorRow is one machine-wide listening port with its owning process and
// whether it belongs to a wt-managed worktree (vs a foreign squatter).
type DoctorRow struct {
	Port    int
	PID     int
	Proc    string
	Managed bool   // true = within a wt worktree's allocated block
	Owner   string // "repo/wtname" when managed
}

// ownerMap maps every wt-allocated port to its "repo/wtname" owner.
func ownerMap() (map[int]string, error) {
	allocs, err := Allocations()
	if err != nil {
		return nil, err
	}
	m := map[int]string{}
	for _, a := range allocs {
		for _, p := range PortsForBase(a.PortBase) {
			m[p] = a.Repo + "/" + a.WtName
		}
	}
	return m, nil
}

// classify joins live listeners with the wt ownership map, sorted by port.
func classify(listeners map[int]Listener, owners map[int]string) []DoctorRow {
	rows := make([]DoctorRow, 0, len(listeners))
	for port, l := range listeners {
		owner, managed := owners[port]
		rows = append(rows, DoctorRow{
			Port:    port,
			PID:     l.PID,
			Proc:    l.Proc,
			Managed: managed,
			Owner:   owner,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Port < rows[j].Port })
	return rows
}

// Doctor returns every listening TCP port on the machine, flagged as wt-managed
// or foreign, so cross-project port squatters (e.g. an unrelated server on 8000)
// can be identified at a glance.
func Doctor() ([]DoctorRow, error) {
	owners, err := ownerMap()
	if err != nil {
		return nil, err
	}
	listeners, _ := AllListeners()
	return classify(listeners, owners), nil
}

// DoctorList writes a human-readable table of all listening ports.
func DoctorList(out io.Writer) error {
	rows, err := Doctor()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "LISTEN 中のポートはありません（ss 不在の可能性）")
		return nil
	}
	_, _ = fmt.Fprintf(out, "%-7s %-10s %-8s %s\n", "PORT", "PROC", "PID", "区分")
	for _, r := range rows {
		kind := "foreign"
		if r.Managed {
			kind = "wt:" + r.Owner
		}
		proc := r.Proc
		if proc == "" {
			proc = "-"
		}
		_, _ = fmt.Fprintf(out, "%-7d %-10s %-8d %s\n", r.Port, proc, r.PID, kind)
	}
	return nil
}
