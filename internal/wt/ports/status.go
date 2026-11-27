package ports

import (
	"log/slog"
	"time"

	"wt/internal/wt/devserver"
	"wt/internal/wt/listencache"
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
	// Headless reports that the dev service for this port is declared headless
	// (binds no port by design). It distinguishes a genuine worker/scheduler from
	// a port-binding service that failed to come up.
	Headless bool
	PID      int
	Proc     string
}

// Unhealthy reports a service that is expected to LISTEN but isn't: its process
// is alive yet it holds no LISTEN socket and was not declared headless. This is
// the build-failed / crashed-on-startup case, distinct from a genuine headless
// worker (Running && !Listening && Headless), which is benign.
func (p PortState) Unhealthy() bool {
	return p.Running && !p.Listening && !p.Headless
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
//
// Each call scans the sockets and saves the result for StatusCached.
func Status() ([]Row, error) {
	band := settings.Load().DevPorts
	return statusWith(RefreshListenerCache(band.Start, band.End))
}

// StatusCached is Status, but takes the LISTEN sockets from the cache when it
// is younger than maxAge, and scans (and saves) only otherwise. Everything
// else (worktrees, recorded PIDs, dev config) is read live. #205
func StatusCached(maxAge time.Duration) ([]Row, error) {
	band := settings.Load().DevPorts
	if entries, ok := listencache.Load(band.Start, band.End, maxAge, time.Now()); ok {
		listeners := make(map[int]Listener, len(entries))
		for _, e := range entries {
			listeners[e.Port] = Listener(e)
		}
		return statusWith(listeners)
	}
	return statusWith(RefreshListenerCache(band.Start, band.End))
}

// RefreshListenerCache scans the LISTEN sockets in [start, end], saves them
// for StatusCached and returns them. A failed save is logged, not returned:
// the scan itself is still good. When the scan itself fails (neither ss nor
// lsof ran) the old cache is dropped instead, so a failing scan in one
// environment (e.g. the daemon's) never stands in for a real result.
func RefreshListenerCache(start, end int) map[int]Listener {
	listeners, ok := scanListeners(start, end)
	if !ok {
		if err := listencache.Invalidate(); err != nil {
			slog.Debug("listener cache not invalidated", "err", err)
		}
		return listeners
	}
	entries := make([]listencache.Entry, 0, len(listeners))
	for _, l := range listeners {
		entries = append(entries, listencache.Entry(l))
	}
	if err := listencache.Save(start, end, entries, time.Now()); err != nil {
		slog.Debug("listener cache not saved", "err", err)
	}
	return listeners
}

// scanListeners is a var so tests can stand in for the socket scan.
var scanListeners = scanSockets

func statusWith(listeners map[int]Listener) ([]Row, error) {
	allocs, err := Allocations()
	if err != nil {
		return nil, err
	}

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
		// Effective dev config maps service i → port base+i, so its Headless
		// declaration tells apart a genuine worker from a failed-to-listen server.
		// Caveat: this reads the *current* config, not the one in effect when the
		// services were last started. Editing .wt/dev.toml (reordering/toggling
		// headless) while old processes are still running can momentarily mislabel
		// a service's Headless/health until the next `wt service up`; restart to resync.
		cfg, _, _ := devserver.EffectiveConfig(a.Path)
		for i, p := range PortsForBase(a.PortBase) {
			st := PortState{Port: p}
			if i < len(cfg.Services) {
				st.Headless = cfg.Services[i].Headless
			}
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
