package cmd

import (
	"github.com/spf13/cobra"

	"wt/internal/wt/ports"
)

func registerPortsCmd(parent *cobra.Command) {
	c := &cobra.Command{
		Use:   "ports",
		Short: "worktree のポート割当と稼働状況を表示 (9000-9999)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return ports.List(cmd.OutOrStdout())
		},
	}
	parent.AddCommand(c)
}
