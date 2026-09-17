package core

import (
	"fmt"
	"regexp"
	"strings"
)

// DefaultBranchPattern allows any branch name. It is written by `wt repo add`
// so the key is discoverable in .worktrees.json, and is also the behavior when
// branch_pattern is absent entirely (existing containers must keep working).
const DefaultBranchPattern = ".*"

// compileBranchPattern anchors the configured pattern to the whole branch name.
// Validation regexes that match substrings surprise their authors: a pattern
// like `issue[0-9]+_dsc[0-9]+` would otherwise accept `feat/issue1_dsc2-wip`.
// HTML5's <input pattern> anchors implicitly for the same reason; JSON Schema's
// `pattern` does not, and is a recurring source of confusion.
//
// Patterns that already carry both anchors are used as-is, so authors who write
// them out of habit get what they expect rather than a doubly anchored regex.
func compileBranchPattern(pattern string) (*regexp.Regexp, error) {
	expr := pattern
	if !strings.HasPrefix(expr, "^") || !strings.HasSuffix(expr, "$") {
		expr = "^(?:" + expr + ")$"
	}
	return regexp.Compile(expr)
}

// ValidateBranchName checks branch against the container's branch_pattern.
// An unset or empty pattern permits everything. An invalid pattern is an error
// rather than a silent allow: a broken guard that reports success is worse than
// no guard, because nobody looks at it again.
func ValidateBranchName(container, branch string) error {
	cfg, err := LoadConfig(container)
	if err != nil {
		return fmt.Errorf(".worktrees.json の _config を読めません: %w", err)
	}
	pattern := strings.TrimSpace(cfg.BranchPattern)
	if pattern == "" || pattern == DefaultBranchPattern {
		return nil
	}
	re, err := compileBranchPattern(pattern)
	if err != nil {
		return fmt.Errorf("branch_pattern が正規表現として不正です: %q (%v)\n   .worktrees.json の _config.branch_pattern を修正してください", pattern, err)
	}
	if re.MatchString(branch) {
		return nil
	}
	return fmt.Errorf("ブランチ名 %q はこのリポジトリの branch_pattern に一致しません\n   pattern: %s\n   --branch <name> で慣行に沿った名前を指定してください", branch, pattern)
}
