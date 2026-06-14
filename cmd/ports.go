package cmd

import (
	"github.com/spf13/cobra"

	"wt/internal/wt/ports"
)

func registerPortsCmd(parent *cobra.Command) {
	var all bool
	c := &cobra.Command{
		Use:   "ports",
		Short: "worktree のポート割当と稼働状況を表示（--all でマシン全体）",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if all {
				return ports.DoctorList(cmd.OutOrStdout())
			}
			return ports.List(cmd.OutOrStdout())
		},
	}
	c.Flags().BoolVarP(&all, "all", "a", false, "マシン全体の LISTEN ポートを wt管理/foreign 付きで表示")
	parent.AddCommand(c)
}
