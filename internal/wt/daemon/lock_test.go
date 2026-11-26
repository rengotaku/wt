package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquire_ExclusiveAndProbe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wt", "daemon.lock")

	if running, _, err := Probe(path); err != nil || running {
		t.Fatalf("before: running=%v err=%v, want false/nil", running, err)
	}

	first, err := Acquire(path)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := Acquire(path); err == nil || !strings.Contains(err.Error(), "既に起動") {
		t.Fatalf("second while held: got %v, want 既に起動 error", err)
	}
	running, pid, err := Probe(path)
	if err != nil || !running || pid != os.Getpid() {
		t.Fatalf("while held: running=%v pid=%d err=%v, want true/%d/nil", running, pid, err, os.Getpid())
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if running, _, err := Probe(path); err != nil || running {
		t.Fatalf("after release: running=%v err=%v, want false/nil", running, err)
	}
	again, err := Acquire(path)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	_ = again.Close()
}
