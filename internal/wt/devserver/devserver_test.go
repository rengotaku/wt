package devserver

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wt/internal/wt/core"
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

func TestServe_DeadOnStartupNotRecorded(t *testing.T) {
	// A command that exits immediately (as a port conflict would) must be
	// reported as failed, not as running.
	wt := writeWorktree(t, "[[services]]\nname = \"crasher\"\ncmd = \"echo 'Address already in use' >&2; exit 1\"\n")
	var buf bytes.Buffer
	err := Serve(&buf, wt, 9000)
	if err == nil {
		t.Fatal("expected error when the only service dies on startup")
	}
	if IsRunning(wt) {
		t.Error("IsRunning should be false: the crashed service must not be recorded")
	}
	if !strings.Contains(buf.String(), "Address already in use") {
		t.Errorf("output should include the log tail explaining the failure, got:\n%s", buf.String())
	}
}

func TestServe_PartialFailureKeepsSurvivors(t *testing.T) {
	// One service stays up, one dies: Serve succeeds, records only the survivor.
	wt := writeWorktree(t, `
[[services]]
name = "good"
cmd = "sleep 30"

[[services]]
name = "bad"
cmd = "exit 1"
`)
	var buf bytes.Buffer
	if err := Serve(&buf, wt, 9000); err != nil {
		t.Fatalf("Serve should succeed when at least one service survives: %v", err)
	}
	r, err := loadRunning(wt)
	if err != nil {
		t.Fatalf("loadRunning: %v", err)
	}
	if len(r.Services) != 1 || r.Services[0].Name != "good" {
		t.Errorf("only the surviving service should be recorded, got %+v", r.Services)
	}
	_ = Down(&buf, wt)
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{name: "ok", cfg: Config{Services: []Service{{Name: "api", Cmd: "go run ."}}}},
		{name: "no services", cfg: Config{}, wantErr: true},
		{name: "empty name", cfg: Config{Services: []Service{{Name: "", Cmd: "x"}}}, wantErr: true},
		{name: "empty cmd", cfg: Config{Services: []Service{{Name: "api", Cmd: "  "}}}, wantErr: true},
		{name: "dup name", cfg: Config{Services: []Service{{Name: "api", Cmd: "a"}, {Name: "api", Cmd: "b"}}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSave_RoundTripAndRejectsInvalid(t *testing.T) {
	wt := t.TempDir()
	cfg := Config{Services: []Service{
		{Name: "api", Cmd: "go run . -p ${port}"},
		{Name: "web", Cmd: "npm run dev", Domain: true},
	}}
	if err := Save(wt, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !HasConfig(wt) {
		t.Fatal("HasConfig should be true after Save")
	}
	got, err := Load(wt)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Services) != 2 || got.Services[0].Name != "api" || !got.Services[1].Domain {
		t.Errorf("round-trip mismatch: %+v", got.Services)
	}

	// Invalid config must not be written.
	wt2 := t.TempDir()
	if err := Save(wt2, Config{}); err == nil {
		t.Error("Save should reject empty config")
	}
	if HasConfig(wt2) {
		t.Error("invalid Save must not create a dev.toml")
	}
}

func TestEffectiveConfig_Precedence(t *testing.T) {
	container := t.TempDir()
	wtName := "feat-x"
	worktree := filepath.Join(container, wtName)
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}

	// 1) Nothing → SourceNone.
	if _, src, err := EffectiveConfig(worktree); err != nil || src != SourceNone {
		t.Fatalf("empty: src=%q err=%v, want none", src, err)
	}

	// 2) Repo default only → SourceRepo.
	if err := core.SaveConfig(container, core.EntryConfig{
		DevServices: []core.DevService{{Name: "api", Cmd: "run-default"}},
	}); err != nil {
		t.Fatal(err)
	}
	cfg, src, err := EffectiveConfig(worktree)
	if err != nil || src != SourceRepo || cfg.Services[0].Cmd != "run-default" {
		t.Fatalf("repo default: src=%q cmd=%q err=%v", src, cmdOf(cfg), err)
	}

	// 3) Per-worktree override wins over repo default → SourceWorktree.
	if err := core.PutEntry(container, wtName, &core.Entry{
		Branch:      "feat/x",
		DevServices: []core.DevService{{Name: "api", Cmd: "run-override"}},
	}); err != nil {
		t.Fatal(err)
	}
	cfg, src, err = EffectiveConfig(worktree)
	if err != nil || src != SourceWorktree || cfg.Services[0].Cmd != "run-override" {
		t.Fatalf("override: src=%q cmd=%q err=%v", src, cmdOf(cfg), err)
	}
}

func cmdOf(c Config) string {
	if len(c.Services) == 0 {
		return ""
	}
	return c.Services[0].Cmd
}

func TestLogs_CapturesOutputAndPersists(t *testing.T) {
	wt := writeWorktree(t, "[[services]]\nname = \"noisy\"\ncmd = \"echo HELLO_LOG_LINE; sleep 30\"\n")
	var buf bytes.Buffer
	if err := Serve(&buf, wt, 9000); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	// Output is flushed during the startup grace, so it's available immediately.
	logs := Logs(wt, 0)
	if len(logs) != 1 || logs[0].Name != "noisy" {
		t.Fatalf("logs = %+v, want one 'noisy' entry", logs)
	}
	if !strings.Contains(logs[0].Content, "HELLO_LOG_LINE") {
		t.Errorf("log content missing output: %q", logs[0].Content)
	}
	// Logs persist after stop so a crashed/stopped serve stays inspectable.
	_ = Down(&buf, wt)
	if logs := Logs(wt, 0); len(logs) != 1 {
		t.Errorf("logs should persist after Down, got %+v", logs)
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
