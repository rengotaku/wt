package gc

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestBranchesWithoutWorktree(t *testing.T) {
	old := time.Now().Add(-100 * 24 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)

	tests := []struct {
		name       string
		branches   []string
		checkedOut map[string]bool
		prState    map[string]string
		merged     map[string][]string
		issues     map[int]string
		unpushed   map[string]unpushed
		landed     map[string]bool
		commitAt   time.Time
		olderSecs  int64
		force      bool
		wantCands  []string
		wantSkips  []string
	}{
		{name: "merged PR → candidate",
			branches: []string{"feat/a"}, prState: map[string]string{"feat/a": "MERGED"},
			wantCands: []string{"feat/a"}},
		{name: "closed PR → candidate",
			branches: []string{"feat/a"}, prState: map[string]string{"feat/a": "CLOSED"},
			wantCands: []string{"feat/a"}},
		{name: "open PR → not candidate",
			branches: []string{"feat/a"}, prState: map[string]string{"feat/a": "OPEN"}},
		{name: "no PR, issue closed → candidate",
			branches: []string{"feat/issue-5-x"}, issues: map[int]string{5: "CLOSED"},
			wantCands: []string{"feat/issue-5-x"}},
		{name: "unknown state (gh failure) → not candidate",
			branches: []string{"feat/a"}},
		{name: "main/master excluded",
			branches: []string{"main", "master"},
			prState:  map[string]string{"main": "MERGED", "master": "MERGED"}},
		{name: "checked-out branch excluded",
			branches: []string{"feat/a", "feat/b"}, checkedOut: map[string]bool{"feat/a": true},
			prState:   map[string]string{"feat/a": "MERGED", "feat/b": "MERGED"},
			wantCands: []string{"feat/b"}},
		{name: "unpushed commits, not landed → skip",
			branches: []string{"feat/a"}, prState: map[string]string{"feat/a": "CLOSED"},
			unpushed:  map[string]unpushed{"feat/a": {n: 2}},
			wantSkips: []string{"feat/a"}},
		{name: "unpushed commits, merged and landed → candidate",
			branches: []string{"feat/a"}, prState: map[string]string{"feat/a": "MERGED"},
			unpushed: map[string]unpushed{"feat/a": {n: 2}}, landed: map[string]bool{"feat/a": true},
			wantCands: []string{"feat/a"}},
		{name: "unpushed commits, merged but commits added after merge → skip",
			branches: []string{"feat/a"}, prState: map[string]string{"feat/a": "MERGED"},
			unpushed:  map[string]unpushed{"feat/a": {n: 1}},
			wantSkips: []string{"feat/a"}},
		{name: "unpushed observation failed → skip",
			branches: []string{"feat/a"}, prState: map[string]string{"feat/a": "CLOSED"},
			unpushed:  map[string]unpushed{"feat/a": {err: errors.New("boom")}},
			wantSkips: []string{"feat/a"}},
		{name: "--force deletes despite unpushed",
			branches: []string{"feat/a"}, prState: map[string]string{"feat/a": "CLOSED"},
			unpushed: map[string]unpushed{"feat/a": {n: 3}}, force: true,
			wantCands: []string{"feat/a"}},
		{name: "older-than with old tip → candidate",
			branches: []string{"feat/a"}, prState: map[string]string{"feat/a": "MERGED"},
			commitAt: old, olderSecs: 30 * 86400, wantCands: []string{"feat/a"}},
		{name: "older-than with recent tip → not candidate",
			branches: []string{"feat/a"}, prState: map[string]string{"feat/a": "MERGED"},
			commitAt: recent, olderSecs: 30 * 86400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := branchProbe{
				localBranches: func() ([]string, error) { return tt.branches, nil },
				checkedOut:    func() (map[string]bool, error) { return tt.checkedOut, nil },
				unpushed:      func(b string) unpushed { return tt.unpushed[b] },
				landed:        func(b string, _ []string) bool { return tt.landed[b] },
				lastCommit:    func(string) time.Time { return tt.commitAt },
			}
			issueState := func(n int) string { return tt.issues[n] }
			cands, skips, err := branchesWithoutWorktree("repo", "/m", probe,
				tt.prState, tt.merged, issueState, tt.olderSecs, tt.force)
			if err != nil {
				t.Fatal(err)
			}
			names := func(cs []branchCand) []string {
				var r []string
				for _, c := range cs {
					r = append(r, c.branch)
				}
				return r
			}
			if !reflect.DeepEqual(names(cands), tt.wantCands) {
				t.Errorf("cands = %v, want %v", names(cands), tt.wantCands)
			}
			if !reflect.DeepEqual(names(skips), tt.wantSkips) {
				t.Errorf("skips = %v, want %v", names(skips), tt.wantSkips)
			}
		})
	}
}

func TestBranchesWithoutWorktree_listFailure(t *testing.T) {
	probe := branchProbe{
		localBranches: func() ([]string, error) { return nil, errors.New("boom") },
		checkedOut:    func() (map[string]bool, error) { return nil, nil },
	}
	if _, _, err := branchesWithoutWorktree("r", "/m", probe, nil, nil, nil, 0, false); err == nil {
		t.Fatal("want error")
	}
}

func TestOptionsBranchCleanup(t *testing.T) {
	tests := []struct {
		name string
		o    Options
		want bool
	}{
		{"--done", Options{Done: true}, true},
		{"--merged alias", Options{Merged: true}, true},
		{"--done --keep-branch", Options{Done: true, KeepBranch: true}, false},
		{"--older-than only", Options{OlderThan: "30d"}, false},
		{"--done --older-than", Options{Done: true, OlderThan: "30d"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.o.branchCleanup(); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
