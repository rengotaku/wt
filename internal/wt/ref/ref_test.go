package ref

import (
	"os"
	"path/filepath"
	"testing"

	"wt/internal/cli/clierr"
)

func setup(t *testing.T) (home string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "app")
	for _, d := range []string{"main", "app--feat/sub"} {
		if err := os.MkdirAll(filepath.Join(container, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	meta := `{"main":{"type":"main"},"app--feat":{"branch":"feat"}}`
	if err := os.WriteFile(filepath.Join(container, ".worktrees.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestResolve_Arg(t *testing.T) {
	home := setup(t)
	r, err := Resolve("app/app--feat")
	if err != nil {
		t.Fatal(err)
	}
	if r.Repo != "app" || r.Worktree != "app--feat" || r.Path != filepath.Join(home, "Workspace", "app", "app--feat") {
		t.Errorf("got %+v", r)
	}

	for arg, kind := range map[string]clierr.Kind{
		"app":          clierr.KindUsage,
		"app/a/b":      clierr.KindUsage,
		"../x/main":    clierr.KindUsage,
		"nope/main":    clierr.KindNotFound,
		"app/app--zzz": clierr.KindNotFound,
	} {
		if _, err := Resolve(arg); err == nil || clierr.KindOf(err) != kind {
			t.Errorf("Resolve(%q): got %v, want kind %s", arg, err, kind)
		}
	}
}

func TestFromDir(t *testing.T) {
	home := setup(t)
	r, err := FromDir(filepath.Join(home, "Workspace", "app", "app--feat", "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Repo != "app" || r.Worktree != "app--feat" {
		t.Errorf("got %+v", r)
	}
	if _, err := FromDir(home); clierr.KindOf(err) != clierr.KindUsage {
		t.Errorf("outside a worktree: got %v, want usage error", err)
	}
	if _, err := FromDir(filepath.Join(home, "Workspace", "app")); clierr.KindOf(err) != clierr.KindUsage {
		t.Errorf("container root: got %v, want usage error", err)
	}
}
