package autostart

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"wt/internal/wt/core"
	"wt/internal/wt/devserver"
)

const sleeperToml = "[[services]]\nname = \"sleeper\"\ncmd = \"sleep 30\"\n"

// makeWorktree creates a worktree dir with a .wt/dev.toml and registers an entry
// (with a fixed PortBase so EnsureBase does not need to allocate).
func makeWorktree(t *testing.T, container, name string, base int, autoStart bool) string {
	t.Helper()
	wt := filepath.Join(container, name)
	if err := os.MkdirAll(filepath.Join(wt, ".wt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".wt", "dev.toml"), []byte(sleeperToml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, name, &core.Entry{Type: "feature", PortBase: base, AutoStart: autoStart}); err != nil {
		t.Fatal(err)
	}
	return wt
}

func TestServeAutoStart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // isolate devserver run state
	container := filepath.Join(home, "Workspace", "myrepo")

	autoStarted := makeWorktree(t, container, "wtauto", 9000, true)
	idle := makeWorktree(t, container, "wtidle", 9010, false)
	t.Cleanup(func() {
		_ = devserver.Down(&bytes.Buffer{}, autoStarted)
		_ = devserver.Down(&bytes.Buffer{}, idle)
	})

	var buf bytes.Buffer
	if n := ServeAutoStart(&buf); n != 1 {
		t.Fatalf("ServeAutoStart served %d worktrees, want 1\n%s", n, buf.String())
	}
	if !devserver.IsRunning(autoStarted) {
		t.Error("auto-start worktree should be running after ServeAutoStart")
	}
	if devserver.IsRunning(idle) {
		t.Error("non-auto-start worktree must not be served")
	}

	// A second sweep must not disrupt or double-start an already-running worktree.
	var buf2 bytes.Buffer
	if n := ServeAutoStart(&buf2); n != 0 {
		t.Errorf("second ServeAutoStart served %d, want 0 (already running)", n)
	}
	if !devserver.IsRunning(autoStarted) {
		t.Error("auto-start worktree should still be running after idempotent sweep")
	}
}

func TestServeAutoStart_SkipsMissingDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	container := filepath.Join(home, "Workspace", "myrepo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	// AutoStart entry whose worktree directory does not exist (stale registry).
	if err := core.PutEntry(container, "ghost", &core.Entry{PortBase: 9000, AutoStart: true}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if n := ServeAutoStart(&buf); n != 0 {
		t.Errorf("ServeAutoStart served %d for a missing worktree dir, want 0", n)
	}
}
