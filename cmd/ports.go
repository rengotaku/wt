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

	var pruneYes bool
	pruneCmd := &cobra.Command{
		Use:   "prune",
		Short: "worktree ディレクトリが消えた幽霊エントリを削除しポートブロックを回収する",
		Long: `削除済み worktree の残骸（.worktrees.json に port_base だけ残ったエントリ）を
掃除し、握られていたポートブロックを解放します。既定は候補の表示のみ。
実際に削除するには --yes を指定してください。`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			stale, err := ports.Prune(!pruneYes)
			if err != nil {
				return err
			}
			if len(stale) == 0 {
				_, _ = fmt.Fprintln(out, "回収対象の幽霊エントリはありません")
				return nil
			}
			for _, a := range stale {
				_, _ = fmt.Fprintf(out, "%s %s/%s (%s)\n", pruneMark(pruneYes), a.Repo, a.WtName, ports.RangeString(a.PortBase))
			}
			if pruneYes {
				_, _ = fmt.Fprintf(out, "%d 件の幽霊エントリを削除し、%d ブロックを回収しました\n", len(stale), len(stale))
			} else {
				_, _ = fmt.Fprintf(out, "🔍 %d 件の幽霊エントリ（%d ブロック）が回収可能です。削除するには --yes を指定してください\n", len(stale), len(stale))
			}
			return nil
		},
	}
	pruneCmd.Flags().BoolVarP(&pruneYes, "yes", "y", false, "候補表示ではなく実際に削除する")
	c.AddCommand(pruneCmd)

	parent.AddCommand(c)
}

func pruneMark(done bool) string {
	if done {
		return "🗑️ 削除:"
	}
	return "•"
}
