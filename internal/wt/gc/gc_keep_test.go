package gc

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeepReason(t *testing.T) {
	failed := errors.New("boom")
	tests := []struct {
		name   string
		c      changes
		u      unpushed
		landed bool
		want   string // "" = 削除してよい
	}{
		{"clean, all pushed, not merged → delete", changes{}, unpushed{}, false, ""},
		{"clean, merged → delete", changes{}, unpushed{}, true, ""},
		{"uncommitted → keep", changes{uncommitted: 2}, unpushed{}, true, "未コミットの変更 2 件"},
		{"untracked only → keep", changes{untracked: 3}, unpushed{}, true, "未追跡のファイル 3 件"},
		{"unpushed, not merged → keep", changes{}, unpushed{n: 4}, false, "リモートに無いコミット 4 件"},
		{"unpushed, merged (squash, remote branch deleted) → delete", changes{}, unpushed{n: 4}, true, ""},
		{"status failed → keep", changes{err: failed}, unpushed{}, true, "判定失敗"},
		{"rev-list failed, not merged → keep", changes{}, unpushed{err: failed}, false, "判定失敗"},
		{"rev-list failed, merged → delete", changes{}, unpushed{err: failed}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := keepReason(tt.c, tt.u, tt.landed)
			if tt.want == "" && got != "" {
				t.Fatalf("got %q, want delete", got)
			}
			if tt.want != "" && !strings.HasPrefix(got, tt.want) {
				t.Fatalf("got %q, want prefix %q", got, tt.want)
			}
		})
	}
}

// gcFixture は HOME 配下に ~/Workspace/repo/{main,<worktrees>} を作る。
// コミット日時は過去に寄せ、--older-than 1d だけで（gh を呼ばずに）候補にする。
type gcFixture struct {
	t    *testing.T
	main string
	base string
}

func newGCFixture(t *testing.T) *gcFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_AUTHOR_DATE", "2020-01-01T00:00:00")
	t.Setenv("GIT_COMMITTER_DATE", "2020-01-01T00:00:00")
	base := filepath.Join(home, "Workspace", "repo")
	f := &gcFixture{t: t, main: filepath.Join(base, "main"), base: base}

	bare := filepath.Join(home, "origin.git")
	f.git(home, "init", "--bare", "-b", "main", bare)
	f.git(home, "init", "-b", "main", f.main)
	f.git(f.main, "config", "user.email", "test@example.com")
	f.git(f.main, "config", "user.name", "Test")
	f.git(f.main, "commit", "--allow-empty", "-m", "init")
	f.git(f.main, "remote", "add", "origin", bare)
	f.git(f.main, "push", "-u", "origin", "main")
	return f
}

func (f *gcFixture) git(dir string, args ...string) {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// add は worktree を作り、そのパスを返す。
func (f *gcFixture) add(name string) string {
	f.t.Helper()
	p := filepath.Join(f.base, name)
	f.git(f.main, "worktree", "add", "-b", name, p)
	return p
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func revParse(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestHeadLanded(t *testing.T) {
	f := newGCFixture(t)
	wt := f.add("feat")
	f.git(wt, "commit", "--allow-empty", "-m", "a")
	a := revParse(t, wt)
	f.git(wt, "commit", "--allow-empty", "-m", "b")
	b := revParse(t, wt)

	if !headLanded(wt, []string{b}) {
		t.Error("HEAD == merged head → landed")
	}
	f.git(wt, "reset", "-q", "--hard", a)
	if !headLanded(wt, []string{b}) {
		t.Error("HEAD is an ancestor of merged head → landed")
	}
	f.git(wt, "reset", "-q", "--hard", b)
	f.git(wt, "commit", "--allow-empty", "-m", "after merge")
	if headLanded(wt, []string{b}) {
		t.Error("commit added after the merge → not landed")
	}
	if headLanded(wt, nil) {
		t.Error("no merged head known → not landed")
	}
}

func TestRunKeepsUnsavedWork(t *testing.T) {
	f := newGCFixture(t)

	clean := f.add("clean")

	untracked := f.add("untracked")
	if err := os.WriteFile(filepath.Join(untracked, "memo.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	unpushed := f.add("unpushed")
	f.git(unpushed, "commit", "--allow-empty", "-m", "local only")

	pushed := f.add("pushed")
	f.git(pushed, "commit", "--allow-empty", "-m", "pushed")
	f.git(pushed, "push", "origin", "pushed")

	// --dry-run: 何も消さず、スキップ理由が候補ごとに出る。
	var dry bytes.Buffer
	if err := Run(&dry, Options{OlderThan: "1d", DryRun: true}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"repo/untracked   untracked  未追跡のファイル 1 件",
		"repo/unpushed   unpushed  リモートに無いコミット 1 件",
		"🔍 2 件の候補:",
	} {
		if !strings.Contains(dry.String(), want) {
			t.Errorf("dry-run output missing %q:\n%s", want, dry.String())
		}
	}

	// 既定（--yes のみ）: clean と pushed だけ消え、残り 2 つは残る。
	var out bytes.Buffer
	if err := Run(&out, Options{OlderThan: "1d", Yes: true}); err != nil {
		t.Fatal(err)
	}
	if exists(clean) || exists(pushed) {
		t.Errorf("clean/pushed should be deleted:\n%s", out.String())
	}
	if !exists(untracked) || !exists(unpushed) {
		t.Fatalf("untracked/unpushed must survive without --force:\n%s", out.String())
	}

	// --force: 残っていた 2 つも消える。
	if err := Run(&out, Options{OlderThan: "1d", Yes: true, Force: true}); err != nil {
		t.Fatal(err)
	}
	if exists(untracked) || exists(unpushed) {
		t.Errorf("--force should delete untracked/unpushed:\n%s", out.String())
	}
}
