package tree

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupWtContainer creates a minimal wt-managed container in a temp HOME dir
// and returns (containerDir, mainDir). The caller's HOME is redirected so that
// WorkspaceDirs() finds the test repo.
func setupWtContainer(t *testing.T) (containerDir, mainDir string) {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	containerDir = filepath.Join(tmpHome, "Workspace", "testrepo")
	mainDir = filepath.Join(containerDir, "main")
	if err := os.MkdirAll(mainDir, 0o755); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = mainDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}

	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(mainDir, "README.md"), []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "init")

	if err := os.WriteFile(filepath.Join(containerDir, ".worktrees.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	return containerDir, mainDir
}

// TestAddByBranch_ExistingBranchCheckedOut_ErrorContainsGitMessage verifies that
// when addByBranch fails because the target branch is already checked out,
// the returned error carries the git "fatal: ..." stderr text with a context prefix.
func TestAddByBranch_ExistingBranchCheckedOut_ErrorContainsGitMessage(t *testing.T) {
	_, _ = setupWtContainer(t)

	// "main" is the currently checked-out branch; trying to add a worktree for
	// it must trigger a fatal git error.
	var buf bytes.Buffer
	_, err := addByBranch(&buf, &AddOptions{
		Repo:   "testrepo",
		Branch: "main",
	})

	if err == nil {
		t.Fatal("expected error when adding worktree for already-checked-out branch, got nil")
	}

	if !strings.Contains(err.Error(), "既存ローカルブランチからの worktree 作成に失敗") {
		t.Errorf("error should contain context prefix, got: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "fatal") {
		t.Errorf("error should contain git stderr (e.g. 'fatal: ...'), got: %q", err.Error())
	}
}
