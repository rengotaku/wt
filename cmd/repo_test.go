package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/cli/clierr"
	"wt/internal/wt/core"
)

func TestRepoConfig_UsageErrors(t *testing.T) {
	setupContainer(t)
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"set unknown key", func() error { _, err := runCmd(repoConfigSetCmd(), "app", "color", "x"); return err }},
		{"set bad regex", func() error { _, err := runCmd(repoConfigSetCmd(), "app", "branch_pattern", "("); return err }},
		{"symlink with ..", func() error { _, err := runCmd(repoConfigAddCmd(), "app", "symlink", "../etc"); return err }},
		{"symlink absolute", func() error { _, err := runCmd(repoConfigAddCmd(), "app", "symlink", "/etc"); return err }},
		{"new service without --cmd", func() error { _, err := runCmd(repoConfigAddCmd(), "app", "service", "api"); return err }},
		{"--cmd on symlink", func() error {
			_, err := runCmd(repoConfigAddCmd(), "app", "symlink", "node_modules", "--cmd", "x")
			return err
		}},
		{"rm without value or --all", func() error { _, err := runCmd(repoConfigRmCmd(), "app", "symlink"); return err }},
		{"rm with value and --all", func() error {
			_, err := runCmd(repoConfigRmCmd(), "app", "symlink", "x", "--all")
			return err
		}},
	} {
		if err := tc.run(); clierr.ExitCode(err) != 2 {
			t.Errorf("%s: got %v, want exit 2", tc.name, err)
		}
	}
	if _, err := runCmd(repoConfigRmCmd(), "app", "symlink", "nope"); clierr.ExitCode(err) != 3 {
		t.Errorf("rm missing symlink: got %v, want exit 3", err)
	}
	if _, err := runCmd(repoConfigSetCmd(), "nope", "branch_pattern", ".*"); clierr.ExitCode(err) != 3 {
		t.Errorf("unknown repo: got %v, want exit 3", err)
	}
}

func TestRepoConfig_Service(t *testing.T) {
	container := setupContainer(t)
	if _, err := runCmd(repoConfigAddCmd(), "app", "service", "api", "--cmd", "go run . -p ${port}", "--domain"); err != nil {
		t.Fatal(err)
	}
	// An update without --cmd keeps the command and the other flags.
	got, err := runCmd(repoConfigAddCmd(), "app", "service", "api", "--headless")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := core.LoadConfig(container)
	if len(cfg.DevServices) != 1 {
		t.Fatalf("services: %+v", cfg.DevServices)
	}
	s := cfg.DevServices[0]
	if s.Cmd != "go run . -p ${port}" || !s.Domain || !s.Headless {
		t.Errorf("update lost fields: %+v", s)
	}
	for _, want := range []string{"service.api.cmd\tgo run . -p ${port}", "service.api.domain\ton", "service.api.headless\ton"} {
		if !strings.Contains(got, want) {
			t.Errorf("output must be the repo show form with %q, got:\n%s", want, got)
		}
	}
	if _, err := runCmd(repoConfigRmCmd(), "app", "service", "--all"); err != nil {
		t.Fatal(err)
	}
	cfg, _ = core.LoadConfig(container)
	if len(cfg.DevServices) != 0 {
		t.Errorf("--all left %+v", cfg.DevServices)
	}
}

func TestRepoConfig_Symlink(t *testing.T) {
	container := setupContainer(t)
	for _, p := range []string{"vendor", "node_modules", "vendor"} {
		if _, err := runCmd(repoConfigAddCmd(), "app", "symlink", p); err != nil {
			t.Fatal(err)
		}
	}
	cfg, _ := core.LoadConfig(container)
	if strings.Join(cfg.SymlinkCandidates, ",") != "node_modules,vendor" {
		t.Errorf("candidates: %v", cfg.SymlinkCandidates)
	}
	if _, err := runCmd(repoConfigRmCmd(), "app", "symlink", "vendor"); err != nil {
		t.Fatal(err)
	}
	cfg, _ = core.LoadConfig(container)
	if strings.Join(cfg.SymlinkCandidates, ",") != "node_modules" {
		t.Errorf("after rm: %v", cfg.SymlinkCandidates)
	}
}

func TestRepoRm_NeedsConfirmation(t *testing.T) {
	container := setupContainer(t)
	if err := os.MkdirAll(filepath.Join(container, "main"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", filepath.Join(container, "main"), "init", "-q", "--initial-branch=main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	if _, err := runCmd(repoRmCmd(), "app", "--dry-run"); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if _, err := runCmd(repoRmCmd(), "app"); clierr.ExitCode(err) != 4 {
		t.Errorf("no terminal, no --yes: got %v, want exit 4", err)
	}
	if _, err := os.Stat(container); err != nil {
		t.Fatalf("container removed without confirmation: %v", err)
	}
	out, err := runCmd(repoRmCmd(), "app", "--yes")
	if err != nil {
		t.Fatalf("--yes: %v", err)
	}
	if _, err := os.Stat(container); !os.IsNotExist(err) {
		t.Errorf("container still exists: %v", err)
	}
	if !strings.Contains(out, "result\tremoved") {
		t.Errorf("output: %q", out)
	}
}
