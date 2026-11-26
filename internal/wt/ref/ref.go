// Package ref resolves the "<repo>/<worktree>" target that wt commands take,
// falling back to the worktree that contains the current directory.
package ref

import (
	"os"
	"path/filepath"
	"strings"

	"wt/internal/cli/clierr"
	"wt/internal/wt/core"
)

// Ref is a resolved worktree.
type Ref struct {
	Repo      string
	Worktree  string
	Container string
	Path      string
}

// Resolve turns arg ("<repo>/<worktree>", or "" for the current directory)
// into a Ref. Errors are classified: a malformed arg is a usage error, a
// missing repo or worktree is not found.
func Resolve(arg string) (Ref, error) {
	if arg == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return Ref{}, err
		}
		return FromDir(cwd)
	}
	repo, wt, ok := strings.Cut(arg, "/")
	if !ok || strings.Contains(wt, "/") {
		return Ref{}, clierr.Usagef("対象は <repo>/<worktree> の形で指定してください: %q", arg)
	}
	if err := core.ValidateRepoName(repo); err != nil {
		return Ref{}, clierr.Usagef("%v", err)
	}
	if err := core.ValidateRepoName(wt); err != nil {
		return Ref{}, clierr.Usagef("worktree 名が不正です: %q", wt)
	}
	container, err := core.FindContainer(repo)
	if err != nil {
		return Ref{}, clierr.NotFoundf("リポジトリが見つかりません: %s", repo)
	}
	return lookup(repo, wt, container)
}

// FromDir finds the worktree that contains dir.
func FromDir(dir string) (Ref, error) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return Ref{}, err
	}
	for _, base := range core.WorkspaceDirs() {
		realBase, err := filepath.EvalSymlinks(base)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(realBase, resolved)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) < 2 {
			continue
		}
		return lookup(parts[0], parts[1], filepath.Join(base, parts[0]))
	}
	return Ref{}, clierr.Usagef("worktree の中で実行するか、<repo>/<worktree> を指定してください（現在: %s）", dir)
}

func lookup(repo, wt, container string) (Ref, error) {
	entries, err := core.LoadEntries(container)
	if err != nil {
		return Ref{}, err
	}
	if _, ok := entries[wt]; !ok {
		return Ref{}, clierr.NotFoundf("worktree が見つかりません: %s/%s", repo, wt)
	}
	return Ref{Repo: repo, Worktree: wt, Container: container, Path: filepath.Join(container, wt)}, nil
}
