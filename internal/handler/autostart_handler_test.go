package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/wt/core"
)

// autoStartRequest issues SetTreeAutoStart directly with path values set,
// mirroring how the router would populate {repo}/{wt}.
func autoStartRequest(repo, wt, body string) *httptest.ResponseRecorder {
	h := New()
	r := httptest.NewRequest(http.MethodPut, "/api/trees/"+repo+"/"+wt+"/autostart", strings.NewReader(body))
	r.SetPathValue("repo", repo)
	r.SetPathValue("wt", wt)
	w := httptest.NewRecorder()
	h.SetTreeAutoStart(w, r)
	return w
}

func TestSetTreeAutoStart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "myrepo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "wt1", &core.Entry{Type: "feature"}); err != nil {
		t.Fatal(err)
	}

	// Enable auto-start.
	if w := autoStartRequest("myrepo", "wt1", `{"auto_start":true}`); w.Code != http.StatusOK {
		t.Fatalf("enable: status %d, body %s", w.Code, w.Body.String())
	}
	entries, _ := core.LoadEntries(container)
	if !entries["wt1"].AutoStart {
		t.Error("entry should have AutoStart after PUT auto_start=true")
	}

	// Disable auto-start (idempotent set/unset).
	if w := autoStartRequest("myrepo", "wt1", `{"auto_start":false}`); w.Code != http.StatusOK {
		t.Fatalf("disable: status %d, body %s", w.Code, w.Body.String())
	}
	entries, _ = core.LoadEntries(container)
	if entries["wt1"].AutoStart {
		t.Error("entry should not have AutoStart after PUT auto_start=false")
	}
}

// TestSetTreeAutoStart_DoesNotTouchPinned verifies pin and auto-start remain
// independent flags: toggling one must not mutate the other.
func TestSetTreeAutoStart_DoesNotTouchPinned(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "myrepo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "wt1", &core.Entry{Type: "feature", Pinned: true}); err != nil {
		t.Fatal(err)
	}

	if w := autoStartRequest("myrepo", "wt1", `{"auto_start":true}`); w.Code != http.StatusOK {
		t.Fatalf("enable: status %d, body %s", w.Code, w.Body.String())
	}
	entries, _ := core.LoadEntries(container)
	if !entries["wt1"].Pinned {
		t.Error("pinned flag must survive an auto_start toggle")
	}
	if !entries["wt1"].AutoStart {
		t.Error("entry should have AutoStart after PUT auto_start=true")
	}
}

func TestSetTreeAutoStart_UnknownRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if w := autoStartRequest("nope", "wt1", `{"auto_start":true}`); w.Code != http.StatusBadRequest {
		t.Errorf("unknown repo: status %d, want 400", w.Code)
	}
}

func TestSetTreeAutoStart_UnknownWorktree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "myrepo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "wt1", &core.Entry{Type: "feature"}); err != nil {
		t.Fatal(err)
	}
	if w := autoStartRequest("myrepo", "ghost", `{"auto_start":true}`); w.Code != http.StatusNotFound {
		t.Errorf("unknown worktree: status %d, want 404", w.Code)
	}
}
