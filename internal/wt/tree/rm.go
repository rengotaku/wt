package tree

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"wt/internal/wt/core"
)

// RmOptions captures CLI flags for `wt tree rm`.
type RmOptions struct {
	Branch     string // --branch
	Repo       string // --repo
	KeepBranch bool   // --keep-branch
	Merged     bool   // --merged
	Force      bool   // --force
	DryRun     bool   // --dry-run
	Yes        bool   // --yes (--merged 時に必須)
	Quiet      bool   // --quiet: 削除前の一覧を出さない
	NoSize     bool   // --no-size: サイズ計算（du）をスキップする
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
		renderScan(out, t.Repo, scanWorktree(t.WtPath, !opts.NoSize), opts.Quiet)
		return nil
	}

	// 削除の直前にも同じ一覧を出す（--yes でも）
	renderScan(out, t.Repo, scanWorktree(t.WtPath, !opts.NoSize), opts.Quiet)

	branch := branchOfWorktree(t.MainDir, t.WtPath)

	_ = core.GitRun(t.MainDir, "worktree", "remove", t.WtPath, "--force")

	if !opts.KeepBranch && branch != "" && branch != "main" && branch != "master" {
		_ = exec.Command("git", "-C", t.MainDir, "branch", "-D", branch).Run()
	}

	_ = core.DeleteEntry(t.Container, t.WtName)

	_, _ = fmt.Fprintf(out, "✅ 削除: %s\n", t.WtPath)
	return nil
}

// realDir is a directory that disappears with the worktree but that git cannot
// restore: it is untracked or ignored, and it is a real directory rather than a
// symlink into the main worktree.
type realDir struct {
	Path string // relative to the worktree
	Size int64  // bytes; 0 when not measured or when du failed
}

// symlinkEntry is a worktree-level symlink. Removing it loses only the link, so
// it is reported separately from the directories whose contents disappear.
type symlinkEntry struct {
	Path   string
	Target string
}

// rmScan is what `wt tree rm` reports before a worktree disappears.
type rmScan struct {
	Symlinks []symlinkEntry
	Dirs     []realDir
	Measured bool // false when --no-size skipped du
}

const (
	// realDirShownMax caps how many directories are printed; the rest still
	// count toward the total and the warning.
	realDirShownMax = 5
	// largeRealDirSize is the threshold for warning that an ignored directory is
	// large enough to be worth sharing with main instead of deleting.
	largeRealDirSize = 1 << 30 // 1GB
)

// scanWorktree classifies what a worktree holds before deletion: top-level
// symlinks and every untracked/ignored real directory. Nested directories are
// included because node_modules usually sits under frontend/, not at the root.
// withSize=false skips du.
func scanWorktree(wtPath string, withSize bool) rmScan {
	var s rmScan
	s.Measured = withSize

	if entries, err := os.ReadDir(wtPath); err == nil {
		for _, e := range entries {
			if e.Type()&os.ModeSymlink == 0 {
				continue
			}
			target, rerr := os.Readlink(filepath.Join(wtPath, e.Name()))
			if rerr != nil {
				target = "?"
			}
			s.Symlinks = append(s.Symlinks, symlinkEntry{Path: e.Name(), Target: target})
		}
	}
	sort.Slice(s.Symlinks, func(i, j int) bool { return s.Symlinks[i].Path < s.Symlinks[j].Path })
	s.Dirs = listRealDirs(wtPath, withSize)
	return s
}

// total returns the combined size of every real directory.
func (s rmScan) total() int64 {
	var sum int64
	for _, d := range s.Dirs {
		sum += d.Size
	}
	return sum
}

// listRealDirs returns the untracked and ignored real directories inside wtPath.
// Symlinked candidates (.claude, .venv, node_modules, ...) are files from git's
// point of view, so they never carry a trailing slash here and stay out of the
// list. Sorted by path for stable output when sizes are not measured.
func listRealDirs(wtPath string, withSize bool) []realDir {
	seen := map[string]struct{}{}
	var paths []string
	for _, args := range [][]string{
		{"ls-files", "--others", "--directory", "--exclude-standard"},
		{"ls-files", "--others", "--directory", "--ignored", "--exclude-standard"},
	} {
		out, err := core.GitOutput(wtPath, args...)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasSuffix(line, "/") {
				continue
			}
			p := strings.TrimSuffix(line, "/")
			if _, ok := seen[p]; ok {
				continue
			}
			// Belt and braces: a symlinked directory must never be reported as
			// something that disappears, because removing the link is cheap and
			// the target lives in the main worktree.
			if fi, statErr := os.Lstat(filepath.Join(wtPath, p)); statErr != nil || fi.Mode()&os.ModeSymlink != 0 {
				continue
			}
			seen[p] = struct{}{}
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)

	dirs := make([]realDir, 0, len(paths))
	for _, p := range paths {
		var size int64
		if withSize {
			size = dirSize(filepath.Join(wtPath, p))
		}
		dirs = append(dirs, realDir{Path: p, Size: size})
	}
	return dirs
}

// dirSize returns the size of path in bytes, or 0 when du fails.
func dirSize(path string) int64 {
	out, err := exec.Command("du", "-sk", path).Output()
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0
	}
	kb, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0
	}
	return kb * 1024
}

// humanSize renders bytes as B/KB/MB/GB, with one decimal above 1KB.
func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// renderScan prints the scan. quiet suppresses it (the caller still prints the
// target path). Directories are listed largest first and capped at
// realDirShownMax; the total and the 1GB warning cover every directory found.
func renderScan(out io.Writer, repo string, s rmScan, quiet bool) {
	if quiet {
		return
	}
	for _, l := range s.Symlinks {
		_, _ = fmt.Fprintf(out, "   symlink: %s -> %s\n", l.Path, l.Target)
	}
	if len(s.Dirs) == 0 {
		return
	}

	sorted := append([]realDir(nil), s.Dirs...)
	if s.Measured {
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].Size != sorted[j].Size {
				return sorted[i].Size > sorted[j].Size
			}
			return sorted[i].Path < sorted[j].Path
		})
	}
	shown := sorted
	if len(shown) > realDirShownMax {
		shown = shown[:realDirShownMax]
	}
	for _, d := range shown {
		size := "-"
		if s.Measured {
			size = humanSize(d.Size)
		}
		_, _ = fmt.Fprintf(out, "   実体:    %8s  %s\n", size, d.Path)
	}
	if len(sorted) > len(shown) {
		_, _ = fmt.Fprintf(out, "   （他 %d 件。サイズ上位 %d 件を表示）\n", len(sorted)-len(shown), realDirShownMax)
	}
	if s.Measured {
		_, _ = fmt.Fprintf(out, "   実体の合計: %s\n", humanSize(s.total()))
	}
	for _, d := range sorted {
		if s.Measured && d.Size >= largeRealDirSize {
			_, _ = fmt.Fprintf(out, "⚠️ 実体の %s (%s) は削除されます。main と共有するなら wt symlink add %s %s で候補に登録してください\n",
				d.Path, humanSize(d.Size), repo, d.Path)
		}
	}
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

// eligibleForMerged returns the entries the --merged workflow should consider:
// it skips main/master and, when repoFilter is non-empty, keeps only that repo.
// RmEntries() enumerates every container, so without this filter `--repo <name>
// --merged` would delete merged worktrees of *all* repos (issue #88).
func eligibleForMerged(items []RmEntry, repoFilter string) []RmEntry {
	out := make([]RmEntry, 0, len(items))
	for i := range items {
		it := items[i]
		if it.WtName == "main" || it.WtName == "master" {
			continue
		}
		if repoFilter != "" && it.Repo != repoFilter {
			continue
		}
		out = append(out, it)
	}
	return out
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

	items = eligibleForMerged(items, opts.Repo)
	for i := range items {
		it := &items[i]
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
			if opts.DryRun {
				renderScan(out, cands[i].entry.Repo, scanWorktree(cands[i].entry.WtPath, !opts.NoSize), opts.Quiet)
			}
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
