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

func TestScanWorktree_ClassifiesSymlinksAndDirs(t *testing.T) {
	dir := setupWorktreeForScan(t)

	s := scanWorktree(dir, true)
	if !s.Measured {
		t.Fatal("Measured = false, want true")
	}

	wantLinks := []symlinkEntry{{Path: "linked", Target: "../main/node_modules"}}
	if !reflect.DeepEqual(s.Symlinks, wantLinks) {
		t.Errorf("Symlinks = %+v, want %+v", s.Symlinks, wantLinks)
	}

	var paths []string
	for _, d := range s.Dirs {
		paths = append(paths, d.Path)
		if d.Size <= 0 {
			t.Errorf("%s のサイズが測れていない: %d", d.Path, d.Size)
		}
	}
	if want := []string{".venv", "node_modules"}; !reflect.DeepEqual(paths, want) {
		t.Errorf("Dirs = %v, want %v", paths, want)
	}
}

func TestScanWorktree_NoSize(t *testing.T) {
	dir := setupWorktreeForScan(t)

	s := scanWorktree(dir, false)
	if s.Measured {
		t.Error("Measured = true, want false")
	}
	for _, d := range s.Dirs {
		if d.Size != 0 {
			t.Errorf("%s のサイズ = %d, want 0（--no-size）", d.Path, d.Size)
		}
	}
}

func TestRenderScan_FormatsAndWarns(t *testing.T) {
	s := rmScan{
		Symlinks: []symlinkEntry{{Path: ".venv", Target: "../main/.venv"}},
		Dirs: []realDir{
			{Path: "cache", Size: 5 << 20},
			{Path: ".venv", Size: 3_000_000_000},
		},
		Measured: true,
	}

	var buf bytes.Buffer
	renderScan(&buf, "myrepo", s, false)
	out := buf.String()

	for _, want := range []string{
		"symlink: .venv -> ../main/.venv",
		"2.8GB",
		"5.0MB",
		"実体の合計: 2.8GB",
		"wt symlink add myrepo .venv",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("%q が出力に無い:\n%s", want, out)
		}
	}
	if strings.Index(out, ".venv") > strings.Index(out, "cache") {
		t.Errorf("サイズの大きい順になっていない:\n%s", out)
	}
}

func TestRenderScan_TopFive(t *testing.T) {
	s := rmScan{Measured: true}
	for i, size := range []int64{600, 500, 400, 300, 200, 100} {
		s.Dirs = append(s.Dirs, realDir{Path: "dir" + string(rune('1'+i)), Size: size})
	}

	var buf bytes.Buffer
	renderScan(&buf, "myrepo", s, false)
	out := buf.String()

	if !strings.Contains(out, "他 1 件") {
		t.Errorf("上位 5 件の案内が無い:\n%s", out)
	}
	if strings.Contains(out, "dir6") {
		t.Errorf("6 件目が表示されている:\n%s", out)
	}
}

func TestRenderScan_NoSizeAndQuiet(t *testing.T) {
	s := rmScan{Dirs: []realDir{{Path: ".venv"}}}

	var sized bytes.Buffer
	renderScan(&sized, "myrepo", s, false)
	if strings.Contains(sized.String(), "実体の合計") {
		t.Errorf("--no-size なのに合計が出ている:\n%s", sized.String())
	}
	if strings.Contains(sized.String(), "1.0KB") || strings.Contains(sized.String(), "GB") {
		t.Errorf("--no-size なのにサイズが出ている:\n%s", sized.String())
	}

	var quiet bytes.Buffer
	renderScan(&quiet, "myrepo", s, true)
	if quiet.Len() != 0 {
		t.Errorf("--quiet なのに出力がある: %q", quiet.String())
	}
}

func TestPerformDelete_DryRun_PrintsScan(t *testing.T) {
	dir := setupWorktreeForScan(t)

	var buf bytes.Buffer
	target := RmEntry{WtName: "wtx", Repo: "myrepo", WtPath: dir, MainDir: dir, Container: filepath.Dir(dir)}
	// Untracked .venv makes the worktree dirty, so force is needed to reach the
	// dry-run branch.
	if err := performDelete(&buf, RmOptions{DryRun: true, Force: true}, &target); err != nil {
		t.Fatalf("performDelete: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"削除予定: " + dir,
		"symlink: linked -> ../main/node_modules",
		"実体:",
		".venv",
		"node_modules",
		"実体の合計:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("%q が出力に無い:\n%s", want, out)
		}
	}
	if strings.Contains(out, "src/main.go") || strings.Contains(out, "README.md") {
		t.Errorf("tracked ファイルが実体として出ている:\n%s", out)
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
