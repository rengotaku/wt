// Package gc cross-repo garbage-collects worktrees by various filters.
package gc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"wt/internal/wt/core"
	"wt/internal/wt/tree"
)

// Options captures CLI flags for `wt tree gc`.
type Options struct {
	Merged       bool
	Closed       bool   // issue/PR が closed/merged な worktree を対象に
	IncludeDirty bool   // --closed 時に未コミット変更ありも含める
	OlderThan    string // "30d", "24h"
	NoTmux       bool
	DryRun       bool
	Yes          bool
	KeepTmux     bool
	KeepBranch   bool
	Force        bool
}

// allowDirty reports whether dirty worktrees should be kept as candidates.
func (o Options) allowDirty() bool {
	return o.Force || (o.Closed && o.IncludeDirty)
}

var issueNumRe = regexp.MustCompile(`issue[-_]?(\d+)`)

// issueNumFromBranch extracts an issue number from a branch like
// "feat/issue-84-abc" → 84. Returns 0 when none.
func issueNumFromBranch(branch string) int {
	m := issueNumRe.FindStringSubmatch(strings.ToLower(branch))
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// closedCandidate decides whether a worktree (by branch) is a GC candidate
// under --closed, returning a human-readable reason. Open/draft PR is never a
// candidate; closed/merged PR is; with no PR, a closed issue referenced by the
// branch qualifies. Unknown → not a candidate (safe side).
func closedCandidate(branch string, prState map[string]string, issueState func(int) string) (match bool, reason string) {
	if branch == "" {
		return false, ""
	}
	if st, ok := prState[branch]; ok {
		switch st {
		case "OPEN":
			return false, ""
		case "MERGED":
			return true, "PR merged"
		case "CLOSED":
			return true, "PR closed"
		}
	}
	if n := issueNumFromBranch(branch); n > 0 {
		if issueState(n) == "CLOSED" {
			return true, fmt.Sprintf("issue #%d closed", n)
		}
	}
	return false, ""
}

var olderThanRegex = regexp.MustCompile(`^(\d+)([dh])$`)

// Run executes the cross-repo GC workflow.
func Run(out io.Writer, opts Options) error {
	olderSecs, err := parseOlderThan(opts.OlderThan)
	if err != nil {
		return err
	}

	items, err := tree.RmEntries()
	if err != nil {
		return err
	}

	type cand struct {
		entry  tree.RmEntry
		branch string
		info   string
	}
	var cands []cand

	prCache := map[string][]mergedPR{}
	if opts.Merged {
		_, _ = fmt.Fprintln(out, "🧹 マージ済み PR を取得中...")
		for i := range items {
			if _, ok := prCache[items[i].MainDir]; ok {
				continue
			}
			prCache[items[i].MainDir] = fetchMergedPRs(items[i].MainDir)
		}
	}

	// --closed: branch→PR state と issue state を repo 単位でキャッシュ。
	prStateCache := map[string]map[string]string{}
	issueCache := map[string]map[int]string{} // mainDir → issueNum → state
	if opts.Closed {
		_, _ = fmt.Fprintln(out, "🧹 closed な issue/PR を判定中...")
		for i := range items {
			md := items[i].MainDir
			if _, ok := prStateCache[md]; !ok {
				prStateCache[md] = fetchPRStates(md)
				issueCache[md] = map[int]string{}
			}
		}
	}

	for i := range items {
		it := &items[i]
		if it.WtName == "main" || it.WtName == "master" {
			continue
		}
		if core.IsDirty(it.WtPath) && !opts.allowDirty() {
			continue
		}

		branch, _ := core.GitOutput(it.WtPath, "branch", "--show-current")
		info := ""

		if opts.Merged {
			if branch == "" {
				continue
			}
			matched := false
			for _, pr := range prCache[it.MainDir] {
				if pr.HeadRefName != branch {
					continue
				}
				date := pr.MergedAt
				if len(date) > 10 {
					date = date[:10]
				}
				info += fmt.Sprintf("PR #%d merged %s", pr.Number, date)
				matched = true
				break
			}
			if !matched {
				continue
			}
		}

		if opts.Closed {
			md := it.MainDir
			issueState := func(n int) string {
				if st, ok := issueCache[md][n]; ok {
					return st
				}
				st := fetchIssueState(md, n)
				issueCache[md][n] = st
				return st
			}
			ok, reason := closedCandidate(branch, prStateCache[md], issueState)
			if !ok {
				continue
			}
			if info != "" {
				info += "  "
			}
			info += reason
		}

		if olderSecs > 0 {
			lastCT, _ := core.GitOutput(it.WtPath, "log", "-1", "--format=%ct")
			ct, _ := strconv.ParseInt(strings.TrimSpace(lastCT), 10, 64)
			if time.Since(time.Unix(ct, 0)).Seconds() <= float64(olderSecs) {
				continue
			}
			lastDate, _ := core.GitOutput(it.WtPath, "log", "-1", "--format=%ci")
			date := strings.TrimSpace(lastDate)
			if i := strings.Index(date, " "); i > 0 {
				date = date[:i]
			}
			if info != "" {
				info += "  "
			}
			info += "last commit " + date
		}

		if opts.NoTmux {
			if exec.Command("tmux", "has-session", "-t", it.WtName).Run() == nil {
				continue
			}
		}

		cands = append(cands, cand{entry: *it, branch: branch, info: info})
	}

	if len(cands) == 0 {
		_, _ = fmt.Fprintln(out, "条件に合う worktree はありません")
		return nil
	}

	display := func(c cand) string {
		brn := c.branch
		if brn != "" {
			brn += "  "
		}
		return fmt.Sprintf("%s/%s   %s%s", c.entry.Repo, c.entry.WtName, brn, c.info)
	}

	if opts.DryRun || !opts.Yes {
		_, _ = fmt.Fprintf(out, "🔍 %d 件の候補:\n", len(cands))
		for i := range cands {
			_, _ = fmt.Fprintln(out, display(cands[i]))
		}
		if !opts.Yes && !opts.DryRun {
			_, _ = fmt.Fprintln(out, "実行するには --yes を指定してください")
		}
		return nil
	}

	rmOpts := tree.RmOptions{
		KeepTmux:   opts.KeepTmux,
		KeepBranch: opts.KeepBranch,
		Force:      opts.allowDirty(),
	}
	for i := range cands {
		c := &cands[i]
		if c.branch == "" {
			_, _ = fmt.Fprintf(out, "⚠️  ブランチ不明のためスキップ: %s\n", c.entry.WtPath)
			continue
		}
		iterOpts := rmOpts
		iterOpts.Branch = c.branch
		iterOpts.Repo = c.entry.Repo
		_ = tree.Rm(out, iterOpts)
	}
	return nil
}

func parseOlderThan(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	m := olderThanRegex.FindStringSubmatch(s)
	if m == nil {
		return 0, errors.New("--older-than の形式が不正（例: 30d, 24h）: " + s)
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	switch m[2] {
	case "d":
		return n * 86400, nil
	case "h":
		return n * 3600, nil
	}
	return 0, errors.New("--older-than の単位は d または h")
}

type mergedPR struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	MergedAt    string `json:"mergedAt"`
}

// repoSlug returns "owner/repo" from the origin remote, or "" if not GitHub.
func repoSlug(mainDir string) string {
	remote, err := core.GitOutput(mainDir, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	slug := remote
	if i := strings.Index(slug, "github.com"); i >= 0 {
		slug = slug[i+len("github.com"):]
		if slug != "" && (slug[0] == ':' || slug[0] == '/') {
			slug = slug[1:]
		}
	}
	return strings.TrimSuffix(slug, ".git")
}

func fetchMergedPRs(mainDir string) []mergedPR {
	slug := repoSlug(mainDir)
	if slug == "" {
		return nil
	}
	out, err := exec.Command("gh", "pr", "list",
		"--state", "merged",
		"--json", "number,headRefName,mergedAt",
		"--limit", "200",
		"--repo", slug).Output()
	if err != nil {
		return nil
	}
	var prs []mergedPR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil
	}
	return prs
}

// fetchPRStates returns branch→state (OPEN/CLOSED/MERGED) for all PRs of a repo.
func fetchPRStates(mainDir string) map[string]string {
	slug := repoSlug(mainDir)
	if slug == "" {
		return map[string]string{}
	}
	out, err := exec.Command("gh", "pr", "list",
		"--state", "all",
		"--json", "headRefName,state",
		"--limit", "300",
		"--repo", slug).Output()
	if err != nil {
		return map[string]string{}
	}
	var prs []struct {
		HeadRefName string `json:"headRefName"`
		State       string `json:"state"`
	}
	if err := json.Unmarshal(out, &prs); err != nil {
		return map[string]string{}
	}
	states := make(map[string]string, len(prs))
	for _, p := range prs {
		// 同一ブランチに複数 PR があれば OPEN を優先（安全側）。
		if cur, ok := states[p.HeadRefName]; ok && cur == "OPEN" {
			continue
		}
		states[p.HeadRefName] = p.State
	}
	return states
}

// fetchIssueState returns "OPEN"/"CLOSED" for an issue, or "" on error.
func fetchIssueState(mainDir string, num int) string {
	slug := repoSlug(mainDir)
	if slug == "" {
		return ""
	}
	out, err := exec.Command("gh", "issue", "view", strconv.Itoa(num),
		"--repo", slug, "--json", "state", "-q", ".state").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
