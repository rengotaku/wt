package devserver

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wt/internal/wt/listencache"
)

// #205: Down changes what listens, so it must drop the cached socket scan
// even when nothing was recorded for the worktree.
func TestDownInvalidatesListenCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p := filepath.Join(t.TempDir(), "listeners.json")
	t.Cleanup(listencache.SetPathForTest(p))
	if err := listencache.Save(9000, 9999, nil, time.Now()); err != nil {
		t.Fatal(err)
	}

	_ = Down(io.Discard, t.TempDir())

	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("cache still exists after Down: %v", err)
	}
}
