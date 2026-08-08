package autostart

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"wt/internal/wt/core"
	"wt/internal/wt/devserver"
	"wt/internal/wt/ports"
)

// retryState tracks crash-recovery attempts for a single worktree within a
// cooldown window. windowStart resets whenever a Tick sees the worktree
// crashed after the previous window has elapsed, so a worktree that keeps
// flapping is retried at most MaxRetries times per Cooldown period rather
// than forever.
type retryState struct {
	count       int
	windowStart time.Time
}

// HealthReaper periodically detects dev services that died without being
// explicitly stopped (a crash, as opposed to a deliberate `wt dev down` /
// idle-reaper stop) and re-serves them. It is intentionally a separate
// component from Reaper (which only ever stops idle worktrees): the two have
// opposite trigger conditions and mixing them would make Reaper's idle logic
// harder to reason about. #137
//
// A worktree is considered crashed when devserver.Recorded returns a non-empty
// list (it has been served and never explicitly Down'ed — Down always clears
// the record) while devserver.IsRunning reports false (every recorded service
// has died). This intentionally only covers the "all services dead" case;
// partial (degraded) failures are out of scope (#137).
//
// devserver.Serve always calls Down internally before (re)starting a service,
// so a Serve call that fails to start still leaves running.json empty/absent.
// Tick therefore does not treat "Recorded is empty" as a hard signal that the
// worktree was never served or was explicitly stopped: if r.retries already
// has an in-progress crash-recovery attempt for this worktree, Tick keeps
// calling recover (which owns the MaxRetries/Cooldown accounting) instead of
// silently dropping the retry count and abandoning recovery after a single
// failed Serve.
//
// Cooldown/MaxRetries bound how many times a single worktree is retried
// within a rolling window, so a worktree whose dev config is simply broken
// cannot make HealthReaper itself become a new failure amplifier (spawning
// systemd scopes forever). For the same reason, observing IsRunning==true
// does not unconditionally clear r.retries: a worktree that keeps
// crashing/recovering within a single Cooldown window must not have its
// attempt counter reset by an intermediate successful recovery, or the
// MaxRetries guard tested by Case 4 would never trigger. The counter is only
// cleared once the Cooldown window has actually elapsed.
type HealthReaper struct {
	Interval   time.Duration
	Cooldown   time.Duration
	MaxRetries int
	Now        func() time.Time
	Serve      func(out io.Writer, worktree string, base int) error
	Down       func(out io.Writer, worktree string) error
	IsRunning  func(worktree string) bool
	Recorded   func(worktree string) []devserver.RunningService
	retries    map[string]retryState // worktree path -> crash-recovery attempt state
}

// NewHealthReaper builds a HealthReaper that ticks every interval, allowing
// up to maxRetries recovery attempts per worktree within a rolling cooldown
// window.
func NewHealthReaper(interval, cooldown time.Duration, maxRetries int) *HealthReaper {
	return &HealthReaper{
		Interval:   interval,
		Cooldown:   cooldown,
		MaxRetries: maxRetries,
		Now:        time.Now,
		Serve:      devserver.Serve,
		Down:       devserver.Down,
		IsRunning:  devserver.IsRunning,
		Recorded:   devserver.Recorded,
		retries:    make(map[string]retryState),
	}
}

// Run ticks Tick every Interval until ctx is canceled.
func (r *HealthReaper) Run(ctx context.Context, out io.Writer) {
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

// Tick sweeps every wt-managed container once, re-serving any worktree whose
// recorded services have all died without an explicit Down.
func (r *HealthReaper) Tick(out io.Writer) {
	for _, container := range core.ListContainers() {
		entries, err := core.LoadEntries(container)
		if err != nil {
			continue
		}
		for name := range entries {
			worktree := filepath.Join(container, name)
			if fi, err := os.Stat(worktree); err != nil || !fi.IsDir() {
				delete(r.retries, worktree) // stale entry: nothing to track
				continue
			}
			if len(r.Recorded(worktree)) == 0 {
				if _, tracking := r.retries[worktree]; !tracking {
					// Never served, or explicitly Down'ed (Down always
					// clears running.json) — never auto-start a worktree
					// the user hasn't opted into running.
					continue
				}
				// A crash-recovery attempt is already in flight for this
				// worktree. devserver.Serve always calls Down before
				// starting, so a Serve call that failed to start also
				// leaves running.json empty — fall through to recover,
				// which owns the MaxRetries/Cooldown accounting, instead
				// of treating the failed attempt as "never served".
			} else if r.IsRunning(worktree) {
				// Alive (or recovered since the last crash). Only clear
				// the attempt counter once the cooldown window has
				// elapsed — clearing it on every successful recovery
				// would let a worktree that keeps flapping within a
				// single window dodge the MaxRetries guard forever.
				if st, tracking := r.retries[worktree]; !tracking || r.Now().Sub(st.windowStart) > r.Cooldown {
					delete(r.retries, worktree)
				}
				continue
			}
			r.recover(out, container, name, worktree)
		}
	}
}

// recover attempts to re-serve a crashed worktree, subject to the
// Cooldown/MaxRetries guard.
func (r *HealthReaper) recover(out io.Writer, container, name, worktree string) {
	now := r.Now()
	st, ok := r.retries[worktree]
	if !ok || now.Sub(st.windowStart) > r.Cooldown {
		st = retryState{windowStart: now}
	}
	if st.count >= r.MaxRetries {
		r.retries[worktree] = st
		slog.Warn("crash-recovery giving up", "worktree", worktree, "attempts", st.count)
		_, _ = fmt.Fprintf(out, "⚠️  crash-recovery: %s は直近 %d 回試行済みのため一時停止します（%s 経過後に再試行）\n",
			name, st.count, r.Cooldown)
		return
	}
	base, err := ports.EnsureBase(container, name)
	if err != nil {
		r.retries[worktree] = st
		_, _ = fmt.Fprintf(out, "⚠️  crash-recovery: %s のポート割当に失敗: %v\n", name, err)
		return
	}
	st.count++
	r.retries[worktree] = st
	slog.Info("devserver action", "worktree", worktree, "action", "serve", "trigger", "crash-recovery")
	if err := r.Serve(out, worktree, base); err != nil {
		_, _ = fmt.Fprintf(out, "⚠️  crash-recovery: %s の再起動に失敗: %v\n", name, err)
		return
	}
	_, _ = fmt.Fprintf(out, "♻️  crash-recovery: %s を再起動しました\n", name)
}
