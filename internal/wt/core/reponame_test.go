package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRepoName(t *testing.T) {
	for _, ok := range []string{"wt", "dotfiles-ubuntu", "a.b", "_x", "repo_1", ".github", "日本語", "with space", "-flag"} {
		if err := ValidateRepoName(ok); err != nil {
			t.Errorf("%q: want ok, got %v", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "..", "../x", "a/b", "/abs", `a\b`, "a\x00b"} {
		if err := ValidateRepoName(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestFindContainer_RejectsTraversal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "Workspace", "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"..", ".", "repo/..", "../Workspace"} {
		if got, err := FindContainer(bad); err == nil {
			t.Errorf("FindContainer(%q) = %q, want error", bad, got)
		}
	}
	got, err := FindContainer("repo")
	if err != nil || got != filepath.Join(home, "Workspace", "repo") {
		t.Errorf("FindContainer(repo) = %q, %v", got, err)
	}
}

func TestIsWorkspaceChild(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cases := map[string]bool{
		filepath.Join(home, "Workspace", "repo"):   true,
		filepath.Join(home, "MyWorkspace", "repo"): true,
		home:                             false,
		filepath.Join(home, "Workspace"): false,
		filepath.Join(home, "Workspace", "repo", "sub"): false,
		filepath.Join(home, "Workspace", ".."):          false,
	}
	for dir, want := range cases {
		if got := IsWorkspaceChild(dir); got != want {
			t.Errorf("IsWorkspaceChild(%q) = %v, want %v", dir, got, want)
		}
	}
}
