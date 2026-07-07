package autostart

import (
	"bytes"
	"io"
	"path/filepath"
	"testing"
	"time"

	"wt/internal/wt/devserver"
)

func TestReaper_Tick(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // isolate devserver run state
	container := filepath.Join(home, "Workspace", "myrepo")

	// Pinned and running worktree
	wt := makeWorktree(t, container, "wtpinned", 9000, true)

	var buf bytes.Buffer
	// Emulate it being running
	if err := devserver.Serve(&buf, wt, 9000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = devserver.Down(&buf, wt)
	})

	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	downCalled := 0

	r := &Reaper{
		TTL:      30 * time.Minute,
		Interval: 2 * time.Minute,
		Now:      func() time.Time { return now },
		Active:   func(base int) bool { return false },
		Down: func(out io.Writer, worktree string) error {
			downCalled++
			return devserver.Down(out, worktree)
		},
		last: make(map[string]time.Time),
	}

	// 1. 初回 Tick (last 未記録 → 初回グレース、Down されない)
	r.Tick(&buf)
	if downCalled != 0 {
		t.Errorf("expected 0 Down calls on first tick, got %d", downCalled)
	}
	if !devserver.IsRunning(wt) {
		t.Error("worktree should still be running")
	}
	if _, ok := r.last[wt]; !ok {
		t.Error("last map should be updated with first seen time")
	}

	// 2. TTL 未満 (Now を進めるが 30m 未満)
	now = now.Add(15 * time.Minute)
	r.Tick(&buf)
	if downCalled != 0 {
		t.Errorf("expected 0 Down calls within TTL, got %d", downCalled)
	}

	// 3. 活動中なら更新される (Active = true)
	r.Active = func(base int) bool { return true }
	now = now.Add(15 * time.Minute) // 12:30
	r.Tick(&buf)
	if downCalled != 0 {
		t.Errorf("expected 0 Down calls when active, got %d", downCalled)
	}

	// 4. 無活動で TTL 超過 (Active=false に戻し、TTL 31分経過)
	r.Active = func(base int) bool { return false }
	now = now.Add(31 * time.Minute) // 13:01 (31m since last active update at 12:30)
	r.Tick(&buf)
	if downCalled != 1 {
		t.Errorf("expected 1 Down call after TTL exceeded, got %d", downCalled)
	}
	if devserver.IsRunning(wt) {
		t.Error("worktree should be down after TTL")
	}
	if _, ok := r.last[wt]; ok {
		t.Error("worktree should be removed from last map")
	}
}
