package tree

import (
	"os"
	"path/filepath"
	"testing"

	"wt/internal/wt/core"
)

func TestSetAutoStart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "repo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "repo--feat", &core.Entry{Branch: "feat"}); err != nil {
		t.Fatal(err)
	}

	if err := SetAutoStart("repo", "repo--feat", true); err != nil {
		t.Fatalf("on: %v", err)
	}
	entries, err := core.LoadEntries(container)
	if err != nil {
		t.Fatal(err)
	}
	if got := entries["repo--feat"]; !got.AutoStart || got.Branch != "feat" {
		t.Errorf("after on: got %+v, want AutoStart=true and Branch kept", got)
	}

	if err := SetAutoStart("repo", "repo--feat", false); err != nil {
		t.Fatalf("off: %v", err)
	}
	entries, _ = core.LoadEntries(container)
	if entries["repo--feat"].AutoStart {
		t.Error("after off: AutoStart still true")
	}
}

func TestSetAutoStart_Errors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "repo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := SetAutoStart("missing", "x", true); err == nil {
		t.Error("unknown repo: want error")
	}
	if err := SetAutoStart("repo", "missing", true); err == nil {
		t.Error("unknown worktree: want error")
	}
}
