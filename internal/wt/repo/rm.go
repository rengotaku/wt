package repo

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"wt/internal/wt/core"
)

// RmOptions captures CLI flags for `wt repo rm`.
type RmOptions struct {
	Force bool
}

// Rm deletes the entire container directory plus the ~/code/<repo> symlink.
// Without --force, prints the deletion plan and exits without acting.
func Rm(out io.Writer, repoName string, opts RmOptions) error {
	container, err := core.FindContainer(repoName)
	if err != nil {
		_, _ = fmt.Fprintln(out, "ℹ️  確認: wt repo ls")
		return err
	}

	mainDir, _ := core.ResolveMain(container)

	_, _ = fmt.Fprintln(out, "削除対象:")
	_, _ = fmt.Fprintf(out, "  コンテナ: %s\n", container)
	if mainDir != "" {
		if listOut, err := core.GitOutput(mainDir, "worktree", "list"); err == nil {
			_, _ = fmt.Fprintln(out, "  worktree:")
			for _, line := range strings.Split(listOut, "\n") {
				if line == "" {
					continue
				}
				_, _ = fmt.Fprintf(out, "    %s\n", line)
			}
		}
	}

	masterLink := filepath.Join(core.MasterDir(), repoName)
	if fi, err := os.Lstat(masterLink); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		dst, _ := os.Readlink(masterLink)
		_, _ = fmt.Fprintf(out, "  symlink: %s → %s\n", masterLink, dst)
	}

	// Every check runs before the first destructive step (#180), and before
	// the preview returns so a dry run already reports a refusal.
	linked, err := checkRemovable(container, mainDir)
	if err != nil {
		return err
	}
	if !opts.Force {
		return nil
	}
	for _, wt := range linked {
		_ = core.GitRunTo(out, mainDir, "worktree", "remove", wt, "--force")
	}

	if err := os.RemoveAll(container); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✅ コンテナ削除: %s\n", container)

	if fi, err := os.Lstat(masterLink); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(masterLink); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "✅ symlink 削除: %s\n", masterLink)
	}

	_, _ = fmt.Fprintf(out, "✅ 完了: %s\n", repoName)
	return nil
}

// checkRemovable verifies container is a wt-managed container that is a real
// directory directly under a workspace dir, and that every linked worktree
// lives inside it. It returns the linked worktrees (main excluded) to remove.
// Any doubt aborts: this guards a recursive delete.
func checkRemovable(container, mainDir string) ([]string, error) {
	if !core.IsWorkspaceChild(container) {
		return nil, fmt.Errorf("ワークスペース直下ではないため削除しません: %s", container)
	}
	fi, err := os.Lstat(container)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("実ディレクトリではないため削除しません（symlink など）: %s", container)
	}
	if _, err := os.Stat(core.MetaFile(container)); err != nil {
		return nil, fmt.Errorf("wt 管理のコンテナではない（.worktrees.json が無い）ため削除しません: %s", container)
	}
	if mainDir == "" {
		return nil, fmt.Errorf("main / master が見つからないため削除しません: %s", container)
	}
	listOut, err := core.GitOutput(mainDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list に失敗したため削除しません: %w", err)
	}
	realContainer, err := filepath.EvalSymlinks(container)
	if err != nil {
		return nil, err
	}
	realMain, err := filepath.EvalSymlinks(mainDir)
	if err != nil {
		return nil, err
	}
	prefix := realContainer + string(filepath.Separator)
	var linked, outside []string
	for _, line := range strings.Split(listOut, "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		wt := strings.TrimPrefix(line, "worktree ")
		realWt, evalErr := filepath.EvalSymlinks(wt)
		if evalErr != nil {
			// Already gone: nothing to protect, git will prune it.
			realWt = filepath.Clean(wt)
		}
		if realWt == realMain {
			continue
		}
		if !strings.HasPrefix(realWt+string(filepath.Separator), prefix) {
			outside = append(outside, wt)
			continue
		}
		linked = append(linked, wt)
	}
	if len(outside) > 0 {
		return nil, fmt.Errorf("コンテナの外に worktree があるため削除しません（先に git worktree remove で外す）: %s", strings.Join(outside, ", "))
	}
	return linked, nil
}
