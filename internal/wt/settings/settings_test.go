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
