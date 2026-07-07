package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MissingFileReturnsDefault(t *testing.T) {
	t.Setenv("WT_CONFIG_DIR", t.TempDir())
	got := Load()
	if got.DevPorts.Start != DefaultDevPortStart || got.DevPorts.End != DefaultDevPortEnd {
		t.Errorf("Load() = %+v, want default %d-%d", got.DevPorts, DefaultDevPortStart, DefaultDevPortEnd)
	}
}

func TestSaveThenLoad_RoundTrip(t *testing.T) {
	t.Setenv("WT_CONFIG_DIR", t.TempDir())
	want := Settings{DevPorts: DevPorts{Start: 9500, End: 9700}}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := Load()
	if got.DevPorts != want.DevPorts {
		t.Errorf("round-trip = %+v, want %+v", got.DevPorts, want.DevPorts)
	}
	if _, err := os.Stat(Path()); err != nil {
		t.Errorf("settings file not written: %v", err)
	}
}

func TestLoad_InvalidFileFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WT_CONFIG_DIR", dir)
	// start >= end is invalid → must fall back to default.
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"),
		[]byte("[dev_ports]\nstart = 9999\nend = 9000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load()
	if got.DevPorts.Start != DefaultDevPortStart {
		t.Errorf("invalid file: got %+v, want default", got.DevPorts)
	}
}

func TestLoad_IdleReaper(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WT_CONFIG_DIR", dir)

	// 1. 未記載 -> default (enabled=true, ttl=30, interval=2)
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load()
	if !got.IdleReaper.Enabled || got.IdleReaper.TTLMinutes != 30 || got.IdleReaper.IntervalMinutes != 2 {
		t.Errorf("empty file: got %+v, want default enabled=true, ttl=30, interval=2", got.IdleReaper)
	}

	// 2. 明示値 -> 尊重
	content := `
[idle_reaper]
enabled = false
ttl_minutes = 60
interval_minutes = 5
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got = Load()
	if got.IdleReaper.Enabled || got.IdleReaper.TTLMinutes != 60 || got.IdleReaper.IntervalMinutes != 5 {
		t.Errorf("explicit file: got %+v, want enabled=false, ttl=60, interval=5", got.IdleReaper)
	}

	// 3. enabled=false 以外未記載 -> 補完されること
	contentPart := `
[idle_reaper]
enabled = false
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(contentPart), 0o644); err != nil {
		t.Fatal(err)
	}
	got = Load()
	if got.IdleReaper.Enabled || got.IdleReaper.TTLMinutes != 30 || got.IdleReaper.IntervalMinutes != 2 {
		t.Errorf("partial file: got %+v, want enabled=false, ttl=30, interval=2", got.IdleReaper)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		dp      DevPorts
		wantErr bool
	}{
		{name: "default ok", dp: DevPorts{Start: 9000, End: 9999}},
		{name: "narrow but >= one block", dp: DevPorts{Start: 9000, End: 9004}},
		{name: "start >= end", dp: DevPorts{Start: 9000, End: 9000}, wantErr: true},
		{name: "too narrow for a block", dp: DevPorts{Start: 9000, End: 9003}, wantErr: true},
		{name: "below privileged range", dp: DevPorts{Start: 80, End: 9000}, wantErr: true},
		{name: "above 65535", dp: DevPorts{Start: 9000, End: 70000}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Settings{DevPorts: tt.dp}.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
