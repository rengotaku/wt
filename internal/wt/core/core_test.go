package core_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/wt/core"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}

	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "init")

	return dir
}

// TestGitRun_ErrorContainsStderr verifies that when a git command fails,
// the returned error contains the git stderr text (e.g. "fatal: ..."),
// not just the opaque "exit status N".
func TestGitRun_ErrorContainsStderr(t *testing.T) {
	dir := setupGitRepo(t)

	// "main" is already checked out — adding a worktree for it must fail
	// with a "fatal: already checked out" message from git.
	worktreePath := t.TempDir()
	err := core.GitRun(dir, "worktree", "add", worktreePath, "main")
	if err == nil {
		t.Fatal("expected error when adding worktree for already-checked-out branch, got nil")
	}

	if !strings.Contains(err.Error(), "fatal") {
		t.Errorf("error should contain git stderr (e.g. 'fatal: ...'), got: %q", err.Error())
	}
}
