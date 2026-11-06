package gc

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"wt/internal/wt/tree"
)

// fakeDeps は外部コマンドを呼ばない deps。repo "r" は main だけで、
// merged な feat/a と open な feat/b のローカルブランチを持つ。
func fakeDeps(rm func() ([]tree.RmEntry, error), deleted *[]string) deps {
	return deps{
		rmEntries: rm,
		repos:     func() []repoRef { return []repoRef{{Repo: "r", MainDir: "/m"}} },
		prStates: func(string) (map[string]string, map[string][]string) {
			return map[string]string{"feat/a": "MERGED", "feat/b": "OPEN"}, map[string][]string{}
		},
		issueState: func(string, int) string { return "" },
		probe: func(string) branchProbe {
			return branchProbe{
				localBranches: func() ([]string, error) { return []string{"main", "feat/a", "feat/b"}, nil },
				checkedOut:    func() (map[string]bool, error) { return map[string]bool{"main": true}, nil },
				unpushed:      func(string) unpushed { return unpushed{} },
				landed:        func(string, []string) bool { return false },
				lastCommit:    func(string) time.Time { return time.Time{} },
			}
		},
		deleteBranch: func(_, b string) error { *deleted = append(*deleted, b); return nil },
	}
}

func TestRun_branchScanIndependentOfWorktrees(t *testing.T) {
	noWt := func() ([]tree.RmEntry, error) { return nil, errors.New("削除可能なworktreeがありません") }
	tests := []struct {
		name       string
		opts       Options
		wantErr    bool
		wantDelete []string
		wantOut    string
	}{
		{"main only repo, worktree 0 件 + --done → ブランチ候補", Options{Done: true, DryRun: true}, false, nil, "feat/a"},
		{"worktree 0 件 + --done -y → ブランチ削除", Options{Done: true, Yes: true}, false, []string{"feat/a"}, "✅ 削除: r/feat/a"},
		{"worktree 0 件 + --older-than 単独 → 従来のエラー", Options{OlderThan: "30d"}, true, nil, ""},
		{"worktree 0 件 + --done --keep-branch → ブランチ走査せず従来のエラー", Options{Done: true, KeepBranch: true, Yes: true}, true, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var deleted []string
			var buf bytes.Buffer
			err := run(&buf, tt.opts, fakeDeps(noWt, &deleted))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(deleted, tt.wantDelete) {
				t.Errorf("deleted = %v, want %v", deleted, tt.wantDelete)
			}
			if !strings.Contains(buf.String(), tt.wantOut) {
				t.Errorf("out = %q, want contains %q", buf.String(), tt.wantOut)
			}
		})
	}
}
