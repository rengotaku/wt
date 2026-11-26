package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"wt/internal/cli/clierr"
	"wt/internal/cli/out"
	"wt/internal/wt/core"
	"wt/internal/wt/doctor"
)

func registerDoctorCmd(parent *cobra.Command) {
	parent.AddCommand(doctorCmd())
}

func doctorCmd() *cobra.Command {
	var fix bool
	c := &cobra.Command{
		Use:   "doctor [<repo>]",
		Short: "リポジトリ・worktree・ポートの不整合を見つける（--fix で直す）",
		Long: `不整合を CHECK / REPO / WORKTREE / STATUS / DETAIL の表で出す。何も無ければ見出しだけ。

CHECK:
  main_branch       main / master のフォルダに別のブランチが checkout されている
                    （--fix: そのブランチを別の worktree に移し、フォルダを main / master に戻す）
  stale_port        worktree のディレクトリが無いのにポートを持っている（--fix: 登録を消す）
  unallocated_port  main 以外の worktree にポートが割り当てられていない（--fix: 割り当てる）
                    main はポートを service up のときに割り当てるので対象外
  service_config    dev サービスの設定が壊れている（直すのは wt repo config）
  metadata          .worktrees.json を読めない（手で直す）

STATUS は found（見つけた）/ fixed（直した）/ fix_failed（直せなかった）/ manual（自動では直さない）。
fixed 以外が 1 件でも残れば終了コード 4。直す途中の進捗と git の出力は stderr。` + formatHelp,
		Args: maxArgs(1),
	}
	c.Flags().BoolVar(&fix, "fix", false, "直せるものを直す")
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		containers := core.ListContainers()
		if len(args) == 1 {
			container, err := findRepo(args[0])
			if err != nil {
				return err
			}
			containers = []string{container}
		}
		findings, err := doctor.Run(stderr, containers, fix)
		if err != nil {
			return err
		}
		if findings == nil {
			findings = []doctor.Finding{}
		}
		t := &out.Table{Header: []string{"CHECK", "REPO", "WORKTREE", "STATUS", "DETAIL"}}
		left := 0
		for _, f := range findings {
			t.Rows = append(t.Rows, []string{f.Check, f.Repo, f.Worktree, f.Status, f.Detail})
			if f.Unresolved() {
				left++
			}
		}
		if err := out.Render(cmd.OutOrStdout(), o, t, findings, "doctor"); err != nil {
			return err
		}
		if left > 0 {
			hint := ""
			if !fix {
				hint = "（--fix で直せるものは直します）"
			}
			return clierr.Preconditionf("%s", fmt.Sprintf("%d 件の不整合が残っています%s", left, hint))
		}
		return nil
	}
	return c
}
