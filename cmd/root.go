// Package cmd wires Cobra commands to internal services.
package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"wt/internal/config"
	"wt/internal/logger"
	"wt/internal/wt/tree"
)

// Version is set at build time.
var Version = "dev"

var logOut io.Writer = os.Stderr

var rootCmd = &cobra.Command{
	Use:   "wt",
	Short: "worktree 管理コマンド",
	Long:  "worktree とリポジトリを横断管理する CLI ツール。",
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		c, err := config.Load(cmd.Context())
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		slog.SetDefault(logger.New(logOut, logger.Options{
			AppEnv:   c.AppEnv,
			LogLevel: c.LogLevel,
		}))
		return nil
	},
	// 引数なしで `wt` だけ叩いた場合は wt tree ls 相当の一覧出力。
	RunE: func(_ *cobra.Command, _ []string) error {
		tree.List()
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "バージョンを表示",
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "wt version %s\n", Version)
		return err
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	registerTreeCmd(rootCmd)
	registerRepoCmd(rootCmd)
	registerSymlinkCmd(rootCmd)
	registerWebCmd(rootCmd)
	registerPortsCmd(rootCmd)
}

// Execute runs the root command with a background context.
func Execute() error {
	return rootCmd.ExecuteContext(context.Background())
}
