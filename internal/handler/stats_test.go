package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCalcLevel(t *testing.T) {
	warn := uint64(2048)
	danger := uint64(4096)

	tests := []struct {
		total uint64
		want  string
	}{
		{0, "ok"},
		{2047, "ok"},
		{2048, "warn"},
		{4095, "warn"},
		{4096, "danger"},
		{5000, "danger"},
	}

	for _, tt := range tests {
		got := calcLevel(tt.total, warn, danger)
		if got != tt.want {
			t.Errorf("calcLevel(%d) = %q, want %q", tt.total, got, tt.want)
		}
	}
}

func TestGetProcessStats(t *testing.T) {
	// 実環境の settings.toml に依存しないよう設定を空ディレクトリへ隔離する
	// （しきい値の既定値 2048/4096 を検証するため）。
	t.Setenv("WT_CONFIG_DIR", t.TempDir())
	h := New(0)

	req := httptest.NewRequest(http.MethodGet, "/api/process-stats", http.NoBody)
	w := httptest.NewRecorder()

	h.GetProcessStats(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want %d", res.StatusCode, http.StatusOK)
	}

	var resp ProcessStatsResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Items == nil {
		t.Error("want items to be [] instead of null")
	}
	if resp.WarnBytes != 2048*1024*1024 {
		t.Errorf("want warn_bytes = 2048MB, got %d", resp.WarnBytes)
	}
	if resp.DangerBytes != 4096*1024*1024 {
		t.Errorf("want danger_bytes = 4096MB, got %d", resp.DangerBytes)
	}
}
