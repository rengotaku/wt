package cmd

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
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
		if args[0] == "" {
			// An empty target would resolve to the current worktree; never
			// let an unset variable in a script delete where it runs.
			return clierr.Usagef("削除する worktree を <repo>/<worktree> で指定してください")
		}
		r, err := ref.Resolve(args[0])
		if err != nil {
			return err
		}
		key := r.Repo + "/" + r.Worktree
		opts.NoSize = true
		if !opts.DryRun {
			preview := opts
			preview.DryRun = true
			if err := tree.RemoveAt(stderr, preview, r.Repo, r.Container, r.Worktree); err != nil {
				return clierr.Preconditionf("%v", err)
			}
			if err := confirm(yes, key+" を消しますか"); err != nil {
				return err
			}
		}
		if err := tree.RemoveAt(stderr, opts, r.Repo, r.Container, r.Worktree); err != nil {
			return clierr.Preconditionf("%v", err)
		}
		if !opts.DryRun {
			if _, err := os.Stat(r.Path); err == nil {
				return clierr.External("削除後も worktree のディレクトリが残っています: "+r.Path, nil)
			}
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
			"--done は worktree の無いマージ済みブランチも消す。端末なら確認を出し、--yes で省略する。端末でなく --yes も無ければ終了コード 4。\n" +
			"進捗は stderr、結果は REPO / WORKTREE / BRANCH / RESULT / DETAIL の表で stdout に出す。WORKTREE が - の行は worktree の無いブランチ。\n" +
			"RESULT は deleted / skipped（消さずに残した。理由は DETAIL）/ candidate（--dry-run の候補）/ failed。failed があれば終了コード 4。" +
			formatHelp,
		Args: noArgs,
	}
	c.Flags().BoolVar(&opts.Done, "done", false, "PR merged / closed か issue closed の worktree")
	c.Flags().StringVar(&opts.OlderThan, "older-than", "", "最終コミットが N(d|h) 以上前")
	c.Flags().BoolVar(&opts.KeepBranch, "keep-branch", false, "ブランチを残す")
	c.Flags().BoolVar(&opts.DryRun, "dry-run", false, "候補だけ出す")
	c.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "確認を省く")
	c.Flags().BoolVar(&opts.Force, "force", false, "未コミット・未追跡のファイル・リモートに無いコミットがあっても消す")
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		if !opts.Done && opts.OlderThan == "" {
			return clierr.Usagef("--done か --older-than を指定してください")
		}
		if !opts.DryRun && !opts.Yes {
			// Confirm inside the run so exactly the listed candidates are
			// deleted (no second scan between the prompt and the delete).
			opts.Confirm = func() error { return confirm(false, "上の候補を消しますか") }
		}
		results, err := gc.RunResults(stderr, opts)
		if err != nil {
			return err
		}
		if results == nil {
			results = []gc.Result{}
		}
		t := &out.Table{Header: []string{"REPO", "WORKTREE", "BRANCH", "RESULT", "DETAIL"}}
		failed := 0
		for _, r := range results {
			t.Rows = append(t.Rows, []string{r.Repo, r.Worktree, r.Branch, r.Result, r.Detail})
			if r.Result == gc.ResultFailed {
				failed++
			}
		}
		if err := out.Render(cmd.OutOrStdout(), o, t, results, "worktree gc"); err != nil {
			return err
		}
		if failed > 0 {
			return clierr.Preconditionf("%d 件を消せませんでした", failed)
		}
		return nil
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
		// Validate every pair before writing anything: a bad later pair must
		// not leave an earlier one applied.
		autostart := ""
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
				autostart = v
			default:
				return clierr.Usagef("知らないキーです: %q（使えるキー: autostart）", k)
			}
		}
		if autostart != "" {
			if err := tree.SetAutoStart(r.Repo, r.Worktree, autostart == "on"); err != nil {
				return err
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
			// Only a service of this worktree: the name becomes part of a path.
			if !slices.Contains(names, args[1]) {
				return clierr.NotFoundf("サービスがありません: %q（あるもの: %s）", args[1], strings.Join(names, ", "))
			}
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
