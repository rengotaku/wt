package ports

import (
	"wt/internal/wt/devserver"
	"wt/internal/wt/settings"
)

// PortState is the live status of a single allocated port.
type PortState struct {
	Port      int
	Listening bool
	// Running reports that wt's recorded service for this port has a live PID.
	// A headless service (e.g. a worker/scheduler) binds no port, so it is
	// Running without being Listening — that is exactly the case where the
	// port-only view used to show it as idle.
	Running bool
	PID     int
	Proc    string
}

// Row combines a worktree's allocation with the live status of each of its
// ports. Ports is empty when the worktree has no allocation yet.
type Row struct {
	Repo     string
	WtName   string
	Branch   string
	Path     string
	PortBase int
	Ports    []PortState
	Exists   bool // whether the worktree directory still exists on disk
}

// Status returns one Row per worktree across all containers, with live status
// filled in for each allocated port: Listening from the machine's LISTEN
// sockets, and Running from wt's recorded service PIDs (so a headless service
// that binds no port still shows as up).
func Status() ([]Row, error) {
	allocs, err := Allocations()
	if err != nil {
		return nil, err
	}
	band := settings.Load().DevPorts
	listeners, _ := Listeners(band.Start, band.End)

	rows := make([]Row, 0, len(allocs))
	for _, a := range allocs {
		row := Row{
			Repo:     a.Repo,
			WtName:   a.WtName,
			Branch:   a.Branch,
			Path:     a.Path,
			PortBase: a.PortBase,
			Exists:   a.Exists,
		}
		// Recorded services with a live PID, keyed by port. Lets a headless
		// service that binds no port still register as running.
		alive := devserver.AliveByPort(a.Path)
		for _, p := range PortsForBase(a.PortBase) {
			st := PortState{Port: p}
			if l, ok := listeners[p]; ok {
				st.Listening = true
				st.PID = l.PID
				st.Proc = l.Proc
			}
			if s, ok := alive[p]; ok {
				st.Running = true
				// Fall back to the recorded service's identity when there is no
				// LISTEN socket to read the PID/name from.
				if st.PID == 0 {
					st.PID = s.PID
				}
				if st.Proc == "" {
					st.Proc = s.Name
				}
			}
			row.Ports = append(row.Ports, st)
		}
		rows = append(rows, row)
	}
	return rows, nil
}
