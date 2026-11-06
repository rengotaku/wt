package gc

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"wt/internal/wt/core"
)

// branchCand is a local branch without a worktree that --done wants to delete.
type branchCand struct {
	mainDir string
	repo    string
	branch  string
	info    string // 候補の理由、またはスキップ理由
}

// branchProbe abstracts the git observations on one repo's local branches so
// tests never spawn git.
type branchProbe struct {
	localBranches func() ([]string, error)
	checkedOut    func() (map[string]bool, error)
	unpushed      func(branch string) unpushed
	landed        func(branch string, mergedHeads []string) bool
	lastCommit    func(branch string) time.Time // olderSecs>0 のときだけ呼ぶ
}

// branchesWithoutWorktree picks, for one repo, the worktree-less local branches
// whose PR/issue is closed (closedCandidate). Branches whose tip holds commits
// that exist neither in a merged PR head nor on any remote go to skips unless
// force. Any observation failure → not a candidate / skipped (fail-closed).
func branchesWithoutWorktree(
	repo, mainDir string,
	p branchProbe,
	prState map[string]string,
	mergedHeads map[string][]string,
	issueState func(int) string,
	olderSecs int64,
	force bool,
) (cands, skips []branchCand, err error) {
	all, err := p.localBranches()
	if err != nil {
		return nil, nil, err
	}
	out, err := p.checkedOut()
	if err != nil {
		return nil, nil, err
	}
	for _, b := range all {
		if b == "main" || b == "master" || out[b] {
			continue
		}
		ok, reason := closedCandidate(b, prState, issueState)
		if !ok {
			continue
		}
		if olderSecs > 0 {
			t := p.lastCommit(b)
			if t.IsZero() || time.Since(t).Seconds() <= float64(olderSecs) {
				continue
			}
		}
		c := branchCand{mainDir: mainDir, repo: repo, branch: b, info: reason}
		if !force {
			landed := prState[b] == "MERGED" && p.landed(b, mergedHeads[b])
			if r := branchKeepReason(p.unpushed(b), landed); r != "" {
				c.info = r
				skips = append(skips, c)
				continue
			}
		}
		cands = append(cands, c)
	}
	return cands, skips, nil
}

// branchKeepReason is keepReason without the worktree-only observations.
func branchKeepReason(u unpushed, landed bool) string {
	if landed {
		return ""
	}
	if u.err != nil {
		return "判定失敗（git rev-list: " + u.err.Error() + "）"
	}
	if u.n > 0 {
		return fmt.Sprintf("リモートに無いコミット %d 件", u.n)
	}
	return ""
}

// gitBranchProbe is the real git-backed probe for a repo's main dir.
func gitBranchProbe(mainDir string) branchProbe {
	return branchProbe{
		localBranches: func() ([]string, error) {
			out, err := core.GitOutput(mainDir, "for-each-ref", "--format=%(refname:short)", "refs/heads")
			if err != nil {
				return nil, err
			}
			return splitLines(out), nil
		},
		checkedOut: func() (map[string]bool, error) {
			out, err := core.GitOutput(mainDir, "worktree", "list", "--porcelain")
			if err != nil {
				return nil, err
			}
			m := map[string]bool{}
			for _, l := range splitLines(out) {
				if ref, ok := strings.CutPrefix(l, "branch refs/heads/"); ok {
					m[ref] = true
				}
			}
			return m, nil
		},
		unpushed: func(b string) unpushed {
			out, err := core.GitOutput(mainDir, "rev-list", "--count", "refs/heads/"+b, "--not", "--remotes")
			if err != nil {
				return unpushed{err: err}
			}
			n, err := strconv.Atoi(strings.TrimSpace(out))
			return unpushed{n: n, err: err}
		},
		landed: func(b string, heads []string) bool {
			tip, err := core.GitOutput(mainDir, "rev-parse", "refs/heads/"+b)
			if err != nil || tip == "" {
				return false
			}
			for _, oid := range heads {
				if oid == tip || core.GitCheck(mainDir, "merge-base", "--is-ancestor", tip, oid) {
					return true
				}
			}
			return false
		},
		lastCommit: func(b string) time.Time {
			out, err := core.GitOutput(mainDir, "log", "-1", "--format=%ct", "refs/heads/"+b)
			if err != nil {
				return time.Time{}
			}
			ct, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
			if err != nil {
				return time.Time{}
			}
			return time.Unix(ct, 0)
		},
	}
}

func splitLines(s string) []string {
	var r []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			r = append(r, l)
		}
	}
	return r
}
