package view

import (
	"testing"

	"wt/internal/wt/status"
)

func TestRepoSummaries(t *testing.T) {
	containers := []string{"/w/b", "/w/a", "/m/a", "/w/empty"}
	wts := []status.Worktree{
		{Repo: "a", Path: "/w/a/main", State: status.StateNone},
		{Repo: "a", Path: "/w/a/a--1", State: status.StateRunning, AutoStart: true},
		{Repo: "a", Path: "/w/a/a--2", State: status.StateIdle},
		{Repo: "a", Path: "/m/a/a--3", State: status.StateUnhealthy}, // same name, other workspace
		{Repo: "b", Path: "/w/b/b--1", State: status.StateStale},
	}
	got := RepoSummaries(containers, wts)
	want := []RepoSummary{
		{Repo: "a", Path: "/m/a", Worktrees: 1, Unhealthy: 1},
		{Repo: "a", Path: "/w/a", Worktrees: 3, Running: 1, Idle: 1, AutoStart: 1},
		{Repo: "b", Path: "/w/b", Worktrees: 1, Stale: 1},
		{Repo: "empty", Path: "/w/empty"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}
