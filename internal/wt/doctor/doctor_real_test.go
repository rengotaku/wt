package doctor

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/wt/core"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestRun_RealFix runs --fix with the real repair functions: the main folder
// on another branch, a stale entry and a worktree without ports.
func TestRun_RealFix(t *testing.T) {
	container := setup(t)
	mainDir := filepath.Join(container, "main")
	if err := os.RemoveAll(filepath.Join(mainDir, ".git")); err != nil {
		t.Fatal(err)
	}
	gitIn(t, mainDir, "init", "-q", "--initial-branch=main")
	gitIn(t, mainDir, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "a")
	gitIn(t, mainDir, "checkout", "-q", "-b", "feat/x")
	if err := os.MkdirAll(filepath.Join(container, "app--old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "app--old", &core.Entry{Branch: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "app--gone", &core.Entry{Branch: "gone", PortBase: 9900}); err != nil {
		t.Fatal(err)
	}

	got, err := Run(io.Discard, []string{container}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range got {
		if f.Unresolved() {
			t.Errorf("unresolved after --fix: %+v", f)
		}
	}
	if b := gitIn(t, mainDir, "branch", "--show-current"); b != "main" {
		t.Errorf("main folder is on %q, want main", b)
	}
	if !strings.Contains(gitIn(t, mainDir, "worktree", "list"), "[feat/x]") {
		t.Error("feat/x was not moved to its own worktree")
	}
	entries, _ := core.LoadEntries(container)
	if _, ok := entries["app--gone"]; ok {
		t.Error("stale entry still registered")
	}
	if entries["app--old"].PortBase == 0 {
		t.Error("app--old still has no ports")
	}

	again, err := Run(io.Discard, []string{container}, false)
	if err != nil || len(again) != 0 {
		t.Errorf("second run must be clean: %+v, %v", again, err)
	}
}
