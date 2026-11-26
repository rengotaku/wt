package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"wt/internal/wt/status"
)

const statusLong = `worktree の管理状態を一覧する。

既定（AI・スクリプト向け）:
  先頭に "#" で始まる daemon / proxy の行、続いてタブ区切りの表（1 行 1 worktree）。
  列: REPO WORKTREE BRANCH STATE PORTS AUTOSTART URL。空の値は "-"。
  STATE: running / unhealthy（LISTEN すべきサービスが socket を持たない）/ idle（ポート割当済み・未稼働）/
         stale（ポート割当済みだが worktree ディレクトリが無い）/ -（ポート未割当）

--json: 同じ内容とサービスごとの詳細（port / pid / listening / running / headless / unhealthy）を JSON で出す。
--rich: 人が読む表示。リポジトリごとの罫線の表と色分け（端末でないときは色なし）。`

func registerStatusCmd(parent *cobra.Command) {
	var asJSON, rich bool
	c := &cobra.Command{
		Use:   "status",
		Short: "worktree の稼働・ポート・AutoStart・proxy URL と wt daemon の状態を一覧（既定は AI 向け、--rich で人向け）",
		Long:  statusLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if asJSON && rich {
				return errors.New("--json と --rich は同時に指定できません")
			}
			rep, err := status.Collect()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			switch {
			case asJSON:
				return status.WriteJSON(out, &rep)
			case rich:
				return status.WriteRich(out, &rep, isTerminal(out))
			default:
				return status.WriteTSV(out, &rep)
			}
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "JSON で出力する")
	c.Flags().BoolVar(&rich, "rich", false, "人が読む表示にする（罫線・色分け）")
	parent.AddCommand(c)
}

// isTerminal reports whether w is a character device (a terminal).
func isTerminal(w any) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
