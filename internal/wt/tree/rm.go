package tree

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"wt/internal/wt/core"
)

// RmOptions captures CLI flags for `wt tree rm`.
type RmOptions struct {
	Branch     string // --branch
	Repo       string // --repo
	KeepBranch bool   // --keep-branch
	Merged     bool   // --merged
	KeepTmux   bool   // --keep-tmux
	Force      bool   // --force
	DryRun     bool   // --dry-run
	Yes        bool   // --yes (--merged 時に必須)
	Positional []string
}

// Rm runs the `wt tree rm` workflow (non-interactive).
func Rm(out io.Writer, opts RmOptions) error {
	if opts.Merged {
		return rmMerged(out, opts)
	}
	if opts.Branch != "" {
		if opts.Repo == "" {
			return errors.New("--branch には --repo <repo> が必要です")
		}
		target, err := resolveByBranch(opts.Repo, opts.Branch)
		if err != nil {
			return err
		}
		return performDelete(out, opts, &target)
	}

	if len(opts.Positional) == 1 {
		items, err := RmEntries()
		if err != nil {
			return err
		}
		// "[name - repo]" or "repo/name" parsing.
		raw := opts.Positional[0]
		repo, name, ok := parseRmListKey(raw)
		if !ok {
			return errors.New("引数形式が不正です（例: 'repo/name' または '--branch <branch> --repo <repo>'）")
		}
		for i := range items {
			if items[i].Repo == repo && items[i].WtName == name {
				return performDelete(out, opts, &items[i])
			}
		}
		return fmt.Errorf("worktree が見つかりません: %s", raw)
	}

	return errors.New("Usage: wt tree rm --branch <branch> --repo <repo> | <repo>/<wt-name> | --merged [--yes]") //nolint:staticcheck // user-facing usage string
}

// resolveByBranch finds an RmEntry by repo+branch.
func resolveByBranch(repo, branch string) (RmEntry, error) {
	container, err := core.FindContainer(repo)
	if err != nil {
		return RmEntry{}, err
	}
	mainDir, _ := core.ResolveMain(container)
	if mainDir == "" {
		return RmEntry{}, fmt.Errorf("main/master worktree が見つかりません: %s", container)
	}

	out, err := core.GitOutput(mainDir, "worktree", "list", "--porcelain")
	if err != nil {
		return RmEntry{}, err
	}
	var lastWt string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			lastWt = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch refs/heads/"+branch):
			return RmEntry{
				Repo:      repo,
				WtName:    filepath.Base(lastWt),
				WtPath:    lastWt,
				MainDir:   mainDir,
				Container: container,
			}, nil
		}
	}
	return RmEntry{}, fmt.Errorf("ブランチ '%s' に対応する worktree が見つかりません", branch)
}

// parseRmListKey extracts repo + name from "[name - repo]" or "repo/name" forms.
func parseRmListKey(s string) (repo, name string, ok bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") {
		if end := strings.Index(s, "]"); end > 0 {
			inner := s[1:end]
			parts := strings.SplitN(inner, " - ", 2)
			if len(parts) == 2 {
				return parts[1], parts[0], true
			}
		}
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i], s[i+1:], true
	}
	return "", "", false
}

// performDelete executes the actual deletion logic shared by all modes.
func performDelete(out io.Writer, opts RmOptions, t *RmEntry) error {
	if t.WtName == "main" || t.WtName == "master" {
		return fmt.Errorf("main/master は削除できません: %s", t.WtPath)
	}

	if core.IsDirty(t.WtPath) && !opts.Force {
		return fmt.Errorf("未コミット変更のためスキップ: %s（--force で強行）", t.WtPath)
	}

	if opts.DryRun {
		_, _ = fmt.Fprintf(out, "🔍 [dry-run] 削除予定: %s\n", t.WtPath)
		return nil
	}

	if !opts.KeepTmux {
		if err := killTmuxSession(out, t.WtName, opts.Force); err != nil {
			return err
		}
	}

	branch := branchOfWorktree(t.MainDir, t.WtPath)

	_ = core.GitRun(t.MainDir, "worktree", "remove", t.WtPath, "--force")

	if !opts.KeepBranch && branch != "" && branch != "main" && branch != "master" {
		_ = exec.Command("git", "-C", t.MainDir, "branch", "-D", branch).Run()
	}

	_ = core.DeleteEntry(t.Container, t.WtName)

	_, _ = fmt.Fprintf(out, "✅ 削除: %s\n", t.WtPath)
	return nil
}

// branchOfWorktree returns the branch name attached to wtPath via git worktree list.
func branchOfWorktree(mainDir, wtPath string) string {
	out, err := core.GitOutput(mainDir, "worktree", "list")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, wtPath+" ") {
			continue
		}
		// Expected format: "<path>  <sha> [<branch>]"
		l := strings.Index(line, "[")
		r := strings.LastIndex(line, "]")
		if l >= 0 && r > l {
			return line[l+1 : r]
		}
	}
	return ""
}

// killTmuxSession terminates the tmux session if it exists; if it's the
// currently attached session, requires force to proceed.
func killTmuxSession(out io.Writer, name string, force bool) error {
	if exec.Command("tmux", "has-session", "-t", name).Run() != nil {
		return nil
	}
	cur, _ := exec.Command("tmux", "display-message", "-p", "#S").Output()
	curName := strings.TrimSpace(string(cur))
	if curName == name {
		if !force {
			_, _ = fmt.Fprintf(out, "⚠️  現在 attach 中のセッションはスキップ: %s（--force で強行）\n", name)
			return errors.New("aborted: current tmux session")
		}
		_, _ = fmt.Fprintf(out, "⚠️  現在のセッションを強制 kill: %s\n", name)
	}
	if err := exec.Command("tmux", "kill-session", "-t", name).Run(); err != nil {
		return nil //nolint:nilerr // tmux exit codes are advisory here
	}
	if curName != name {
		_, _ = fmt.Fprintf(out, "🔪 tmux kill-session: %s\n", name)
	}
	return nil
}

// rmMerged drives the --merged workflow.
// 候補をリスト表示し、--yes 指定時のみ全削除する。
func rmMerged(out io.Writer, opts RmOptions) error {
	items, err := RmEntries()
	if err != nil {
		return err
	}

	prCache := map[string][]mergedPR{}
	type cand struct {
		entry RmEntry
		pr    mergedPR
		brn   string
	}
	var cands []cand

	_, _ = fmt.Fprintln(out, "🧹 マージ済み worktree を検出中...")

	for i := range items {
		it := &items[i]
		if it.WtName == "main" || it.WtName == "master" {
			continue
		}
		prs, ok := prCache[it.MainDir]
		if !ok {
			prs = fetchMergedPRs(it.MainDir)
			prCache[it.MainDir] = prs
		}
		brn, _ := core.GitOutput(it.WtPath, "branch", "--show-current")
		if brn == "" {
			continue
		}
		var match *mergedPR
		for j := range prs {
			if prs[j].HeadRefName == brn {
				match = &prs[j]
				break
			}
		}
		if match == nil {
			continue
		}
		cands = append(cands, cand{entry: *it, pr: *match, brn: brn})
	}

	if len(cands) == 0 {
		_, _ = fmt.Fprintln(out, "マージ済みの worktree はありません")
		return nil
	}

	display := func(c cand) string {
		date := c.pr.MergedAt
		if len(date) > 10 {
			date = date[:10]
		}
		return fmt.Sprintf("%s/%s   %s   PR #%d merged %s",
			c.entry.Repo, c.entry.WtName, c.brn, c.pr.Number, date)
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

	for i := range cands {
		_ = performDelete(out, opts, &cands[i].entry)
	}
	return nil
}

// mergedPR is the JSON shape returned by `gh pr list --json number,headRefName,mergedAt`.
type mergedPR struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	MergedAt    string `json:"mergedAt"`
}

// fetchMergedPRs queries gh for merged PRs of the repo backing mainDir.
func fetchMergedPRs(mainDir string) []mergedPR {
	remote, err := core.GitOutput(mainDir, "remote", "get-url", "origin")
	if err != nil {
		return nil
	}
	slug := remote
	if i := strings.Index(slug, "github.com"); i >= 0 {
		slug = slug[i+len("github.com"):]
		if slug != "" && (slug[0] == ':' || slug[0] == '/') {
			slug = slug[1:]
		}
	}
	slug = strings.TrimSuffix(slug, ".git")
	if slug == "" {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"--state", "merged",
		"--json", "number,headRefName,mergedAt",
		"--limit", "200",
		"--repo", slug)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var prs []mergedPR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil
	}
	return prs
}
