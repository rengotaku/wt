package tree

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupDirtyRepo creates a temp git repo with an uncommitted change and
// returns its path.
func setupDirtyRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")

	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("init\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "init")

	// Modify the file to make the worktree dirty.
	if err := os.WriteFile(readme, []byte("changed\n"), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestPerformDelete_DirtyWorktree_ReturnsError(t *testing.T) {
	dir := setupDirtyRepo(t)

	entry := &RmEntry{
		WtName:    "feature-branch",
		WtPath:    dir,
		MainDir:   dir,
		Container: dir,
	}

	var buf bytes.Buffer
	err := performDelete(&buf, RmOptions{Force: false}, entry)

	if err == nil {
		t.Fatal("expected error for dirty worktree, got nil")
	}
	if !strings.Contains(err.Error(), "未コミット変更") {
		t.Errorf("expected error to mention uncommitted changes, got: %v", err)
	}
}

func TestPerformDelete_DirtyWorktree_WithForce_NoError(t *testing.T) {
	dir := setupDirtyRepo(t)

	// Use a container dir that exists (same as wtPath for simplicity).
	// We don't care about the actual deletion succeeding in the test env,
	// only that the dirty check is bypassed when Force=true.
	entry := &RmEntry{
		WtName:    "feature-branch",
		WtPath:    dir,
		MainDir:   dir,
		Container: dir,
	}

	var buf bytes.Buffer
	// Force=true should bypass the dirty check. The deletion itself may fail
	// (not a real worktree), but the dirty-check guard must not block it.
	err := performDelete(&buf, RmOptions{Force: true, DryRun: true}, entry)

	if err != nil {
		t.Errorf("unexpected error with Force=true: %v", err)
	}
}

func TestPerformDelete_CleanWorktree_NoError(t *testing.T) {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")

	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("init\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "init")

	entry := &RmEntry{
		WtName:    "feature-branch",
		WtPath:    dir,
		MainDir:   dir,
		Container: dir,
	}

	var buf bytes.Buffer
	// DryRun=true so we don't attempt actual deletion; just verify the dirty
	// check passes for a clean repo.
	err := performDelete(&buf, RmOptions{Force: false, DryRun: true}, entry)

	if err != nil {
		t.Errorf("unexpected error for clean worktree: %v", err)
	}
}
