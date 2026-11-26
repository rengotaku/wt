package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquireDaemonLock_Exclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wt", "daemon.lock")

	first, err := acquireDaemonLock(path)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := acquireDaemonLock(path); err == nil || !strings.Contains(err.Error(), "既に起動") {
		t.Fatalf("second while held: got %v, want 既に起動 error", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := acquireDaemonLock(path)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	_ = again.Close()
}
