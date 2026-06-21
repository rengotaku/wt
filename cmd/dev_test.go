package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/wt/devserver"
)

func hasWarning(ws []string, substr string) bool {
	for _, w := range ws {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

func writeDevToml(t *testing.T, worktree string) {
	t.Helper()
	dir := filepath.Join(worktree, ".wt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[[services]]\nname = \"x\"\ncmd = \"y ${port}\"\n"
	if err := os.WriteFile(filepath.Join(dir, "dev.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDevShowWarnings(t *testing.T) {
	repoCfg := devserver.Config{Services: []devserver.Service{{Name: "api", Cmd: "c ${port}"}}}
	const fileShadow = "repo 既定が優先される"

	t.Run("worktree override shadows repo default", func(t *testing.T) {
		wt := t.TempDir()
		writeDevToml(t, wt) // even with a file present, override is what's in effect
		ws := devShowWarnings(wt, devserver.SourceWorktree, repoCfg)
		if !hasWarning(ws, "専用の上書き") {
			t.Errorf("expected override warning, got %v", ws)
		}
		// The file-shadow warning must NOT fire: the repo default is not effective.
		if hasWarning(ws, fileShadow) {
			t.Errorf("file-shadow warning should not fire under worktree override: %v", ws)
		}
	})

	t.Run("repo default effective with committed file => shadow warning", func(t *testing.T) {
		wt := t.TempDir()
		writeDevToml(t, wt)
		ws := devShowWarnings(wt, devserver.SourceRepo, repoCfg)
		if !hasWarning(ws, fileShadow) {
			t.Errorf("expected file-shadow warning, got %v", ws)
		}
	})

	t.Run("repo default effective without file => no shadow warning", func(t *testing.T) {
		wt := t.TempDir()
		ws := devShowWarnings(wt, devserver.SourceRepo, repoCfg)
		if hasWarning(ws, fileShadow) {
			t.Errorf("no file => no file-shadow warning: %v", ws)
		}
	})

	t.Run("empty repo default warns to add", func(t *testing.T) {
		wt := t.TempDir()
		ws := devShowWarnings(wt, devserver.SourceNone, devserver.Config{})
		if !hasWarning(ws, "未設定") {
			t.Errorf("expected 'not configured' warning, got %v", ws)
		}
	})
}
