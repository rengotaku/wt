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
	if err := os.WriteFile(readme, []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "init")

	// Modify the file to make the worktree dirty.
	if err := os.WriteFile(readme, []byte("changed\n"), 0o644); err != nil {
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
	if err := os.WriteFile(readme, []byte("init\n"), 0o644); err != nil {
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

// TestEligibleForMerged_RepoFilter は issue #88 の回帰テスト。
// --repo 指定時、他リポの worktree が --merged の対象に混入しないことを保証する。
func TestEligibleForMerged_RepoFilter(t *testing.T) {
	items := []RmEntry{
		{Repo: "stock-dashboard", WtName: "stock-dashboard--feat-1"},
		{Repo: "stock-dashboard", WtName: "main"},
		{Repo: "saas-readiness", WtName: "saas-readiness--feat-issue-81"},
		{Repo: "saas-readiness", WtName: "saas-readiness--fix-issue-69"},
	}

	// --repo 指定: 対象リポの非 main のみ。
	got := eligibleForMerged(items, "stock-dashboard")
	if len(got) != 1 || got[0].WtName != "stock-dashboard--feat-1" {
		t.Fatalf("repo filter = %+v, want [stock-dashboard--feat-1]", got)
	}
	for _, e := range got {
		if e.Repo != "stock-dashboard" {
			t.Errorf("他リポが混入: %s/%s", e.Repo, e.WtName)
		}
	}

	// --repo 無し: 全リポの非 main/master。
	all := eligibleForMerged(items, "")
	if len(all) != 3 {
		t.Errorf("no filter = %d 件, want 3 (main を除く全て)", len(all))
	}
}
