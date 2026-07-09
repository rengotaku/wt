package handler

import (
	"testing"
)

func TestRestartDevIfRunning_NotRunning(t *testing.T) {
	// 未稼働（devserver.IsRunning が false になる一時ディレクトリ）
	worktree := t.TempDir()

	restarted, err := defaultRestartDevIfRunning("dummy_container", "dummy_wtName", worktree)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if restarted {
		t.Errorf("expected restarted to be false, got true")
	}
}
