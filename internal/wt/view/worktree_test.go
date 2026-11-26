package view

import (
	"os"
	"path/filepath"
	"testing"

	"wt/internal/wt/core"
	"wt/internal/wt/ref"
	"wt/internal/wt/status"
)

// A worktree without a port block (main before its first `service up`) still
// lists its configured services, with domain.
func TestLoadWorktree_ServicesWithoutPorts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "app")
	path := filepath.Join(container, "main")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "main", &core.Entry{Type: "main", Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	cfg := core.EntryConfig{DevServices: []core.DevService{
		{Name: "web", Cmd: "npm run dev", Domain: true},
		{Name: "api", Cmd: "go run ."},
	}}
	if err := core.SaveConfig(container, &cfg); err != nil {
		t.Fatal(err)
	}
	rep := &status.Report{Worktrees: []status.Worktree{{Repo: "app", Worktree: "main", Path: path}}}

	d, err := LoadWorktree(ref.Ref{Repo: "app", Worktree: "main", Container: container, Path: path}, rep)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ServiceDetail{}
	for _, s := range d.Services {
		got[s.Name] = s
	}
	if s, ok := got["web"]; !ok || !s.Domain || s.State != "stopped" || s.Port != 0 {
		t.Errorf("web: %+v (all: %+v)", s, d.Services)
	}
	if s, ok := got["api"]; !ok || s.Domain {
		t.Errorf("api: %+v", s)
	}
}
