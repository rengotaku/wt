package tree

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"wt/internal/wt/core"
	"wt/internal/wt/ports"
)

// allocatePortBase returns a free dev port base, printing a warning and
// returning 0 (unallocated) when the band is full. A full band must not block
// worktree creation, which has already succeeded by this point.
func allocatePortBase(out io.Writer) int {
	base, err := ports.Allocate()
	if err != nil {
		_, _ = fmt.Fprintf(out, "⚠️  ポート割当をスキップ: %v\n", err)
		return 0
	}
	return base
}

// AddOptions holds CLI inputs for `wt tree add`.
type AddOptions struct {
	Repo        string
	Issue       string
	Description string
	// Symlink is accepted for backward compatibility only. Symlinking is on by
	// default now, so the flag changes nothing; use NoSymlink to turn it off.
	Symlink     bool
	NoSymlink   bool   // --no-symlink: do not link _config.symlink_candidates
	Yes         bool   // accepted for backward compatibility; behavior is non-interactive regardless
	Branch      string // --branch <name>
	BranchType  string // --type for --branch mode (default "feature")
	IssueURL    string // --issue <github-url>
	PositionDir string // worktree dir name override (positional after --branch)
	Positional  []string
}

// AddResult is returned to the CLI for printing CD: and BRANCH: instructions.
type AddResult struct {
	WorktreePath string
	BranchName   string
}

// symlinkPlan is the resolved view of _config.symlink_candidates for one
// `wt tree add` run. The plan block and the actual creation both read this
// struct, so the printed plan cannot drift from what gets created.
type symlinkPlan struct {
	Disabled bool     // --no-symlink
	Targets  []string // candidates that exist as directories in main
	Missing  []string // candidates registered but absent from main
}

// resolveSymlinkPlan reads _config.symlink_candidates and splits it into paths
// that exist as directories in main and paths that do not. A config read error
// yields an empty plan: `wt tree add` must not fail here, and the plan line then
// reports 候補なし.
func resolveSymlinkPlan(containerDir, mainDir string, noSymlink bool) symlinkPlan {
	if noSymlink {
		return symlinkPlan{Disabled: true}
	}
	cfg, err := core.LoadConfig(containerDir)
	if err != nil {
		return symlinkPlan{}
	}
	var plan symlinkPlan
	for _, c := range cfg.SymlinkCandidates {
		if fi, statErr := os.Stat(filepath.Join(mainDir, c)); statErr == nil && fi.IsDir() {
			plan.Targets = append(plan.Targets, c)
			continue
		}
		plan.Missing = append(plan.Missing, c)
	}
	return plan
}

// line renders the one-line symlink summary of the plan block. The four states
// (on / off / empty / registered-but-absent) must stay distinguishable: a
// registered candidate that is missing from main would otherwise be dropped
// silently, which is the failure this display exists to expose.
func (p symlinkPlan) line(repo string) string {
	switch {
	case p.Disabled:
		return "なし（--no-symlink）"
	case len(p.Targets) > 0 && len(p.Missing) > 0:
		return fmt.Sprintf("%s（main に無い候補: %s）", strings.Join(p.Targets, " "), strings.Join(p.Missing, " "))
	case len(p.Targets) > 0:
		return strings.Join(p.Targets, " ")
	case len(p.Missing) > 0:
		return fmt.Sprintf("なし（候補 %s が main に無い）", strings.Join(p.Missing, " "))
	default:
		return fmt.Sprintf("候補なし（wt symlink add %s <path> で追加）", repo)
	}
}

// printPlan writes the block that tells the caller what is about to happen.
// Both the --issue/positional path and the --branch path use it.
func printPlan(out io.Writer, repo, branch, worktreePath, description string, plan symlinkPlan) {
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "─────────────────────────────────────")
	_, _ = fmt.Fprintf(out, "リポジトリ: %s\n", repo)
	_, _ = fmt.Fprintf(out, "ブランチ:   %s\n", branch)
	_, _ = fmt.Fprintf(out, "worktree:   %s\n", worktreePath)
	if description != "" {
		_, _ = fmt.Fprintf(out, "説明:       %s\n", description)
	}
	_, _ = fmt.Fprintf(out, "symlink:    %s\n", plan.line(repo))
	_, _ = fmt.Fprintln(out, "─────────────────────────────────────")
}

// createCandidateSymlinks links each target inside worktreePath to the sibling
// main worktree and returns the targets it created. A failure is reported as a
// warning rather than swallowed: a failed link used to look identical to a
// successful run, with the real directory left in place.
func createCandidateSymlinks(out io.Writer, worktreePath, mainName string, targets []string) []string {
	var created []string
	for _, target := range targets {
		if parent := filepath.Dir(target); parent != "." {
			_ = os.MkdirAll(filepath.Join(worktreePath, parent), 0o755)
		}
		depth := strings.Count(target, "/") + 1
		relPrefix := strings.Repeat("../", depth)
		link := filepath.Join(worktreePath, target)
		src := relPrefix + mainName + "/" + target
		if err := os.Symlink(src, link); err != nil {
			_, _ = fmt.Fprintf(out, "⚠️  symlink を作成できません: %s (%v)\n", target, err)
			continue
		}
		_, _ = fmt.Fprintf(out, "🔗 %s → %s\n", target, src)
		created = append(created, target)
	}
	return created
}

// issueURLRegex matches https://github.com/<owner>/<repo>/issues/<num>.
var issueURLRegex = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/issues/(\d+)$`)

// Add runs the `wt tree add` workflow (non-interactive — all inputs come from
// flags or positional arguments).
func Add(_ io.Reader, out io.Writer, opts *AddOptions) (*AddResult, error) {
	// ── --issue モード ──
	if opts.IssueURL != "" {
		m := issueURLRegex.FindStringSubmatch(opts.IssueURL)
		if m == nil {
			return nil, fmt.Errorf("URL 形式が不正です: %s\n   期待する形式: https://github.com/<owner>/<repo>/issues/<num>", opts.IssueURL)
		}
		owner, parsedRepo, parsedIssue := m[1], m[2], m[3]
		_, _ = fmt.Fprintf(out, "📡 GitHub Issue を取得中... %s/%s#%s\n", owner, parsedRepo, parsedIssue)
		ghOut, err := exec.Command("gh", "issue", "view", parsedIssue,
			"--repo", owner+"/"+parsedRepo,
			"--json", "title", "-q", ".title").Output()
		if err != nil || strings.TrimSpace(string(ghOut)) == "" {
			return nil, fmt.Errorf("issue が取得できません: %s", opts.IssueURL)
		}
		title := strings.TrimSpace(string(ghOut))
		if opts.Repo == "" {
			opts.Repo = parsedRepo
		}
		if opts.Issue == "" {
			opts.Issue = parsedIssue
		}
		if opts.Description == "" {
			opts.Description = title
		}
	}

	// ── --branch モード（remote/local どちらにも対応） ──
	if opts.Branch != "" {
		return addByBranch(out, opts)
	}

	// ── 残りは positional 引数モード（<repo> <issue> [<desc>]） ──
	if len(opts.Positional) >= 1 && opts.Repo == "" {
		opts.Repo = opts.Positional[0]
	}
	if len(opts.Positional) >= 2 {
		opts.Issue = opts.Positional[1]
	}
	if len(opts.Positional) >= 3 {
		opts.Description = opts.Positional[2]
	}

	repos := availableRepos()
	if len(repos) == 0 {
		return nil, errors.New("コンテナ構成のリポジトリが見つかりません (wt repo init で作成してください)")
	}

	if opts.Repo == "" {
		return nil, fmt.Errorf("リポジトリ名が必要です\n利用可能: %s", strings.Join(repos, " "))
	}
	found := false
	for _, r := range repos {
		if r == opts.Repo {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("リポジトリが見つかりません: %s\n利用可能: %s", opts.Repo, strings.Join(repos, " "))
	}

	if opts.Issue == "" {
		return nil, errors.New("issue番号が必要です（または --branch <name> --repo <repo> で直接指定）") //nolint:staticcheck // user-facing message
	}

	containerDir, mainDir, err := locateContainer(opts.Repo)
	if err != nil {
		return nil, err
	}
	mainName := filepath.Base(mainDir)

	issueNum := opts.Issue
	description := opts.Description
	branchName := newIssueBranchName(issueNum)
	if err := core.ValidateBranchName(containerDir, branchName); err != nil {
		return nil, fmt.Errorf("--issue から自動生成された名前は使えません\n   %w", err)
	}
	issueRef := "#" + issueNum
	worktreeName := opts.Repo + "--" + strings.ReplaceAll(branchName, "/", "-")
	typeStr := "feature"

	// ── symlink 候補の解決（既定で有効、--no-symlink で無効） ──
	plan := resolveSymlinkPlan(containerDir, mainDir, opts.NoSymlink)

	worktreePath := filepath.Join(containerDir, worktreeName)

	// ── プラン表示 ──
	printPlan(out, opts.Repo, branchName, worktreePath, description, plan)

	// ── worktree 作成 ──
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "📥 origin を fetch 中...")
	if out2, err := exec.Command("git", "-C", mainDir, "fetch", "origin").CombinedOutput(); err != nil {
		return nil, fmt.Errorf("fetch 失敗: %w\n%s", err, strings.TrimSpace(string(out2)))
	}
	_, _ = fmt.Fprintln(out, "⏳ worktree を作成中...")
	defaultBranch := mainName
	if v, err := core.GitOutput(mainDir, "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil && v != "" {
		defaultBranch = strings.TrimPrefix(v, "refs/remotes/")
	}
	if err := addWorktreeNewBranch(out, mainDir, worktreePath, branchName, defaultBranch, containerDir); err != nil {
		return nil, err
	}

	// ── シンボリックリンク作成 ──
	symlinkTargets := createCandidateSymlinks(out, worktreePath, mainName, plan.Targets)

	// ── .claude シンボリックリンク（暗黙） ──
	claudeSrc := filepath.Join(mainDir, ".claude")
	claudeDst := filepath.Join(worktreePath, ".claude")
	if fi, err := os.Stat(claudeSrc); err == nil && fi.IsDir() {
		if _, err := os.Lstat(claudeDst); errors.Is(err, os.ErrNotExist) {
			if err := os.Symlink("../"+mainName+"/.claude", claudeDst); err == nil {
				_, _ = fmt.Fprintf(out, "🔗 .claude → ../%s/.claude\n", mainName)
			}
		}
	}

	// ── メタデータ書き込み ──
	entry := core.Entry{
		Type:        typeStr,
		Created:     time.Now().Format("2006-01-02"),
		Branch:      branchName,
		Description: description,
		Issue:       issueRef,
		Symlinked:   symlinkTargets,
		PortBase:    allocatePortBase(out),
	}
	if err := core.PutEntry(containerDir, worktreeName, &entry); err != nil {
		return nil, fmt.Errorf(".worktrees.json の更新に失敗しました（元ファイルは保持）: %w", err)
	}

	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintf(out, "✅ 作成完了: %s\n", worktreePath)
	return &AddResult{WorktreePath: worktreePath, BranchName: branchName}, nil
}

// newIssueBranchName generates a unique branch name for an issue in the format
// feat/issue-<num>-<shortid> to prevent collision on retry.
func newIssueBranchName(issueNum string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "feat/issue-" + issueNum + "-" + hex.EncodeToString(b)
}

// addByBranch handles the non-interactive --branch mode (existing local
// branch / origin/<branch> / new branch from default).
func addByBranch(out io.Writer, opts *AddOptions) (*AddResult, error) {
	if opts.Repo == "" {
		return nil, errors.New("--branch には --repo <repo> が必要です")
	}
	dir := opts.PositionDir
	if len(opts.Positional) >= 1 {
		dir = opts.Positional[0]
	}
	if dir == "" {
		dir = opts.Repo + "--" + strings.ReplaceAll(opts.Branch, "/", "-")
	}

	containerDir, mainDir, err := locateContainer(opts.Repo)
	if err != nil {
		return nil, err
	}
	mainName := filepath.Base(mainDir)

	if err := core.ValidateBranchName(containerDir, opts.Branch); err != nil {
		return nil, err
	}

	worktreePath := filepath.Join(containerDir, dir)
	if _, err := os.Stat(worktreePath); err == nil {
		return nil, fmt.Errorf("既に存在します: %s", worktreePath)
	}

	// ── symlink 候補の解決（既定で有効、--no-symlink で無効） ──
	plan := resolveSymlinkPlan(containerDir, mainDir, opts.NoSymlink)

	// ── プラン表示 ──
	printPlan(out, opts.Repo, opts.Branch, worktreePath, "", plan)

	switch {
	case core.GitCheck(mainDir, "rev-parse", "--verify", "--quiet", opts.Branch):
		if err := addWorktreeExistingBranch(out, mainDir, worktreePath, opts.Branch, containerDir); err != nil {
			return nil, err
		}
	case core.GitCheck(mainDir, "rev-parse", "--verify", "--quiet", "origin/"+opts.Branch):
		if err := addWorktreeNewBranch(out, mainDir, worktreePath, opts.Branch, "origin/"+opts.Branch, containerDir); err != nil {
			return nil, err
		}
	default:
		defaultBranch := mainName
		if v, err := core.GitOutput(mainDir, "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil && v != "" {
			defaultBranch = strings.TrimPrefix(v, "refs/remotes/")
		}
		if err := addWorktreeNewBranch(out, mainDir, worktreePath, opts.Branch, defaultBranch, containerDir); err != nil {
			return nil, err
		}
	}

	// ── シンボリックリンク作成 ──
	_ = createCandidateSymlinks(out, worktreePath, mainName, plan.Targets)

	// ── .claude シンボリックリンク（暗黙） ──
	claudeSrc := filepath.Join(mainDir, ".claude")
	claudeDst := filepath.Join(worktreePath, ".claude")
	if fi, err := os.Stat(claudeSrc); err == nil && fi.IsDir() {
		if _, err := os.Lstat(claudeDst); errors.Is(err, os.ErrNotExist) {
			if err := os.Symlink("../"+mainName+"/.claude", claudeDst); err == nil {
				_, _ = fmt.Fprintf(out, "🔗 .claude → ../%s/.claude\n", mainName)
			}
		}
	}

	branchType := opts.BranchType
	if branchType == "" {
		branchType = "feature"
	}
	entry := core.Entry{
		Type:     branchType,
		Created:  time.Now().Format("2006-01-02"),
		Branch:   opts.Branch,
		PortBase: allocatePortBase(out),
	}
	_ = core.PutEntry(containerDir, dir, &entry)

	_, _ = fmt.Fprintf(out, "✅ 作成完了: %s\n", worktreePath)
	return &AddResult{WorktreePath: worktreePath}, nil
}

// availableRepos returns repos that have a wt-managed main worktree.
func availableRepos() []string {
	var repos []string
	seen := map[string]struct{}{}
	for _, base := range core.WorkspaceDirs() {
		entries, _ := os.ReadDir(base)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			c := filepath.Join(base, e.Name())
			if mainDir, _ := core.ResolveMain(c); mainDir == "" {
				continue
			}
			if _, ok := seen[e.Name()]; ok {
				continue
			}
			seen[e.Name()] = struct{}{}
			repos = append(repos, e.Name())
		}
	}
	sort.Strings(repos)
	return repos
}

// locateContainer locates the container path and main worktree dir for a repo.
func locateContainer(repo string) (containerDir, mainDir string, err error) {
	for _, base := range core.WorkspaceDirs() {
		c := filepath.Join(base, repo)
		if mainDir, _ := core.ResolveMain(c); mainDir != "" {
			return c, mainDir, nil
		}
	}
	return "", "", fmt.Errorf("リポジトリが見つかりません: %s", repo)
}
