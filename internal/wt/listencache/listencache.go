// Package listencache keeps the last scan of LISTEN sockets on disk, so that
// `wt repo ls` can skip the system-wide lsof call, which takes over a second
// on macOS (#205).
//
// Only the socket scan is cached. Worktrees, ports, AutoStart and the recorded
// service PIDs are still read live by the callers, so the cache can only make
// the listening state of a port stale, never the list of repositories.
//
// Writers: every fresh scan (ports.Status), and `wt daemon` once a minute.
// devserver.Serve / Down remove the file, since they change what listens.
package listencache

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Entry is one LISTEN socket inside the dev port band.
type Entry struct {
	Port int    `json:"port"`
	PID  int    `json:"pid"`
	Proc string `json:"proc"`
}

type file struct {
	Taken   time.Time `json:"taken"`
	Start   int       `json:"start"`
	End     int       `json:"end"`
	Entries []Entry   `json:"listeners"`
}

// pathFunc is a var so tests can point the cache at a temp dir.
var pathFunc = defaultPath

func defaultPath() string {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(base, "wt", "listeners.json")
}

// Path is where the cache lives.
func Path() string { return pathFunc() }

// SetPathForTest points the cache at p and returns a func that restores it.
// Tests of other packages use it so they never touch the user's cache.
func SetPathForTest(p string) (restore func()) {
	old := pathFunc
	pathFunc = func() string { return p }
	return func() { pathFunc = old }
}

// Load returns the cached sockets when the cache was taken for the same port
// band [start, end] and is younger than maxAge at now. ok is false when the
// cache is missing, unreadable, for another band or too old; the caller then
// scans again.
func Load(start, end int, maxAge time.Duration, now time.Time) (entries []Entry, ok bool) {
	b, err := os.ReadFile(Path())
	if err != nil {
		return nil, false
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, false
	}
	if f.Start != start || f.End != end {
		return nil, false
	}
	age := now.Sub(f.Taken)
	if age < 0 || age > maxAge {
		return nil, false
	}
	return f.Entries, true
}

// Save writes the sockets of one scan. It writes a temp file and renames it,
// so a reader never sees a half-written cache.
func Save(start, end int, entries []Entry, now time.Time) error {
	if entries == nil {
		entries = []Entry{}
	}
	b, err := json.Marshal(file{Taken: now, Start: start, End: end, Entries: entries})
	if err != nil {
		return err
	}
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".listeners-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, p); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// Invalidate removes the cache. A missing cache is not an error.
func Invalidate() error {
	if err := os.Remove(Path()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
