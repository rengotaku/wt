package gc

import (
	"errors"
	"io"
	"testing"

	"wt/internal/wt/tree"
)

func byKey(rs []Result) map[string]Result {
	m := map[string]Result{}
	for _, r := range rs {
		m[r.Repo+"/"+r.Worktree+"/"+r.Branch] = r
	}
	return m
}

// Branches without a worktree: candidate on --dry-run, deleted with --yes.
func TestRunResults_Branches(t *testing.T) {
	noWt := func() ([]tree.RmEntry, error) { return nil, errors.New("削除可能なworktreeがありません") }
	var deleted []string

	got, err := runResults(io.Discard, Options{Done: true, DryRun: true}, fakeDeps(noWt, &deleted))
	if err != nil {
		t.Fatal(err)
	}
	if r := byKey(got)["r//feat/a"]; r.Result != ResultCandidate {
		t.Errorf("dry-run: %+v", got)
	}

	got, err = runResults(io.Discard, Options{Done: true, Yes: true}, fakeDeps(noWt, &deleted))
	if err != nil {
		t.Fatal(err)
	}
	if r := byKey(got)["r//feat/a"]; r.Result != ResultDeleted {
		t.Errorf("--yes: %+v", got)
	}

	d := fakeDeps(noWt, &deleted)
	d.deleteBranch = func(string, string) error { return errors.New("locked") }
	got, _ = runResults(io.Discard, Options{Done: true, Yes: true}, d)
	if r := byKey(got)["r//feat/a"]; r.Result != ResultFailed || r.Detail != "locked" {
		t.Errorf("failed delete: %+v", got)
	}
}

// Worktrees: kept ones are skipped with the reason, the rest deleted.
func TestRunResults_Worktrees(t *testing.T) {
	f := newGCFixture(t)
	f.add("clean")
	unpushed := f.add("unpushed")
	f.git(unpushed, "commit", "--allow-empty", "-m", "local only")

	got, err := RunResults(io.Discard, Options{OlderThan: "1d", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	m := byKey(got)
	if r := m["repo/clean/clean"]; r.Result != ResultDeleted {
		t.Errorf("clean: %+v", got)
	}
	if r := m["repo/unpushed/unpushed"]; r.Result != ResultSkipped || r.Detail != "リモートに無いコミット 1 件" {
		t.Errorf("unpushed: %+v", got)
	}
}
