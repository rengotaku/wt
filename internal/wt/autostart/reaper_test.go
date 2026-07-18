package autostart

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wt/internal/wt/core"
	"wt/internal/wt/devserver"
)

func TestReaper_Tick(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // isolate devserver run state
	container := filepath.Join(home, "Workspace", "myrepo")

	// AutoStart and running worktree
	wt := makeWorktree(t, container, "wtauto", 9000, true)

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

// TestReaper_Tick_SkipsHeadlessWorktree ensures a worktree whose dev config
// contains any Headless=true service is never idle-stopped by the reaper.
// Headless services (workers/schedulers) don't open TCP listeners, so the
// port-based Active check would spuriously flag them as idle. #129
func TestReaper_Tick_SkipsHeadlessWorktree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	container := filepath.Join(home, "Workspace", "myrepo")

	wt := filepath.Join(container, "wtheadless")
	if err := os.MkdirAll(filepath.Join(wt, ".wt"), 0o755); err != nil {
		t.Fatal(err)
	}
	// web (listens) + worker (headless). Realistic mixed config.
	tomlBody := "" +
		"[[services]]\nname = \"web\"\ncmd = \"sleep 30\"\n" +
		"[[services]]\nname = \"worker\"\ncmd = \"sleep 30\"\nheadless = true\n"
	if err := os.WriteFile(filepath.Join(wt, ".wt", "dev.toml"), []byte(tomlBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "wtheadless", &core.Entry{Type: "feature", PortBase: 9030, AutoStart: true}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := devserver.Serve(&buf, wt, 9030); err != nil {
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

	// Sweep well past TTL. Without the headless skip, the second sweep would
	// arm r.last[wt] and the third would call Down.
	r.Tick(&buf)
	now = now.Add(31 * time.Minute)
	r.Tick(&buf)
	now = now.Add(31 * time.Minute)
	r.Tick(&buf)

	if downCalled != 0 {
		t.Errorf("expected 0 Down calls for a worktree with a headless service, got %d", downCalled)
	}
	if _, ok := r.last[wt]; ok {
		t.Error("worktree with headless service must not be tracked by the reaper")
	}
	if !devserver.IsRunning(wt) {
		t.Error("worktree with headless service should remain running")
	}
}

// TestReaper_Tick_IgnoresPinnedWithoutAutoStart ensures Pinned alone does not
// make a worktree a reaper target: Pinned only controls list ordering, while
// the reaper (like auto-serve) is keyed off AutoStart.
func TestReaper_Tick_IgnoresPinnedWithoutAutoStart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	container := filepath.Join(home, "Workspace", "myrepo")

	wt := filepath.Join(container, "wtpinnedonly")
	if err := os.MkdirAll(filepath.Join(wt, ".wt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".wt", "dev.toml"), []byte(sleeperToml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "wtpinnedonly", &core.Entry{Type: "feature", PortBase: 9020, Pinned: true, AutoStart: false}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := devserver.Serve(&buf, wt, 9020); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = devserver.Down(&buf, wt)
	})

	downCalled := 0
	r := &Reaper{
		TTL:      30 * time.Minute,
		Interval: 2 * time.Minute,
		Now:      func() time.Time { return time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC) },
		Active:   func(base int) bool { return false },
		Down: func(out io.Writer, worktree string) error {
			downCalled++
			return devserver.Down(out, worktree)
		},
		last: make(map[string]time.Time),
	}

	r.Tick(&buf)
	if downCalled != 0 {
		t.Errorf("expected 0 Down calls for a pinned-only worktree, got %d", downCalled)
	}
	if _, ok := r.last[wt]; ok {
		t.Error("pinned-only worktree must not be tracked by the reaper")
	}
	if !devserver.IsRunning(wt) {
		t.Error("pinned-only worktree should remain running (reaper must ignore it)")
	}
}
