// Package status builds the one-screen view of every worktree's state that
// `wt worktree ls` and `wt daemon status` print: dev service state, ports,
// AutoStart, proxy URL, and whether `wt daemon` is running.
package status

import (
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"wt/internal/wt/core"
	"wt/internal/wt/daemon"
	"wt/internal/wt/ports"
	"wt/internal/wt/proxy"
	"wt/internal/wt/settings"
)

// Worktree states. They are stable strings meant to be matched by scripts.
const (
	StateRunning   = "running"   // at least one service is up and none is unhealthy
	StateUnhealthy = "unhealthy" // a service that should LISTEN is running without a socket
	StateIdle      = "idle"      // ports allocated, nothing up
	StateStale     = "stale"     // ports allocated but the worktree dir is gone
	StateNone      = "-"         // no ports allocated
)

// Service is one allocated port of a worktree.
type Service struct {
	Port      int    `json:"port"`
	Proc      string `json:"proc,omitempty"`
	PID       int    `json:"pid,omitempty"`
	Listening bool   `json:"listening"`
	Running   bool   `json:"running"`
	Headless  bool   `json:"headless"`
	Unhealthy bool   `json:"unhealthy"`
}

// Worktree is one row of the status view.
type Worktree struct {
	Repo      string    `json:"repo"`
	Worktree  string    `json:"worktree"`
	Branch    string    `json:"branch"`
	Path      string    `json:"path"`
	State     string    `json:"state"`
	Ports     string    `json:"ports"` // "9000-9004" or "-"
	AutoStart bool      `json:"autostart"`
	URL       string    `json:"url,omitempty"`
	Services  []Service `json:"services"`
}

// Daemon reports whether `wt daemon` holds its lock.
type Daemon struct {
	Running bool `json:"running"`
	PID     int  `json:"pid,omitempty"`
}

// Proxy is the configured built-in proxy.
type Proxy struct {
	Enabled bool   `json:"enabled"`
	Addr    string `json:"addr"`
}

// Report is the whole status view.
type Report struct {
	Daemon    Daemon     `json:"daemon"`
	Proxy     Proxy      `json:"proxy"`
	Worktrees []Worktree `json:"worktrees"`
}

// Input is everything Build needs, gathered by Collect. Kept separate so the
// assembly can be tested without touching the machine.
type Input struct {
	Rows      []ports.Row
	AutoStart map[string]bool // key: worktree path
	Routes    []proxy.Route
	Daemon    Daemon
	Proxy     settings.Proxy
	// DaemonProxy is the proxy the running daemon actually listens on. When
	// known it overrides the settings (the daemon may run with --no-proxy or
	// --proxy-port, or settings may have changed since it started).
	DaemonProxy      string
	DaemonProxyKnown bool
}

// Collect gathers the current machine state.
func Collect() (Report, error) {
	rows, err := ports.Status()
	if err != nil {
		return Report{}, err
	}
	in := Input{Rows: rows, AutoStart: autoStartFlags(rows), Proxy: settings.Load().Proxy}
	in.Routes, _ = proxy.Routes() // routes are best effort: no URL column on error
	if info, err := daemon.Probe(daemon.LockPath()); err == nil {
		in.Daemon = Daemon{Running: info.Running, PID: info.PID}
		in.DaemonProxy, in.DaemonProxyKnown = info.Proxy, info.ProxyKnown
	}
	return Build(&in), nil
}

// autoStartFlags reads each worktree's AutoStart from its own container (the
// parent dir of the worktree path), so same-named repos under different
// workspace dirs don't mix. Keyed by worktree path.
func autoStartFlags(rows []ports.Row) map[string]bool {
	out := map[string]bool{}
	loaded := map[string]map[string]core.Entry{}
	for _, r := range rows {
		container := filepath.Dir(r.Path)
		entries, ok := loaded[container]
		if !ok {
			entries, _ = core.LoadEntries(container)
			loaded[container] = entries
		}
		out[r.Path] = entries[r.WtName].AutoStart
	}
	return out
}

// Build assembles the report from gathered input.
func Build(in *Input) Report {
	bind := strings.TrimSpace(in.Proxy.Bind)
	if bind == "" {
		bind = settings.DefaultProxyBind
	}
	rep := Report{
		Daemon:    in.Daemon,
		Proxy:     Proxy{Enabled: in.Proxy.Enabled, Addr: net.JoinHostPort(bind, strconv.Itoa(in.Proxy.Port))},
		Worktrees: make([]Worktree, 0, len(in.Rows)),
	}
	if in.Daemon.Running && in.DaemonProxyKnown {
		rep.Proxy = Proxy{Enabled: in.DaemonProxy != "", Addr: in.DaemonProxy}
	}
	proxyPort := in.Proxy.Port
	if _, p, err := net.SplitHostPort(rep.Proxy.Addr); err == nil {
		proxyPort, _ = strconv.Atoi(p)
	}

	urls := map[string]string{}
	if rep.Proxy.Enabled {
		for _, rt := range in.Routes {
			urls[rt.Repo+"/"+rt.WtName] = "http://" + net.JoinHostPort(rt.Domain(), strconv.Itoa(proxyPort))
		}
	}

	for i := range in.Rows {
		r := &in.Rows[i]
		key := r.Repo + "/" + r.WtName
		wt := Worktree{
			Repo:      r.Repo,
			Worktree:  r.WtName,
			Branch:    r.Branch,
			Path:      r.Path,
			State:     stateOf(r),
			Ports:     StateNone,
			AutoStart: in.AutoStart[r.Path],
			URL:       urls[key],
			Services:  []Service{},
		}
		if r.PortBase != 0 {
			wt.Ports = ports.RangeString(r.PortBase)
		}
		for _, p := range r.Ports {
			wt.Services = append(wt.Services, Service{
				Port: p.Port, Proc: p.Proc, PID: p.PID,
				Listening: p.Listening, Running: p.Running,
				Headless: p.Headless, Unhealthy: p.Unhealthy(),
			})
		}
		rep.Worktrees = append(rep.Worktrees, wt)
	}
	return rep
}

func stateOf(r *ports.Row) string {
	if r.PortBase == 0 {
		return StateNone
	}
	if !r.Exists {
		return StateStale
	}
	up, unhealthy := false, false
	for _, p := range r.Ports {
		if p.Listening || p.Running {
			up = true
		}
		if p.Unhealthy() {
			unhealthy = true
		}
	}
	switch {
	case unhealthy:
		return StateUnhealthy
	case up:
		return StateRunning
	default:
		return StateIdle
	}
}

// TSVHeader is the column order of `wt worktree ls`. Scripts may rely on it.
var TSVHeader = []string{"REPO", "WORKTREE", "BRANCH", "STATE", "PORTS", "AUTOSTART", "URL"}
