package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
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

// A probe that holds the shared lock for a moment must not make a starting
// daemon think another daemon is running.
func TestAcquire_NotBlockedByProbeLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.lock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	probe, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(probe.Fd()), syscall.LOCK_SH); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = probe.Close()
	}()
	f, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire while a probe briefly held the lock: %v", err)
	}
	_ = f.Close()
}

func TestAcquire_RefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "victim")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "daemon.lock")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if f, err := Acquire(link); err == nil {
		_ = f.Close()
		t.Fatal("Acquire through a symlink: want error")
	}
	if b, _ := os.ReadFile(target); string(b) != "keep" {
		t.Errorf("symlink target was modified: %q", b)
	}
}
