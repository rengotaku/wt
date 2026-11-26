package tree

import (
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

// RmOptions captures CLI flags for `wt worktree rm`.
type RmOptions struct {
	Branch     string // --branch
	Repo       string // --repo
	KeepBranch bool   // --keep-branch
	Force      bool   // --force
	DryRun     bool   // --dry-run
	Quiet      bool   // --quiet: 削除前の一覧を出さない
	NoSize     bool   // --no-size: サイズ計算（du）をスキップする
	Positional []string
	// StrictBranch makes a failed branch deletion an error (*BranchKeptError)
	// after the worktree and its entry are gone. Without it the failure is
	// only a warning, as the old commands print it.
	StrictBranch bool
}

// BranchKeptError reports that the worktree was removed but its branch was
// not deleted.
type BranchKeptError struct {
	Branch string
	Err    error
}

func (e *BranchKeptError) Error() string {
	return "worktree は消したが、ブランチ " + e.Branch + " を削除できなかった: " + e.Err.Error()
}

func (e *BranchKeptError) Unwrap() error { return e.Err }

// RemoveAt deletes the worktree wtName of the given container — the exact
// target the caller resolved, not a name looked up again across workspaces.
// RemoveEntry deletes exactly the scanned worktree t (its WtPath, which may be
// outside the container). With opts.Branch set, it refuses when the worktree
// is no longer on that branch.
func RemoveEntry(out io.Writer, opts RmOptions, t *RmEntry) error {
	return performDelete(out, opts, t)
}

func RemoveAt(out io.Writer, opts RmOptions, repo, container, wtName string) error {
	mainDir, _ := core.ResolveMain(container)
	if mainDir == "" {
		return fmt.Errorf("main/master worktree が見つかりません: %s", container)
	}
	return performDelete(out, opts, &RmEntry{
		Repo: repo, WtName: wtName, WtPath: filepath.Join(container, wtName),
		MainDir: mainDir, Container: container,
	})
}

// performDelete executes the actual deletion logic shared by all modes.
func performDelete(out io.Writer, opts RmOptions, t *RmEntry) error {
	if t.WtName == "main" || t.WtName == "master" {
		return fmt.Errorf("main/master は削除できません: %s", t.WtPath)
	}

	if core.IsDirty(t.WtPath) && !opts.Force {
		return fmt.Errorf("未コミット変更のためスキップ: %s（--force で強行）", t.WtPath)
	}
	// Untracked files are not restorable from git and disappear with the
	// worktree, so they need --force too.
	if core.HasUntracked(t.WtPath) && !opts.Force {
		return fmt.Errorf("未追跡のファイルがあるためスキップ: %s（--force で強行）", t.WtPath)
	}

	if opts.DryRun {
		_, _ = fmt.Fprintf(out, "🔍 [dry-run] 削除予定: %s\n", t.WtPath)
		renderScan(out, t.Repo, scanWorktree(t.WtPath, !opts.NoSize), opts.Quiet)
		return nil
	}

	// 削除の直前にも同じ一覧を出す（--yes でも）
	renderScan(out, t.Repo, scanWorktree(t.WtPath, !opts.NoSize), opts.Quiet)

	branch := branchOfWorktree(t.MainDir, t.WtPath)
	if opts.Branch != "" && branch != opts.Branch {
		return fmt.Errorf("ブランチが変わっているため消しません: %s（%s → %s）", t.WtPath, opts.Branch, branch)
	}

	// Stop before touching the registry if git refuses (locked worktree etc.):
	// otherwise the directory stays but wt forgets it.
	if err := core.GitRunTo(out, t.MainDir, "worktree", "remove", t.WtPath, "--force"); err != nil {
		return fmt.Errorf("worktree を削除できませんでした: %s: %w", t.WtPath, err)
	}

	var branchErr error
	if !opts.KeepBranch && branch != "" && branch != "main" && branch != "master" {
		if err := exec.Command("git", "-C", t.MainDir, "branch", "-D", branch).Run(); err != nil {
			_, _ = fmt.Fprintf(out, "⚠️  ブランチを削除できませんでした: %s: %v\n", branch, err)
			branchErr = &BranchKeptError{Branch: branch, Err: err}
		}
	}

	// The registry is keyed by the folder name under the container. A
	// worktree outside the container with the same name is not that entry.
	if samePath(filepath.Join(t.Container, t.WtName), t.WtPath) {
		if err := core.DeleteEntry(t.Container, t.WtName); err != nil {
			return fmt.Errorf("登録の削除に失敗しました: %s: %w", t.WtName, err)
		}
	}

	_, _ = fmt.Fprintf(out, "✅ 削除: %s\n", t.WtPath)
	if opts.StrictBranch {
		return branchErr
	}
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

// rmScan is what `wt worktree rm` reports before a worktree disappears.
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
			_, _ = fmt.Fprintf(out, "⚠️ 実体の %s (%s) は削除されます。main と共有するなら wt repo config add %s symlink %s で候補に登録してください\n",
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

// samePath compares two paths after resolving symlinks of their parents (the
// worktree itself is already gone when this is called).
func samePath(a, b string) bool {
	resolve := func(p string) string {
		if d, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
			return filepath.Join(d, filepath.Base(p))
		}
		return filepath.Clean(p)
	}
	return resolve(a) == resolve(b)
}
