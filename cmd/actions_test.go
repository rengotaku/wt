package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"wt/internal/cli/clierr"
	"wt/internal/wt/core"
)

func TestConfirm(t *testing.T) {
	if err := confirm(true, "q"); err != nil {
		t.Errorf("--yes: %v", err)
	}
	// stdin is not a terminal in tests: without --yes it must refuse, not hang.
	orig := stdin
	stdin = strings.NewReader("y\n")
	defer func() { stdin = orig }()
	if err := confirm(false, "q"); clierr.ExitCode(err) != 4 {
		t.Errorf("no terminal, no --yes: got %v (exit %d), want exit 4", err, clierr.ExitCode(err))
	}
}

func TestWriteTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "svc.log")
	if err := os.WriteFile(path, []byte("a\nb\nc\nd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for n, want := range map[int]string{2: "c\nd\n", 0: "a\nb\nc\nd\n", 10: "a\nb\nc\nd\n"} {
		var b bytes.Buffer
		if err := writeTail(&b, path, n); err != nil {
			t.Fatal(err)
		}
		if b.String() != want {
			t.Errorf("n=%d: got %q, want %q", n, b.String(), want)
		}
	}
	if err := writeTail(&bytes.Buffer{}, filepath.Join(t.TempDir(), "none.log"), 10); clierr.ExitCode(err) != 3 {
		t.Errorf("missing log: got %v, want exit 3", err)
	}
}

func setupContainer(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "app")
	if err := os.MkdirAll(filepath.Join(container, "app--feat"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "app--feat", &core.Entry{Branch: "feat"}); err != nil {
		t.Fatal(err)
	}
	return container
}

func runCmd(c *cobra.Command, args ...string) (string, error) {
	var b bytes.Buffer
	c.SetOut(&b)
	c.SetErr(&bytes.Buffer{})
	c.SetArgs(args)
	err := c.Execute()
	return b.String(), err
}

func TestWorktreeSet(t *testing.T) {
	container := setupContainer(t)

	for _, args := range [][]string{
		{"app/app--feat"},                  // no key
		{"app/app--feat", "autostart"},     // no "="
		{"app/app--feat", "autostart=yes"}, // bad value
		{"app/app--feat", "color=blue"},    // unknown key
	} {
		if _, err := runCmd(worktreeSetCmd(), args...); clierr.ExitCode(err) != 2 {
			t.Errorf("%v: got %v, want exit 2", args, err)
		}
	}
	if _, err := runCmd(worktreeSetCmd(), "app/nope", "autostart=on"); clierr.ExitCode(err) != 3 {
		t.Errorf("unknown worktree: got %v, want exit 3", err)
	}

	out, err := runCmd(worktreeSetCmd(), "app/app--feat", "autostart=on")
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	entries, _ := core.LoadEntries(container)
	if !entries["app--feat"].AutoStart {
		t.Error("autostart not saved")
	}
	if !strings.Contains(out, "autostart\ton") {
		t.Errorf("output must be the worktree show form, got:\n%s", out)
	}
}

func TestWorktreeRm_EmptyTargetIsUsageError(t *testing.T) {
	setupContainer(t)
	if _, err := runCmd(worktreeRmCmd(), "", "--yes"); clierr.ExitCode(err) != 2 {
		t.Errorf("empty target: got %v, want exit 2", err)
	}
}

// A bad later pair must not leave an earlier pair applied.
func TestWorktreeSet_NoPartialWrite(t *testing.T) {
	container := setupContainer(t)
	if _, err := runCmd(worktreeSetCmd(), "app/app--feat", "autostart=on", "color=blue"); clierr.ExitCode(err) != 2 {
		t.Fatalf("got %v, want exit 2", err)
	}
	entries, _ := core.LoadEntries(container)
	if entries["app--feat"].AutoStart {
		t.Error("autostart was written although the command failed")
	}
}

func TestServiceLogs_OnlyKnownServices(t *testing.T) {
	container := setupContainer(t)
	e := core.Entry{Branch: "feat", PortBase: 9000, DevServices: []core.DevService{{Name: "api", Cmd: "true"}}}
	if err := core.PutEntry(container, "app--feat", &e); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../../etc/passwd", "nope"} {
		if _, err := runCmd(serviceLogsCmd(), "app/app--feat", name); clierr.ExitCode(err) != 3 {
			t.Errorf("service %q: got %v, want exit 3", name, err)
		}
	}
}

// Bare `wt daemon` used to run the daemon; an old service definition must fail.
func TestDaemonBareFails(t *testing.T) {
	root := &cobra.Command{Use: "wt", SilenceErrors: true, SilenceUsage: true}
	registerDaemonCmd(root)
	if _, err := runCmd(root, "daemon"); clierr.ExitCode(err) != 2 {
		t.Errorf("bare daemon: got %v, want exit 2", err)
	}
}
