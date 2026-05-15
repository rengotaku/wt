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
)

// AddOptions holds CLI inputs for `wt tree add`.
type AddOptions struct {
	Repo        string
	Issue       string
	Description string
	Symlink     bool
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
		opts.Symlink = true
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
	issueRef := "#" + issueNum
	worktreeName := opts.Repo + "--" + strings.ReplaceAll(branchName, "/", "-")
	typeStr := "feature"

	// ── シンボリックリンク（--symlink 指定時のみ、確認なし） ──
	cfg, _ := core.LoadConfig(containerDir)
	candidates := cfg.SymlinkCandidates
	var symlinkTargets []string
	doSymlink := false
	if opts.Symlink && len(candidates) > 0 {
		doSymlink = true
		for _, c := range candidates {
			if fi, err := os.Stat(filepath.Join(mainDir, c)); err == nil && fi.IsDir() {
				symlinkTargets = append(symlinkTargets, c)
			}
		}
	}

	worktreePath := filepath.Join(containerDir, worktreeName)

	// ── プラン表示 ──
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "─────────────────────────────────────")
	_, _ = fmt.Fprintf(out, "リポジトリ: %s\n", opts.Repo)
	_, _ = fmt.Fprintf(out, "ブランチ:   %s\n", branchName)
	_, _ = fmt.Fprintf(out, "worktree:   %s\n", worktreePath)
	if description != "" {
		_, _ = fmt.Fprintf(out, "説明:       %s\n", description)
	}
	if len(symlinkTargets) > 0 {
		_, _ = fmt.Fprintf(out, "symlink:    %s\n", strings.Join(symlinkTargets, " "))
	}
	_, _ = fmt.Fprintln(out, "─────────────────────────────────────")

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
	if doSymlink {
		for _, target := range symlinkTargets {
			parent := filepath.Dir(target)
			if parent != "." {
				_ = os.MkdirAll(filepath.Join(worktreePath, parent), 0o755)
			}
			depth := strings.Count(target, "/") + 1
			relPrefix := strings.Repeat("../", depth)
			link := filepath.Join(worktreePath, target)
			src := relPrefix + mainName + "/" + target
			if err := os.Symlink(src, link); err == nil {
				_, _ = fmt.Fprintf(out, "🔗 %s → %s\n", target, src)
			}
		}
	}

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

	worktreePath := filepath.Join(containerDir, dir)
	if _, err := os.Stat(worktreePath); err == nil {
		return nil, fmt.Errorf("既に存在します: %s", worktreePath)
	}

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

	// ── --symlink 指定時のみ symlink を作成 ──
	if opts.Symlink {
		cfg, _ := core.LoadConfig(containerDir)
		for _, target := range cfg.SymlinkCandidates {
			if fi, err := os.Stat(filepath.Join(mainDir, target)); err != nil || !fi.IsDir() {
				continue
			}
			parent := filepath.Dir(target)
			if parent != "." {
				_ = os.MkdirAll(filepath.Join(worktreePath, parent), 0o755)
			}
			depth := strings.Count(target, "/") + 1
			relPrefix := strings.Repeat("../", depth)
			link := filepath.Join(worktreePath, target)
			src := relPrefix + mainName + "/" + target
			if err := os.Symlink(src, link); err == nil {
				_, _ = fmt.Fprintf(out, "🔗 %s → %s\n", target, src)
			}
		}
	}

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
		Type:    branchType,
		Created: time.Now().Format("2006-01-02"),
		Branch:  opts.Branch,
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
