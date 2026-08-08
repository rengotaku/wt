package handler

import (
	"bytes"
	"log/slog"

	"wt/internal/wt/devserver"
	"wt/internal/wt/ports"
)

// restartDevIfRunning restarts the worktree's dev services when they are
// currently running, preserving the allocated port block (ports.EnsureBase is
// idempotent for an already-allocated worktree, so the same base is reused).
// Returns true if a restart was performed. A worktree with nothing running is a
// no-op (false, nil) — pull must never *start* a dev server that wasn't up.
var restartDevIfRunning = defaultRestartDevIfRunning

func defaultRestartDevIfRunning(container, wtName, worktree string) (bool, error) {
	if !devserver.IsRunning(worktree) {
		return false, nil
	}
	base, err := ports.EnsureBase(container, wtName)
	if err != nil {
		return false, err
	}
	var buf bytes.Buffer
	slog.Info("devserver action", "worktree", worktree, "action", "down", "trigger", "pull-restart")
	if err := devserver.Down(&buf, worktree); err != nil {
		return false, err
	}
	slog.Info("devserver action", "worktree", worktree, "action", "serve", "trigger", "pull-restart")
	if err := devserver.Serve(&buf, worktree, base); err != nil {
		return false, err
	}
	return true, nil
}
