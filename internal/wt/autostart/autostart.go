// Package autostart serves worktrees that are marked Pinned in .worktrees.json
// when the wt web server starts, so pinned dev environments come up without a
// manual "serve" click.
package autostart

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"wt/internal/wt/core"
	"wt/internal/wt/devserver"
	"wt/internal/wt/ports"
)

// ServePinned starts dev services for every pinned worktree that is not already
// running, across all wt-managed containers. It returns the number of worktrees
// it (re)started. Errors for a single worktree are reported to out and skipped —
// one broken worktree must not block the rest of the sweep.
//
// Worktrees that are already running are left untouched: when wt web is merely
// restarted, in-flight dev servers keep running rather than being bounced.
func ServePinned(out io.Writer) int {
	served := 0
	for _, container := range core.ListContainers() {
		entries, err := core.LoadEntries(container)
		if err != nil {
			continue
		}
		for name := range entries {
			// Index rather than bind the value to avoid copying the Entry struct.
			if !entries[name].Pinned {
				continue
			}
			worktree := filepath.Join(container, name)
			if fi, err := os.Stat(worktree); err != nil || !fi.IsDir() {
				continue // stale entry: the worktree directory is gone
			}
			if devserver.IsRunning(worktree) {
				continue // already up — don't disrupt a running environment
			}
			base, err := ports.EnsureBase(container, name)
			if err != nil {
				_, _ = fmt.Fprintf(out, "⚠️  pin auto-serve: %s のポート割当に失敗: %v\n", name, err)
				continue
			}
			if err := devserver.Serve(out, worktree, base); err != nil {
				_, _ = fmt.Fprintf(out, "⚠️  pin auto-serve: %s の起動に失敗: %v\n", name, err)
				continue
			}
			served++
		}
	}
	return served
}
