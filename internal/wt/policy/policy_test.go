package policy

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/wt/core"
)

func newContainer(t *testing.T) string {
	t.Helper()
	container := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(core.MetaFile(container), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return container
}

func setPolicy(t *testing.T, container, rel string) {
	t.Helper()
	cfg, err := core.LoadConfig(container)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AIPolicy = rel
	if err := core.SaveConfig(container, &cfg); err != nil {
		t.Fatal(err)
	}
}

func TestResolveUnset(t *testing.T) {
	container := newContainer(t)
	_, configured, err := Resolve(container)
	if err != nil {
		t.Fatal(err)
	}
	if configured {
		t.Fatal("expected unconfigured")
	}
}

func TestShowUnsetIsSilent(t *testing.T) {
	container := newContainer(t)
	var buf bytes.Buffer
	if err := Show(&buf, container); err != nil {
		t.Fatalf("Show: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("unset must produce no output, got %q", buf.String())
	}
}

func TestPathUnset(t *testing.T) {
	container := newContainer(t)
	var buf bytes.Buffer
	err := Path(&buf, container)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestShowReadsFile(t *testing.T) {
	container := newContainer(t)
	setPolicy(t, container, ".wt/ai-policy.md")
	want := "# hello\n"
	if err := os.MkdirAll(filepath.Join(container, ".wt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(container, ".wt", "ai-policy.md"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := Show(&buf, container); err != nil {
		t.Fatal(err)
	}
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

func TestShowMissingConfiguredFile(t *testing.T) {
	container := newContainer(t)
	setPolicy(t, container, ".wt/ai-policy.md")

	var buf bytes.Buffer
	err := Show(&buf, container)
	if err == nil {
		t.Fatal("expected an error for a configured but missing file")
	}
	if !strings.Contains(err.Error(), "ありません") {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("must not emit output on error, got %q", buf.String())
	}
}

func TestRejectsEscape(t *testing.T) {
	for _, rel := range []string{"../outside.md", "/etc/passwd", ".", "a/../../outside.md"} {
		t.Run(rel, func(t *testing.T) {
			container := newContainer(t)
			setPolicy(t, container, rel)
			if _, _, err := Resolve(container); err == nil {
				t.Fatalf("expected %q to be rejected", rel)
			}
		})
	}
}

func TestRejectsEscapeOnInit(t *testing.T) {
	container := newContainer(t)
	setPolicy(t, container, "../outside.md")
	var buf bytes.Buffer
	if err := Init(&buf, container); err == nil {
		t.Fatal("expected Init to reject an escaping path")
	}
}

func TestInitCreatesTemplateAndIsIdempotent(t *testing.T) {
	container := newContainer(t)
	mainDir := filepath.Join(container, "main")
	if err := os.MkdirAll(mainDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := Init(&buf, container); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(container, ".wt", "ai-policy.md")
	body, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("template not created: %v", err)
	}
	if !strings.Contains(string(body), "AI 作業方針") {
		t.Fatalf("template looks wrong: %q", string(body))
	}
	cfg, err := core.LoadConfig(container)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AIPolicy != DefaultRelPath {
		t.Fatalf("ai_policy = %q, want %q", cfg.AIPolicy, DefaultRelPath)
	}

	// A second init must not overwrite a user-edited file.
	custom := "# my policy\n"
	if err := os.WriteFile(p, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := Init(&buf, container); err != nil {
		t.Fatal(err)
	}
	body, err = os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != custom {
		t.Fatalf("Init overwrote the file: %q", string(body))
	}
	if !strings.Contains(buf.String(), "既にあります") {
		t.Fatalf("expected an exists message, got %q", buf.String())
	}

	// The git repository must stay untouched.
	entries, err := os.ReadDir(mainDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("wt policy wrote into the repo: %v", entries)
	}
}

func TestInitKeepsConfiguredPath(t *testing.T) {
	container := newContainer(t)
	setPolicy(t, container, "custom/p.md")

	var buf bytes.Buffer
	if err := Init(&buf, container); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(container, "custom", "p.md")); err != nil {
		t.Fatalf("custom path not created: %v", err)
	}
	cfg, err := core.LoadConfig(container)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AIPolicy != "custom/p.md" {
		t.Fatalf("ai_policy = %q, want custom/p.md", cfg.AIPolicy)
	}
}

func TestInitRecordsExistingFile(t *testing.T) {
	container := newContainer(t)
	if err := os.MkdirAll(filepath.Join(container, ".wt"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(container, ".wt", "ai-policy.md")
	if err := os.WriteFile(p, []byte("# existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := Init(&buf, container); err != nil {
		t.Fatal(err)
	}
	cfg, err := core.LoadConfig(container)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AIPolicy != DefaultRelPath {
		t.Fatalf("ai_policy = %q, want %q", cfg.AIPolicy, DefaultRelPath)
	}
}
