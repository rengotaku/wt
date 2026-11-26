package repo

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// `wt repo rm .. --force` must never remove HOME or the workspace dir. #180
func TestRm_RejectsTraversal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WT_MASTER_DIR", filepath.Join(home, "code"))
	ws := filepath.Join(home, "Workspace")
	sentinel := filepath.Join(ws, "keep", "file")
	if err := os.MkdirAll(filepath.Dir(sentinel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, bad := range []string{"..", ".", "keep/.."} {
		if err := Rm(io.Discard, bad, RmOptions{Force: true}); err == nil {
			t.Errorf("Rm(%q): want error", bad)
		}
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel removed: %v", err)
	}
}
