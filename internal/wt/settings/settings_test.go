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
	want := Settings{DevPorts: DevPorts{Start: 9500, End: 9700}, Proxy: Proxy{Enabled: true, Port: DefaultProxyPort}}
	if err := Save(&want); err != nil {
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

func TestLoad_PortReaper(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WT_CONFIG_DIR", dir)

	// 1. 未記載 -> default (enabled=true, interval=1440 = 24h)
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load()
	if !got.PortReaper.Enabled || got.PortReaper.IntervalMinutes != 24*60 {
		t.Errorf("empty file: got %+v, want default enabled=true, interval=1440", got.PortReaper)
	}

	// 2. 明示値 -> 尊重
	content := `
[port_reaper]
enabled = false
interval_minutes = 60
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got = Load()
	if got.PortReaper.Enabled || got.PortReaper.IntervalMinutes != 60 {
		t.Errorf("explicit file: got %+v, want enabled=false, interval=60", got.PortReaper)
	}

	// 3. interval_minutes <= 0 -> 既定値へ補正
	contentZero := `
[port_reaper]
enabled = true
interval_minutes = 0
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(contentZero), 0o644); err != nil {
		t.Fatal(err)
	}
	got = Load()
	if !got.PortReaper.Enabled || got.PortReaper.IntervalMinutes != 24*60 {
		t.Errorf("interval<=0: got %+v, want enabled=true, interval=1440 (default)", got.PortReaper)
	}

	// 4. enabled=false 以外未記載 -> 補完されること
	contentPart := `
[port_reaper]
enabled = false
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(contentPart), 0o644); err != nil {
		t.Fatal(err)
	}
	got = Load()
	if got.PortReaper.Enabled || got.PortReaper.IntervalMinutes != 24*60 {
		t.Errorf("partial file: got %+v, want enabled=false, interval=1440", got.PortReaper)
	}
}

func TestLoad_HealthReaper(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WT_CONFIG_DIR", dir)

	// 1. 未記載 -> default (enabled=true, interval=2, cooldown=10, max_retries=3)
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load()
	if !got.HealthReaper.Enabled || got.HealthReaper.IntervalMinutes != 2 ||
		got.HealthReaper.CooldownMinutes != 10 || got.HealthReaper.MaxRetries != 3 {
		t.Errorf("empty file: got %+v, want default enabled=true, interval=2, cooldown=10, max_retries=3", got.HealthReaper)
	}

	// 2. 明示値 -> 尊重
	content := `
[health_reaper]
enabled = false
interval_minutes = 5
cooldown_minutes = 20
max_retries = 5
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got = Load()
	if got.HealthReaper.Enabled || got.HealthReaper.IntervalMinutes != 5 ||
		got.HealthReaper.CooldownMinutes != 20 || got.HealthReaper.MaxRetries != 5 {
		t.Errorf("explicit file: got %+v, want enabled=false, interval=5, cooldown=20, max_retries=5", got.HealthReaper)
	}

	// 3. 0 以下の値 -> 既定値へ補正
	contentZero := `
[health_reaper]
enabled = true
interval_minutes = 0
cooldown_minutes = 0
max_retries = 0
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(contentZero), 0o644); err != nil {
		t.Fatal(err)
	}
	got = Load()
	if !got.HealthReaper.Enabled || got.HealthReaper.IntervalMinutes != 2 ||
		got.HealthReaper.CooldownMinutes != 10 || got.HealthReaper.MaxRetries != 3 {
		t.Errorf("<=0 values: got %+v, want default interval=2, cooldown=10, max_retries=3", got.HealthReaper)
	}

	// 4. enabled=false 以外未記載 -> 補完されること
	contentPart := `
[health_reaper]
enabled = false
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(contentPart), 0o644); err != nil {
		t.Fatal(err)
	}
	got = Load()
	if got.HealthReaper.Enabled || got.HealthReaper.IntervalMinutes != 2 ||
		got.HealthReaper.CooldownMinutes != 10 || got.HealthReaper.MaxRetries != 3 {
		t.Errorf("partial file: got %+v, want enabled=false, interval=2, cooldown=10, max_retries=3", got.HealthReaper)
	}
}

func TestLoad_ProcessStats(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WT_CONFIG_DIR", dir)

	// 1. 未記載 -> 既定 2048/4096
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load()
	if got.ProcessStats.WarnMB != 2048 || got.ProcessStats.DangerMB != 4096 {
		t.Errorf("empty file: got %+v, want 2048/4096", got.ProcessStats)
	}

	// 2. 明示値 -> 尊重
	content := `
[process_stats]
warn_mb = 100
danger_mb = 500
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got = Load()
	if got.ProcessStats.WarnMB != 100 || got.ProcessStats.DangerMB != 500 {
		t.Errorf("explicit file: got %+v, want 100/500", got.ProcessStats)
	}

	// 3. warn>=danger -> 既定に戻る
	contentInv := `
[process_stats]
warn_mb = 1000
danger_mb = 500
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(contentInv), 0o644); err != nil {
		t.Fatal(err)
	}
	got = Load()
	if got.ProcessStats.WarnMB != 2048 || got.ProcessStats.DangerMB != 4096 {
		t.Errorf("invalid warn>=danger file: got %+v, want 2048/4096", got.ProcessStats)
	}
}

func TestLoad_Proxy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WT_CONFIG_DIR", dir)

	// 1. 未記載 -> default (enabled=true, port=8088)
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load()
	if !got.Proxy.Enabled || got.Proxy.Port != DefaultProxyPort {
		t.Errorf("empty file: got %+v, want enabled=true port=%d", got.Proxy, DefaultProxyPort)
	}

	// 2. 明示値 -> 尊重
	content := `
[proxy]
enabled = false
port = 8100
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got = Load()
	if got.Proxy.Enabled || got.Proxy.Port != 8100 {
		t.Errorf("explicit file: got %+v, want enabled=false port=8100", got.Proxy)
	}

	// 3. port=0 -> 既定値へ補正（enabled は保持）
	contentZero := `
[proxy]
enabled = true
port = 0
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(contentZero), 0o644); err != nil {
		t.Fatal(err)
	}
	got = Load()
	if !got.Proxy.Enabled || got.Proxy.Port != DefaultProxyPort {
		t.Errorf("port=0: got %+v, want enabled=true port=%d (default)", got.Proxy, DefaultProxyPort)
	}

	// 4. port 範囲外 -> Validate 失敗で全体が Default にフォールバック
	contentOOR := `
[dev_ports]
start = 9000
end = 9999
[proxy]
enabled = true
port = 80
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(contentOOR), 0o644); err != nil {
		t.Fatal(err)
	}
	got = Load()
	if got.Proxy.Port != DefaultProxyPort {
		t.Errorf("port<1024: got %+v, want default fallback", got.Proxy)
	}
}

func TestLoad_ProxyBind(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WT_CONFIG_DIR", dir)

	// 1. bind 未記載 -> default (0.0.0.0)。この field を持たない古い
	//    settings.toml がそのまま残っていても LAN 公開の既定が効く。
	content := `
[proxy]
enabled = true
port = 8088
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(); got.Proxy.Bind != DefaultProxyBind {
		t.Errorf("bind 未記載: got %q, want %q", got.Proxy.Bind, DefaultProxyBind)
	}

	// 2. 明示値 -> 尊重（loopback へ戻せることの担保）
	contentLoopback := `
[proxy]
enabled = true
port = 8088
bind = "127.0.0.1"
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(contentLoopback), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(); got.Proxy.Bind != "127.0.0.1" {
		t.Errorf("bind 明示: got %q, want 127.0.0.1", got.Proxy.Bind)
	}

	// 3. 空文字・空白のみ -> default へ補正（net.Listen が "" を全 interface と
	//    解釈するのに依存せず、意図した既定値を明示的に入れる）
	contentBlank := `
[proxy]
enabled = true
port = 8088
bind = "   "
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(contentBlank), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(); got.Proxy.Bind != DefaultProxyBind {
		t.Errorf("bind 空白: got %q, want %q", got.Proxy.Bind, DefaultProxyBind)
	}

	// 4. Default() 自体が 0.0.0.0 を返す
	if Default().Proxy.Bind != DefaultProxyBind {
		t.Errorf("Default(): got %q, want %q", Default().Proxy.Bind, DefaultProxyBind)
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
			s := Settings{DevPorts: tt.dp, Proxy: Proxy{Port: DefaultProxyPort}}
			err := s.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
