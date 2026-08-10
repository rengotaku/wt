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

// Options captures CLI flags for `wt tree gc`. The three groups (filter /
// retention / safety) map onto the flag groups shown in `wt tree gc --help`.
type Options struct {
	// --- filter ---
	Done      bool   // --done: PR merged/closed または issue closed の worktree を対象
	Merged    bool   // --merged: --done の alias（後方互換）
	OlderThan string // --older-than: 最終コミットが N(d|h) 以上前
	// --- retention ---
	KeepBranch bool // --keep-branch: ブランチを残す
	// --- safety ---
	DryRun bool // --dry-run: 候補列挙のみ
	Yes    bool // --yes: 確認なしで削除
	Force  bool // --force: dirty も対象に含める
}

// closedFilter reports whether the --done family (--done / --merged alias) is on.
func (o Options) closedFilter() bool { return o.Done || o.Merged }

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

// noFilterErrMsg is returned by Run when no filter (--done/--merged or
// --older-than) is given. Exported as a var (not embedded inline) so tests
// can assert on the exact guard without duplicating the string.
var noFilterErrMsg = "フィルタ条件（--done/--merged または --older-than）を1つ以上指定してください。" +
	"未指定のまま実行すると main/master 以外の全 worktree が対象になります"

// Run executes the cross-repo GC workflow.
func Run(out io.Writer, opts Options) error {
	olderSecs, err := parseOlderThan(opts.OlderThan)
	if err != nil {
		return err
	}

	// フィルタ（--done/--merged または --older-than）が1つも指定されていない
	// 場合、後段のループはどちらの絞り込みも素通りし main/master 以外の全
	// worktree が対象になってしまう（fail-open）。fail-safe に倒し、明示的に
	// フィルタを要求する。
	if !opts.closedFilter() && olderSecs <= 0 {
		return errors.New(noFilterErrMsg)
	}

	items, err := tree.RmEntries()
	if err != nil {
		return err
	}

	// --merged は --done の後方互換 alias。使ったら通知する。
	if opts.Merged && !opts.Done {
		_, _ = fmt.Fprintln(out, "ℹ️  --merged は --done の alias です（PR merged/closed または issue closed を対象）")
	}

	type cand struct {
		entry  tree.RmEntry
		branch string
		info   string
	}
	var cands []cand

	// --done: branch→PR state と issue state を repo 単位でキャッシュ。
	prStateCache := map[string]map[string]string{}
	issueCache := map[string]map[int]string{} // mainDir → issueNum → state
	if opts.closedFilter() {
		_, _ = fmt.Fprintln(out, "🧹 closed/merged な issue/PR を判定中...")
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
		if core.IsDirty(it.WtPath) && !opts.Force {
			continue
		}

		branch, _ := core.GitOutput(it.WtPath, "branch", "--show-current")
		info := ""

		if opts.closedFilter() {
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
		KeepBranch: opts.KeepBranch,
		Force:      opts.Force,
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
