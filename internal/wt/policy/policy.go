// Package policy resolves the per-repository AI work policy stored in a wt
// container. It never reads or writes the git repository itself: the policy
// lives outside the repo, next to .worktrees.json.
package policy

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"wt/internal/wt/core"
)

// DefaultRelPath is used by Init when _config.ai_policy is empty.
const DefaultRelPath = ".wt/ai-policy.md"

// ErrNotConfigured reports that the repository has no ai_policy configured.
var ErrNotConfigured = errors.New("AI 作業方針が設定されていません")

// Resolve returns the absolute policy path for container. configured is false
// when _config.ai_policy is empty. A configured path must stay inside the
// container; absolute paths and ".." escapes are rejected.
func Resolve(container string) (path string, configured bool, err error) {
	cfg, err := core.LoadConfig(container)
	if err != nil {
		return "", false, err
	}
	rel := strings.TrimSpace(cfg.AIPolicy)
	if rel == "" {
		return "", false, nil
	}
	p, err := safeJoin(container, rel)
	if err != nil {
		return "", false, err
	}
	return p, true, nil
}

// Path writes the absolute policy path. It returns ErrNotConfigured when the
// repository has no policy, so callers can exit non-zero.
func Path(w io.Writer, container string) error {
	p, configured, err := Resolve(container)
	if err != nil {
		return err
	}
	if !configured {
		return ErrNotConfigured
	}
	_, err = fmt.Fprintln(w, p)
	return err
}

// Show writes the policy body. An unconfigured repository produces no output
// and no error, so a global instruction can call it unconditionally. A
// configured path that does not exist is an error: it is a broken setting, not
// "no policy".
func Show(w io.Writer, container string) error {
	p, configured, err := Resolve(container)
	if err != nil {
		return err
	}
	if !configured {
		return nil
	}
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("ai_policy のファイルがありません: %s", p)
		}
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(w, f)
	return err
}

// Init creates the policy file from a template and records it in
// _config.ai_policy. It never overwrites an existing file, so running it twice
// is safe.
func Init(w io.Writer, container string) error {
	cfg, err := core.LoadConfig(container)
	if err != nil {
		return err
	}
	rel := strings.TrimSpace(cfg.AIPolicy)
	if rel == "" {
		rel = DefaultRelPath
	}
	p, err := safeJoin(container, rel)
	if err != nil {
		return err
	}

	if _, err := os.Stat(p); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(template), 0o644); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(w, "作成しました: %s\n", p)
	} else {
		_, _ = fmt.Fprintf(w, "既にあります: %s\n", p)
	}

	if strings.TrimSpace(cfg.AIPolicy) != rel {
		cfg.AIPolicy = rel
		if err := core.SaveConfig(container, &cfg); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(w, "ai_policy を設定しました: %s\n", rel)
	}
	return nil
}

// safeJoin joins a container-relative path and rejects anything that could
// leave the container. The policy is user-editable metadata, so a typo must be
// an error rather than a read of an unrelated file.
func safeJoin(container, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("ai_policy はコンテナからの相対パスで指定してください: %s", rel)
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("ai_policy がコンテナの外を指しています: %s", rel)
	}
	return filepath.Join(container, clean), nil
}

const template = `# AI 作業方針

<!-- wt policy init が作成したテンプレート。リポジトリごとに編集する。 -->

## 成果物の置き場所

- 調査・検討・作業記録の正本をどこに置くか（Issue / リポジトリ / 両方）。
- リポジトリに残す文書の範囲。

## 自走と停止

- どこまで自走するか。
- どの段階で人に確認するか。
- 確認するときの出し方（1問にまとめる、選択肢と影響を書く）。

## 計画の深さ

- 小さな変更の計画。
- 大きな変更の計画。
- 仕様が曖昧なときの扱い。

## 優先順位

- リポジトリの規約・組織の制約が優先。
- この文書と矛盾したら厳しい方を採る。
- 本番接続・削除・外部公開・課金は、この文書では許可しない。既存の gate に従う。

## やらないこと

- 依頼されていない変更。
- リポジトリに置かない文書。
`
