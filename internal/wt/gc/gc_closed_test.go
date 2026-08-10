package gc

import (
	"errors"
	"io"
	"testing"
)

func TestIssueNumFromBranch(t *testing.T) {
	tests := []struct {
		branch string
		want   int
	}{
		{"feat/issue-84-abc", 84},
		{"bdiff--feat-issue-99-x", 99},
		{"Issue-7-copy", 7},
		{"bot/issue_42_x", 42},
		{"feature/login", 0},
		{"", 0},
	}
	for _, tt := range tests {
		if got := issueNumFromBranch(tt.branch); got != tt.want {
			t.Errorf("issueNumFromBranch(%q) = %d, want %d", tt.branch, got, tt.want)
		}
	}
}

func TestClosedCandidate(t *testing.T) {
	pr := map[string]string{
		"open-branch":   "OPEN",
		"closed-branch": "CLOSED",
		"merged-branch": "MERGED",
	}
	noIssue := func(int) string { return "" }
	closedIssue := func(int) string { return "CLOSED" }

	tests := []struct {
		name    string
		branch  string
		issueFn func(int) string
		want    bool
	}{
		{"open PR → keep", "open-branch", noIssue, false},
		{"closed PR → gc", "closed-branch", noIssue, true},
		{"merged PR → gc", "merged-branch", noIssue, true},
		{"no PR, closed issue → gc", "feat/issue-5-x", closedIssue, true},
		{"no PR, open issue → keep", "feat/issue-5-x", func(int) string { return "OPEN" }, false},
		{"no PR, no issue → keep (safe)", "feature/login", noIssue, false},
		{"empty branch → keep", "", closedIssue, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := closedCandidate(tt.branch, pr, tt.issueFn)
			if got != tt.want {
				t.Errorf("closedCandidate(%q) = %v, want %v", tt.branch, got, tt.want)
			}
		})
	}
}

func TestClosedFilter(t *testing.T) {
	if (Options{}).closedFilter() {
		t.Error("default should not enable the closed filter")
	}
	if !(Options{Done: true}).closedFilter() {
		t.Error("--done should enable the closed filter")
	}
	if !(Options{Merged: true}).closedFilter() {
		t.Error("--merged (alias) should enable the closed filter")
	}
	if !(Options{Done: true, Merged: true}).closedFilter() {
		t.Error("--done and --merged together should enable the closed filter")
	}
}

// Run はフィルタ（--done/--merged, --older-than）が1つも指定されていない
// 場合、main/master 以外の全 worktree を対象にしてしまう前に fail-safe で
// エラーを返さなければならない（実 git 環境を必要としない、ガード自体の検証）。
func TestRunRequiresAtLeastOneFilter(t *testing.T) {
	err := Run(io.Discard, Options{})
	if err == nil {
		t.Fatal("Run(Options{}) should error: no filter would target every worktree")
	}
	if !errors.Is(err, ErrNoFilter) {
		t.Errorf("Run(Options{}) error = %v, want errors.Is(err, ErrNoFilter)", err)
	}
}

func TestRunAcceptsDoneFilterAlone(t *testing.T) {
	// フィルタが有効な場合はガードを通過し、後段（tree.RmEntries 等）まで
	// 進む。実 git 環境が無い CI ではそこでエラーになりうるが、少なくとも
	// 「フィルタ未指定エラー」ではないことを確認する。
	err := Run(io.Discard, Options{Done: true})
	if errors.Is(err, ErrNoFilter) {
		t.Fatal("Run with --done should not hit the no-filter guard")
	}
}

func TestRunAcceptsOlderThanFilterAlone(t *testing.T) {
	err := Run(io.Discard, Options{OlderThan: "30d"})
	if errors.Is(err, ErrNoFilter) {
		t.Fatal("Run with --older-than should not hit the no-filter guard")
	}
}

// "0d"/"0h" は形式は正しいが期間が0なので、フィルタ未指定と同じ扱いに
// しなければならない（フロントの isPositiveOlderThan と判定基準を一致させる）。
func TestRunRejectsZeroOlderThan(t *testing.T) {
	for _, v := range []string{"0d", "0h"} {
		err := Run(io.Discard, Options{OlderThan: v})
		if !errors.Is(err, ErrNoFilter) {
			t.Errorf("Run(OlderThan=%q) error = %v, want errors.Is(err, ErrNoFilter)", v, err)
		}
	}
}
