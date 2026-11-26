// Package daemon holds the single-instance lock shared by `wt daemon` and the
// commands that report whether it is running.
package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// LockPath is the lock file shared by every wt daemon of this user.
func LockPath() string {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(base, "wt", "daemon.lock")
}

// Acquire takes an exclusive, non-blocking flock on path and writes the
// current PID into it. Two daemons would both serve the same AutoStart
// worktrees and overwrite each other's running.json, so a second one must
// refuse to start. The lock is released when the returned file is closed or
// the process exits.
func Acquire(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// O_NOFOLLOW: never truncate/write through a symlink planted at the path.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockExclusive(f); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("wt daemon は既に起動しています（lock: %s）", path)
		}
		return nil, fmt.Errorf("lock を取得できません（%s）: %w", path, err)
	}
	// The PID is informational (for `wt status`); failing to record it must
	// not stop the daemon.
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return f, nil
}

// lockExclusive takes the lock without blocking on a running daemon, but
// retries briefly: Probe holds a shared lock for a moment, and a daemon
// starting at that instant must not mistake it for another daemon.
func lockExclusive(f *os.File) error {
	var err error
	for i := 0; i < 10; i++ {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return err
}

// Probe reports whether a daemon currently holds the lock at path, and its
// PID when recorded (0 if unknown). A missing lock file means not running.
func Probe(path string) (running bool, pid int, err error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, 0, nil
		}
		return false, 0, err
	}
	defer func() { _ = f.Close() }()

	lockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
	switch {
	case lockErr == nil:
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return false, 0, nil
	case errors.Is(lockErr, syscall.EWOULDBLOCK):
		data, _ := os.ReadFile(path)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		return true, pid, nil
	default:
		return false, 0, lockErr
	}
}
