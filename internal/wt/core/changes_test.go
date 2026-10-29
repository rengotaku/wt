package core_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"wt/internal/wt/core"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func TestWorktreeChanges(t *testing.T) {
	dir := setupGitRepo(t)

	u, n, err := core.WorktreeChanges(dir)
	if err != nil || u != 0 || n != 0 {
		t.Fatalf("clean repo: got (%d, %d, %v), want (0, 0, nil)", u, n, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "memo.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	u, n, err = core.WorktreeChanges(dir)
	if err != nil || u != 0 || n != 1 {
		t.Fatalf("untracked only: got (%d, %d, %v), want (0, 1, nil)", u, n, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	u, n, err = core.WorktreeChanges(dir)
	if err != nil || u != 1 || n != 1 {
		t.Fatalf("modified + untracked: got (%d, %d, %v), want (1, 1, nil)", u, n, err)
	}
}

func TestWorktreeChanges_IgnoresShowUntrackedConfig(t *testing.T) {
	dir := setupGitRepo(t)
	gitIn(t, dir, "config", "status.showUntrackedFiles", "no")
	if err := os.WriteFile(filepath.Join(dir, "memo.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, n, err := core.WorktreeChanges(dir); err != nil || n != 1 {
		t.Fatalf("showUntrackedFiles=no: got (%d, %v), want (1, nil)", n, err)
	}
}

func TestWorktreeChanges_ErrorOnNonRepo(t *testing.T) {
	if _, _, err := core.WorktreeChanges(t.TempDir()); err == nil {
		t.Fatal("expected error for a non-git directory (must not report clean)")
	}
}

func TestUnpushedCommits(t *testing.T) {
	dir := setupGitRepo(t)

	// No remote at all: every commit exists only locally.
	if n, err := core.UnpushedCommits(dir); err != nil || n != 1 {
		t.Fatalf("no remote: got (%d, %v), want (1, nil)", n, err)
	}

	bare := t.TempDir()
	gitIn(t, bare, "init", "--bare", "-b", "main")
	gitIn(t, dir, "remote", "add", "origin", bare)
	gitIn(t, dir, "push", "-u", "origin", "main")
	if n, err := core.UnpushedCommits(dir); err != nil || n != 0 {
		t.Fatalf("after push: got (%d, %v), want (0, nil)", n, err)
	}

	gitIn(t, dir, "checkout", "-b", "feat")
	gitIn(t, dir, "commit", "--allow-empty", "-m", "local only")
	if n, err := core.UnpushedCommits(dir); err != nil || n != 1 {
		t.Fatalf("local commit on branch: got (%d, %v), want (1, nil)", n, err)
	}
}

func TestUnpushedCommits_ErrorOnNonRepo(t *testing.T) {
	if _, err := core.UnpushedCommits(t.TempDir()); err == nil {
		t.Fatal("expected error for a non-git directory")
	}
}
