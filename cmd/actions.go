package cmd

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"wt/internal/cli/clierr"
	"wt/internal/cli/out"
	"wt/internal/wt/devserver"
	"wt/internal/wt/gc"
	"wt/internal/wt/ports"
	"wt/internal/wt/ref"
	"wt/internal/wt/status"
	"wt/internal/wt/tree"
	"wt/internal/wt/view"
)

// Write commands of the noun-verb layout (#188, step 2). They print progress
// to stderr and the resulting resource to stdout in the same form as `show`.

// stderr is where write commands report progress. A var for tests.
var stderr io.Writer = os.Stderr

// stdin is where confirmations are read. A var for tests.
var stdin io.Reader = os.Stdin

// confirm asks before a destructive step. --yes skips it; without a terminal
// and without --yes it refuses (exit 4) instead of hanging on a prompt.
func confirm(yes bool, question string) error {
	if yes {
		return nil
	}
	if !out.IsTerminal(stdin) {
		return clierr.Preconditionf("確認が要ります。--yes を付けて実行してください（--dry-run で対象だけ確認できます）")
	}
	_, _ = fmt.Fprintf(stderr, "%s [y/N]: ", question)
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return clierr.Preconditionf("中止しました")
	}
	return nil
}

// printWorktree prints r in the `worktree show` form.
func printWorktree(w io.Writer, o *out.Opts, r ref.Ref) error {
	rep, err := status.Collect()
	if err != nil {
		return err
	}
	d, err := view.LoadWorktree(r, &rep)
	if err != nil {
		return err
	}
	return writeWorktreeShow(w, o, &d)
}

func refArg(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	return ""
}

func worktreeAddCmd() *cobra.Command {
	var opts tree.AddOptions
	c := &cobra.Command{
		Use:   "add <repo> (--issue <URL> | --branch <branch>)",
		Short: "worktree を作る（出力は worktree show と同じ形）",
		Long:  "worktree を作り、作った worktree を wt worktree show と同じ形で出す。進捗は stderr。" + formatHelp,
		Args:  exactArgs(1),
	}
	c.Flags().StringVar(&opts.IssueURL, "issue", "", "GitHub Issue の URL（ブランチ名を自動で決める）")
	c.Flags().StringVar(&opts.Branch, "branch", "", "ブランチ名")
	c.Flags().StringVar(&opts.BranchType, "type", "feature", "--branch のときの type")
	c.Flags().StringVar(&opts.PositionDir, "dir", "", "worktree のディレクトリ名（--branch のとき）")
	c.Flags().BoolVar(&opts.NoSymlink, "no-symlink", false, "symlink 候補を張らない")
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		if (opts.IssueURL == "") == (opts.Branch == "") {
			return clierr.Usagef("--issue と --branch のどちらか 1 つを指定してください")
		}
		opts.Repo = args[0]
		opts.Yes = true
		res, err := tree.Add(strings.NewReader(""), stderr, &opts)
		if err != nil {
			return err
		}
		r, err := ref.Resolve(args[0] + "/" + filepath.Base(res.WorktreePath))
		if err != nil {
			return err
		}
		return printWorktree(cmd.OutOrStdout(), o, r)
	}
	return c
}

func worktreeRmCmd() *cobra.Command {
	var opts tree.RmOptions
	var yes bool
	c := &cobra.Command{
		Use:   "rm <repo>/<worktree>",
		Short: "worktree を消す（確認あり。--yes で省略、--dry-run で対象だけ表示）",
		Long: "worktree とブランチを消す。端末なら確認を出し、--yes で省略する。端末でなく --yes も無ければ終了コード 4。\n" +
			"出力は KEY / VALUE（worktree と result=removed / dry-run）。" + formatHelp,
		Args: exactArgs(1),
	}
	c.Flags().BoolVar(&opts.KeepBranch, "keep-branch", false, "ブランチを残す")
	c.Flags().BoolVar(&opts.Force, "force", false, "未コミット・未追跡のファイルがあっても消す")
	c.Flags().BoolVar(&opts.DryRun, "dry-run", false, "消さずに、消える symlink と実体を表示する")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "確認を省く")
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		r, err := ref.Resolve(args[0])
		if err != nil {
			return err
		}
		key := r.Repo + "/" + r.Worktree
		opts.Positional = []string{key}
		opts.NoSize = true
		if !opts.DryRun {
			preview := opts
			preview.DryRun = true
			if err := tree.Rm(stderr, preview); err != nil {
				return err
			}
			if err := confirm(yes, key+" を消しますか"); err != nil {
				return err
			}
		}
		if err := tree.Rm(stderr, opts); err != nil {
			return err
		}
		result := "removed"
		if opts.DryRun {
			result = "dry-run"
		}
		return out.Render(cmd.OutOrStdout(), o, kvTable([][2]string{{"worktree", key}, {"result", result}}),
			map[string]string{"worktree": key, "result": result}, "")
	}
	return c
}

func worktreeGcCmd() *cobra.Command {
	var opts gc.Options
	c := &cobra.Command{
		Use:   "gc",
		Short: "条件に合う worktree をまとめて消す（--done / --older-than。確認あり）",
		Long: "条件に合う worktree をまとめて消す。--done は PR merged / closed か issue closed、--older-than は最終コミットの古さ（30d, 24h）。\n" +
			"候補は stderr に出す。端末なら確認を出し、--yes で省略する。端末でなく --yes も無ければ終了コード 4。",
		Args: noArgs,
	}
	c.Flags().BoolVar(&opts.Done, "done", false, "PR merged / closed か issue closed の worktree")
	c.Flags().StringVar(&opts.OlderThan, "older-than", "", "最終コミットが N(d|h) 以上前")
	c.Flags().BoolVar(&opts.KeepBranch, "keep-branch", false, "ブランチを残す")
	c.Flags().BoolVar(&opts.DryRun, "dry-run", false, "候補だけ出す")
	c.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "確認を省く")
	c.Flags().BoolVar(&opts.Force, "force", false, "未コミット・未追跡のファイル・リモートに無いコミットがあっても消す")
	c.RunE = func(_ *cobra.Command, _ []string) error {
		if !opts.Done && opts.OlderThan == "" {
			return clierr.Usagef("--done か --older-than を指定してください")
		}
		if opts.DryRun {
			return gc.Run(stderr, opts)
		}
		if !opts.Yes {
			preview := opts
			preview.DryRun = true
			if err := gc.Run(stderr, preview); err != nil {
				return err
			}
			if err := confirm(false, "上の候補を消しますか"); err != nil {
				return err
			}
			opts.Yes = true
		}
		return gc.Run(stderr, opts)
	}
	return c
}

func worktreeSetCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "set <repo>/<worktree> <key>=<value>...",
		Short: "worktree 単位の設定を変える（autostart=on|off）",
		Long:  "worktree 単位の設定を変え、結果を wt worktree show と同じ形で出す。キー: autostart（on / off）。" + formatHelp,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) < 2 {
				return clierr.Usagef("<repo>/<worktree> と <key>=<value> を指定してください")
			}
			return nil
		},
	}
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		r, err := ref.Resolve(args[0])
		if err != nil {
			return err
		}
		for _, kv := range args[1:] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return clierr.Usagef("<key>=<value> の形で指定してください: %q", kv)
			}
			switch k {
			case "autostart":
				if v != "on" && v != "off" {
					return clierr.Usagef("autostart は on か off: %q", v)
				}
				if err := tree.SetAutoStart(r.Repo, r.Worktree, v == "on"); err != nil {
					return err
				}
			default:
				return clierr.Usagef("知らないキーです: %q（使えるキー: autostart）", k)
			}
		}
		return printWorktree(cmd.OutOrStdout(), o, r)
	}
	return c
}

func worktreeUpdateCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "update [<repo>/<worktree>]",
		Short: "ブランチの最新を git pull --ff-only で取り込む（未コミットの変更があると止まる）",
		Long:  "ブランチの最新を取り込み、結果を wt worktree show と同じ形で出す。git の出力は stderr。" + formatHelp,
		Args:  maxArgs(1),
	}
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		r, err := ref.Resolve(refArg(args))
		if err != nil {
			return err
		}
		msg, err := tree.Update(r.Path)
		if err != nil {
			return clierr.Preconditionf("%v", err)
		}
		_, _ = fmt.Fprintln(stderr, msg)
		return printWorktree(cmd.OutOrStdout(), o, r)
	}
	return c
}

func registerServiceCmd(parent *cobra.Command) {
	c := &cobra.Command{
		Use:   "service",
		Short: "worktree の dev サービス（up / down / logs）",
	}
	c.AddCommand(serviceUpCmd(), serviceDownCmd(), serviceLogsCmd())
	parent.AddCommand(c)
}

func serviceUpCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "up [<repo>/<worktree>]",
		Short: "dev サービスを割当ポートで起動する（未割当なら採番。他の worktree には影響しない）",
		Long:  "dev サービスを起動し、結果を wt worktree show と同じ形で出す。起動の進捗は stderr。" + formatHelp,
		Args:  maxArgs(1),
	}
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		r, err := ref.Resolve(refArg(args))
		if err != nil {
			return err
		}
		base, err := ports.EnsureBase(r.Container, r.Worktree)
		if err != nil {
			return err
		}
		slog.Info("devserver action", "worktree", r.Path, "action", "serve", "trigger", "cli-serve")
		if err := devserver.Serve(stderr, r.Path, base); err != nil {
			return err
		}
		waitListening(r)
		return printWorktree(cmd.OutOrStdout(), o, r)
	}
	return c
}

// waitListening gives freshly started services a moment to bind their ports,
// so `service up` does not report a slow starter as unhealthy. It returns
// as soon as nothing is unhealthy, or after the timeout.
func waitListening(r ref.Ref) {
	deadline := time.Now().Add(upWait)
	for time.Now().Before(deadline) {
		rep, err := status.Collect()
		if err != nil {
			return
		}
		d, err := view.LoadWorktree(r, &rep)
		if err != nil || d.State != status.StateUnhealthy {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// upWait bounds waitListening. A var for tests.
var upWait = 5 * time.Second

func serviceDownCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "down [<repo>/<worktree>]",
		Short: "dev サービスを止める（wt daemon・他の worktree には影響しない）",
		Long:  "dev サービスを止め、結果を wt worktree show と同じ形で出す。" + formatHelp,
		Args:  maxArgs(1),
	}
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		r, err := ref.Resolve(refArg(args))
		if err != nil {
			return err
		}
		slog.Info("devserver action", "worktree", r.Path, "action", "down", "trigger", "cli-down")
		if err := devserver.Down(stderr, r.Path); err != nil {
			return err
		}
		return printWorktree(cmd.OutOrStdout(), o, r)
	}
	return c
}

func serviceLogsCmd() *cobra.Command {
	var lines int
	c := &cobra.Command{
		Use:   "logs [<repo>/<worktree>] [<service>]",
		Short: "dev サービスのログを出す（既定は各サービスの末尾 100 行）",
		Long: "dev サービスのログ（stdout と stderr）を出す。<service> を省くと全サービスを \"==> <name> <==\" の見出し付きで出す。\n" +
			"-n 0 で全行。ログの場所は wt worktree show の service.<name>.log。",
		Args: maxArgs(2),
	}
	c.Flags().IntVarP(&lines, "lines", "n", 100, "末尾から何行出すか（0 で全行）")
	c.RunE = func(cmd *cobra.Command, args []string) error {
		r, err := ref.Resolve(refArg(args[:min(len(args), 1)]))
		if err != nil {
			return err
		}
		rep, err := status.Collect()
		if err != nil {
			return err
		}
		d, err := view.LoadWorktree(r, &rep)
		if err != nil {
			return err
		}
		var names []string
		for _, s := range d.Services {
			if s.Name != "" {
				names = append(names, s.Name)
			}
		}
		if len(args) == 2 {
			names = []string{args[1]}
		}
		if len(names) == 0 {
			return clierr.NotFoundf("dev サービスがありません: %s/%s", r.Repo, r.Worktree)
		}
		w := cmd.OutOrStdout()
		for i, name := range names {
			if len(names) > 1 {
				if i > 0 {
					_, _ = fmt.Fprintln(w)
				}
				_, _ = fmt.Fprintf(w, "==> %s <==\n", name)
			}
			if err := writeTail(w, devserver.LogPath(r.Path, name), lines); err != nil {
				return err
			}
		}
		return nil
	}
	return c
}

func writeTail(w io.Writer, path string, n int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return clierr.NotFoundf("ログがありません: %s", path)
		}
		return err
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return nil
	}
	ls := strings.Split(text, "\n")
	if n > 0 && len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	_, err = fmt.Fprintln(w, strings.Join(ls, "\n"))
	return err
}

// deprecate marks an old command as an alias of its noun-verb replacement:
// it keeps working and prints the new name to stderr. (#188)
func deprecate(c *cobra.Command, newName string) *cobra.Command {
	prev := c.PreRunE
	c.PreRunE = func(cmd *cobra.Command, args []string) error {
		_, _ = fmt.Fprintf(stderr, "wt: %s は %s に移行しました（旧名は移行が終わると消えます）\n", cmd.CommandPath(), newName)
		if prev != nil {
			return prev(cmd, args)
		}
		return nil
	}
	return c
}
