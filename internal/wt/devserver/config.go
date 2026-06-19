// Package devserver starts and stops a worktree's dev servers as defined in its
// .wt/dev.toml, injecting the worktree's allocated ports. Each service gets one
// port from the worktree's block (base + declaration index); the port is made
// available to the command via the ${port} placeholder and the PORT env var.
package devserver

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"wt/internal/wt/core"
)

// Service is one dev service. Declaration order matters: service i is assigned
// port base+i. JSON tags mirror the TOML keys so the Web API can read/write the
// same shape.
type Service struct {
	Name   string `toml:"name" json:"name"`
	Cmd    string `toml:"cmd" json:"cmd"`
	Domain bool   `toml:"domain" json:"domain"` // expose via reverse proxy (used by #29)
}

// Config is the parsed .wt/dev.toml.
type Config struct {
	Services []Service `toml:"services" json:"services"`
}

// Validate checks a Config is safe to persist and serve: at least one service,
// every service has a non-empty name and command, and names are unique (names
// become WT_PORT_<NAME> env keys, so duplicates would collide).
func (c Config) Validate() error {
	if len(c.Services) == 0 {
		return errors.New("service を最低1つ定義してください")
	}
	seen := map[string]bool{}
	for i, s := range c.Services {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			return fmt.Errorf("service[%d]: name が空です", i)
		}
		if strings.TrimSpace(s.Cmd) == "" {
			return fmt.Errorf("service %q: cmd が空です", name)
		}
		if seen[name] {
			return fmt.Errorf("service 名が重複しています: %q", name)
		}
		seen[name] = true
	}
	return nil
}

// Save validates cfg and atomically writes it as .wt/dev.toml in the worktree,
// creating the .wt directory as needed.
func Save(worktree string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	dir := filepath.Join(worktree, ".wt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	dst := ConfigPath(worktree)
	tmp, err := os.CreateTemp(dir, "dev-*.toml.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, dst)
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

// Config sources, returned by EffectiveConfig.
const (
	SourceNone     = ""         // no config anywhere
	SourceWorktree = "worktree" // per-worktree metadata override
	SourceRepo     = "repo"     // repository default in _config
	SourceFile     = "file"     // committed .wt/dev.toml
)

// sourceLabel returns a human-readable label for an EffectiveConfig source,
// so callers can show which layer is actually in effect.
func sourceLabel(source string) string {
	switch source {
	case SourceFile:
		return "committed .wt/dev.toml"
	case SourceRepo:
		return "リポジトリ既定 (_config.dev_services)"
	case SourceWorktree:
		return "worktree 上書き"
	default:
		return "なし"
	}
}

// sameServices reports whether two configs declare identical services (order,
// name, cmd, domain). Service is comparable (all fields are string/bool), so
// struct equality is exact.
func sameServices(a, b Config) bool {
	if len(a.Services) != len(b.Services) {
		return false
	}
	for i := range a.Services {
		if a.Services[i] != b.Services[i] {
			return false
		}
	}
	return true
}

// fromCore converts stored metadata services into a runtime Config.
func fromCore(in []core.DevService) Config {
	out := make([]Service, 0, len(in))
	for _, s := range in {
		out = append(out, Service{Name: s.Name, Cmd: s.Cmd, Domain: s.Domain})
	}
	return Config{Services: out}
}

// EffectiveConfig resolves the dev config for a worktree by precedence:
// per-worktree metadata override > repository default (metadata) > committed
// .wt/dev.toml file. source reports which one was used (SourceNone when none).
func EffectiveConfig(worktree string) (cfg Config, source string, err error) {
	container := filepath.Dir(worktree)
	wtName := filepath.Base(worktree)

	if entries, e := core.LoadEntries(container); e == nil {
		if entry, ok := entries[wtName]; ok && len(entry.DevServices) > 0 {
			return fromCore(entry.DevServices), SourceWorktree, nil
		}
	}
	if rc, e := core.LoadConfig(container); e == nil && len(rc.DevServices) > 0 {
		return fromCore(rc.DevServices), SourceRepo, nil
	}
	if HasConfig(worktree) {
		c, e := Load(worktree)
		if e != nil {
			return Config{}, SourceNone, e
		}
		return c, SourceFile, nil
	}
	return Config{}, SourceNone, nil
}

// HasEffectiveConfig reports whether a worktree resolves to any dev config
// (override, repo default, or committed file).
func HasEffectiveConfig(worktree string) bool {
	_, source, err := EffectiveConfig(worktree)
	return err == nil && source != SourceNone
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
