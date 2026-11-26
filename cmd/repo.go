package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"wt/internal/cli/clierr"
	"wt/internal/cli/out"
	"wt/internal/wt/core"
	"wt/internal/wt/repo"
	"wt/internal/wt/view"
)

func registerRepoCmd(parent *cobra.Command) {
	repoCmd := &cobra.Command{
		Use:   "repo",
		Short: "リポジトリを見る・足す・消す・同期する・設定する",
	}
	repoCmd.AddCommand(repoLsCmd(), repoShowCmd(), repoAddCmd(), repoInitCmd(), repoRmCmd(), repoSyncCmd(),
		repoConfigCmd(), deprecate(repoRescueMainCmd(), "wt doctor --fix"))
	parent.AddCommand(repoCmd)
}

// findRepo validates a repository name and returns its container.
func findRepo(name string) (string, error) {
	if err := core.ValidateRepoName(name); err != nil {
		return "", clierr.Usagef("%v", err)
	}
	container, err := core.FindContainer(name)
	if err != nil {
		return "", clierr.NotFoundf("リポジトリが見つかりません: %s", name)
	}
	return container, nil
}

func printRepo(cmd *cobra.Command, o *out.Opts, name string) error {
	d, err := view.LoadRepo(name)
	if err != nil {
		return err
	}
	return writeRepoShow(cmd.OutOrStdout(), o, &d)
}

func repoAddCmd() *cobra.Command {
	var gitCryptKey string
	c := &cobra.Command{
		Use:   "add <url> [container_base]",
		Short: "GitHub の URL から clone してコンテナにする（既定の置き場は ~/Workspace）",
		Long:  "進捗と git の出力は stderr、結果は repo show の形で stdout に出す。" + formatHelp,
		Args:  rangeArgs(1, 2),
	}
	c.Flags().StringVar(&gitCryptKey, "git-crypt-key", "", "git-crypt の対称鍵ファイルのパス")
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		opts := repo.AddOptions{URL: args[0], GitCryptKey: gitCryptKey}
		if len(args) == 2 {
			opts.Base = args[1]
		}
		name, err := repo.Add(stderr, opts)
		if err != nil {
			return clierr.Preconditionf("%v", err)
		}
		return printRepo(cmd, o, name)
	}
	return c
}

func repoInitCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "init <src-dir> [target-dir]",
		Short: "<src-dir> 直下の git リポジトリをコンテナの形に移す",
		Long: "<src-dir> の子の git リポジトリを <target-dir>（既定は ~/Workspace）へ移し、コンテナにする。\n" +
			"進捗は stderr、作ったリポジトリは REPO / PATH の表で stdout に出す。" + formatHelp,
		Args: rangeArgs(1, 2),
	}
	c.Flags().BoolVar(&force, "force", false, "対象を開いているプロセスがあっても続ける")
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		opts := repo.InitOptions{Src: args[0], Force: force}
		if len(args) == 2 {
			opts.Target = args[1]
		}
		made, initErr := repo.Init(stderr, opts)
		type row struct {
			Repo string `json:"repo"`
			Path string `json:"path"`
		}
		t := &out.Table{Header: []string{"REPO", "PATH"}}
		rows := []row{}
		for _, p := range made {
			t.Rows = append(t.Rows, []string{filepath.Base(p), p})
			rows = append(rows, row{filepath.Base(p), p})
		}
		if err := out.Render(cmd.OutOrStdout(), o, t, rows, ""); err != nil {
			return err
		}
		if initErr != nil {
			return clierr.Preconditionf("%v", initErr)
		}
		return nil
	}
	return c
}

func repoRmCmd() *cobra.Command {
	var yes, dryRun bool
	c := &cobra.Command{
		Use:   "rm <repo>",
		Short: "リポジトリのコンテナを丸ごと消す（確認あり。--yes で省略、--dry-run で対象だけ表示）",
		Long: "コンテナ・その中の worktree・~/code の symlink を消す。端末なら確認を出し、--yes で省略する。\n" +
			"端末でなく --yes も無ければ終了コード 4。出力は KEY / VALUE（repo と result=removed / dry-run）。" + formatHelp,
		Args: exactArgs(1),
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, "確認を省く")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "消さずに対象を表示する")
	// --force was the old way to say "really delete"; keep it working.
	c.Flags().BoolVarP(&yes, "force", "f", false, "--yes と同じ（旧名）")
	_ = c.Flags().MarkHidden("force")
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		container, err := findRepo(args[0])
		if err != nil {
			return err
		}
		if err := repo.Rm(stderr, args[0], repo.RmOptions{}); err != nil {
			return clierr.Preconditionf("%v", err)
		}
		result := "dry-run"
		if !dryRun {
			if err := confirm(yes, args[0]+" を丸ごと消しますか"); err != nil {
				return err
			}
			if err := repo.Rm(stderr, args[0], repo.RmOptions{Force: true}); err != nil {
				return clierr.Preconditionf("%v", err)
			}
			if _, err := os.Lstat(container); err == nil {
				return clierr.External("削除後もコンテナが残っています: "+container, nil)
			}
			result = "removed"
		}
		return out.Render(cmd.OutOrStdout(), o, kvTable([][2]string{{"repo", args[0]}, {"result", result}}),
			map[string]string{"repo": args[0], "result": result}, "")
	}
	return c
}

func repoSyncCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "sync [<repo>]",
		Short: "main / master を git pull --ff-only する（省略時は全リポジトリ）",
		Long: "出力は REPO / BRANCH / RESULT / DETAIL の表。RESULT は updated / up_to_date / skipped / wrong_branch / failed。\n" +
			"failed があれば終了コード 5、wrong_branch があれば 4。~/.config/wt/sync-skip に載ったリポジトリは、名前を指定しない限り skipped。" +
			formatHelp,
		Args: maxArgs(1),
	}
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		name := ""
		if len(args) == 1 {
			if _, err := findRepo(args[0]); err != nil {
				return err
			}
			name = args[0]
		}
		results, err := repo.Sync(stderr, name)
		if err != nil {
			return clierr.NotFoundf("%v", err)
		}
		if results == nil {
			results = []repo.SyncResult{}
		}
		t := &out.Table{Header: []string{"REPO", "BRANCH", "RESULT", "DETAIL"}}
		var failed, wrong int
		for _, r := range results {
			t.Rows = append(t.Rows, []string{r.Repo, r.Branch, r.Result, r.Detail})
			switch r.Result {
			case repo.SyncFailed:
				failed++
			case repo.SyncWrongBranch:
				wrong++
			}
		}
		if err := out.Render(cmd.OutOrStdout(), o, t, results, "repo sync"); err != nil {
			return err
		}
		switch {
		case failed > 0:
			return clierr.External(fmt.Sprintf("%d 件のリポジトリを同期できませんでした", failed), nil)
		case wrong > 0:
			return clierr.Preconditionf("%d 件のリポジトリで main / master に別のブランチが checkout されています（wt doctor --fix）", wrong)
		}
		return nil
	}
	return c
}

func repoRescueMainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rescue-main <repo>",
		Short: "main/master フォルダに別ブランチが checkout された状態を修復（別ブランチを worktree 化して main/master に戻す）",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return repo.RescueMain(cmd.OutOrStdout(), args[0])
		},
	}
}

// rangeArgs accepts between minN and maxN positional arguments.
func rangeArgs(minN, maxN int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) < minN || len(args) > maxN {
			return clierr.Usagef("引数は %d〜%d 個です（%d 個: %s）", minN, maxN, len(args), strings.Join(args, " "))
		}
		return nil
	}
}
