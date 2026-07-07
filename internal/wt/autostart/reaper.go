package autostart

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"wt/internal/wt/core"
	"wt/internal/wt/devserver"
	"wt/internal/wt/ports"
	"wt/internal/wt/settings"
)

type Reaper struct {
	TTL, Interval time.Duration
	Now           func() time.Time
	Active        func(base int) bool
	Down          func(out io.Writer, worktree string) error
	last          map[string]time.Time // worktree path -> 最終活動時刻
}

func NewReaper(ttl, interval time.Duration) *Reaper {
	var lastEst map[int]bool
	var lastEstTime time.Time

	return &Reaper{
		TTL:      ttl,
		Interval: interval,
		Now:      time.Now,
		Active: func(base int) bool {
			// Cache Established results for 1 second to ensure it's pulled only
			// once per sweep (Tick).
			if time.Since(lastEstTime) > time.Second {
				s := settings.Load()
				lastEst, _ = ports.Established(s.DevPorts.Start, s.DevPorts.End)
				lastEstTime = time.Now()
			}
			for _, p := range ports.PortsForBase(base) {
				if lastEst[p] {
					return true
				}
			}
			return false
		},
		Down: devserver.Down,
		last: make(map[string]time.Time),
	}
}

func (r *Reaper) Run(ctx context.Context, out io.Writer) {
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Tick(out)
		}
	}
}

func (r *Reaper) Tick(out io.Writer) {
	for _, container := range core.ListContainers() {
		entries, err := core.LoadEntries(container)
		if err != nil {
			continue
		}
		for name := range entries {
			if !entries[name].Pinned {
				continue
			}
			worktree := filepath.Join(container, name)
			if fi, err := os.Stat(worktree); err != nil || !fi.IsDir() {
				continue
			}
			if !devserver.IsRunning(worktree) {
				delete(r.last, worktree)
				continue
			}
			base, err := ports.EnsureBase(container, name)
			if err != nil {
				continue
			}
			if r.Active(base) {
				r.last[worktree] = r.Now()
				continue
			}
			if _, ok := r.last[worktree]; !ok {
				r.last[worktree] = r.Now()
				continue
			}
			if r.Now().Sub(r.last[worktree]) > r.TTL {
				_ = r.Down(out, worktree)
				delete(r.last, worktree)
				fmt.Fprintf(out, "⏸ idle stop: %s\n", name)
			}
		}
	}
}
