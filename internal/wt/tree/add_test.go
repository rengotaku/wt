package tree

import (
	"bytes"
	"encoding/json"
	"errors"
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

// writeSymlinkCandidates replaces the container's _config.symlink_candidates.
func writeSymlinkCandidates(t *testing.T, containerDir string, paths ...string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"_config": map[string]any{"symlink_candidates": paths},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(containerDir, ".worktrees.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertSymlinkTo fails unless link is a symlink pointing at want.
func assertSymlinkTo(t *testing.T, link, want string) {
	t.Helper()
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("symlink が無い: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s は symlink ではない（mode=%v）", link, fi.Mode())
	}
	got, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("symlink 先 = %q, want %q", got, want)
	}
}

// assertAbsent fails when path exists at all (symlink or real entry).
func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s が存在する（err=%v）", path, err)
	}
}

// TestAddByBranch_SymlinksCandidatesByDefault verifies the default is now "on":
// registered candidates become symlinks to the sibling main worktree.
func TestAddByBranch_SymlinksCandidatesByDefault(t *testing.T) {
	containerDir, mainDir := setupWtContainer(t)
	if err := os.Mkdir(filepath.Join(mainDir, ".venv"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSymlinkCandidates(t, containerDir, ".venv")

	var buf bytes.Buffer
	if _, err := addByBranch(&buf, &AddOptions{Repo: "testrepo", Branch: "feat/link"}); err != nil {
		t.Fatalf("addByBranch: %v\n%s", err, buf.String())
	}

	assertSymlinkTo(t, filepath.Join(containerDir, "testrepo--feat-link", ".venv"), "../main/.venv")
	if !strings.Contains(buf.String(), "symlink:    .venv") {
		t.Errorf("プラン表示に symlink 行が無い:\n%s", buf.String())
	}
}

// TestAddByBranch_NoSymlink verifies --no-symlink suppresses the default.
func TestAddByBranch_NoSymlink(t *testing.T) {
	containerDir, mainDir := setupWtContainer(t)
	if err := os.Mkdir(filepath.Join(mainDir, ".venv"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSymlinkCandidates(t, containerDir, ".venv")

	var buf bytes.Buffer
	if _, err := addByBranch(&buf, &AddOptions{Repo: "testrepo", Branch: "feat/off", NoSymlink: true}); err != nil {
		t.Fatalf("addByBranch: %v\n%s", err, buf.String())
	}

	assertAbsent(t, filepath.Join(containerDir, "testrepo--feat-off", ".venv"))
	if !strings.Contains(buf.String(), "なし（--no-symlink）") {
		t.Errorf("無効化がプラン表示に出ていない:\n%s", buf.String())
	}
}

// TestAddByBranch_EmptyCandidatesWithLegacyFlag verifies the backward-compatible
// --symlink flag stays harmless when nothing is registered.
func TestAddByBranch_EmptyCandidatesWithLegacyFlag(t *testing.T) {
	containerDir, _ := setupWtContainer(t)

	var buf bytes.Buffer
	if _, err := addByBranch(&buf, &AddOptions{Repo: "testrepo", Branch: "feat/empty", Symlink: true}); err != nil {
		t.Fatalf("addByBranch: %v\n%s", err, buf.String())
	}

	assertAbsent(t, filepath.Join(containerDir, "testrepo--feat-empty", ".venv"))
	if !strings.Contains(buf.String(), "候補なし（wt symlink add testrepo <path> で追加）") {
		t.Errorf("候補なしの案内が出ていない:\n%s", buf.String())
	}
}

// TestAddByBranch_CandidateMissingInMain verifies a registered candidate that is
// absent from main is reported instead of being dropped silently.
func TestAddByBranch_CandidateMissingInMain(t *testing.T) {
	containerDir, _ := setupWtContainer(t)
	writeSymlinkCandidates(t, containerDir, ".missing")

	var buf bytes.Buffer
	if _, err := addByBranch(&buf, &AddOptions{Repo: "testrepo", Branch: "feat/missing"}); err != nil {
		t.Fatalf("addByBranch: %v\n%s", err, buf.String())
	}

	assertAbsent(t, filepath.Join(containerDir, "testrepo--feat-missing", ".missing"))
	if !strings.Contains(buf.String(), "なし（候補 .missing が main に無い）") {
		t.Errorf("実体なしの警告が出ていない:\n%s", buf.String())
	}
}

// TestSymlinkPlanLine covers every state the plan line must distinguish.
func TestSymlinkPlanLine(t *testing.T) {
	cases := []struct {
		name string
		plan symlinkPlan
		want string
	}{
		{"disabled", symlinkPlan{Disabled: true}, "なし（--no-symlink）"},
		{"on", symlinkPlan{Targets: []string{".venv"}}, ".venv"},
		{"partial", symlinkPlan{Targets: []string{".venv"}, Missing: []string{".env"}}, ".venv（main に無い候補: .env）"},
		{"missing only", symlinkPlan{Missing: []string{".venv"}}, "なし（候補 .venv が main に無い）"},
		{"empty", symlinkPlan{}, "候補なし（wt symlink add myrepo <path> で追加）"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.plan.line("myrepo"); got != tc.want {
				t.Errorf("line() = %q, want %q", got, tc.want)
			}
		})
	}
}
