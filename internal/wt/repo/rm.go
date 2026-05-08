package repo

import (
	"errors"
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

	if !opts.Force {
		_, _ = fmt.Fprintln(out, "")
		_, _ = fmt.Fprintln(out, "実行するには --force を指定してください")
		return nil
	}

	if mainDir != "" {
		if listOut, err := core.GitOutput(mainDir, "worktree", "list", "--porcelain"); err == nil {
			for _, line := range strings.Split(listOut, "\n") {
				if !strings.HasPrefix(line, "worktree ") {
					continue
				}
				wt := strings.TrimPrefix(line, "worktree ")
				if wt == mainDir {
					continue
				}
				_ = core.GitRun(mainDir, "worktree", "remove", wt, "--force")
			}
		}
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

// ErrUsage indicates incorrect arguments to repo rm.
var ErrUsage = errors.New("Usage: wt repo rm <repo-name> [--force]") //nolint:staticcheck // user-facing usage string
