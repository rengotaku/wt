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

func mockGitForTree(t *testing.T, output string) {
	dir := t.TempDir()
	gitScript := filepath.Join(dir, "git")
	script := `#!/bin/sh
for arg in "$@"; do
	if [ "$arg" = "status" ]; then
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

func TestUpdateTree_RestartLogic(t *testing.T) {
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
			if err := os.MkdirAll(container, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := core.PutEntry(container, "wt1", &core.Entry{Type: "feature"}); err != nil {
				t.Fatal(err)
			}

			mockGitForTree(t, tt.gitOutput)

			calledRestart := false
			oldRestart := restartDevIfRunning
			restartDevIfRunning = func(c, w, wt string) (bool, error) {
				calledRestart = true
				return true, nil
			}
			t.Cleanup(func() { restartDevIfRunning = oldRestart })

			h := New(0)
			r := httptest.NewRequest(http.MethodPost, "/api/trees/myrepo/wt1/update", http.NoBody)
			r.SetPathValue("repo", "myrepo")
			r.SetPathValue("wt", "wt1")
			w := httptest.NewRecorder()
			h.UpdateTree(w, r)

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

// GC のフィルタ未指定は修正可能なクライアントエラーであり、サーバ障害
// （500）ではない。older_than のフォーマットエラーと同じ 400 で返す。
func TestGcTrees_NoFilterReturns400(t *testing.T) {
	h := New(0)
	body := strings.NewReader(`{"done":false,"older_than":"","dry_run":true,"yes":false,"force":false}`)
	r := httptest.NewRequest(http.MethodPost, "/api/trees/gc", body)
	w := httptest.NewRecorder()
	h.GcTrees(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d. body: %s", w.Code, w.Body.String())
	}
}

func TestListTrees_HiddenRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	containerVisible := filepath.Join(home, "Workspace", "visible-repo")
	containerHidden := filepath.Join(home, "Workspace", "hidden-repo")

	for _, c := range []string{containerVisible, containerHidden} {
		if err := os.MkdirAll(filepath.Join(c, "wt1"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := core.PutEntry(c, "wt1", &core.Entry{Type: "feature"}); err != nil {
			t.Fatal(err)
		}
	}

	// Hide the hidden-repo
	cfg := core.EntryConfig{Hidden: true}
	if err := core.SaveConfig(containerHidden, cfg); err != nil {
		t.Fatal(err)
	}

	h := New(0)
	r := httptest.NewRequest(http.MethodGet, "/api/trees", http.NoBody)
	w := httptest.NewRecorder()
	h.ListTrees(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var items []treeItem
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}

	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
	if len(items) > 0 && items[0].Repo != "visible-repo" {
		t.Errorf("expected visible-repo, got %s", items[0].Repo)
	}
}
