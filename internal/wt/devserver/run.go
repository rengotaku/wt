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

// Serve starts every service in .wt/dev.toml, assigning service i the port
// base+i, and records their PIDs. Any previously started services are stopped
// first. base must be a non-zero allocated port base.
func Serve(out io.Writer, worktree string, base int) error {
	cfg, err := Load(worktree)
	if err != nil {
		return fmt.Errorf(".wt/dev.toml が読み込めません: %w", err)
	}
	if len(cfg.Services) == 0 {
		return errors.New(".wt/dev.toml に services が定義されていません")
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

	var r running
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
		r.Services = append(r.Services, RunningService{Name: svc.Name, PID: c.Process.Pid, Port: port, Cmd: cmdStr})
		_, _ = fmt.Fprintf(out, "▶ %s :%d (pid %d)\n", svc.Name, port, c.Process.Pid)
	}
	if len(r.Services) == 0 {
		return errors.New("起動できたサービスがありません")
	}
	return saveRunning(worktree, r)
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
