package autostart

import (
	"context"
	"fmt"
	"io"
	"time"

	"wt/internal/wt/ports"
)

// PortReaper periodically prunes "ghost" port allocations: registry entries
// whose worktree directory has been removed outside `wt tree rm` (a manual
// `git worktree remove`, a deleted directory, an interrupted create) but whose
// port_base still lingers, permanently withholding a block from the dev band.
//
// It reuses ports.Prune, the exact same logic backing the manual
// `GET /api/ports/stale` / `POST /api/ports/prune` endpoints, so automatic and
// manual pruning always agree on what counts as stale (a directory-less entry
// that still owns a port_base — see ports.Stale). A worktree that still exists
// on disk is never touched.
type PortReaper struct {
	Interval time.Duration
	Prune    func(dryRun bool) ([]ports.Allocation, error)
}

// NewPortReaper builds a PortReaper that ticks every interval and delegates
// pruning to ports.Prune.
func NewPortReaper(interval time.Duration) *PortReaper {
	return &PortReaper{
		Interval: interval,
		Prune:    ports.Prune,
	}
}

// Run prunes once immediately — so ghosts left over from a previous session
// are cleared at startup rather than after a full Interval (the default is a
// whole day) — then ticks Tick every Interval until ctx is canceled.
func (r *PortReaper) Run(ctx context.Context, out io.Writer) {
	r.Tick(out)
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

// Tick prunes every stale (ghost) port allocation once, reporting each removed
// entry to out. Errors are reported and swallowed — a failed sweep must not
// crash the wt web process, only be retried on the next tick.
func (r *PortReaper) Tick(out io.Writer) {
	removed, err := r.Prune(false)
	if err != nil {
		_, _ = fmt.Fprintf(out, "⚠️  ghost port reaper: prune に失敗: %v\n", err)
		return
	}
	for _, a := range removed {
		_, _ = fmt.Fprintf(out, "🧹 ghost port prune: %s/%s (port %s)\n", a.Repo, a.WtName, ports.RangeString(a.PortBase))
	}
}
