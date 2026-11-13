package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupHelpRepo creates a wt container with the given symlink_candidates under a
// throwaway HOME so that `wt tree add --help` can resolve `--repo`.
func setupHelpRepo(t *testing.T, candidates string) {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	container := filepath.Join(tmpHome, "Workspace", "testrepo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"_config":{"symlink_candidates":` + candidates + `}}`
	if err := os.WriteFile(filepath.Join(container, ".worktrees.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runTreeAddHelp executes `wt tree add <args> --help` and returns its output.
func runTreeAddHelp(t *testing.T, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	c := treeAddCmd()
	c.SetOut(&buf)
	c.SetErr(&buf)
	c.SetArgs(append(args, "--help"))
	if err := c.Execute(); err != nil {
		t.Fatalf("Execute: %v\n%s", err, buf.String())
	}
	return buf.String()
}

func TestTreeAddHelp_ShowsCandidates(t *testing.T) {
	setupHelpRepo(t, `[".venv"]`)

	out := runTreeAddHelp(t, "--repo", "testrepo")
	if !strings.Contains(out, "このリポジトリの symlink 候補: .venv") {
		t.Errorf("候補が --help に出ていない:\n%s", out)
	}
	if !strings.Contains(out, "--no-symlink") {
		t.Errorf("--no-symlink が --help に出ていない:\n%s", out)
	}
}

func TestTreeAddHelp_NoCandidates(t *testing.T) {
	setupHelpRepo(t, `[]`)

	out := runTreeAddHelp(t, "--repo", "testrepo")
	if !strings.Contains(out, "候補なし（wt symlink add testrepo <path> で追加）") {
		t.Errorf("候補なしの案内が出ていない:\n%s", out)
	}
}

func TestTreeAddHelp_WithoutRepo(t *testing.T) {
	setupHelpRepo(t, `[".venv"]`)

	out := runTreeAddHelp(t)
	if !strings.Contains(out, "wt symlink ls") {
		t.Errorf("--repo 無しで確認方法が出ていない:\n%s", out)
	}
}

func TestTreeAddFlags_SymlinkDeprecatedHidden(t *testing.T) {
	c := treeAddCmd()

	legacy := c.Flags().Lookup("symlink")
	if legacy == nil {
		t.Fatal("--symlink が無い")
	}
	if !legacy.Hidden {
		t.Error("--symlink が help から隠れていない")
	}
	if legacy.Deprecated == "" {
		t.Error("--symlink に廃止メッセージが無い")
	}
	if c.Flags().Lookup("no-symlink") == nil {
		t.Error("--no-symlink が無い")
	}
}
