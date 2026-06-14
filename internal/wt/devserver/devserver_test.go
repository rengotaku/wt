package devserver

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSharedEnv(t *testing.T) {
	svcs := []Service{{Name: "api"}, {Name: "web-ui"}}
	got := sharedEnv(svcs, 9000)
	want := []string{"WT_PORT_API=9000", "WT_PORT_WEB_UI=9001"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("env[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestApplyPort(t *testing.T) {
	got := applyPort("go run . web -p ${port}", 9001)
	if got != "go run . web -p 9001" {
		t.Errorf("applyPort = %q", got)
	}
	// No placeholder → unchanged.
	if got := applyPort("npm run dev", 9002); got != "npm run dev" {
		t.Errorf("applyPort no-placeholder = %q", got)
	}
}

// writeWorktree creates a temp worktree dir with a .wt/dev.toml.
func writeWorktree(t *testing.T, toml string) string {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // isolate run state
	wt := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wt, ".wt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ConfigPath(wt), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	return wt
}

func TestLoad_PreservesServiceOrder(t *testing.T) {
	wt := writeWorktree(t, `
[[services]]
name = "api"
cmd = "go run . -p ${port}"

[[services]]
name = "web"
cmd = "npm run dev"
domain = true
`)
	cfg, err := Load(wt)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Services) != 2 {
		t.Fatalf("got %d services, want 2", len(cfg.Services))
	}
	if cfg.Services[0].Name != "api" || cfg.Services[1].Name != "web" {
		t.Errorf("order not preserved: %+v", cfg.Services)
	}
	if !cfg.Services[1].Domain {
		t.Error("web.domain should be true")
	}
}

func TestServeAndDown(t *testing.T) {
	wt := writeWorktree(t, `
[[services]]
name = "sleeper"
cmd = "sleep 30"
`)
	var buf bytes.Buffer
	if err := Serve(&buf, wt, 9000); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if !IsRunning(wt) {
		t.Fatal("IsRunning should be true after Serve")
	}

	if err := Down(&buf, wt); err != nil {
		t.Fatalf("Down: %v", err)
	}
	// Give the process group a moment to die.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !IsRunning(wt) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if IsRunning(wt) {
		t.Error("IsRunning should be false after Down")
	}
}

func TestServe_NoConfig(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var buf bytes.Buffer
	if err := Serve(&buf, t.TempDir(), 9000); err == nil {
		t.Error("expected error when .wt/dev.toml is missing")
	}
}

func TestServe_NoPortBase(t *testing.T) {
	wt := writeWorktree(t, "[[services]]\nname = \"x\"\ncmd = \"sleep 1\"\n")
	var buf bytes.Buffer
	if err := Serve(&buf, wt, 0); err == nil {
		t.Error("expected error when base is 0 (unallocated)")
	}
}
