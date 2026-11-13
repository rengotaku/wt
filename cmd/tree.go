package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"wt/internal/wt/core"
	"wt/internal/wt/gc"
	"wt/internal/wt/tree"
)

func registerTreeCmd(parent *cobra.Command) {
	treeCmd := &cobra.Command{
		Use:   "tree",
		Short: "worktree 操作",
	}

	treeCmd.AddCommand(treeLsCmd())
	treeCmd.AddCommand(treeRmListCmd())
	treeCmd.AddCommand(treeAddCmd())
	treeCmd.AddCommand(treeRmCmd())
	treeCmd.AddCommand(treeGcCmd())
	treeCmd.AddCommand(treeFixCmd())

	parent.AddCommand(treeCmd)
}

func treeLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "worktree 一覧",
		RunE: func(_ *cobra.Command, _ []string) error {
			tree.List()
			return nil
		},
	}
}

func treeRmListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm-list",
		Short: "削除可能な worktree 一覧（生データ）",
		RunE: func(_ *cobra.Command, _ []string) error {
			return tree.RmList()
		},
	}
}

// treeAddLong is the base help text for `wt tree add`. The current symlink
// candidates are appended by the help func below, because they live in
// .worktrees.json outside the repository and cannot be discovered by grepping
// the repo.
const treeAddLong = `_config.symlink_candidates に登録されたパスを worktree に symlink する。

既定で有効。無効化するときは --no-symlink を付ける。候補は wt symlink add <repo> <path> で登録する。`

// symlinkCandidatesSection renders the help appendix that lists the symlink
// candidates registered for the target repository. Anything that goes wrong is
// reported as text, never as an error: `--help` must not fail.
func symlinkCandidatesSection(repo string) string {
	if repo == "" {
		return "symlink 候補: --repo <repo> を付けて実行すると、そのリポジトリの現在の候補を表示する\n  （確認だけなら wt symlink ls <repo>）"
	}
	container, err := core.FindContainer(repo)
	if err != nil {
		return fmt.Sprintf("symlink 候補: 取得できません（%v）", err)
	}
	cfg, err := core.LoadConfig(container)
	if err != nil {
		return fmt.Sprintf("symlink 候補: 取得できません（%v）", err)
	}
	if len(cfg.SymlinkCandidates) == 0 {
		return fmt.Sprintf("このリポジトリの symlink 候補: 候補なし（wt symlink add %s <path> で追加）", repo)
	}
	return fmt.Sprintf("このリポジトリの symlink 候補: %s", strings.Join(cfg.SymlinkCandidates, " "))
}

func treeAddCmd() *cobra.Command {
	var opts tree.AddOptions
	c := &cobra.Command{
		Use:   "add [args...]",
		Short: "worktree を作成",
		Long:  treeAddLong,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Positional = args
			res, err := tree.Add(os.Stdin, cmd.OutOrStdout(), &opts)
			if err != nil {
				return err
			}
			if res != nil {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "CD:"+res.WorktreePath)
				if res.BranchName != "" {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "BRANCH:"+res.BranchName)
				}
			}
			return nil
		},
	}
	c.Flags().StringVar(&opts.Repo, "repo", "", "リポジトリ名")
	c.Flags().StringVar(&opts.Branch, "branch", "", "ブランチ名（直接指定）")
	c.Flags().StringVar(&opts.BranchType, "type", "feature", "--branch 時の type")
	c.Flags().StringVar(&opts.IssueURL, "issue", "", "GitHub Issue URL")
	c.Flags().BoolVar(&opts.Symlink, "symlink", false, "互換用（既定で symlink されるため実質 no-op）")
	// 既定で symlink されるようになったので、このフラグは互換のためだけに残す。
	// MarkDeprecated は help から隠し、使ったときだけ stderr に案内を出す。
	_ = c.Flags().MarkDeprecated("symlink", "既定で symlink されます。無効化するには --no-symlink を使ってください")
	c.Flags().BoolVar(&opts.NoSymlink, "no-symlink", false, "_config.symlink_candidates による symlink を作らない")
	c.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "後方互換用フラグ（現在は常に非対話）")

	// --help にこのリポジトリの候補を出す。cobra は flags をパースしてから
	// help を呼ぶので、ここから --repo の値が読める。
	defaultHelp := c.HelpFunc()
	c.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		repo, _ := cmd.Flags().GetString("repo")
		cmd.Long = treeAddLong + "\n\n" + symlinkCandidatesSection(repo)
		defaultHelp(cmd, args)
	})
	return c
}

func treeRmCmd() *cobra.Command {
	var opts tree.RmOptions
	c := &cobra.Command{
		Use:   "rm [args...]",
		Short: "worktree を削除",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Positional = args
			return tree.Rm(cmd.OutOrStdout(), opts)
		},
	}
	c.Flags().StringVar(&opts.Branch, "branch", "", "削除対象をブランチ名で指定")
	c.Flags().StringVar(&opts.Repo, "repo", "", "リポジトリ名（--branch 時必須）")
	c.Flags().BoolVar(&opts.KeepBranch, "keep-branch", false, "ブランチを残す")
	c.Flags().BoolVar(&opts.Merged, "merged", false, "マージ済み worktree を一括削除")
	c.Flags().BoolVar(&opts.Force, "force", false, "dirty でも強行")
	c.Flags().BoolVar(&opts.DryRun, "dry-run", false, "削除せず候補と消える実体（サイズ付き）を表示")
	c.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "--merged 時に確認なしで全削除")
	return c
}

func treeGcCmd() *cobra.Command {
	var opts gc.Options
	c := &cobra.Command{
		Use:   "gc",
		Short: "横断 GC（フィルタ／保持／安全の3群フラグ、複数フィルタは AND）",
		Long: `横断 GC — 複数の worktree を条件で絞り込んで削除する。

フラグは意味論で3群に分かれる:

  Filter    どの worktree を対象にするか（複数指定は AND）
    --done            PR merged/closed または issue closed の worktree
                      （worktree の無いローカルブランチも対象。--keep-branch 時は除く）
    --merged          --done の後方互換 alias
    --older-than=STR  最終コミットが STR (30d, 24h) 以上前

  Retention  削除後に何を残すか
    --keep-branch     ブランチを残す

  Safety    実行前の確認
    --dry-run         候補列挙のみ、削除しない
    -y, --yes         確認なしで全候補を削除
    --force           dirty な worktree も対象に含める`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return gc.Run(cmd.OutOrStdout(), opts)
		},
	}
	// Filter
	c.Flags().BoolVar(&opts.Done, "done", false, "PR merged/closed または issue closed の worktree だけ")
	c.Flags().BoolVar(&opts.Merged, "merged", false, "--done の alias（後方互換）")
	c.Flags().StringVar(&opts.OlderThan, "older-than", "", "最終コミットが N(d|h) 以上前")
	// Retention
	c.Flags().BoolVar(&opts.KeepBranch, "keep-branch", false, "ブランチを残す")
	// Safety
	c.Flags().BoolVar(&opts.DryRun, "dry-run", false, "候補のみ列挙")
	c.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "確認なしで全候補を削除")
	c.Flags().BoolVar(&opts.Force, "force", false, "未コミット・未追跡のファイル・リモートに無いコミットがあっても削除する")
	return c
}

func treeFixCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fix [repo]",
		Short: "main worktree のブランチを揃える",
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := ""
			if len(args) > 0 {
				repo = args[0]
			}
			return tree.Fix(cmd.OutOrStdout(), repo)
		},
	}
}
