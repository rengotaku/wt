package tree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRun runs a git command in dir, failing the test on error.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.email=test@example.com", "-c", "user.name=Test"}, args...)
	cmd := exec.Command("git", full...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// setupOriginAndClone creates a bare origin with one commit and clones it,
// returning the clone path (a worktree-like checkout) and the origin path.
func setupOriginAndClone(t *testing.T) (clone, origin string) {
	t.Helper()
	root := t.TempDir()
	origin = filepath.Join(root, "origin.git")
	seed := filepath.Join(root, "seed")
	clone = filepath.Join(root, "clone")

	gitRun(t, root, "init", "--bare", "-b", "main", origin)

	// seed リポで初期コミットを作り origin へ push する。
	gitRun(t, root, "clone", origin, seed)
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, seed, "add", ".")
	gitRun(t, seed, "commit", "-m", "init")
	gitRun(t, seed, "push", "origin", "main")

	// 検証対象の clone を作る。
	gitRun(t, root, "clone", origin, clone)
	return clone, origin
}

func TestUpdate_FastForward(t *testing.T) {
	clone, origin := setupOriginAndClone(t)

	// origin に追加コミットを積む（別の seed 経由で push）。
	root := filepath.Dir(origin)
	seed2 := filepath.Join(root, "seed2")
	gitRun(t, root, "clone", origin, seed2)
	if err := os.WriteFile(filepath.Join(seed2, "next.txt"), []byte("more\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, seed2, "add", ".")
	gitRun(t, seed2, "commit", "-m", "second")
	gitRun(t, seed2, "push", "origin", "main")

	msg, err := Update(clone)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if msg != "1 commits pulled" {
		t.Fatalf("want %q, got %q", "1 commits pulled", msg)
	}
}

func TestUpdate_AlreadyUpToDate(t *testing.T) {
	clone, _ := setupOriginAndClone(t)

	msg, err := Update(clone)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if msg != "Already up to date" {
		t.Fatalf("want %q, got %q", "Already up to date", msg)
	}
}

func TestUpdate_DirtyTreeRejected(t *testing.T) {
	clone, _ := setupOriginAndClone(t)

	// 追跡ファイルに未コミット変更を作る。
	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Update(clone)
	if err == nil {
		t.Fatal("Update should reject dirty worktree, got nil error")
	}
	if !strings.Contains(err.Error(), "未コミットの変更があるため最新化できません") {
		t.Fatalf("unexpected error: %v", err)
	}
}
