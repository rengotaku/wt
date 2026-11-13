package tree

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// setupWorktreeForScan creates a temp git repo that looks like a worktree with
// a tracked dir, an untracked dir, an ignored dir, and a symlinked dir.
func setupWorktreeForScan(t *testing.T) string {
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

	mustWrite := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("README.md", "init\n")
	mustWrite(".gitignore", "node_modules/\n")
	mustWrite("src/main.go", "package main\n")
	run("git", "add", ".")
	run("git", "commit", "-m", "init")

	// Untracked real directory (the .venv case).
	mustWrite(".venv/lib/marker", "x")
	// Ignored real directory.
	mustWrite("node_modules/pkg/marker", "x")
	// Symlink to the main worktree: removing the link is cheap, so it must not
	// be reported as a lost directory.
	if err := os.Symlink("../main/node_modules", filepath.Join(dir, "linked")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestListRealDirs_ListsUntrackedAndIgnoredSkipsSymlinks(t *testing.T) {
	dir := setupWorktreeForScan(t)

	got := make([]string, 0)
	for _, d := range listRealDirs(dir) {
		got = append(got, d.Path)
		if d.Size <= 0 {
			t.Errorf("%s のサイズが測れていない: %d", d.Path, d.Size)
		}
	}

	want := []string{".venv", "node_modules"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("listRealDirs = %v, want %v", got, want)
	}
}

func TestPerformDelete_DryRun_PrintsRealDirs(t *testing.T) {
	dir := setupWorktreeForScan(t)

	var buf bytes.Buffer
	target := RmEntry{WtName: "wtx", WtPath: dir, MainDir: dir, Container: filepath.Dir(dir)}
	// Untracked .venv makes the worktree dirty, so force is needed to reach the
	// dry-run branch.
	if err := performDelete(&buf, RmOptions{DryRun: true, Force: true}, &target); err != nil {
		t.Fatalf("performDelete: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "削除予定: "+dir) {
		t.Errorf("削除予定が出ていない:\n%s", out)
	}
	if !strings.Contains(out, "消える実体:") || !strings.Contains(out, ".venv") {
		t.Errorf("消える実体が %s に出ていない:\n%s", ".venv", out)
	}
	if !strings.Contains(out, "node_modules") {
		t.Errorf("ignored ディレクトリが出ていない:\n%s", out)
	}
	if strings.Contains(out, "linked") {
		t.Errorf("symlink が消える実体として出ている:\n%s", out)
	}
	if strings.Contains(out, "src") {
		t.Errorf("tracked ディレクトリが消える実体として出ている:\n%s", out)
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1048576, "1.0MB"},
		{3_000_000_000, "2.8GB"},
	}
	for _, tc := range cases {
		if got := humanSize(tc.in); got != tc.want {
			t.Errorf("humanSize(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
