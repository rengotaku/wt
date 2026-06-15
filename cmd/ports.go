package cmd

import (
	"fmt"

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

	allocCmd := &cobra.Command{
		Use:   "alloc",
		Short: "未割当の既存 worktree すべてにポートブロックを割り当てる",
		RunE: func(cmd *cobra.Command, _ []string) error {
			made, err := ports.EnsureAll()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(made) == 0 {
				_, _ = fmt.Fprintln(out, "未割当の worktree はありません")
				return nil
			}
			for _, a := range made {
				_, _ = fmt.Fprintf(out, "✔ %s/%s → %s\n", a.Repo, a.WtName, ports.RangeString(a.PortBase))
			}
			_, _ = fmt.Fprintf(out, "%d 件のworktreeに割り当てました\n", len(made))
			return nil
		},
	}
	c.AddCommand(allocCmd)

	parent.AddCommand(c)
}
