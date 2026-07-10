package autostart

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wt/internal/wt/core"
	"wt/internal/wt/ports"
)

func TestPortReaper_Tick_PrunesGhostsOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "myrepo")

	// "live" worktree: directory exists on disk → must survive the sweep.
	live := makeWorktree(t, container, "live", 9000, false)
	if _, err := os.Stat(live); err != nil {
		t.Fatal(err)
	}

	// "ghost" worktree: registered with a port_base, but its directory was
	// removed outside `wt tree rm` (manual rm, interrupted create, etc.).
	if err := core.PutEntry(container, "ghost", &core.Entry{Type: "feature", PortBase: 9005}); err != nil {
		t.Fatal(err)
	}

	r := NewPortReaper(24 * time.Hour)

	var buf bytes.Buffer
	r.Tick(&buf)

	entries, err := core.LoadEntries(container)
	if err != nil {
		t.Fatalf("LoadEntries: %v", err)
	}
	if _, ok := entries["ghost"]; ok {
		t.Error("ghost entry should have been pruned")
	}
	if _, ok := entries["live"]; !ok {
		t.Error("live entry (real worktree dir) must survive the sweep")
	}
	if !strings.Contains(buf.String(), "ghost") {
		t.Errorf("expected the pruned ghost to be reported, got: %q", buf.String())
	}

	// A second sweep is a no-op: nothing left to prune.
	buf.Reset()
	r.Tick(&buf)
	if buf.Len() != 0 {
		t.Errorf("second tick should report nothing, got: %q", buf.String())
	}
}

func TestPortReaper_Tick_ReportsPruneError(t *testing.T) {
	r := &PortReaper{
		Interval: time.Hour,
		Prune: func(bool) ([]ports.Allocation, error) {
			return nil, os.ErrPermission
		},
	}
	var buf bytes.Buffer
	r.Tick(&buf)
	if !strings.Contains(buf.String(), "失敗") {
		t.Errorf("expected a failure message, got: %q", buf.String())
	}
}

func TestPortReaper_Run_StopsOnContextCancel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	called := make(chan struct{}, 1)
	r := &PortReaper{
		Interval: time.Millisecond,
		Prune: func(bool) ([]ports.Allocation, error) {
			select {
			case called <- struct{}{}:
			default:
			}
			return nil, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		r.Run(ctx, &bytes.Buffer{})
		close(done)
	}()

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("Prune was never invoked by Run")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}
