package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	"wt/internal/wt/symlink"
)

func registerSymlinkCmd(parent *cobra.Command) {
	c := &cobra.Command{
		Use:   "symlink",
		Short: "worktree 作成時の symlink 候補を編集",
	}

	c.AddCommand(&cobra.Command{
		Use:   "ls <repo>",
		Short: "symlink 候補一覧",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.New("Usage: wt symlink ls <repo>") //nolint:staticcheck // user-facing usage string
			}
			return symlink.Ls(cmd.OutOrStdout(), args[0])
		},
	})

	c.AddCommand(&cobra.Command{
		Use:   "add <repo> <sub-path>",
		Short: "symlink 候補を追加",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return errors.New("Usage: wt symlink add <repo> <sub-path>") //nolint:staticcheck // user-facing usage string
			}
			return symlink.Add(cmd.OutOrStdout(), args[0], args[1])
		},
	})

	c.AddCommand(&cobra.Command{
		Use:   "rm <repo> <sub-path>",
		Short: "symlink 候補を削除",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return errors.New("Usage: wt symlink rm <repo> <sub-path>") //nolint:staticcheck // user-facing usage string
			}
			return symlink.Rm(cmd.OutOrStdout(), args[0], args[1])
		},
	})

	parent.AddCommand(c)
}
