package autostart

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"wt/internal/wt/core"
	"wt/internal/wt/devserver"
)

// TestHealthReaper_Tick_CrashRecovery is Case 1 from the issue #137 test spec:
// a service killed out-of-band (SIGKILL on the process group, running.json
// left intact — i.e. not going through devserver.Down) must be detected as
// crashed and re-served by the next Tick.
func TestHealthReaper_Tick_CrashRecovery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // isolate devserver run state
	t.Setenv("WT_NO_SYSTEMD_RUN", "1")      // deterministic PID -> process group
	container := filepath.Join(home, "Workspace", "myrepo")

	wt := makeWorktree(t, container, "wtcrash", 9040, true)

	var buf bytes.Buffer
	if err := devserver.Serve(&buf, wt, 9040); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	t.Cleanup(func() {
		_ = devserver.Down(&buf, wt)
	})

	recs := devserver.Recorded(wt)
	if len(recs) == 0 {
		t.Fatal("expected recorded services after Serve")
	}
	// Simulate a crash: kill the process group directly (bypassing
	// devserver.Down, so running.json is left recording a now-dead PID).
	for _, s := range recs {
		if err := syscall.Kill(-s.PID, syscall.SIGKILL); err != nil {
			t.Fatalf("kill process group: %v", err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && devserver.IsRunning(wt) {
		time.Sleep(50 * time.Millisecond)
	}
	if devserver.IsRunning(wt) {
		t.Fatal("service should be dead after SIGKILL before Tick")
	}

	hr := NewHealthReaper(2*time.Minute, 10*time.Minute, 3)
	hr.Tick(&buf)

	if !devserver.IsRunning(wt) {
		t.Errorf("expected HealthReaper.Tick to recover the crashed worktree\noutput:\n%s", buf.String())
	}
}

// TestHealthReaper_Tick_RunningIsUntouched is Case 2: a healthy, still-running
// worktree must not be re-served.
func TestHealthReaper_Tick_RunningIsUntouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	container := filepath.Join(home, "Workspace", "myrepo")
	wt := filepath.Join(container, "wtalive")

	serveCalled := 0
	hr := &HealthReaper{
		Cooldown:   10 * time.Minute,
		MaxRetries: 3,
		Now:        func() time.Time { return time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC) },
		Serve: func(out io.Writer, worktree string, base int) error {
			serveCalled++
			return nil
		},
		Down:      func(out io.Writer, worktree string) error { return nil },
		IsRunning: func(worktree string) bool { return true },
		Recorded: func(worktree string) []devserver.RunningService {
			return []devserver.RunningService{{Name: "sleeper", PID: 123, Port: 9040}}
		},
		retries: make(map[string]retryState),
	}
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "wtalive", &core.Entry{Type: "feature", PortBase: 9040, AutoStart: true}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	hr.Tick(&buf)
	if serveCalled != 0 {
		t.Errorf("expected 0 Serve calls for a running worktree, got %d", serveCalled)
	}
}

// TestHealthReaper_Tick_SkipsUnservedOrExplicitlyDown is Case 3: a worktree
// that was never served (3a), or that was served then explicitly Down'ed
// (3b, Recorded is empty afterwards), must never be auto-recovered.
func TestHealthReaper_Tick_SkipsUnservedOrExplicitlyDown(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	container := filepath.Join(home, "Workspace", "myrepo")

	serveCalled := 0
	newHR := func() *HealthReaper {
		return &HealthReaper{
			Cooldown:   10 * time.Minute,
			MaxRetries: 3,
			Now:        func() time.Time { return time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC) },
			Serve: func(out io.Writer, worktree string, base int) error {
				serveCalled++
				return nil
			},
			Down:      func(out io.Writer, worktree string) error { return nil },
			IsRunning: func(worktree string) bool { return false },
			Recorded: func(worktree string) []devserver.RunningService {
				return nil // 3a: never served. 3b: explicitly Down'ed (running.json removed).
			},
			retries: make(map[string]retryState),
		}
	}

	// 3a: never served.
	neverServed := filepath.Join(container, "wtnever")
	if err := os.MkdirAll(neverServed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "wtnever", &core.Entry{Type: "feature", PortBase: 9050, AutoStart: true}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	newHR().Tick(&buf)
	if serveCalled != 0 {
		t.Errorf("3a: expected 0 Serve calls for a never-served worktree, got %d", serveCalled)
	}

	// 3b: served then explicitly Down'ed.
	downed := filepath.Join(container, "wtdowned")
	if err := os.MkdirAll(downed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "wtdowned", &core.Entry{Type: "feature", PortBase: 9060, AutoStart: true}); err != nil {
		t.Fatal(err)
	}
	serveCalled = 0
	newHR().Tick(&buf)
	if serveCalled != 0 {
		t.Errorf("3b: expected 0 Serve calls for an explicitly-down worktree, got %d", serveCalled)
	}
}

// TestHealthReaper_Tick_CooldownStopsInfiniteRetry is Case 4: a worktree that
// keeps crashing must be retried up to MaxRetries within CooldownMinutes, then
// left alone until the cooldown window passes — HealthReaper must never become
// a new failure amplifier by spawning restarts forever.
func TestHealthReaper_Tick_CooldownStopsInfiniteRetry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	container := filepath.Join(home, "Workspace", "myrepo")
	wt := filepath.Join(container, "wtflapping")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "wtflapping", &core.Entry{Type: "feature", PortBase: 9070, AutoStart: true}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	serveCalled := 0
	var out bytes.Buffer
	hr := &HealthReaper{
		Cooldown:   10 * time.Minute,
		MaxRetries: 3,
		Now:        func() time.Time { return now },
		Serve: func(w io.Writer, worktree string, base int) error {
			serveCalled++
			return nil // "starts" but the mocked IsRunning below always reports it dead.
		},
		Down:      func(w io.Writer, worktree string) error { return nil },
		IsRunning: func(worktree string) bool { return false },
		Recorded: func(worktree string) []devserver.RunningService {
			return []devserver.RunningService{{Name: "sleeper", PID: 999, Port: 9070}}
		},
		retries: make(map[string]retryState),
	}

	for i := 0; i < 4; i++ {
		hr.Tick(&out)
		now = now.Add(1 * time.Minute)
	}
	if serveCalled != 3 {
		t.Errorf("expected exactly 3 Serve calls (MaxRetries) within the cooldown window, got %d", serveCalled)
	}
	if !bytes.Contains(out.Bytes(), []byte("crash-recovery")) {
		t.Errorf("expected a giving-up message to mention crash-recovery, got:\n%s", out.String())
	}
}

// TestHealthReaper_Tick_RetriesAfterFailedServeClearsRecorded is a regression
// test for a codex review P1 finding on #137: devserver.Serve always calls
// Down internally before (re)starting, so a Serve call that fails to start a
// crashed worktree also leaves running.json empty — exactly the same
// on-disk state as an explicit `wt dev down`. Before the fix, Tick treated
// "Recorded is empty" as an unconditional "never served / explicitly
// stopped" signal and dropped the retry counter after a single failed
// Serve, permanently abandoning crash-recovery well before MaxRetries.
func TestHealthReaper_Tick_RetriesAfterFailedServeClearsRecorded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	container := filepath.Join(home, "Workspace", "myrepo")
	wt := filepath.Join(container, "wtfailstart")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "wtfailstart", &core.Entry{Type: "feature", PortBase: 9080, AutoStart: true}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	serveCalled := 0
	var out bytes.Buffer
	hr := &HealthReaper{
		Cooldown:   10 * time.Minute,
		MaxRetries: 3,
		Now:        func() time.Time { return now },
		Serve: func(w io.Writer, worktree string, base int) error {
			serveCalled++
			return errors.New("boom: dev config broken")
		},
		Down:      func(w io.Writer, worktree string) error { return nil },
		IsRunning: func(worktree string) bool { return false },
		Recorded: func(worktree string) []devserver.RunningService {
			if serveCalled == 0 {
				// Crashed with a stale running.json before the first
				// recovery attempt.
				return []devserver.RunningService{{Name: "sleeper", PID: 999, Port: 9080}}
			}
			// devserver.Serve calls Down internally before starting, so
			// once a (failed) recovery attempt has happened, Recorded is
			// empty exactly like after an explicit stop.
			return nil
		},
		retries: make(map[string]retryState),
	}

	for i := 0; i < 4; i++ {
		hr.Tick(&out)
		now = now.Add(1 * time.Minute)
	}
	if serveCalled != 3 {
		t.Errorf("expected Serve to be retried up to MaxRetries (3) even though Recorded became empty after the first failed attempt, got %d calls", serveCalled)
	}
	if !bytes.Contains(out.Bytes(), []byte("crash-recovery")) {
		t.Errorf("expected a giving-up message to mention crash-recovery, got:\n%s", out.String())
	}
}

// TestHealthReaper_Tick_FlappingWorktreeStillHitsMaxRetries is a regression
// test for a codex review P1 finding on #137: before the fix, Tick reset
// r.retries unconditionally the instant IsRunning observed true, so a
// worktree that kept crashing and recovering within a single Cooldown
// window (alternating dead/alive every other tick) had its attempt counter
// zeroed by every intermediate successful recovery and never actually hit
// MaxRetries — defeating the very guard Case 4 exists to enforce.
func TestHealthReaper_Tick_FlappingWorktreeStillHitsMaxRetries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	container := filepath.Join(home, "Workspace", "myrepo")
	wt := filepath.Join(container, "wtflapalive")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "wtflapalive", &core.Entry{Type: "feature", PortBase: 9090, AutoStart: true}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	serveCalled := 0
	tick := 0
	var out bytes.Buffer
	hr := &HealthReaper{
		Cooldown:   10 * time.Minute,
		MaxRetries: 3,
		Now:        func() time.Time { return now },
		Serve: func(w io.Writer, worktree string, base int) error {
			serveCalled++
			return nil
		},
		Down: func(w io.Writer, worktree string) error { return nil },
		IsRunning: func(worktree string) bool {
			// Alternates dead (even ticks) / briefly-recovered (odd
			// ticks), all within a single Cooldown window.
			return tick%2 == 1
		},
		Recorded: func(worktree string) []devserver.RunningService {
			return []devserver.RunningService{{Name: "sleeper", PID: 999, Port: 9090}}
		},
		retries: make(map[string]retryState),
	}

	for tick = 0; tick < 10; tick++ {
		hr.Tick(&out)
		now = now.Add(1 * time.Minute)
	}
	if serveCalled != 3 {
		t.Errorf("expected exactly 3 Serve calls (MaxRetries) even though the worktree alternated dead/alive within the cooldown window, got %d", serveCalled)
	}
	if !bytes.Contains(out.Bytes(), []byte("crash-recovery")) {
		t.Errorf("expected a giving-up message to mention crash-recovery, got:\n%s", out.String())
	}
}
