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

// pinRequest issues SetTreePin directly with path values set, mirroring how the
// router would populate {repo}/{wt}.
func pinRequest(repo, wt, body string) *httptest.ResponseRecorder {
	h := New(0)
	r := httptest.NewRequest(http.MethodPut, "/api/trees/"+repo+"/"+wt+"/pin", strings.NewReader(body))
	r.SetPathValue("repo", repo)
	r.SetPathValue("wt", wt)
	w := httptest.NewRecorder()
	h.SetTreePin(w, r)
	return w
}

func TestSetTreePin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "myrepo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "wt1", &core.Entry{Type: "feature"}); err != nil {
		t.Fatal(err)
	}

	// Pin it.
	if w := pinRequest("myrepo", "wt1", `{"pinned":true}`); w.Code != http.StatusOK {
		t.Fatalf("pin: status %d, body %s", w.Code, w.Body.String())
	}
	entries, _ := core.LoadEntries(container)
	if !entries["wt1"].Pinned {
		t.Error("entry should be pinned after PUT pinned=true")
	}

	// Unpin it.
	if w := pinRequest("myrepo", "wt1", `{"pinned":false}`); w.Code != http.StatusOK {
		t.Fatalf("unpin: status %d, body %s", w.Code, w.Body.String())
	}
	entries, _ = core.LoadEntries(container)
	if entries["wt1"].Pinned {
		t.Error("entry should be unpinned after PUT pinned=false")
	}
}

func TestSetTreePin_UnknownRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if w := pinRequest("nope", "wt1", `{"pinned":true}`); w.Code != http.StatusBadRequest {
		t.Errorf("unknown repo: status %d, want 400", w.Code)
	}
}

// TestSetTreePin_DoesNotTouchAutoStart is the symmetric counterpart of
// TestSetTreeAutoStart_DoesNotTouchPinned: toggling pinned must not mutate
// AutoStart, since the two flags are independent.
func TestSetTreePin_DoesNotTouchAutoStart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "myrepo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "wt1", &core.Entry{Type: "feature", AutoStart: true}); err != nil {
		t.Fatal(err)
	}

	if w := pinRequest("myrepo", "wt1", `{"pinned":true}`); w.Code != http.StatusOK {
		t.Fatalf("pin: status %d, body %s", w.Code, w.Body.String())
	}
	entries, _ := core.LoadEntries(container)
	if !entries["wt1"].AutoStart {
		t.Error("AutoStart flag must survive a pinned toggle")
	}
	if !entries["wt1"].Pinned {
		t.Error("entry should be pinned after PUT pinned=true")
	}
}

func TestSetTreePin_UnknownWorktree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "myrepo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "wt1", &core.Entry{Type: "feature"}); err != nil {
		t.Fatal(err)
	}
	if w := pinRequest("myrepo", "ghost", `{"pinned":true}`); w.Code != http.StatusNotFound {
		t.Errorf("unknown worktree: status %d, want 404", w.Code)
	}
}
