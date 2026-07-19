package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"wt/internal/buildinfo"
)

// resetBuildInfoCache clears the process-level TTL cache between subtests so
// they observe the buildinfo vars they set instead of a stale earlier snapshot.
func resetBuildInfoCache() {
	buildInfoMu.Lock()
	buildInfoCache = nil
	buildInfoCached = time.Time{}
	buildInfoMu.Unlock()
}

func withBuildinfo(t *testing.T, commit, commitTime, sourceRepo string, start time.Time) {
	t.Helper()
	origC, origCT, origSR, origST := buildinfo.Commit, buildinfo.CommitTime, buildinfo.SourceRepo, buildinfo.StartTime
	buildinfo.Commit = commit
	buildinfo.CommitTime = commitTime
	buildinfo.SourceRepo = sourceRepo
	buildinfo.StartTime = start
	resetBuildInfoCache()
	t.Cleanup(func() {
		buildinfo.Commit = origC
		buildinfo.CommitTime = origCT
		buildinfo.SourceRepo = origSR
		buildinfo.StartTime = origST
		resetBuildInfoCache()
	})
}

// initGitRepo makes a throwaway repo at dir with one commit whose author-date
// is committedAt. Returns after `git commit` finishes.
func initGitRepo(t *testing.T, dir string, committedAt time.Time) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	run("git", "init", "-q", "-b", "main")
	run("git", "config", "user.email", "t@example.com")
	run("git", "config", "user.name", "T")
	// Empty commit with a controlled date.
	ts := committedAt.Format(time.RFC3339)
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", "x")
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(),
		"GIT_AUTHOR_DATE="+ts,
		"GIT_COMMITTER_DATE="+ts,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

func TestComputeBuildInfo_UnknownSourceRepo(t *testing.T) {
	withBuildinfo(t, "", "", "", time.Now())
	resp := computeBuildInfo(context.Background())
	if resp.IsStale {
		t.Fatalf("expected IsStale=false when source repo unknown, got true")
	}
	if resp.Error == "" {
		t.Fatalf("expected Error to be set")
	}
}

func TestComputeBuildInfo_DevBuildSkipsGit(t *testing.T) {
	if !buildinfo.IsDev {
		t.Skip("only meaningful under -tags dev")
	}
	withBuildinfo(t, "", "", "/nonexistent", time.Now())
	resp := computeBuildInfo(context.Background())
	if resp.IsStale {
		t.Fatal("dev build must not report stale")
	}
	if resp.Error != "" {
		t.Fatalf("dev build should skip git and stay error-free, got %q", resp.Error)
	}
}

func TestComputeBuildInfo_FreshBinaryNotStale(t *testing.T) {
	if buildinfo.IsDev {
		t.Skip("prod-only test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	committedAt := time.Now().Add(-1 * time.Hour)
	initGitRepo(t, dir, committedAt)
	// Binary started AFTER the last commit → not stale.
	withBuildinfo(t, "", "", filepath.Clean(dir), time.Now())

	resp := computeBuildInfo(context.Background())
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.IsStale {
		t.Fatalf("expected IsStale=false, got true (head=%d start=%d)", resp.HeadCommitTime, resp.StartTime)
	}
	if resp.HeadCommit == "" {
		t.Fatal("HeadCommit should be populated")
	}
	if resp.HeadBranch != "main" {
		t.Fatalf("HeadBranch=%q want main", resp.HeadBranch)
	}
}

func TestComputeBuildInfo_StaleWhenCommitAfterStart(t *testing.T) {
	if buildinfo.IsDev {
		t.Skip("prod-only test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	committedAt := time.Now()
	initGitRepo(t, dir, committedAt)
	// Binary started 1 hour BEFORE the last commit → stale.
	withBuildinfo(t, "", "", filepath.Clean(dir), time.Now().Add(-1*time.Hour))

	resp := computeBuildInfo(context.Background())
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if !resp.IsStale {
		t.Fatalf("expected IsStale=true, got false (head=%d start=%d)", resp.HeadCommitTime, resp.StartTime)
	}
}

func TestGetBuildInfoHTTP(t *testing.T) {
	withBuildinfo(t, "abc", "", "", time.Now())
	h := New(0)
	srv := httptest.NewServer(h.Routes(http.NotFoundHandler()))
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/build-info")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 200 {
		t.Fatalf("status=%d", res.StatusCode)
	}
	var body BuildInfoResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.BuildCommit != "abc" {
		t.Fatalf("BuildCommit=%q want abc", body.BuildCommit)
	}
}
