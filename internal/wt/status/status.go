// Package status builds the one-screen view of every worktree's state that
// `wt status` prints: dev service state, ports, AutoStart, proxy URL, and
// whether `wt daemon` is running.
package status

import (
	"net"
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
	AutoStart map[string]bool // key: repo + "/" + worktree
	Routes    []proxy.Route
	Daemon    Daemon
	Proxy     settings.Proxy
}

// Collect gathers the current machine state.
func Collect() (Report, error) {
	rows, err := ports.Status()
	if err != nil {
		return Report{}, err
	}
	in := Input{Rows: rows, AutoStart: autoStartFlags(rows), Proxy: settings.Load().Proxy}
	in.Routes, _ = proxy.Routes() // routes are best effort: no URL column on error
	running, pid, err := daemon.Probe(daemon.LockPath())
	if err == nil {
		in.Daemon = Daemon{Running: running, PID: pid}
	}
	return Build(&in), nil
}

func autoStartFlags(rows []ports.Row) map[string]bool {
	out := map[string]bool{}
	loaded := map[string]bool{}
	for _, r := range rows {
		if loaded[r.Repo] {
			continue
		}
		loaded[r.Repo] = true
		container, err := core.FindContainer(r.Repo)
		if err != nil {
			continue
		}
		entries, err := core.LoadEntries(container)
		if err != nil {
			continue
		}
		for name := range entries {
			out[r.Repo+"/"+name] = entries[name].AutoStart
		}
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

	urls := map[string]string{}
	if in.Proxy.Enabled {
		for _, rt := range in.Routes {
			urls[rt.Repo+"/"+rt.WtName] = "http://" + net.JoinHostPort(rt.Domain(), strconv.Itoa(in.Proxy.Port))
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
			AutoStart: in.AutoStart[key],
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
