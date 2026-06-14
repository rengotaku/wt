package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"wt/internal/wt/core"
	"wt/internal/wt/devserver"
)

// currentWorktree resolves the worktree root from the current directory along
// with its allocated port base from the registry.
func currentWorktree() (worktree string, base int, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", 0, err
	}
	top, err := core.GitOutput(cwd, "rev-parse", "--show-toplevel")
	if err != nil || top == "" {
		return "", 0, errors.New("git worktree 内で実行してください")
	}
	container := filepath.Dir(top)
	name := filepath.Base(top)
	entries, _ := core.LoadEntries(container)
	e, ok := entries[name]
	if !ok {
		return top, 0, fmt.Errorf("worktree がレジストリに見つかりません: %s", name)
	}
	return top, e.PortBase, nil
}

func registerServeCmd(parent *cobra.Command) {
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: ".wt/dev.toml のサーバーを割当ポートで起動",
		RunE: func(cmd *cobra.Command, _ []string) error {
			wt, base, err := currentWorktree()
			if err != nil {
				return err
			}
			return devserver.Serve(cmd.OutOrStdout(), wt, base)
		},
	}
	downCmd := &cobra.Command{
		Use:   "down",
		Short: "wt serve で起動したサーバーを停止",
		RunE: func(cmd *cobra.Command, _ []string) error {
			wt, _, err := currentWorktree()
			if err != nil {
				return err
			}
			return devserver.Down(cmd.OutOrStdout(), wt)
		},
	}
	parent.AddCommand(serveCmd, downCmd)
}
