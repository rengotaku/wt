// Package settings holds wt's global, user-editable configuration, persisted
// as TOML at ~/.config/wt/settings.toml. Missing or invalid files fall back to
// built-in defaults so wt always has a usable configuration.
package settings

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	// DefaultDevPortStart / DefaultDevPortEnd are the default dev port band.
	DefaultDevPortStart = 9000
	DefaultDevPortEnd   = 9999
	// minBandSpan is the smallest allowed band width: it must fit at least one
	// port block (BlockSize in package ports, kept in sync here to avoid an
	// import cycle).
	minBandSpan = 5
)

// DevPorts is the configurable dev port band.
type DevPorts struct {
	Start int `toml:"start"`
	End   int `toml:"end"`
}

// IdleReaper configures the idle worktree reaper.
type IdleReaper struct {
	Enabled         bool `toml:"enabled"`
	TTLMinutes      int  `toml:"ttl_minutes"`
	IntervalMinutes int  `toml:"interval_minutes"`
}

// PortReaper configures the ghost-port reaper: it periodically prunes
// registry entries whose worktree directory is gone but whose port_base
// still lingers (see `wt ports prune`).
type PortReaper struct {
	Enabled         bool `toml:"enabled"`
	IntervalMinutes int  `toml:"interval_minutes"`
}

// ProcessStats configures memory thresholds for the process-stats view.
type ProcessStats struct {
	WarnMB   int `toml:"warn_mb"`
	DangerMB int `toml:"danger_mb"`
}

// Settings is the full wt settings document.
type Settings struct {
	DevPorts     DevPorts     `toml:"dev_ports"`
	IdleReaper   IdleReaper   `toml:"idle_reaper"`
	PortReaper   PortReaper   `toml:"port_reaper"`
	ProcessStats ProcessStats `toml:"process_stats"`
}

// defaultPortReaperIntervalMinutes is once a day.
const defaultPortReaperIntervalMinutes = 24 * 60

// Default returns the built-in settings.
func Default() Settings {
	return Settings{
		DevPorts:     DevPorts{Start: DefaultDevPortStart, End: DefaultDevPortEnd},
		IdleReaper:   IdleReaper{Enabled: true, TTLMinutes: 30, IntervalMinutes: 2},
		PortReaper:   PortReaper{Enabled: true, IntervalMinutes: defaultPortReaperIntervalMinutes},
		ProcessStats: ProcessStats{WarnMB: 2048, DangerMB: 4096},
	}
}

// Path returns the settings file location. Honors WT_CONFIG_DIR (used in tests).
func Path() string {
	if v := os.Getenv("WT_CONFIG_DIR"); v != "" {
		return filepath.Join(v, "settings.toml")
	}
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "wt", "settings.toml")
}

// Load reads settings from disk, falling back to defaults when the file is
// missing, unparseable, or invalid.
func Load() Settings {
	def := Default()
	data, err := os.ReadFile(Path())
	if err != nil {
		return def
	}
	loaded := Default()
	if _, err := toml.Decode(string(data), &loaded); err != nil {
		return def
	}
	// Fill unset (zero) fields from defaults, then validate the result.
	if loaded.DevPorts.Start == 0 {
		loaded.DevPorts.Start = def.DevPorts.Start
	}
	if loaded.DevPorts.End == 0 {
		loaded.DevPorts.End = def.DevPorts.End
	}
	if loaded.IdleReaper.TTLMinutes <= 0 {
		loaded.IdleReaper.TTLMinutes = 30
	}
	if loaded.IdleReaper.IntervalMinutes <= 0 {
		loaded.IdleReaper.IntervalMinutes = 2
	}
	if loaded.PortReaper.IntervalMinutes <= 0 {
		loaded.PortReaper.IntervalMinutes = defaultPortReaperIntervalMinutes
	}
	if loaded.ProcessStats.WarnMB <= 0 {
		loaded.ProcessStats.WarnMB = def.ProcessStats.WarnMB
	}
	if loaded.ProcessStats.DangerMB <= 0 {
		loaded.ProcessStats.DangerMB = def.ProcessStats.DangerMB
	}
	if loaded.ProcessStats.WarnMB >= loaded.ProcessStats.DangerMB {
		loaded.ProcessStats.WarnMB = def.ProcessStats.WarnMB
		loaded.ProcessStats.DangerMB = def.ProcessStats.DangerMB
	}
	if err := loaded.Validate(); err != nil {
		return def
	}
	return loaded
}

// Save validates and atomically writes settings to disk.
func Save(s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(s); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "settings-*.toml.tmp")
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
	return os.Rename(tmpPath, p)
}

// Validate ensures the dev port band is sane and fits at least one block.
func (s Settings) Validate() error {
	d := s.DevPorts
	if d.Start < 1024 || d.Start > 65535 {
		return fmt.Errorf("dev_ports.start は 1024-65535 の範囲で指定してください: %d", d.Start)
	}
	if d.End < 1024 || d.End > 65535 {
		return fmt.Errorf("dev_ports.end は 1024-65535 の範囲で指定してください: %d", d.End)
	}
	if d.Start >= d.End {
		return fmt.Errorf("dev_ports.start (%d) は end (%d) より小さくしてください", d.Start, d.End)
	}
	if d.End-d.Start+1 < minBandSpan {
		return fmt.Errorf("dev ポート帯は最低 %d ポート必要です（start=%d end=%d）", minBandSpan, d.Start, d.End)
	}
	return nil
}
