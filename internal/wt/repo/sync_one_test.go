package repo

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// setupClone makes a remote with one commit and a clone at <tmp>/app/main.
func setupClone(t *testing.T) (remote, mainDir string) {
	t.Helper()
	tmp := t.TempDir()
	remote = filepath.Join(tmp, "remote")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, remote, "init", "-q", "--initial-branch=main")
	gitIn(t, remote, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "a")
	mainDir = filepath.Join(tmp, "app", "main")
	if out, err := exec.Command("git", "clone", "-q", remote, mainDir).CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	return remote, mainDir
}

func TestSyncOne(t *testing.T) {
	remote, mainDir := setupClone(t)

	if r := syncOne("app", "main", mainDir); r.Result != SyncUpToDate {
		t.Errorf("up to date: %+v", r)
	}

	gitIn(t, remote, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "b")
	if r := syncOne("app", "main", mainDir); r.Result != SyncUpdated || r.Detail != "1 commits" {
		t.Errorf("updated: %+v", r)
	}

	if err := os.WriteFile(filepath.Join(mainDir, "f"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, mainDir, "add", "f")
	if r := syncOne("app", "main", mainDir); r.Result != SyncSkipped {
		t.Errorf("uncommitted: %+v", r)
	}
	gitIn(t, mainDir, "reset", "-q", "--hard")

	gitIn(t, mainDir, "checkout", "-q", "-b", "feat/x")
	if r := syncOne("app", "main", mainDir); r.Result != SyncWrongBranch || r.Branch != "feat/x" {
		t.Errorf("wrong branch: %+v", r)
	}
	gitIn(t, mainDir, "checkout", "-q", "main")

	if err := os.RemoveAll(remote); err != nil {
		t.Fatal(err)
	}
	if r := syncOne("app", "main", mainDir); r.Result != SyncFailed {
		t.Errorf("remote gone: %+v", r)
	}
}

// Naming one container must not sync a same-named one in the other workspace.
func TestSync_OnlyTheGivenContainer(t *testing.T) {
	_, mainA := setupClone(t)
	_, mainB := setupClone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	var containers []string
	for i, m := range []string{mainA, mainB} {
		c := filepath.Join(home, []string{"Workspace", "MyWorkspace"}[i], "app")
		if err := os.MkdirAll(c, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(m, filepath.Join(c, "main")); err != nil {
			t.Fatal(err)
		}
		containers = append(containers, c)
	}
	got, err := Sync(io.Discard, containers[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("synced %d repos, want 1: %+v", len(got), got)
	}
}
