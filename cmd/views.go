package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"wt/internal/cli/clierr"
	"wt/internal/cli/out"
	"wt/internal/wt/core"
	"wt/internal/wt/daemon"
	"wt/internal/wt/ref"
	"wt/internal/wt/settings"
	"wt/internal/wt/status"
	"wt/internal/wt/view"
)

// Read commands of the noun-verb layout (#188). Output: tab-separated by
// default, --json, --rich for people. See internal/cli/out.

const formatHelp = `
出力: 既定はタブ区切り（1 行 1 件、空は "-"）。--json で JSON、--rich で人向けの表。`

var repoLsHeader = []string{"REPO", "PATH", "WORKTREES", "RUNNING", "UNHEALTHY", "IDLE", "STALE", "AUTOSTART"}

func repoLsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "ls",
		Short: "リポジトリごとに 1 行の要約（worktree 数・状態ごとの件数・AutoStart 数）",
		Long: "リポジトリごとに 1 行の要約を出す。列: " + strings.Join(repoLsHeader, " ") + "\n" +
			"1 リポジトリの worktree は wt worktree ls <repo>、設定は wt repo show <repo>。" + formatHelp,
		Args: noArgs,
	}
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		containers := core.ListContainers()
		if len(containers) == 0 {
			return clierr.NotFoundf("登録リポジトリがありません（wt repo add で追加してください）")
		}
		rep, err := status.Collect()
		if err != nil {
			return err
		}
		sums := view.RepoSummaries(containers, rep.Worktrees)
		t := &out.Table{Header: repoLsHeader}
		for _, s := range sums {
			t.Rows = append(t.Rows, []string{s.Repo, s.Path, fmt.Sprint(s.Worktrees), fmt.Sprint(s.Running),
				fmt.Sprint(s.Unhealthy), fmt.Sprint(s.Idle), fmt.Sprint(s.Stale), fmt.Sprint(s.AutoStart)})
		}
		return out.Render(cmd.OutOrStdout(), o, t, sums, "")
	}
	return c
}

func repoShowCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "show <repo>",
		Short: "1 リポジトリの設定（パス・既定ブランチ・branch_pattern・symlink 候補・dev サービス・方針ファイル）",
		Long:  "1 リポジトリの設定を KEY / VALUE の 2 列で出す。リストの値は \",\" 区切り。" + formatHelp,
		Args:  exactArgs(1),
	}
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		d, err := view.LoadRepo(args[0])
		if err != nil {
			return err
		}
		t := kvTable([][2]string{
			{"repo", d.Repo}, {"path", d.Path}, {"main", d.Main}, {"default_branch", d.DefaultBranch},
			{"branch_pattern", d.BranchPattern}, {"symlink_candidates", strings.Join(d.SymlinkCandidates, ",")},
			{"dev_services", strings.Join(d.DevServices, ",")}, {"ai_policy", d.AIPolicy},
			{"worktrees", fmt.Sprint(d.Worktrees)},
		})
		return out.Render(cmd.OutOrStdout(), o, t, d, "")
	}
	return c
}

func registerWorktreeCmd(parent *cobra.Command) {
	c := &cobra.Command{
		Use:   "worktree",
		Short: "worktree を見る（ls / show）",
	}
	c.AddCommand(worktreeLsCmd(), worktreeShowCmd())
	parent.AddCommand(c)
}

func worktreeLsCmd() *cobra.Command {
	var all bool
	c := &cobra.Command{
		Use:   "ls [<repo>]",
		Short: "worktree の一覧（ブランチ・状態・ポート・AutoStart・URL）",
		Long: "worktree を 1 行ずつ出す。列: " + strings.Join(status.TSVHeader, " ") + "\n" +
			"STATE: running / unhealthy / idle（ポート割当済み・未稼働）/ stale（ディレクトリが無い）/ -（ポート未割当）\n" +
			"<repo> を省くと今いるリポジトリ。--all で全リポジトリ。" + formatHelp,
		Args: maxArgs(1),
	}
	c.Flags().BoolVar(&all, "all", false, "全リポジトリの worktree を出す")
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		container, err := lsTarget(args, all)
		if err != nil {
			return err
		}
		rep, err := status.Collect()
		if err != nil {
			return err
		}
		wts := filterByContainer(rep.Worktrees, container)
		t := &out.Table{Header: status.TSVHeader}
		for i := range wts {
			w := &wts[i]
			t.Rows = append(t.Rows, []string{w.Repo, w.Worktree, w.Branch, w.State, w.Ports, out.OnOff(w.AutoStart), w.URL})
		}
		return out.Render(cmd.OutOrStdout(), o, t, wts, "")
	}
	return c
}

// lsTarget returns the container to list ("" = all).
func lsTarget(args []string, all bool) (string, error) {
	switch {
	case all && len(args) > 0:
		return "", clierr.Usagef("<repo> と --all は同時に指定できません")
	case all:
		return "", nil
	case len(args) == 1:
		if err := core.ValidateRepoName(args[0]); err != nil {
			return "", clierr.Usagef("%v", err)
		}
		c, err := core.FindContainer(args[0])
		if err != nil {
			return "", clierr.NotFoundf("リポジトリが見つかりません: %s", args[0])
		}
		return c, nil
	default:
		r, err := ref.Resolve("")
		if err != nil {
			return "", clierr.Usagef("リポジトリの中で実行するか、<repo> か --all を指定してください")
		}
		return r.Container, nil
	}
}

func filterByContainer(wts []status.Worktree, container string) []status.Worktree {
	if container == "" {
		return wts
	}
	res := []status.Worktree{}
	prefix := container + "/"
	for i := range wts {
		if strings.HasPrefix(wts[i].Path, prefix) {
			res = append(res, wts[i])
		}
	}
	return res
}

var serviceHeader = []string{"SERVICE", "PORT", "STATE", "PID", "PROC", "LOG"}

func worktreeShowCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "show [<repo>/<worktree>]",
		Short: "1 worktree の詳細（状態・ポート・URL・Issue・サービスごとの PID とログの場所）",
		Long: "1 worktree の詳細を KEY / VALUE の 2 列で出す。サービスは service.<name>.<項目> のキーで並ぶ。\n" +
			"対象を省くと今いる worktree。" + formatHelp,
		Args: maxArgs(1),
	}
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		arg := ""
		if len(args) == 1 {
			arg = args[0]
		}
		r, err := ref.Resolve(arg)
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
		return writeWorktreeShow(cmd.OutOrStdout(), o, &d)
	}
	return c
}

func writeWorktreeShow(w io.Writer, o *out.Opts, d *view.WorktreeDetail) error {
	kv := [][2]string{
		{"repo", d.Repo}, {"worktree", d.Worktree.Worktree}, {"branch", d.Branch}, {"path", d.Path},
		{"state", d.State}, {"ports", d.Ports}, {"autostart", out.OnOff(d.AutoStart)}, {"url", d.URL},
		{"issue", d.Issue}, {"created", d.Created},
	}
	switch {
	case o.JSON:
		return out.WriteJSON(w, d)
	case o.Rich:
		color := out.IsTerminal(w)
		if err := out.WriteRich(w, kvTable(kv), d.Repo+"/"+d.Worktree.Worktree, color); err != nil {
			return err
		}
		st := &out.Table{Header: serviceHeader}
		for _, s := range d.Services {
			st.Rows = append(st.Rows, []string{s.Name, fmt.Sprint(s.Port), s.State, out.Itoa(s.PID), s.Proc, s.Log})
		}
		return out.WriteRich(w, st, "\nservices", color)
	default:
		for _, s := range d.Services {
			p := "service." + out.Cell(s.Name) + "."
			kv = append(kv, [2]string{p + "port", fmt.Sprint(s.Port)}, [2]string{p + "state", s.State},
				[2]string{p + "pid", out.Itoa(s.PID)}, [2]string{p + "proc", s.Proc}, [2]string{p + "log", s.Log})
		}
		return out.WriteTSV(w, kvTable(kv))
	}
}

func daemonStatusCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "status",
		Short: "wt daemon が動いているか・PID・proxy の待ち受け先",
		Long:  "wt daemon の状態を KEY / VALUE の 2 列で出す。daemon は running / stopped。" + formatHelp,
		Args:  noArgs,
	}
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		running, pid, err := daemon.Probe(daemon.LockPath())
		if err != nil {
			return err
		}
		rep := status.Build(&status.Input{Daemon: status.Daemon{Running: running, PID: pid}, Proxy: settings.Load().Proxy})
		state := "stopped"
		if running {
			state = "running"
		}
		t := kvTable([][2]string{
			{"daemon", state}, {"pid", out.Itoa(pid)},
			{"proxy", map[bool]string{true: "enabled", false: "disabled"}[rep.Proxy.Enabled]}, {"proxy_addr", rep.Proxy.Addr},
		})
		data := map[string]any{"daemon": rep.Daemon, "proxy": rep.Proxy}
		return out.Render(cmd.OutOrStdout(), o, t, data, "")
	}
	return c
}

func kvTable(kv [][2]string) *out.Table {
	t := &out.Table{Header: []string{"KEY", "VALUE"}}
	for _, p := range kv {
		t.Rows = append(t.Rows, []string{p[0], p[1]})
	}
	return t
}

// Argument validators that classify their errors as usage errors (exit 2).
var noArgs = exactArgs(0)

func exactArgs(n int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != n {
			return clierr.Usagef("引数は %d 個です（%d 個渡されました）", n, len(args))
		}
		return nil
	}
}

func maxArgs(n int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > n {
			return clierr.Usagef("引数は %d 個までです（%d 個渡されました）", n, len(args))
		}
		return nil
	}
}
