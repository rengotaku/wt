package repo

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// makeLocalRemote creates a bare git repo and a clone of it, returning
// (remoteDir, cloneDir). The clone is always empty (no commits).
func makeLocalRemote(t *testing.T) (remoteDir, cloneDir string) {
	t.Helper()
	tmp := t.TempDir()
	remoteDir = filepath.Join(tmp, "remote.git")
	cloneDir = filepath.Join(tmp, "clone")

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("git", "init", "--bare", "--initial-branch=main", remoteDir)
	run("git", "clone", remoteDir, cloneDir)
	// configure local identity so commit works without global git config
	run("git", "-C", cloneDir, "config", "user.email", "test@example.com")
	run("git", "-C", cloneDir, "config", "user.name", "Test")
	return remoteDir, cloneDir
}

func TestInitEmptyRepo_EmptyRepo(t *testing.T) {
	_, cloneDir := makeLocalRemote(t)
	var buf bytes.Buffer
	if err := initEmptyRepo(&buf, cloneDir, "myrepo", "main"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// README.md must exist with correct content
	content, err := os.ReadFile(filepath.Join(cloneDir, "README.md"))
	if err != nil {
		t.Fatalf("README.md not found: %v", err)
	}
	if got := strings.TrimSpace(string(content)); got != "# myrepo" {
		t.Errorf("README.md content = %q, want %q", got, "# myrepo")
	}

	// remote must have a commit
	out, err := exec.Command("git", "-C", cloneDir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD failed: %v", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Error("expected a commit on HEAD after initEmptyRepo")
	}
}

func TestInitEmptyRepo_NonEmptyRepo(t *testing.T) {
	_, cloneDir := makeLocalRemote(t)

	// pre-seed a commit so the repo is non-empty
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("git", "-C", cloneDir, "commit", "--allow-empty", "-m", "existing commit")

	var buf bytes.Buffer
	if err := initEmptyRepo(&buf, cloneDir, "myrepo", "main"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// README.md must NOT have been created
	if _, err := os.Stat(filepath.Join(cloneDir, "README.md")); err == nil {
		t.Error("README.md should not be created for non-empty repo")
	}
}

func TestHttpsToSSH(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "HTTPS without .git suffix",
			in:   "https://github.com/owner/repo",
			want: "git@github.com:owner/repo.git",
		},
		{
			name: "HTTPS with .git suffix",
			in:   "https://github.com/owner/repo.git",
			want: "git@github.com:owner/repo.git",
		},
		{
			name: "SSH URL unchanged",
			in:   "git@github.com:owner/repo.git",
			want: "git@github.com:owner/repo.git",
		},
		{
			name: "non-GitHub HTTPS URL unchanged",
			in:   "https://gitlab.com/owner/repo.git",
			want: "https://gitlab.com/owner/repo.git",
		},
		{
			name: "empty string unchanged",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := httpsToSSH(tt.in)
			if got != tt.want {
				t.Errorf("httpsToSSH(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
