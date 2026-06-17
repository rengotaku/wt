package repo

import (
	"fmt"
	"io"
	"strings"

	"wt/internal/wt/core"
	"wt/internal/wt/tree"
)

// RescueMain repairs a worktree-first violation: when the canonical main/master
// folder of <repo> has a non-canonical branch checked out (e.g. someone ran
// `git checkout -b feat/x` directly in the main folder), it moves that branch
// into its own worktree and switches the main folder back to main/master,
// then brings it up to date.
//
// It is intentionally conservative: any uncommitted change (staged, unstaged,
// or untracked) aborts the operation so the user can decide how to preserve it.
// Committed work is never lost — the branch ref is preserved and re-checked-out
// in the new worktree.
func RescueMain(out io.Writer, repoName string) error {
	container, err := core.FindContainer(repoName)
	if err != nil {
		_, _ = fmt.Fprintln(out, "ℹ️  確認: wt repo ls")
		return err
	}

	mainDir, mainName := core.ResolveMain(container)
	if mainDir == "" {
		return fmt.Errorf("%s: main/master worktree が見つかりません", repoName)
	}

	cur, err := core.GitOutput(mainDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("%s: 現在ブランチを取得できません: %w", repoName, err)
	}
	if cur == mainName {
		_, _ = fmt.Fprintf(out, "✅ %s: フォルダ %q は既にブランチ %q です（修復不要）\n", repoName, mainName, mainName)
		return nil
	}
	if cur == "HEAD" {
		return fmt.Errorf("%s: フォルダ %q が detached HEAD 状態です（手動対応が必要）", repoName, mainName)
	}

	// 未コミット（staged / unstaged / untracked いずれも）があれば中断する。
	status, _ := core.GitOutput(mainDir, "status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		_, _ = fmt.Fprintf(out, "⚠️  %s: フォルダ %q（ブランチ %q）に未コミット変更があります。修復を中断しました。\n", repoName, mainName, cur)
		_, _ = fmt.Fprintln(out, "   先に commit / stash してから再実行してください:")
		_, _ = fmt.Fprintf(out, "     git -C %s status\n", mainDir)
		_, _ = fmt.Fprintf(out, "     git -C %s stash   # もしくは commit\n", mainDir)
		return fmt.Errorf("%s: 未コミット変更あり（修復中断）", repoName)
	}

	if !core.GitCheck(mainDir, "rev-parse", "--verify", "--quiet", mainName) {
		return fmt.Errorf("%s: ローカルに %q ブランチがありません（手動対応が必要）", repoName, mainName)
	}

	_, _ = fmt.Fprintf(out, "🔧 %s: フォルダ %q のブランチ %q を別 worktree へ退避します...\n", repoName, mainName, cur)

	// 1) main フォルダを main/master に戻す。feat ブランチの ref は残るのでコミットは保全される。
	if err := core.GitRun(mainDir, "checkout", mainName); err != nil {
		return fmt.Errorf("%s: %q への切替に失敗: %w", repoName, mainName, err)
	}

	// 2) 退避したブランチを専用 worktree として作成（.worktrees.json も登録される）。
	if _, err := tree.Add(nil, out, &tree.AddOptions{Repo: repoName, Branch: cur}); err != nil {
		_, _ = fmt.Fprintf(out, "⚠️  %s: フォルダを %q に戻しましたが worktree 作成に失敗しました。\n", repoName, mainName)
		_, _ = fmt.Fprintf(out, "   ブランチ %q は ref として残っています。手動で: wt tree add --repo %s --branch %s\n", cur, repoName, cur)
		return err
	}

	// 3) main/master を最新へ。失敗しても切替・退避は完了しているので警告のみ。
	if err := core.GitRun(mainDir, "pull", "--ff-only"); err != nil {
		_, _ = fmt.Fprintf(out, "⚠️  %s: %q への切替は完了しましたが pull に失敗しました: %v\n", repoName, mainName, err)
		return nil
	}

	_, _ = fmt.Fprintf(out, "✅ %s: フォルダ %q を %q に復旧し、ブランチ %q を worktree 化しました。\n", repoName, mainName, mainName, cur)
	return nil
}
