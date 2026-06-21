package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"wt/internal/wt/devserver"
)

// registerDevCmd wires `wt dev`, which edits the repository-wide default dev
// services (_config.dev_services in the container's .worktrees.json). The
// command group is cwd-based like `wt serve`: it resolves the container from the
// current worktree, so an AI agent working inside a worktree can configure the
// repo's dev services without extra arguments.
func registerDevCmd(parent *cobra.Command) {
	devCmd := &cobra.Command{
		Use:   "dev",
		Short: "dev サービス設定（repo 既定 _config.dev_services）を CLI から編集",
		Long: `現在の worktree が属するリポジトリの「repo 既定 dev サービス」(_config.dev_services) を
編集する。ここで定義したサービスは、専用の上書きを持たない全 worktree の wt serve /
Web の「起動」で使われる（メタデータ管理なのでリポジトリにはコミットされない）。

スキーマ:
  - 各 service は宣言順に割当ブロックの base+i ポートを受け取り、cmd 内の ${port} に置換される。
  - 全 service のポートは WT_PORT_<NAME> 環境変数で相互参照できる（例: WT_PORT_API）。
  - --domain を付けた service は wt proxy 経由の名前アクセス対象になる。

例:
  wt dev add api --cmd 'go run . web -p ${port}'
  wt dev add web --cmd 'npm run dev -- --port ${port} --strictPort' --domain
  wt dev show
  wt dev validate`,
	}
	devCmd.AddCommand(devAddCmd(), devRmCmd(), devShowCmd(), devValidateCmd(), devClearCmd())
	parent.AddCommand(devCmd)
}

func devAddCmd() *cobra.Command {
	var cmdStr string
	var domain bool
	c := &cobra.Command{
		Use:   "add <name>",
		Short: "dev サービスを追加/更新（name 単位の upsert）",
		Long: `repo 既定に dev サービスを 1 件追加する。同名があれば cmd/domain を更新する。
--cmd を省略すると既存サービスの cmd を引き継ぐ（--domain だけ変更する等）。新規サービスでは
--cmd が必須。同様に --domain を省略すると既存値を引き継ぐ（新規では false）。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, container, _, err := currentWorktree()
			if err != nil {
				return err
			}
			name := strings.TrimSpace(args[0])
			if name == "" {
				return errors.New("name が空です")
			}
			existing, found, err := devserver.FindRepoService(container, name)
			if err != nil {
				return err
			}
			svc := devserver.Service{Name: name}
			switch {
			case cmd.Flags().Changed("cmd"):
				if strings.TrimSpace(cmdStr) == "" {
					return errors.New("--cmd が空です")
				}
				svc.Cmd = cmdStr
			case found:
				svc.Cmd = existing.Cmd
			default:
				return errors.New("--cmd を指定してください（新規 service には必須）")
			}
			switch {
			case cmd.Flags().Changed("domain"):
				svc.Domain = domain
			case found:
				svc.Domain = existing.Domain
			}
			updated, err := devserver.UpsertRepoService(container, svc)
			if err != nil {
				return err
			}
			verb := "追加"
			if updated {
				verb = "更新"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "✅ service %q を%sしました（repo 既定）\n", name, verb)
			return err
		},
	}
	c.Flags().StringVar(&cmdStr, "cmd", "", "起動コマンド（${port} が割当ポートに置換される）")
	c.Flags().BoolVar(&domain, "domain", false, "wt proxy 経由の名前アクセス対象にする")
	return c
}

func devRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: "dev サービスを削除",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, container, _, err := currentWorktree()
			if err != nil {
				return err
			}
			name := strings.TrimSpace(args[0])
			found, err := devserver.RemoveRepoService(container, name)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("service %q がありません", name)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "🗑  service %q を削除しました（repo 既定）\n", name)
			return err
		},
	}
}

func devValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "repo 既定 dev 設定を検証（>=1 service・name 非空/一意・cmd 非空）",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, container, _, err := currentWorktree()
			if err != nil {
				return err
			}
			cfg, err := devserver.LoadRepoDefault(container)
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "✅ OK: %d service\n", len(cfg.Services))
			return err
		},
	}
}

func devClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "repo 既定 dev サービスを全削除",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, container, _, err := currentWorktree()
			if err != nil {
				return err
			}
			if err := devserver.ClearRepoDefault(container); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "🗑  repo 既定 dev サービスを全削除しました")
			return err
		},
	}
}

func devShowCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "show",
		Short: "repo 既定 dev 設定と、この worktree の実効ソース・警告を表示",
		RunE: func(cmd *cobra.Command, _ []string) error {
			worktree, container, name, err := currentWorktree()
			if err != nil {
				return err
			}
			repoDefault, err := devserver.LoadRepoDefault(container)
			if err != nil {
				return err
			}
			eff, source, err := devserver.EffectiveConfig(worktree)
			if err != nil {
				return err
			}
			warnings := devShowWarnings(worktree, source, repoDefault)
			if asJSON {
				return printDevShowJSON(cmd.OutOrStdout(), name, container, repoDefault, eff, source, warnings)
			}
			return printDevShowText(cmd.OutOrStdout(), name, container, repoDefault, source, warnings)
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "機械可読な JSON で出力（AI 向け）")
	return c
}

// devShowWarnings surfaces the layering footguns relevant to editing the repo
// default: a per-worktree override shadowing it, or a committed .wt/dev.toml
// being shadowed by it.
func devShowWarnings(worktree, source string, repoDefault devserver.Config) []string {
	var w []string
	if source == devserver.SourceWorktree {
		w = append(w, "この worktree には専用の上書き(dev_services)があり、repo 既定より優先されます。"+
			"repo 既定の変更はこの worktree には反映されません。")
	}
	// Only when the repo default is the *effective* layer is the committed file
	// shadowed by it. Under a worktree override the file is shadowed by the
	// override (covered above), not by the repo default.
	if source == devserver.SourceRepo && devserver.HasConfig(worktree) {
		w = append(w, "コミット済み .wt/dev.toml が存在しますが、repo 既定が優先されるため使われません。")
	}
	if len(repoDefault.Services) == 0 {
		w = append(w, "repo 既定 dev サービスは未設定です。`wt dev add` で追加してください。")
	}
	return w
}

func printDevShowText(w io.Writer, name, container string, repoDefault devserver.Config, source string, warnings []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "コンテナ: %s\n", container)
	fmt.Fprintf(&b, "repo 既定 dev services (_config.dev_services):\n")
	if len(repoDefault.Services) == 0 {
		b.WriteString("  (なし)\n")
	} else {
		for i, s := range repoDefault.Services {
			dom := ""
			if s.Domain {
				dom = "  [domain]"
			}
			fmt.Fprintf(&b, "  [%d] %s%s\n      cmd: %s\n", i, s.Name, dom, s.Cmd)
		}
	}
	fmt.Fprintf(&b, "この worktree (%s) の実効ソース: %s\n", name, devserver.SourceLabel(source))
	for _, msg := range warnings {
		fmt.Fprintf(&b, "⚠ %s\n", msg)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

type devShowJSON struct {
	Container       string              `json:"container"`
	CurrentWorktree string              `json:"current_worktree"`
	RepoDefault     []devserver.Service `json:"repo_default"`
	Effective       []devserver.Service `json:"effective"`
	EffectiveSource string              `json:"effective_source"`
	Warnings        []string            `json:"warnings"`
}

func printDevShowJSON(w io.Writer, name, container string, repoDefault, eff devserver.Config, source string, warnings []string) error {
	src := source
	if src == devserver.SourceNone {
		src = "none"
	}
	out := devShowJSON{
		Container:       container,
		CurrentWorktree: name,
		RepoDefault:     emptyServices(repoDefault.Services),
		Effective:       emptyServices(eff.Services),
		EffectiveSource: src,
		Warnings:        emptyStrings(warnings),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// emptyServices/emptyStrings normalize nil slices to empty so JSON renders [].
func emptyServices(s []devserver.Service) []devserver.Service {
	if s == nil {
		return []devserver.Service{}
	}
	return s
}

func emptyStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
