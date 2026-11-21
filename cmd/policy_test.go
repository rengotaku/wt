package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"wt/internal/wt/core"
)

// setupPolicyRepo creates a throwaway HOME with one wt container.
func setupPolicyRepo(t *testing.T) (home, container string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	container = filepath.Join(home, "Workspace", "testrepo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(core.MetaFile(container), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return home, container
}

func setPolicyConfig(t *testing.T, container, rel string) {
	t.Helper()
	cfg, err := core.LoadConfig(container)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AIPolicy = rel
	if err := core.SaveConfig(container, &cfg); err != nil {
		t.Fatal(err)
	}
}

func writePolicyFile(t *testing.T, container, rel, body string) {
	t.Helper()
	p := filepath.Join(container, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runPolicy(t *testing.T, c *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	c.SetOut(&buf)
	c.SetErr(&buf)
	c.SetArgs(args)
	err := c.Execute()
	return buf.String(), err
}

func TestPolicyShowByRepo(t *testing.T) {
	_, container := setupPolicyRepo(t)
	setPolicyConfig(t, container, ".wt/ai-policy.md")
	writePolicyFile(t, container, ".wt/ai-policy.md", "# policy\n")

	out, err := runPolicy(t, policyShowCmd(), "--repo", "testrepo")
	if err != nil {
		t.Fatalf("show: %v\n%s", err, out)
	}
	if out != "# policy\n" {
		t.Fatalf("got %q", out)
	}
}

func TestPolicyShowUnsetIsSilent(t *testing.T) {
	setupPolicyRepo(t)
	out, err := runPolicy(t, policyShowCmd(), "--repo", "testrepo")
	if err != nil {
		t.Fatalf("show: %v\n%s", err, out)
	}
	if out != "" {
		t.Fatalf("unset must be silent, got %q", out)
	}
}

func TestPolicyPathByRepo(t *testing.T) {
	_, container := setupPolicyRepo(t)
	setPolicyConfig(t, container, ".wt/ai-policy.md")

	out, err := runPolicy(t, policyPathCmd(), "--repo", "testrepo")
	if err != nil {
		t.Fatalf("path: %v\n%s", err, out)
	}
	want := filepath.Join(container, ".wt", "ai-policy.md") + "\n"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestPolicyPathUnsetFails(t *testing.T) {
	setupPolicyRepo(t)
	out, err := runPolicy(t, policyPathCmd(), "--repo", "testrepo")
	if err == nil {
		t.Fatalf("expected an error, got %q", out)
	}
}

func TestPolicyInitIsIdempotent(t *testing.T) {
	_, container := setupPolicyRepo(t)

	out, err := runPolicy(t, policyInitCmd(), "--repo", "testrepo")
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	p := filepath.Join(container, ".wt", "ai-policy.md")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("template not created: %v", err)
	}
	cfg, err := core.LoadConfig(container)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AIPolicy != ".wt/ai-policy.md" {
		t.Fatalf("ai_policy = %q", cfg.AIPolicy)
	}

	out, err = runPolicy(t, policyInitCmd(), "--repo", "testrepo")
	if err != nil {
		t.Fatalf("second init: %v\n%s", err, out)
	}
	if !strings.Contains(out, "既にあります") {
		t.Fatalf("expected exists message, got %q", out)
	}
}

func TestPolicyShowFromWorktreeCwd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	_, container := setupPolicyRepo(t)
	setPolicyConfig(t, container, ".wt/ai-policy.md")
	writePolicyFile(t, container, ".wt/ai-policy.md", "# from cwd\n")

	mainDir := filepath.Join(container, "main")
	if err := os.MkdirAll(mainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = mainDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	t.Chdir(mainDir)
	out, err := runPolicy(t, policyShowCmd())
	if err != nil {
		t.Fatalf("show from cwd: %v\n%s", err, out)
	}
	if out != "# from cwd\n" {
		t.Fatalf("got %q", out)
	}
}
