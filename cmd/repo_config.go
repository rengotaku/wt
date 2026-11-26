package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"wt/internal/cli/clierr"
	"wt/internal/cli/out"
	"wt/internal/wt/devserver"
	"wt/internal/wt/repo"
)

// Keys of `wt repo config`.
const (
	cfgBranchPattern = "branch_pattern"
	cfgSymlink       = "symlink"
	cfgService       = "service"
)

const repoConfigLong = `リポジトリの設定（.worktrees.json の _config）を変える。結果は repo show の形で stdout に出す。

  wt repo config set <repo> branch_pattern <正規表現>   新しいブランチ名の制約（"" で外す）
  wt repo config add <repo> symlink <パス>              worktree を作るときに main から symlink するパス
  wt repo config add <repo> service <名前> --cmd <コマンド> [--domain] [--headless]
                                                        dev サービスを足す。同名があれば更新する
  wt repo config rm  <repo> symlink|service <値>        1 件消す（--all で全部）

service の --cmd では ${port} が割り当てたポートに置き換わる。--domain は proxy の名前アクセスの対象、
--headless はポートを張らない worker 用。更新のときに省いたフラグは今の値を引き継ぐ。`

func repoConfigCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "リポジトリの設定（branch_pattern・symlink 候補・dev サービス）を変える",
		Long:  repoConfigLong,
	}
	c.AddCommand(repoConfigSetCmd(), repoConfigAddCmd(), repoConfigRmCmd())
	return c
}

func repoConfigSetCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "set <repo> branch_pattern <value>",
		Short: "値を 1 つ持つ設定を変える（branch_pattern）",
		Long:  repoConfigLong + formatHelp,
		Args:  exactArgs(3),
	}
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		if args[1] != cfgBranchPattern {
			return clierr.Usagef("set できるのは branch_pattern だけです: %s", args[1])
		}
		container, err := findRepo(args[0])
		if err != nil {
			return err
		}
		if err := repo.SetBranchPattern(container, args[2]); err != nil {
			return clierr.Usagef("%v", err)
		}
		return printRepo(cmd, o, args[0])
	}
	return c
}

func repoConfigAddCmd() *cobra.Command {
	var svcCmd string
	var domain, headless bool
	c := &cobra.Command{
		Use:   "add <repo> symlink|service <value>",
		Short: "symlink 候補か dev サービスを足す（service は同名なら更新）",
		Long:  repoConfigLong + formatHelp,
		Args:  exactArgs(3),
	}
	c.Flags().StringVar(&svcCmd, "cmd", "", "service の起動コマンド（${port} が割当ポートになる）")
	c.Flags().BoolVar(&domain, "domain", false, "service を proxy の名前アクセスの対象にする")
	c.Flags().BoolVar(&headless, "headless", false, "service をポートを張らない worker として扱う")
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		key, value := args[1], args[2]
		serviceFlags := cmd.Flags().Changed("cmd") || cmd.Flags().Changed("domain") || cmd.Flags().Changed("headless")
		if key != cfgService && serviceFlags {
			return clierr.Usagef("--cmd / --domain / --headless は service にだけ付けられます")
		}
		container, err := findRepo(args[0])
		if err != nil {
			return err
		}
		switch key {
		case cfgSymlink:
			if err := repo.AddSymlink(container, value); err != nil {
				return clierr.Usagef("%v", err)
			}
		case cfgService:
			svc, err := mergeService(cmd, container, strings.TrimSpace(value), svcCmd, domain, headless)
			if err != nil {
				return err
			}
			if _, err := devserver.UpsertRepoService(container, svc); err != nil {
				return clierr.Usagef("%v", err)
			}
		default:
			return clierr.Usagef("add できるのは symlink か service です: %s", key)
		}
		return printRepo(cmd, o, args[0])
	}
	return c
}

// mergeService builds the service to upsert: flags that were not given keep
// the current value of an existing service.
func mergeService(cmd *cobra.Command, container, name, svcCmd string, domain, headless bool) (devserver.Service, error) {
	if name == "" {
		return devserver.Service{}, clierr.Usagef("service の名前が空です")
	}
	existing, found, err := devserver.FindRepoService(container, name)
	if err != nil {
		return devserver.Service{}, err
	}
	svc := devserver.Service{Name: name}
	if found {
		svc = existing
	}
	changed := cmd.Flags().Changed
	if changed("cmd") {
		if strings.TrimSpace(svcCmd) == "" {
			return devserver.Service{}, clierr.Usagef("--cmd が空です")
		}
		svc.Cmd = svcCmd
	} else if !found {
		return devserver.Service{}, clierr.Usagef("新しい service には --cmd が要ります")
	}
	if changed("domain") {
		svc.Domain = domain
	}
	if changed("headless") {
		svc.Headless = headless
	}
	return svc, nil
}

func repoConfigRmCmd() *cobra.Command {
	var all bool
	c := &cobra.Command{
		Use:   "rm <repo> symlink|service [<value>]",
		Short: "symlink 候補か dev サービスを 1 件消す（--all で全部）",
		Long:  repoConfigLong + formatHelp,
		Args:  rangeArgs(2, 3),
	}
	c.Flags().BoolVar(&all, "all", false, "その種類を全部消す")
	o := out.AddFlags(c)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Validate(); err != nil {
			return err
		}
		key := args[1]
		if key != cfgSymlink && key != cfgService {
			return clierr.Usagef("rm できるのは symlink か service です: %s", key)
		}
		if all == (len(args) == 3) {
			return clierr.Usagef("消す値を 1 つ指定するか、--all を付けてください（両方は付けられません）")
		}
		container, err := findRepo(args[0])
		if err != nil {
			return err
		}
		if err := removeConfig(container, key, args, all); err != nil {
			return err
		}
		return printRepo(cmd, o, args[0])
	}
	return c
}

func removeConfig(container, key string, args []string, all bool) error {
	var found bool
	var err error
	switch {
	case key == cfgSymlink && all:
		return repo.ClearSymlinks(container)
	case key == cfgService && all:
		return devserver.ClearRepoDefault(container)
	case key == cfgSymlink:
		found, err = repo.RmSymlink(container, args[2])
	default:
		found, err = devserver.RemoveRepoService(container, strings.TrimSpace(args[2]))
	}
	if err != nil {
		return err
	}
	if !found {
		return clierr.NotFoundf("%s に %s はありません", key, args[2])
	}
	return nil
}
