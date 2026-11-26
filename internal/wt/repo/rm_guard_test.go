package repo

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupRmHome(t *testing.T) (home, ws string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WT_MASTER_DIR", filepath.Join(home, "code"))
	ws = filepath.Join(home, "Workspace")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	return home, ws
}

func mustRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
}

// makeContainer creates <ws>/<name>/main as a git repo with one commit and a
// .worktrees.json, i.e. a wt-managed container.
func makeContainer(t *testing.T, ws, name string) (container, mainDir string) {
	t.Helper()
	container = filepath.Join(ws, name)
	mainDir = filepath.Join(container, "main")
	if err := os.MkdirAll(mainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustRun(t, mainDir, "git", "init", "-q", "--initial-branch=main")
	mustRun(t, mainDir, "git", "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init")
	if err := os.WriteFile(filepath.Join(container, ".worktrees.json"), []byte(`{"main":{"type":"main"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return container, mainDir
}

func writeSentinel(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s was removed: %v", path, err)
	}
}

// `wt repo rm .. --force` must never remove HOME or the workspace dir. #180
func TestRm_RejectsTraversal(t *testing.T) {
	_, ws := setupRmHome(t)
	sentinel := filepath.Join(ws, "keep", "file")
	writeSentinel(t, sentinel)

	for _, bad := range []string{"..", ".", "keep/.."} {
		if err := Rm(io.Discard, bad, RmOptions{Force: true}); err == nil {
			t.Errorf("Rm(%q): want error", bad)
		}
	}
	assertExists(t, sentinel)
}

// A plain directory under the workspace (no .worktrees.json) is not a wt
// container and must not be removed.
func TestRm_RejectsUnmanagedDir(t *testing.T) {
	_, ws := setupRmHome(t)
	sentinel := filepath.Join(ws, "notes", "file")
	writeSentinel(t, sentinel)

	if err := Rm(io.Discard, "notes", RmOptions{Force: true}); err == nil {
		t.Error("unmanaged dir: want error")
	}
	assertExists(t, sentinel)
}

// A linked worktree outside the container would be force-removed with its
// uncommitted work; abort before touching anything.
func TestRm_RejectsWorktreeOutsideContainer(t *testing.T) {
	home, ws := setupRmHome(t)
	container, mainDir := makeContainer(t, ws, "repo")
	outside := filepath.Join(home, "elsewhere")
	mustRun(t, mainDir, "git", "worktree", "add", "-q", "-b", "ext", outside)
	sentinel := filepath.Join(outside, "uncommitted.txt")
	writeSentinel(t, sentinel)

	if err := Rm(io.Discard, "repo", RmOptions{Force: true}); err == nil {
		t.Error("outside worktree: want error")
	}
	assertExists(t, sentinel)
	assertExists(t, container)
}

func TestRm_RemovesManagedContainer(t *testing.T) {
	_, ws := setupRmHome(t)
	container, mainDir := makeContainer(t, ws, "repo")
	mustRun(t, mainDir, "git", "worktree", "add", "-q", "-b", "feat", filepath.Join(container, "repo--feat"))

	if err := Rm(io.Discard, "repo", RmOptions{Force: true}); err != nil {
		t.Fatalf("Rm: %v", err)
	}
	if _, err := os.Stat(container); !os.IsNotExist(err) {
		t.Errorf("container still exists: %v", err)
	}
}
