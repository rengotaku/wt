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

func hasHeadless(cfg devserver.Config) bool {
	for _, s := range cfg.Services {
		if s.Headless {
			return true
		}
	}
	return false
}

func (r *Reaper) Tick(out io.Writer) {
	for _, container := range core.ListContainers() {
		entries, err := core.LoadEntries(container)
		if err != nil {
			continue
		}
		for name := range entries {
			if !entries[name].AutoStart {
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
			// Headless サービス（worker/scheduler 等、ポート listen しない設計）を
			// 1つでも含む worktree は idle 判定の対象外にする: reaper は ESTABLISHED な
			// TCP 接続だけを見て活動を判定するため、常駐バックグラウンド処理を
			// 「アイドル」と誤検知して停止してしまう。#129
			if cfg, _, err := devserver.EffectiveConfig(worktree); err == nil && hasHeadless(cfg) {
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
				_, _ = fmt.Fprintf(out, "⏸ idle stop: %s\n", name)
			}
		}
	}
}
