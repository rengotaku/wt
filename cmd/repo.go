package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	"wt/internal/wt/repo"
)

func registerRepoCmd(parent *cobra.Command) {
	repoCmd := &cobra.Command{
		Use:   "repo",
		Short: "リポジトリ操作",
	}

	repoCmd.AddCommand(repoSyncCmd())
	repoCmd.AddCommand(repoAddCmd())
	repoCmd.AddCommand(repoRmCmd())
	repoCmd.AddCommand(repoLsCmd())
	repoCmd.AddCommand(repoInitCmd())

	parent.AddCommand(repoCmd)
}

func repoSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "全リポの main/master を git pull --ff-only",
		RunE: func(_ *cobra.Command, _ []string) error {
			repo.Sync()
			return nil
		},
	}
}

func repoAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <url> [container_base]",
		Short: "GitHub URL から clone してコンテナ化",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.New("Usage: wt repo add <url> [container_base]") //nolint:staticcheck // user-facing usage string
			}
			base := ""
			if len(args) >= 2 {
				base = args[1]
			}
			return repo.Add(cmd.OutOrStdout(), args[0], base)
		},
	}
}

func repoRmCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "rm <repo-name> --force",
		Short: "リポジトリ全体を削除（--force 必須）",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return repo.ErrUsage
			}
			return repo.Rm(cmd.OutOrStdout(), args[0], repo.RmOptions{Force: force})
		},
	}
	c.Flags().BoolVarP(&force, "force", "f", false, "実行する（指定しない場合は削除予定の表示のみ）")
	return c
}

func repoLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "登録リポジトリ一覧",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return repo.Ls(cmd.OutOrStdout())
		},
	}
}

func repoInitCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "init <src-dir> [target-dir]",
		Short: "既存 git リポをコンテナ形式に scaffold",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.New("Usage: wt repo init <src-dir> [target-dir] [--force]") //nolint:staticcheck // user-facing usage string
			}
			opts := repo.InitOptions{Src: args[0], Force: force}
			if len(args) >= 2 {
				opts.Target = args[1]
			}
			return repo.Init(cmd.OutOrStdout(), opts)
		},
	}
	c.Flags().BoolVar(&force, "force", false, "lsof で開いているプロセスを検出しても続行する")
	return c
}
