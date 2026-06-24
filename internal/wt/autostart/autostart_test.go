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
func makeWorktree(t *testing.T, container, name string, base int, pinned bool) string {
	t.Helper()
	wt := filepath.Join(container, name)
	if err := os.MkdirAll(filepath.Join(wt, ".wt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".wt", "dev.toml"), []byte(sleeperToml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, name, &core.Entry{Type: "feature", PortBase: base, Pinned: pinned}); err != nil {
		t.Fatal(err)
	}
	return wt
}

func TestServePinned(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // isolate devserver run state
	container := filepath.Join(home, "Workspace", "myrepo")

	pinned := makeWorktree(t, container, "wtpinned", 9000, true)
	idle := makeWorktree(t, container, "wtidle", 9010, false)
	t.Cleanup(func() {
		_ = devserver.Down(&bytes.Buffer{}, pinned)
		_ = devserver.Down(&bytes.Buffer{}, idle)
	})

	var buf bytes.Buffer
	if n := ServePinned(&buf); n != 1 {
		t.Fatalf("ServePinned served %d worktrees, want 1\n%s", n, buf.String())
	}
	if !devserver.IsRunning(pinned) {
		t.Error("pinned worktree should be running after ServePinned")
	}
	if devserver.IsRunning(idle) {
		t.Error("unpinned worktree must not be served")
	}

	// A second sweep must not disrupt or double-start an already-running worktree.
	var buf2 bytes.Buffer
	if n := ServePinned(&buf2); n != 0 {
		t.Errorf("second ServePinned served %d, want 0 (already running)", n)
	}
	if !devserver.IsRunning(pinned) {
		t.Error("pinned worktree should still be running after idempotent sweep")
	}
}

func TestServePinned_SkipsMissingDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	container := filepath.Join(home, "Workspace", "myrepo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pinned entry whose worktree directory does not exist (stale registry).
	if err := core.PutEntry(container, "ghost", &core.Entry{PortBase: 9000, Pinned: true}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if n := ServePinned(&buf); n != 0 {
		t.Errorf("ServePinned served %d for a missing worktree dir, want 0", n)
	}
}
