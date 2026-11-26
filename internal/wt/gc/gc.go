// Package gc cross-repo garbage-collects worktrees by various filters.
package gc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"wt/internal/wt/core"
	"wt/internal/wt/tree"
)

// Options captures CLI flags for `wt worktree gc`, grouped as filter /
// retention / safety.
type Options struct {
	// --- filter ---
	Done      bool   // --done: PR merged/closed または issue closed の worktree を対象
	OlderThan string // --older-than: 最終コミットが N(d|h) 以上前
	// --- retention ---
	KeepBranch bool // --keep-branch: ブランチを残す
	// --- safety ---
	DryRun bool // --dry-run: 候補列挙のみ
	Yes    bool // --yes: 確認なしで削除
	Force  bool // --force: dirty も対象に含める
	// Confirm, when set, is called after the candidates are listed and before
	// anything is deleted; an error aborts. The same candidates are deleted
	// (no re-scan), so what was confirmed is exactly what goes. Yes is ignored
	// when Confirm is set.
	Confirm func() error
}

// closedFilter reports whether --done is on.
func (o Options) closedFilter() bool { return o.Done }

// branchCleanup reports whether worktree-less branches are GC targets:
// only with --done, and never with --keep-branch.
func (o Options) branchCleanup() bool { return o.closedFilter() && !o.KeepBranch }

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

// changes / unpushed are git observations of a worktree; err means the
// observation itself failed.
type changes struct {
	uncommitted, untracked int
	err                    error
}

type unpushed struct {
	n   int
	err error
}

func changesOf(wtPath string) changes {
	u, t, err := core.WorktreeChanges(wtPath)
	return changes{uncommitted: u, untracked: t, err: err}
}

func unpushedOf(wtPath string) unpushed {
	n, err := core.UnpushedCommits(wtPath)
	return unpushed{n: n, err: err}
}

// keepReason returns why a candidate must be kept (without --force), or "" when
// it is safe to delete. Work that exists nowhere else — uncommitted changes,
// untracked files, commits on no remote — keeps the worktree. landed (HEAD is
// within a merged PR's head) is exempt from the unpushed check: after a squash
// merge the remote branch is often deleted, so its commits are on no remote
// even though the work landed. Commits added after the merge are not landed.
// A failed observation keeps the worktree (fail-closed).
func keepReason(c changes, u unpushed, landed bool) string {
	if c.err != nil {
		return "判定失敗（git status: " + c.err.Error() + "）"
	}
	if c.uncommitted > 0 {
		return fmt.Sprintf("未コミットの変更 %d 件", c.uncommitted)
	}
	if c.untracked > 0 {
		return fmt.Sprintf("未追跡のファイル %d 件", c.untracked)
	}
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

var olderThanRegex = regexp.MustCompile(`^(\d+)([dh])$`)

// ErrNoFilter is returned by Run when no filter (--done or
// --older-than) is given. Exported as a sentinel (not just an error string)
// so callers — notably the HTTP handler — can classify it as a client error
// (400) via errors.Is, rather than letting it fall through to a generic
// server error (500).
var ErrNoFilter = errors.New(
	"フィルタ条件（--done または --older-than）を1つ以上指定してください。" +
		"未指定のまま実行すると main/master 以外の全 worktree が対象になります",
)

// repoRef identifies a repository by its container name and main worktree.
type repoRef struct{ Repo, MainDir string }

// listRepos enumerates repositories the same way tree.RmEntries walks
// containers, but independently of whether any removable worktree remains.
func listRepos() []repoRef {
	var repos []repoRef
	for _, base := range core.WorkspaceDirs() {
		containers, _ := filepath.Glob(filepath.Join(base, "*"))
		sort.Strings(containers)
		for _, container := range containers {
			if mainDir, _ := core.ResolveMain(container); mainDir != "" {
				repos = append(repos, repoRef{Repo: filepath.Base(container), MainDir: mainDir})
			}
		}
	}
	return repos
}

// deps are the external observations/actions Run depends on; tests inject fakes.
type deps struct {
	rmEntries    func() ([]tree.RmEntry, error)
	repos        func() []repoRef
	prStates     func(mainDir string) (map[string]string, map[string][]string)
	issueState   func(mainDir string, n int) string
	probe        func(mainDir string) branchProbe
	deleteBranch func(mainDir, branch string) error
}

// realDeps wires the real implementations. git's output goes to out, the
// caller's writer (stdout for the old `tree gc`, stderr for `worktree gc`).
func realDeps(out io.Writer) deps {
	return deps{
		rmEntries:    tree.RmEntries,
		repos:        listRepos,
		prStates:     fetchPRStates,
		issueState:   fetchIssueState,
		probe:        gitBranchProbe,
		deleteBranch: func(mainDir, branch string) error { return core.GitRunTo(out, mainDir, "branch", "-D", branch) },
	}
}

// RunResults is Run that also returns what happened to each worktree and
// branch, for callers that print a result table.
func RunResults(out io.Writer, opts Options) ([]Result, error) {
	return runResults(out, opts, realDeps(out))
}

// Results. Fixed words so callers can match on them.
const (
	ResultDeleted   = "deleted"
	ResultSkipped   = "skipped"
	ResultCandidate = "candidate" // listed, not deleted (--dry-run)
	ResultFailed    = "failed"
)

// Result is the outcome for one worktree, or one branch without a worktree
// (Worktree is empty).
type Result struct {
	Repo     string `json:"repo"`
	Worktree string `json:"worktree"`
	Branch   string `json:"branch"`
	Result   string `json:"result"`
	Detail   string `json:"detail"`
}

type cand struct {
	entry  tree.RmEntry
	branch string
	info   string
}

func (c *cand) result(r string) Result { return c.resultWith(r, c.info) }

func (c *cand) resultWith(r, detail string) Result {
	return Result{Repo: c.entry.Repo, Worktree: c.entry.WtName, Branch: c.branch, Result: r, Detail: detail}
}

func runResults(out io.Writer, opts Options, d deps) ([]Result, error) {
	olderSecs, err := parseOlderThan(opts.OlderThan)
	if err != nil {
		return nil, err
	}

	// フィルタ（--done または --older-than）が1つも指定されていない
	// 場合、後段のループはどちらの絞り込みも素通りし main/master 以外の全
	// worktree が対象になってしまう（fail-open）。fail-safe に倒し、明示的に
	// フィルタを要求する。
	if !opts.closedFilter() && olderSecs <= 0 {
		return nil, ErrNoFilter
	}

	items, err := d.rmEntries()
	if err != nil {
		// 削除可能な worktree が無くても、--done ならブランチ走査は続ける。
		if !opts.branchCleanup() {
			return nil, err
		}
		items = nil
	}
	var repos []repoRef
	if opts.branchCleanup() {
		repos = d.repos()
	}

	var cands, skips []cand

	// --done: branch→PR state と issue state を repo 単位でキャッシュ。
	prStateCache := map[string]map[string]string{}
	mergedHeadCache := map[string]map[string][]string{} // mainDir → branch → merged PR head OIDs
	issueCache := map[string]map[int]string{}           // mainDir → issueNum → state
	if opts.closedFilter() {
		_, _ = fmt.Fprintln(out, "🧹 closed/merged な issue/PR を判定中...")
		ensure := func(md string) {
			if _, ok := prStateCache[md]; !ok {
				prStateCache[md], mergedHeadCache[md] = d.prStates(md)
				issueCache[md] = map[int]string{}
			}
		}
		for i := range items {
			ensure(items[i].MainDir)
		}
		for _, r := range repos {
			ensure(r.MainDir)
		}
	}

	for i := range items {
		it := &items[i]
		if it.WtName == "main" || it.WtName == "master" {
			continue
		}

		branch, _ := core.GitOutput(it.WtPath, "branch", "--show-current")
		info := ""
		landed := false

		if opts.closedFilter() {
			md := it.MainDir
			issueState := func(n int) string {
				if st, ok := issueCache[md][n]; ok {
					return st
				}
				st := d.issueState(md, n)
				issueCache[md][n] = st
				return st
			}
			ok, reason := closedCandidate(branch, prStateCache[md], issueState)
			if !ok {
				continue
			}
			info += reason
			landed = prStateCache[md][branch] == "MERGED" &&
				headLanded(it.WtPath, mergedHeadCache[md][branch])
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

		c := cand{entry: *it, branch: branch, info: info}
		if !opts.Force {
			if r := keepReason(changesOf(it.WtPath), unpushedOf(it.WtPath), landed); r != "" {
				c.info = r
				skips = append(skips, c)
				continue
			}
		}
		cands = append(cands, c)
	}

	// --done: worktree の無いローカルブランチ（--keep-branch 時は対象にしない）。
	var bCands, bSkips []branchCand
	if opts.branchCleanup() {
		seen := map[string]bool{}
		for _, r := range repos {
			md := r.MainDir
			if seen[md] {
				continue
			}
			seen[md] = true
			issueState := func(n int) string {
				if st, ok := issueCache[md][n]; ok {
					return st
				}
				st := d.issueState(md, n)
				issueCache[md][n] = st
				return st
			}
			bc, bs, err := branchesWithoutWorktree(r.Repo, md, d.probe(md),
				prStateCache[md], mergedHeadCache[md], issueState, olderSecs, opts.Force)
			if err != nil {
				_, _ = fmt.Fprintf(out, "⚠️  %s のブランチ一覧の取得に失敗: %v\n", r.Repo, err)
				continue
			}
			bCands = append(bCands, bc...)
			bSkips = append(bSkips, bs...)
		}
	}
	bDisplay := func(c branchCand) string {
		return fmt.Sprintf("%s   %s  %s", c.repo, c.branch, c.info)
	}

	display := func(c cand) string {
		brn := c.branch
		if brn != "" {
			brn += "  "
		}
		return fmt.Sprintf("%s/%s   %s%s", c.entry.Repo, c.entry.WtName, brn, c.info)
	}

	if len(skips) > 0 {
		_, _ = fmt.Fprintf(out, "⏭️  %d 件をスキップ（--force で削除対象に含める）:\n", len(skips))
		for i := range skips {
			_, _ = fmt.Fprintln(out, display(skips[i]))
		}
	}

	if len(bSkips) > 0 {
		_, _ = fmt.Fprintf(out, "⏭️  worktree の無いブランチ %d 件をスキップ（--force で削除対象に含める）:\n", len(bSkips))
		for i := range bSkips {
			_, _ = fmt.Fprintln(out, bDisplay(bSkips[i]))
		}
	}

	var results []Result
	for i := range skips {
		results = append(results, skips[i].result(ResultSkipped))
	}
	for i := range bSkips {
		results = append(results, bSkips[i].result(ResultSkipped))
	}

	if len(cands) == 0 && len(bCands) == 0 {
		_, _ = fmt.Fprintln(out, "条件に合う worktree はありません")
		return results, nil
	}

	listCands := func() {
		if len(cands) > 0 {
			_, _ = fmt.Fprintf(out, "🔍 %d 件の候補:\n", len(cands))
			for i := range cands {
				_, _ = fmt.Fprintln(out, display(cands[i]))
			}
		}
		if len(bCands) > 0 {
			_, _ = fmt.Fprintf(out, "🔍 worktree の無いブランチ %d 件:\n", len(bCands))
			for i := range bCands {
				_, _ = fmt.Fprintln(out, bDisplay(bCands[i]))
			}
		}
	}
	if opts.Confirm != nil && !opts.DryRun {
		listCands()
		if err := opts.Confirm(); err != nil {
			return nil, err
		}
	} else if opts.DryRun || !opts.Yes {
		listCands()
		if !opts.Yes && !opts.DryRun {
			_, _ = fmt.Fprintln(out, "実行するには --yes を指定してください")
		}
		for i := range cands {
			results = append(results, cands[i].result(ResultCandidate))
		}
		for i := range bCands {
			results = append(results, bCands[i].result(ResultCandidate))
		}
		return results, nil
	}

	rmOpts := tree.RmOptions{
		KeepBranch:   opts.KeepBranch,
		Force:        opts.Force,
		StrictBranch: true,
	}
	for i := range cands {
		c := &cands[i]
		if c.branch == "" {
			_, _ = fmt.Fprintf(out, "⚠️  ブランチ不明のためスキップ: %s\n", c.entry.WtPath)
			results = append(results, c.resultWith(ResultFailed, "ブランチ不明"))
			continue
		}
		// Delete the scanned worktree itself (its path, which may be outside
		// the container), and only while it is still on the scanned branch:
		// rebuilding the path from names can hit another worktree.
		iterOpts := rmOpts
		iterOpts.Branch = c.branch
		if err := tree.RemoveEntry(out, iterOpts, &c.entry); err != nil {
			var kept *tree.BranchKeptError
			if !errors.As(err, &kept) {
				_, _ = fmt.Fprintf(out, "⚠️  削除できませんでした: %s: %v\n", c.entry.WtPath, err)
			}
			results = append(results, c.resultWith(ResultFailed, err.Error()))
			continue
		}
		results = append(results, c.result(ResultDeleted))
	}
	for i := range bCands {
		c := &bCands[i]
		if err := d.deleteBranch(c.mainDir, c.branch); err != nil {
			_, _ = fmt.Fprintf(out, "⚠️  ブランチ削除に失敗: %s/%s: %v\n", c.repo, c.branch, err)
			results = append(results, c.resultWith(ResultFailed, err.Error()))
			continue
		}
		_, _ = fmt.Fprintf(out, "✅ 削除: %s/%s (branch)\n", c.repo, c.branch)
		results = append(results, c.result(ResultDeleted))
	}
	return results, nil
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

// fetchPRStates returns branch→state (OPEN/CLOSED/MERGED) for all PRs of a
// repo, and branch→head OIDs of its merged PRs.
func fetchPRStates(mainDir string) (states map[string]string, mergedHeads map[string][]string) {
	states, mergedHeads = map[string]string{}, map[string][]string{}
	slug := repoSlug(mainDir)
	if slug == "" {
		return states, mergedHeads
	}
	out, err := exec.Command("gh", "pr", "list",
		"--state", "all",
		"--json", "headRefName,headRefOid,state",
		"--limit", "300",
		"--repo", slug).Output()
	if err != nil {
		return states, mergedHeads
	}
	var prs []struct {
		HeadRefName string `json:"headRefName"`
		HeadRefOid  string `json:"headRefOid"`
		State       string `json:"state"`
	}
	if err := json.Unmarshal(out, &prs); err != nil {
		return states, mergedHeads
	}
	for _, p := range prs {
		if p.State == "MERGED" && p.HeadRefOid != "" {
			mergedHeads[p.HeadRefName] = append(mergedHeads[p.HeadRefName], p.HeadRefOid)
		}
		// 同一ブランチに複数 PR があれば OPEN を優先（安全側）。
		if cur, ok := states[p.HeadRefName]; ok && cur == "OPEN" {
			continue
		}
		states[p.HeadRefName] = p.State
	}
	return states, mergedHeads
}

// headLanded reports whether the worktree's HEAD is the head of a merged PR
// or an ancestor of one, i.e. no commit was added after the merge.
func headLanded(wtPath string, mergedHeads []string) bool {
	head, err := core.GitOutput(wtPath, "rev-parse", "HEAD")
	if err != nil || head == "" {
		return false
	}
	for _, oid := range mergedHeads {
		if oid == head || core.GitCheck(wtPath, "merge-base", "--is-ancestor", head, oid) {
			return true
		}
	}
	return false
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
