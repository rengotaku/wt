package tree

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/wt/core"
)

// makeGitRepo creates a bare git repo with one commit and returns the clone path.
func makeGitRepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "remote.git")
	clone := filepath.Join(tmp, "repo")

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run(tmp, "git", "init", "--bare", "--initial-branch=main", remote)
	run(tmp, "git", "clone", remote, clone)
	run(clone, "git", "config", "user.email", "test@example.com")
	run(clone, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(clone, "git", "add", "README.md")
	run(clone, "git", "commit", "-m", "init")
	run(clone, "git", "push", "-u", "origin", "main")
	return clone
}

// makeContainer creates a wt container with main worktree and returns (containerDir, mainDir).
func makeContainer(t *testing.T) (containerDir, mainDir string) {
	t.Helper()
	tmp := t.TempDir()
	repoName := "testrepo"
	container := filepath.Join(tmp, repoName)
	mainDir = makeGitRepo(t)
	// Move into container layout
	newMain := filepath.Join(container, "main")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	// Symlink rather than copy to keep it simple
	if err := os.Rename(mainDir, newMain); err != nil {
		t.Fatal(err)
	}
	if err := core.SaveConfig(container, core.EntryConfig{SymlinkCandidates: []string{}}); err != nil {
		t.Fatal(err)
	}
	return container, newMain
}

// ── isGitCryptRepo ────────────────────────────────────────────────────────────

func TestIsGitCryptRepo_WithGitAttributes(t *testing.T) {
	dir := makeGitRepo(t)
	attrs := filepath.Join(dir, ".gitattributes")
	if err := os.WriteFile(attrs, []byte("*.secret filter=git-crypt diff=git-crypt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isGitCryptRepo(dir) {
		t.Error("expected isGitCryptRepo=true when .gitattributes contains filter=git-crypt")
	}
}

func TestIsGitCryptRepo_WithGitConfig(t *testing.T) {
	dir := makeGitRepo(t)
	cmd := exec.Command("git", "-C", dir, "config", "filter.git-crypt.smudge", "git-crypt smudge")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config failed: %s", out)
	}
	if !isGitCryptRepo(dir) {
		t.Error("expected isGitCryptRepo=true when git config has filter.git-crypt.smudge")
	}
}

func TestIsGitCryptRepo_NotGitCrypt(t *testing.T) {
	dir := makeGitRepo(t)
	if isGitCryptRepo(dir) {
		t.Error("expected isGitCryptRepo=false for a plain repo")
	}
}

func TestIsGitCryptRepo_WithUnrelatedGitAttributes(t *testing.T) {
	dir := makeGitRepo(t)
	attrs := filepath.Join(dir, ".gitattributes")
	if err := os.WriteFile(attrs, []byte("*.png binary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isGitCryptRepo(dir) {
		t.Error("expected isGitCryptRepo=false when .gitattributes has no git-crypt filter")
	}
}

// ── findGitCryptKey ───────────────────────────────────────────────────────────

func TestFindGitCryptKey_HomeDefault(t *testing.T) {
	containerDir, mainDir := makeContainer(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	keyPath := filepath.Join(tmpHome, ".git-crypt-key")
	if err := os.WriteFile(keyPath, []byte("fakekey"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := findGitCryptKey(containerDir, mainDir)
	if got != keyPath {
		t.Errorf("findGitCryptKey = %q, want %q", got, keyPath)
	}
}

func TestFindGitCryptKey_NotFound(t *testing.T) {
	containerDir, mainDir := makeContainer(t)
	// Point HOME to an empty dir so ~/.git-crypt-key does not exist
	t.Setenv("HOME", t.TempDir())

	got := findGitCryptKey(containerDir, mainDir)
	if got != "" {
		t.Errorf("findGitCryptKey = %q, want empty string", got)
	}
}

func TestFindGitCryptKey_GitConfigPriorityOverHome(t *testing.T) {
	containerDir, mainDir := makeContainer(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create home default key
	homeKey := filepath.Join(tmpHome, ".git-crypt-key")
	if err := os.WriteFile(homeKey, []byte("homekey"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Create a different key referenced by git config
	configKey := filepath.Join(t.TempDir(), "config.key")
	if err := os.WriteFile(configKey, []byte("configkey"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", mainDir, "config", "wt.gitCryptKey", configKey)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config failed: %s", out)
	}

	got := findGitCryptKey(containerDir, mainDir)
	if got != configKey {
		t.Errorf("findGitCryptKey = %q, want %q (git config key should take priority)", got, configKey)
	}
}

func TestFindGitCryptKey_RegistryPriorityOverGitConfig(t *testing.T) {
	containerDir, mainDir := makeContainer(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create a key in git config
	configKey := filepath.Join(t.TempDir(), "config.key")
	if err := os.WriteFile(configKey, []byte("configkey"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", mainDir, "config", "wt.gitCryptKey", configKey)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config failed: %s", out)
	}

	// Create a different key in the container registry (higher priority)
	registryKey := filepath.Join(t.TempDir(), "registry.key")
	if err := os.WriteFile(registryKey, []byte("registrykey"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := core.SaveConfig(containerDir, core.EntryConfig{GitCryptKey: registryKey}); err != nil {
		t.Fatal(err)
	}

	got := findGitCryptKey(containerDir, mainDir)
	if got != registryKey {
		t.Errorf("findGitCryptKey = %q, want %q (registry key should take priority)", got, registryKey)
	}
}

func TestFindGitCryptKey_RegistryKeyMissing_FallsThrough(t *testing.T) {
	containerDir, mainDir := makeContainer(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Register a key path that doesn't exist
	if err := core.SaveConfig(containerDir, core.EntryConfig{GitCryptKey: "/nonexistent/key"}); err != nil {
		t.Fatal(err)
	}
	// Create home default key as fallback
	homeKey := filepath.Join(tmpHome, ".git-crypt-key")
	if err := os.WriteFile(homeKey, []byte("homekey"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := findGitCryptKey(containerDir, mainDir)
	if got != homeKey {
		t.Errorf("findGitCryptKey = %q, want %q (should fall through to home default)", got, homeKey)
	}
}

// ── isSmudgeError ─────────────────────────────────────────────────────────────

func TestIsSmudgeError_GitCryptSmudge(t *testing.T) {
	output := "fatal: file.enc: smudge filter git-crypt failed"
	if !isSmudgeError(output) {
		t.Error("expected isSmudgeError=true for smudge filter git-crypt failed")
	}
}

func TestIsSmudgeError_GitCryptFilterFailed(t *testing.T) {
	output := "error: external filter 'git-crypt' failed"
	if !isSmudgeError(output) {
		t.Error("expected isSmudgeError=true for filter 'git-crypt' failed")
	}
}

func TestIsSmudgeError_GitCryptErrorPrefix(t *testing.T) {
	output := "git-crypt: Error: Unable to open key file"
	if !isSmudgeError(output) {
		t.Error("expected isSmudgeError=true for git-crypt: Error prefix")
	}
}

func TestIsSmudgeError_UnrelatedError(t *testing.T) {
	output := "fatal: not a git repository"
	if isSmudgeError(output) {
		t.Error("expected isSmudgeError=false for unrelated error")
	}
}

func TestIsSmudgeError_EmptyString(t *testing.T) {
	if isSmudgeError("") {
		t.Error("expected isSmudgeError=false for empty string")
	}
}

// ── addWorktreeNewBranch (no-op path: non-git-crypt repo) ─────────────────────

func TestAddWorktreeNewBranch_NonGitCryptRepo_Success(t *testing.T) {
	containerDir, mainDir := makeContainer(t)

	worktreePath := filepath.Join(t.TempDir(), "new-wt")
	var buf bytes.Buffer
	err := addWorktreeNewBranch(&buf, mainDir, worktreePath, "test-branch", "main", containerDir)
	if err != nil {
		t.Fatalf("addWorktreeNewBranch failed: %v", err)
	}
	// No git-crypt messages should appear for a non-git-crypt repo
	if strings.Contains(buf.String(), "git-crypt") {
		t.Errorf("unexpected git-crypt output for non-git-crypt repo: %s", buf.String())
	}
	// Worktree should exist
	if _, err := os.Stat(worktreePath); err != nil {
		t.Errorf("worktree path should exist: %v", err)
	}
}

func TestAddWorktreeExistingBranch_NonGitCryptRepo_Success(t *testing.T) {
	containerDir, mainDir := makeContainer(t)

	// Create a branch to check out
	cmd := exec.Command("git", "-C", mainDir, "branch", "existing-branch")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch failed: %s", out)
	}

	worktreePath := filepath.Join(t.TempDir(), "existing-wt")
	var buf bytes.Buffer
	err := addWorktreeExistingBranch(&buf, mainDir, worktreePath, "existing-branch", containerDir)
	if err != nil {
		t.Fatalf("addWorktreeExistingBranch failed: %v", err)
	}
	if _, err := os.Stat(worktreePath); err != nil {
		t.Errorf("worktree path should exist: %v", err)
	}
}

// ── logAndUnlock ──────────────────────────────────────────────────────────────

func TestLogAndUnlock_NoKey_PrintsWarning(t *testing.T) {
	containerDir, mainDir := makeContainer(t)
	t.Setenv("HOME", t.TempDir())

	var buf bytes.Buffer
	logAndUnlock(&buf, "/some/worktree", containerDir, mainDir)

	if !strings.Contains(buf.String(), "鍵が見つかりません") {
		t.Errorf("expected missing-key warning, got: %s", buf.String())
	}
}

// ── recoverSmudge ─────────────────────────────────────────────────────────────

func TestRecoverSmudge_NoKey_CreatesWorktreeWithWarning(t *testing.T) {
	containerDir, mainDir := makeContainer(t)
	t.Setenv("HOME", t.TempDir()) // no ~/.git-crypt-key

	worktreePath := filepath.Join(t.TempDir(), "recover-wt")
	var buf bytes.Buffer
	err := recoverSmudge(&buf, mainDir, worktreePath, "recover-branch", "main", containerDir)
	if err != nil {
		t.Fatalf("recoverSmudge returned error: %v", err)
	}
	// Should warn about missing key
	if !strings.Contains(buf.String(), "鍵が見つかりません") {
		t.Errorf("expected missing-key warning, got: %s", buf.String())
	}
	// Worktree should exist (no-checkout)
	if _, err := os.Stat(worktreePath); err != nil {
		t.Errorf("worktree path should exist after --no-checkout: %v", err)
	}
}

func TestRecoverSmudge_BranchAlreadyExists_UsesExistingBranch(t *testing.T) {
	containerDir, mainDir := makeContainer(t)
	t.Setenv("HOME", t.TempDir())

	// Create the branch beforehand to simulate the "branch already created" scenario
	cmd := exec.Command("git", "-C", mainDir, "branch", "pre-existing-branch")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch: %s", out)
	}

	worktreePath := filepath.Join(t.TempDir(), "recover-wt2")
	var buf bytes.Buffer
	// startPoint="" + branch exists → should use existing branch path
	err := recoverSmudge(&buf, mainDir, worktreePath, "pre-existing-branch", "", containerDir)
	if err != nil {
		t.Fatalf("recoverSmudge returned error: %v", err)
	}
	if _, err := os.Stat(worktreePath); err != nil {
		t.Errorf("worktree should exist: %v", err)
	}
}

// ── addWorktreeNewBranch with git-crypt repo (no key) ─────────────────────────

func TestAddWorktreeNewBranch_GitCryptRepo_NoKey_SucceedsWithWarning(t *testing.T) {
	containerDir, mainDir := makeContainer(t)
	t.Setenv("HOME", t.TempDir()) // no ~/.git-crypt-key

	// Mark as git-crypt repo so logAndUnlock path is exercised
	attrs := filepath.Join(mainDir, ".gitattributes")
	if err := os.WriteFile(attrs, []byte("*.secret filter=git-crypt diff=git-crypt\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	worktreePath := filepath.Join(t.TempDir(), "git-crypt-wt")
	var buf bytes.Buffer
	err := addWorktreeNewBranch(&buf, mainDir, worktreePath, "gc-branch", "main", containerDir)
	if err != nil {
		t.Fatalf("expected success with warning, got error: %v", err)
	}
	if !strings.Contains(buf.String(), "鍵が見つかりません") {
		t.Errorf("expected missing-key warning, got: %s", buf.String())
	}
}

func TestAddWorktreeExistingBranch_GitCryptRepo_NoKey_SucceedsWithWarning(t *testing.T) {
	containerDir, mainDir := makeContainer(t)
	t.Setenv("HOME", t.TempDir())

	// Create branch + mark as git-crypt
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("git", "-C", mainDir, "branch", "gc-existing-branch")
	attrs := filepath.Join(mainDir, ".gitattributes")
	if err := os.WriteFile(attrs, []byte("*.secret filter=git-crypt diff=git-crypt\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	worktreePath := filepath.Join(t.TempDir(), "gc-existing-wt")
	var buf bytes.Buffer
	err := addWorktreeExistingBranch(&buf, mainDir, worktreePath, "gc-existing-branch", containerDir)
	if err != nil {
		t.Fatalf("expected success with warning, got error: %v", err)
	}
	if !strings.Contains(buf.String(), "鍵が見つかりません") {
		t.Errorf("expected missing-key warning, got: %s", buf.String())
	}
}
