package cmd

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"wt/internal/wt/core"
	"wt/internal/wt/devserver"
	"wt/internal/wt/ports"
)

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

func registerServeCmd(parent *cobra.Command) {
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "カレント worktree の dev サービスを割当ポートで起動（未割当なら自動採番。他 worktree には影響しない）",
		RunE: func(cmd *cobra.Command, _ []string) error {
			wt, container, name, err := currentWorktree()
			if err != nil {
				return err
			}
			base, err := ports.EnsureBase(container, name)
			if err != nil {
				return err
			}
			return devserver.Serve(cmd.OutOrStdout(), wt, base)
		},
	}
	downCmd := &cobra.Command{
		Use:   "down",
		Short: "カレント worktree の dev サービスのみ停止（wt-web 本体・他 worktree には影響しない）",
		RunE: func(cmd *cobra.Command, _ []string) error {
			wt, _, _, err := currentWorktree()
			if err != nil {
				return err
			}
			return devserver.Down(cmd.OutOrStdout(), wt)
		},
	}
	parent.AddCommand(serveCmd, downCmd)
}
