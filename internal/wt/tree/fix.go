package tree

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"wt/internal/wt/core"
)

// Fix realigns each main worktree HEAD with its folder name (main/master).
// When targetRepo is empty, every wt-managed container is processed.
func Fix(out io.Writer, targetRepo string) error {
	var containers []string
	for _, base := range core.WorkspaceDirs() {
		matches, _ := filepath.Glob(filepath.Join(base, "*", ".worktrees.json"))
		for _, m := range matches {
			c := filepath.Dir(m)
			if targetRepo != "" && filepath.Base(c) != targetRepo {
				continue
			}
			containers = append(containers, c)
		}
	}

	if len(containers) == 0 {
		if targetRepo != "" {
			return fmt.Errorf("リポジトリが見つかりません: %s", targetRepo)
		}
		return errors.New("対象リポジトリがありません")
	}

	var hadErr bool
	for _, c := range containers {
		if err := fixOne(out, c); err != nil {
			hadErr = true
		}
	}
	if hadErr {
		return errors.New("一部のリポで修正に失敗しました")
	}
	return nil
}

func fixOne(out io.Writer, container string) error {
	repo := filepath.Base(container)

	mainDir, mainName := core.ResolveMain(container)
	if mainDir == "" {
		_, _ = fmt.Fprintf(out, "ℹ️  %s: main/master フォルダが見つかりません（スキップ）\n", repo)
		return nil
	}

	current, _ := core.GitOutput(mainDir, "rev-parse", "--abbrev-ref", "HEAD")
	if current == mainName {
		_, _ = fmt.Fprintf(out, "✅ %s: すでに %s ブランチ\n", repo, mainName)
		return nil
	}

	porcelain, _ := core.GitOutput(mainDir, "status", "--porcelain")
	if porcelain != "" {
		fmt.Fprintf(os.Stderr, "❌ %s: %s に未コミットの変更があります（current: %s）\n", repo, mainDir, current)
		for _, line := range strings.Split(porcelain, "\n") {
			fmt.Fprintf(os.Stderr, "    %s\n", line)
		}
		return errors.New("dirty")
	}

	if !core.GitCheck(mainDir, "rev-parse", "--verify", "--quiet", mainName) {
		if core.GitCheck(mainDir, "rev-parse", "--verify", "--quiet", "origin/"+mainName) {
			if err := core.GitRun(mainDir, "branch", mainName, "origin/"+mainName); err != nil {
				fmt.Fprintf(os.Stderr, "❌ %s: %s ブランチの作成に失敗\n", repo, mainName)
				return err
			}
		} else {
			fmt.Fprintf(os.Stderr, "❌ %s: %s ブランチがローカル/リモートともに存在しません\n", repo, mainName)
			return errors.New("missing branch")
		}
	}

	if err := core.GitRun(mainDir, "switch", mainName); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %s: switch に失敗（%s ブランチが他 worktree で使用中の可能性）\n", repo, mainName)
		return err
	}
	_, _ = fmt.Fprintf(out, "✅ %s: %s → %s\n", repo, current, mainName)
	return nil
}
