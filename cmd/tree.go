package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"wt/internal/wt/gc"
	"wt/internal/wt/tree"
)

func registerTreeCmd(parent *cobra.Command) {
	treeCmd := &cobra.Command{
		Use:   "tree",
		Short: "worktree 操作",
	}

	treeCmd.AddCommand(treeLsCmd())
	treeCmd.AddCommand(treeRmListCmd())
	treeCmd.AddCommand(treeAddCmd())
	treeCmd.AddCommand(treeRmCmd())
	treeCmd.AddCommand(treeGcCmd())
	treeCmd.AddCommand(treeFixCmd())

	parent.AddCommand(treeCmd)
}

func treeLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "worktree 一覧",
		RunE: func(_ *cobra.Command, _ []string) error {
			tree.List()
			return nil
		},
	}
}

func treeRmListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm-list",
		Short: "削除可能な worktree 一覧（生データ）",
		RunE: func(_ *cobra.Command, _ []string) error {
			return tree.RmList()
		},
	}
}

func treeAddCmd() *cobra.Command {
	var opts tree.AddOptions
	c := &cobra.Command{
		Use:   "add [args...]",
		Short: "worktree を作成",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Positional = args
			res, err := tree.Add(os.Stdin, cmd.OutOrStdout(), &opts)
			if err != nil {
				return err
			}
			if res != nil {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "CD:"+res.WorktreePath)
				if res.BranchName != "" {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "BRANCH:"+res.BranchName)
				}
			}
			return nil
		},
	}
	c.Flags().StringVar(&opts.Repo, "repo", "", "リポジトリ名")
	c.Flags().StringVar(&opts.Branch, "branch", "", "ブランチ名（直接指定）")
	c.Flags().StringVar(&opts.BranchType, "type", "feature", "--branch 時の type")
	c.Flags().StringVar(&opts.IssueURL, "issue", "", "GitHub Issue URL")
	c.Flags().BoolVar(&opts.Symlink, "symlink", false, "_config.symlink_candidates を symlink")
	c.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "後方互換用フラグ（現在は常に非対話）")
	return c
}

func treeRmCmd() *cobra.Command {
	var opts tree.RmOptions
	c := &cobra.Command{
		Use:   "rm [args...]",
		Short: "worktree を削除",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Positional = args
			return tree.Rm(cmd.OutOrStdout(), opts)
		},
	}
	c.Flags().StringVar(&opts.Branch, "branch", "", "削除対象をブランチ名で指定")
	c.Flags().StringVar(&opts.Repo, "repo", "", "リポジトリ名（--branch 時必須）")
	c.Flags().BoolVar(&opts.KeepBranch, "keep-branch", false, "ブランチを残す")
	c.Flags().BoolVar(&opts.Merged, "merged", false, "マージ済み worktree を一括削除")
	c.Flags().BoolVar(&opts.KeepTmux, "keep-tmux", false, "tmux セッションを残す")
	c.Flags().BoolVar(&opts.Force, "force", false, "dirty / current session でも強行")
	c.Flags().BoolVar(&opts.DryRun, "dry-run", false, "削除せず候補のみ表示")
	c.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "--merged 時に確認なしで全削除")
	return c
}

func treeGcCmd() *cobra.Command {
	var opts gc.Options
	c := &cobra.Command{
		Use:   "gc",
		Short: "横断 GC（複数フィルタ AND）",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return gc.Run(cmd.OutOrStdout(), opts)
		},
	}
	c.Flags().BoolVar(&opts.Merged, "merged", false, "マージ済み PR に紐づくものだけ")
	c.Flags().BoolVar(&opts.Closed, "closed", false, "issue/PR が closed/merged なものだけ")
	c.Flags().BoolVar(&opts.IncludeDirty, "include-dirty", false, "--closed 時に未コミット変更ありも含める")
	c.Flags().StringVar(&opts.OlderThan, "older-than", "", "最終コミットが N(d|h) 以上前")
	c.Flags().BoolVar(&opts.NoTmux, "no-tmux", false, "tmux セッションが無いものだけ")
	c.Flags().BoolVar(&opts.DryRun, "dry-run", false, "候補のみ列挙")
	c.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "確認なしで全候補を削除")
	c.Flags().BoolVar(&opts.KeepTmux, "keep-tmux", false, "tmux セッションを残す")
	c.Flags().BoolVar(&opts.KeepBranch, "keep-branch", false, "ブランチを残す")
	c.Flags().BoolVar(&opts.Force, "force", false, "dirty でも対象に含める")
	return c
}

func treeFixCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fix [repo]",
		Short: "main worktree のブランチを揃える",
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := ""
			if len(args) > 0 {
				repo = args[0]
			}
			return tree.Fix(cmd.OutOrStdout(), repo)
		},
	}
}
