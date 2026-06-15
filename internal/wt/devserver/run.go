package devserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// RunningService records a started process.
type RunningService struct {
	Name string `json:"name"`
	PID  int    `json:"pid"`
	Port int    `json:"port"`
	Cmd  string `json:"cmd"`
}

// running is the persisted state of a worktree's started services.
type running struct {
	Services []RunningService `json:"services"`
}

// runDir returns a per-worktree state directory under the user cache dir.
func runDir(worktree string) string {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	key := strings.ReplaceAll(strings.Trim(worktree, "/"), "/", "_")
	return filepath.Join(base, "wt", "run", key)
}

func statePath(worktree string) string { return filepath.Join(runDir(worktree), "running.json") }
func logPath(worktree, svc string) string {
	return filepath.Join(runDir(worktree), svc+".log")
}

func loadRunning(worktree string) (running, error) {
	var r running
	data, err := os.ReadFile(statePath(worktree))
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(data, &r)
	return r, err
}

func saveRunning(worktree string, r running) error {
	if err := os.MkdirAll(runDir(worktree), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(worktree), data, 0o644)
}

// pidAlive reports whether the process is still running.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// signal 0 probes existence without affecting the process.
	return syscall.Kill(pid, 0) == nil
}

// IsRunning reports whether any service recorded for the worktree is alive.
func IsRunning(worktree string) bool {
	r, err := loadRunning(worktree)
	if err != nil {
		return false
	}
	for _, s := range r.Services {
		if pidAlive(s.PID) {
			return true
		}
	}
	return false
}

// startupGrace is how long Serve waits after spawning before judging whether a
// service stayed up. A misconfigured command or a port conflict (EADDRINUSE)
// makes the child exit within milliseconds, so this catches "started but
// instantly died" while staying short enough not to delay a healthy launch.
var startupGrace = 600 * time.Millisecond

// Serve starts every service in .wt/dev.toml, assigning service i the port
// base+i, and records their PIDs. Any previously started services are stopped
// first. base must be a non-zero allocated port base.
//
// After spawning, Serve waits startupGrace and verifies each process is still
// alive: a service that exited (e.g. its port was already taken) is reported as
// failed with the tail of its log, and is NOT recorded as running. This is why
// Serve never claims success for a process that died on startup.
func Serve(out io.Writer, worktree string, base int) error {
	cfg, source, err := EffectiveConfig(worktree)
	if err != nil {
		return fmt.Errorf("dev 設定が読み込めません: %w", err)
	}
	if source == SourceNone || len(cfg.Services) == 0 {
		return errors.New("dev 設定がありません（リポジトリ既定または worktree 上書きを設定してください）")
	}
	if base == 0 {
		return errors.New("この worktree にはポートが割り当てられていません（wt tree add で作成した worktree が必要）")
	}
	_ = Down(io.Discard, worktree)

	if err := os.MkdirAll(runDir(worktree), 0o755); err != nil {
		return err
	}

	// Every service learns all service ports via WT_PORT_<NAME> so a frontend
	// can proxy to a sibling backend on its allocated port.
	shared := sharedEnv(cfg.Services, base)

	// started tracks each spawned child with a channel that closes when the
	// process exits. We must reap children we started (via Wait) so a process
	// that died on startup is not left a zombie — a zombie still answers
	// kill(pid, 0), which would otherwise make it look alive.
	type spawned struct {
		svc  RunningService
		exit chan struct{}
	}
	var started []spawned
	for i, svc := range cfg.Services {
		port := base + i
		cmdStr := applyPort(svc.Cmd, port)
		c := exec.Command("sh", "-c", cmdStr)
		c.Dir = worktree
		c.Env = append(os.Environ(), shared...)
		c.Env = append(c.Env, fmt.Sprintf("PORT=%d", port))
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		logf, err := os.Create(logPath(worktree, svc.Name))
		if err == nil {
			c.Stdout = logf
			c.Stderr = logf
		}
		if err := c.Start(); err != nil {
			_, _ = fmt.Fprintf(out, "⚠️  %s の起動に失敗: %v\n", svc.Name, err)
			continue
		}
		exit := make(chan struct{})
		go func() { _ = c.Wait(); close(exit) }()
		started = append(started, spawned{
			svc:  RunningService{Name: svc.Name, PID: c.Process.Pid, Port: port, Cmd: cmdStr},
			exit: exit,
		})
	}

	// Give the children a moment, then keep only the ones that survived.
	time.Sleep(startupGrace)
	var r running
	for _, s := range started {
		select {
		case <-s.exit:
			_, _ = fmt.Fprintf(out, "⚠️  %s :%d が起動直後に終了しました\n%s\n",
				s.svc.Name, s.svc.Port, logTail(worktree, s.svc.Name))
		default:
			r.Services = append(r.Services, s.svc)
			_, _ = fmt.Fprintf(out, "▶ %s :%d (pid %d)\n", s.svc.Name, s.svc.Port, s.svc.PID)
		}
	}
	if len(r.Services) == 0 {
		return errors.New("起動できたサービスがありません（ログを確認してください）")
	}
	return saveRunning(worktree, r)
}

// logTail returns the last few lines of a service's log to explain a startup
// failure (e.g. "Address already in use").
func logTail(worktree, svc string) string {
	data, err := os.ReadFile(logPath(worktree, svc))
	if err != nil {
		return "  (ログがありません)"
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	const maxLines = 8
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	for i, l := range lines {
		lines[i] = "  │ " + l
	}
	return strings.Join(lines, "\n")
}

// Down stops all services recorded for the worktree (killing the process group)
// and clears the state. It is a no-op when nothing is recorded.
func Down(out io.Writer, worktree string) error {
	r, err := loadRunning(worktree)
	if err != nil {
		return nil //nolint:nilerr // no state → nothing to stop
	}
	for _, s := range r.Services {
		if !pidAlive(s.PID) {
			continue
		}
		// Negative PID targets the whole process group started with Setpgid.
		_ = syscall.Kill(-s.PID, syscall.SIGTERM)
		_, _ = fmt.Fprintf(out, "■ %s (pid %d) を停止\n", s.Name, s.PID)
	}
	return os.Remove(statePath(worktree))
}
