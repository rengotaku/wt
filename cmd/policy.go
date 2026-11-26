package cmd

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"wt/internal/wt/core"
	"wt/internal/wt/policy"
)

// registerPolicyCmd wires `wt policy`, which resolves the per-repository AI
// work policy stored in the container. The policy lives outside the git
// repository; this command group never writes into a worktree.
func registerPolicyCmd(parent *cobra.Command) {
	policyCmd := &cobra.Command{
		Use:   "policy",
		Short: "リポジトリごとの AI 作業方針を表示する",
		Long: `リポジトリごとの AI 作業方針（_config.ai_policy）を表示する。

方針はコンテナ（.worktrees.json の隣）に置く。リポジトリの中には置かない。
読み込みは利用ユーザのグローバル方針（CLAUDE.md / AGENTS.md 等）が
` + "`wt policy show`" + ` を呼ぶ。wt は保存と解決だけを担当する。

例:
  wt policy show
  wt policy show --repo myrepo
  wt policy path
  wt policy init`,
	}
	policyCmd.AddCommand(policyPathCmd(), policyShowCmd(), policyInitCmd())
	parent.AddCommand(policyCmd)
}

// policyContainer resolves the container either from --repo or from the current
// git worktree.
func policyContainer(repo string) (string, error) {
	if repo != "" {
		return core.FindContainer(repo)
	}
	_, container, _, err := currentWorktree()
	if err != nil {
		return "", err
	}
	return container, nil
}

func policyPathCmd() *cobra.Command {
	var repo string
	c := &cobra.Command{
		Use:          "path",
		Short:        "方針ファイルの絶対パスを表示する（未設定なら exit 1）",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			container, err := policyContainer(repo)
			if err != nil {
				return err
			}
			return policy.Path(cmd.OutOrStdout(), container)
		},
	}
	c.Flags().StringVar(&repo, "repo", "", "リポジトリ名（省略時はカレント worktree）")
	return c
}

func policyShowCmd() *cobra.Command {
	var repo string
	c := &cobra.Command{
		Use:          "show",
		Short:        "方針の本文を表示する（未設定なら無出力で exit 0）",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			container, err := policyContainer(repo)
			if err != nil {
				return err
			}
			return policy.Show(cmd.OutOrStdout(), container)
		},
	}
	c.Flags().StringVar(&repo, "repo", "", "リポジトリ名（省略時はカレント worktree）")
	return c
}

func policyInitCmd() *cobra.Command {
	var repo string
	c := &cobra.Command{
		Use:          "init",
		Short:        "方針ファイルのテンプレートを作成し _config.ai_policy を設定する",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			container, err := policyContainer(repo)
			if err != nil {
				return err
			}
			return policy.Init(cmd.OutOrStdout(), container)
		},
	}
	c.Flags().StringVar(&repo, "repo", "", "リポジトリ名（省略時はカレント worktree）")
	return c
}

// currentWorktree resolves the worktree root and its container from the cwd.
func currentWorktree() (worktree, container, name string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", "", err
	}
	top, err := core.GitOutput(cwd, "rev-parse", "--show-toplevel")
	if err != nil || top == "" {
		return "", "", "", errors.New("git worktree 内で実行してください")
	}
	return top, filepath.Dir(top), filepath.Base(top), nil
}
