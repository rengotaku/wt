package handler

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"wt/internal/buildinfo"
)

// BuildInfoResponse is served by GET /api/build-info. The frontend uses it
// to show a "binary may be stale" badge in the header when the source repo
// has advanced past the running binary's start time.
type BuildInfoResponse struct {
	IsDev           bool   `json:"is_dev"`
	BuildCommit     string `json:"build_commit"`
	BuildCommitTime int64  `json:"build_commit_time"` // unix seconds, 0 if unknown
	StartTime       int64  `json:"start_time"`        // unix seconds
	SourceRepo      string `json:"source_repo"`
	HeadCommit      string `json:"head_commit"`
	HeadCommitTime  int64  `json:"head_commit_time"` // unix seconds, 0 if unknown
	HeadBranch      string `json:"head_branch"`
	IsStale         bool   `json:"is_stale"`
	Error           string `json:"error,omitempty"`
}

// buildInfoTTL bounds how often we shell out to git.
const buildInfoTTL = 5 * time.Second

// buildInfoGitTimeout keeps a slow / stuck git call from blocking API responses.
const buildInfoGitTimeout = 2 * time.Second

var (
	buildInfoMu     sync.Mutex
	buildInfoCache  *BuildInfoResponse
	buildInfoCached time.Time
)

func (h *Handler) GetBuildInfo(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, computeBuildInfo(r.Context()))
}

func computeBuildInfo(ctx context.Context) BuildInfoResponse {
	buildInfoMu.Lock()
	if buildInfoCache != nil && time.Since(buildInfoCached) < buildInfoTTL {
		resp := *buildInfoCache
		buildInfoMu.Unlock()
		return resp
	}
	buildInfoMu.Unlock()

	resp := BuildInfoResponse{
		IsDev:       buildinfo.IsDev,
		BuildCommit: buildinfo.Commit,
		StartTime:   buildinfo.StartTime.Unix(),
		SourceRepo:  buildinfo.SourceRepo,
	}
	if t := buildinfo.ParsedCommitTime(); !t.IsZero() {
		resp.BuildCommitTime = t.Unix()
	}

	repo := buildinfo.SourceRepo
	switch {
	case buildinfo.IsDev:
		// Dev builds rebuild + restart on file change (air), so the check
		// is meaningless. Skip git entirely.
	case repo == "":
		resp.Error = "source repo unknown (build without ldflags)"
	case !isGitWorktree(repo):
		resp.Error = "source repo is not a git working tree: " + repo
	default:
		head, headTime, branch, err := readGitHead(ctx, repo)
		if err != nil {
			resp.Error = "git head lookup failed: " + err.Error()
		} else {
			resp.HeadCommit = head
			resp.HeadCommitTime = headTime
			resp.HeadBranch = branch
			// Stale = the source repo's HEAD commit is newer than the
			// running binary's start time. Dynamic builds (IsDev) are
			// excluded above.
			if headTime > 0 && headTime > resp.StartTime {
				resp.IsStale = true
			}
		}
	}

	buildInfoMu.Lock()
	cached := resp
	buildInfoCache = &cached
	buildInfoCached = time.Now()
	buildInfoMu.Unlock()

	return resp
}

// isGitWorktree returns true if repo looks like a git worktree (has a .git
// file or directory) and the git binary is present.
func isGitWorktree(repo string) bool {
	if _, err := exec.LookPath("git"); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		return false
	}
	return true
}

var errUnexpectedGitOutput = errors.New("unexpected git log output")

func readGitHead(ctx context.Context, repo string) (commit string, commitUnix int64, branch string, err error) {
	c1, cancel1 := context.WithTimeout(ctx, buildInfoGitTimeout)
	defer cancel1()
	out, err := exec.CommandContext(c1, "git", "-C", repo, "log", "-1", "--format=%H%n%ct", "HEAD").Output()
	if err != nil {
		return "", 0, "", err
	}
	lines := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)
	if len(lines) != 2 {
		return "", 0, "", errUnexpectedGitOutput
	}
	commit = strings.TrimSpace(lines[0])
	if sec, perr := strconv.ParseInt(strings.TrimSpace(lines[1]), 10, 64); perr == nil {
		commitUnix = sec
	}

	c2, cancel2 := context.WithTimeout(ctx, buildInfoGitTimeout)
	defer cancel2()
	bOut, bErr := exec.CommandContext(c2, "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if bErr == nil {
		branch = strings.TrimSpace(string(bOut))
	}
	return commit, commitUnix, branch, nil
}
