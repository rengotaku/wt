package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/wt/core"
)

func mockGitForRepo(t *testing.T, output string) {
	dir := t.TempDir()
	gitScript := filepath.Join(dir, "git")
	script := `#!/bin/sh
for arg in "$@"; do
	if [ "$arg" = "status" ]; then
		exit 0
	fi
	if [ "$arg" = "rev-list" ]; then
		echo "0"
		exit 0
	fi
done
echo "` + output + `"
exit 0
`
	err := os.WriteFile(gitScript, []byte(script), 0o755)
	if err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+oldPath)
}

func TestSyncRepo_RestartLogic(t *testing.T) {
	tests := []struct {
		name          string
		gitOutput     string
		expectRestart bool
	}{
		{"Already up to date", "Already up to date", false},
		{"Pulled commits", "Updating 1234567..890abcd\nFast-forward", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			container := filepath.Join(home, "Workspace", "myrepo")
			if err := os.MkdirAll(filepath.Join(container, "main", ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := core.PutEntry(container, "main", &core.Entry{Type: "main"}); err != nil {
				t.Fatal(err)
			}

			mockGitForRepo(t, tt.gitOutput)

			calledRestart := false
			oldRestart := restartDevIfRunning
			restartDevIfRunning = func(c, w, wt string) (bool, error) {
				calledRestart = true
				return true, nil
			}
			t.Cleanup(func() { restartDevIfRunning = oldRestart })

			h := New(0)
			r := httptest.NewRequest(http.MethodPost, "/api/repos/myrepo/sync", strings.NewReader(`{"name":"myrepo"}`))
			w := httptest.NewRecorder()
			h.SyncRepo(w, r)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d. body: %s", w.Code, w.Body.String())
			}
			if calledRestart != tt.expectRestart {
				t.Errorf("expected restart called = %v, got %v", tt.expectRestart, calledRestart)
			}
			var resp map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			if restarted, ok := resp["restarted"].(bool); !ok || restarted != tt.expectRestart {
				t.Errorf("expected restarted JSON field = %v, got %v", tt.expectRestart, resp["restarted"])
			}
		})
	}
}

func TestListRepos_VisibleBeforeHidden(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// name 昇順だと z, hidden-a, visible-b の順になるが、
	// 表示中を非表示より上位にしたいので visible-b, z, hidden-a を期待する。
	repos := []struct {
		name   string
		hidden bool
	}{
		{"z-visible", false},
		{"hidden-a", true},
		{"visible-b", false},
	}
	for _, r := range repos {
		container := filepath.Join(home, "Workspace", r.name)
		if err := os.MkdirAll(container, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := core.PutEntry(container, "main", &core.Entry{Type: "main"}); err != nil {
			t.Fatal(err)
		}
		if r.hidden {
			if err := core.SaveConfig(container, core.EntryConfig{Hidden: true}); err != nil {
				t.Fatal(err)
			}
		}
	}

	h := New(0)
	req := httptest.NewRequest(http.MethodGet, "/api/repos", http.NoBody)
	w := httptest.NewRecorder()
	h.ListRepos(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. body: %s", w.Code, w.Body.String())
	}
	var items []repoItem
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	gotNames := make([]string, len(items))
	for i, it := range items {
		gotNames[i] = it.Name
	}
	wantNames := []string{"visible-b", "z-visible", "hidden-a"}
	if strings.Join(gotNames, ",") != strings.Join(wantNames, ",") {
		t.Errorf("expected order %v, got %v", wantNames, gotNames)
	}
}

func TestSetRepoHidden(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "myrepo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "main", &core.Entry{Type: "main"}); err != nil {
		t.Fatal(err)
	}

	h := New(0)

	// Hide the repo
	reqBody := `{"hidden":true}`
	r := httptest.NewRequest(http.MethodPut, "/api/repos/myrepo/hidden", strings.NewReader(reqBody))
	r.SetPathValue("name", "myrepo")
	w := httptest.NewRecorder()
	h.SetRepoHidden(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if hidden, ok := resp["hidden"].(bool); !ok || !hidden {
		t.Errorf("expected hidden=true in response, got %v", resp["hidden"])
	}

	// Verify the config is updated
	cfg, err := core.LoadConfig(container)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Hidden {
		t.Errorf("expected repo to be hidden in _config, but it was not")
	}

	// Unhide the repo
	reqBody2 := `{"hidden":false}`
	r2 := httptest.NewRequest(http.MethodPut, "/api/repos/myrepo/hidden", strings.NewReader(reqBody2))
	r2.SetPathValue("name", "myrepo")
	w2 := httptest.NewRecorder()
	h.SetRepoHidden(w2, r2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
	cfg2, _ := core.LoadConfig(container)
	if cfg2.Hidden {
		t.Errorf("expected repo to be visible in _config, but it was hidden")
	}
}
