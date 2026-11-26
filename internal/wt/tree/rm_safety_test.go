package tree

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"wt/internal/wt/core"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// setupRmContainer makes <home>/Workspace/repo with main and two linked
// worktrees: repo--a on feat/x-2 and repo--x on feat/x. git lists worktrees
// in path order, so feat/x-2 comes first — a prefix match would pick it.
func setupRmContainer(t *testing.T) (container, mainDir string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	container = filepath.Join(home, "Workspace", "repo")
	mainDir = filepath.Join(container, "main")
	if err := os.MkdirAll(mainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, mainDir, "init", "-q", "--initial-branch=main")
	git(t, mainDir, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init")
	for _, b := range []string{"feat/x-2", "feat/x"} {
		name := "repo--x"
		if b == "feat/x-2" {
			name = "repo--a"
		}
		git(t, mainDir, "worktree", "add", "-q", "-b", b, filepath.Join(container, name))
		if err := core.PutEntry(container, name, &core.Entry{Branch: b}); err != nil {
			t.Fatal(err)
		}
	}
	return container, mainDir
}

func TestResolveByBranch_ExactMatch(t *testing.T) {
	container, _ := setupRmContainer(t)
	got, err := resolveByBranch("repo", "feat/x")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(container, "repo--x"); got.WtPath != want {
		resolved, _ := filepath.EvalSymlinks(want)
		if got.WtPath != resolved {
			t.Errorf("feat/x resolved to %s, want %s (must not prefix-match feat/x-2)", got.WtPath, want)
		}
	}
}

func TestRemoveAt_UntrackedNeedsForce(t *testing.T) {
	container, _ := setupRmContainer(t)
	wt := filepath.Join(container, "repo--x")
	keep := filepath.Join(wt, "notes.txt")
	if err := os.WriteFile(keep, []byte("only copy"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RemoveAt(io.Discard, RmOptions{}, "repo", container, "repo--x"); err == nil {
		t.Fatal("untracked file without --force: want error")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("untracked file was removed: %v", err)
	}

	if err := RemoveAt(io.Discard, RmOptions{Force: true}, "repo", container, "repo--x"); err != nil {
		t.Fatalf("with --force: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("worktree still exists: %v", err)
	}
	entries, _ := core.LoadEntries(container)
	if _, ok := entries["repo--x"]; ok {
		t.Error("registry entry still exists")
	}
	if _, ok := entries["repo--a"]; !ok {
		t.Error("the other worktree's entry was removed")
	}
}
