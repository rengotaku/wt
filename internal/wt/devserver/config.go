// Package devserver starts and stops a worktree's dev servers as defined in its
// .wt/dev.toml, injecting the worktree's allocated ports. Each service gets one
// port from the worktree's block (base + declaration index); the port is made
// available to the command via the ${port} placeholder and the PORT env var.
package devserver

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Service is one dev service. Declaration order matters: service i is assigned
// port base+i.
type Service struct {
	Name   string `toml:"name"`
	Cmd    string `toml:"cmd"`
	Domain bool   `toml:"domain"` // expose via reverse proxy (used by #29)
}

// Config is the parsed .wt/dev.toml.
type Config struct {
	Services []Service `toml:"services"`
}

// ConfigPath returns the .wt/dev.toml path for a worktree.
func ConfigPath(worktree string) string {
	return filepath.Join(worktree, ".wt", "dev.toml")
}

// HasConfig reports whether the worktree defines dev services.
func HasConfig(worktree string) bool {
	_, err := os.Stat(ConfigPath(worktree))
	return err == nil
}

// Load reads and parses .wt/dev.toml.
func Load(worktree string) (Config, error) {
	var c Config
	data, err := os.ReadFile(ConfigPath(worktree))
	if err != nil {
		return c, err
	}
	if _, err := toml.Decode(string(data), &c); err != nil {
		return c, err
	}
	return c, nil
}

// applyPort substitutes ${port} in a command string with the given port.
func applyPort(cmd string, port int) string {
	return strings.ReplaceAll(cmd, "${port}", strconv.Itoa(port))
}

// sharedEnv builds WT_PORT_<NAME>=<port> entries for every service so each
// service can discover its siblings' allocated ports. Service i gets base+i.
func sharedEnv(services []Service, base int) []string {
	repl := strings.NewReplacer("-", "_", " ", "_")
	out := make([]string, 0, len(services))
	for i, s := range services {
		name := strings.ToUpper(repl.Replace(s.Name))
		out = append(out, "WT_PORT_"+name+"="+strconv.Itoa(base+i))
	}
	return out
}
