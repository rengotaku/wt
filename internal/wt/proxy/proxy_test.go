package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"wt/internal/wt/core"
)

func TestLabel(t *testing.T) {
	tests := []struct {
		branch, wt, want string
	}{
		{"main", "repo", "main"},
		{"master", "repo--x", "main"},
		{"feat/issue-10-abc", "repo--feat-issue-10-abc", "issue10"},
		{"Issue-7-copy", "repo--issue7", "issue7"},
		{"bot/issue_42_x", "repo--bot", "issue42"},
		{"feature/login-form", "repo--login", "feature-login-form"},
	}
	for _, tt := range tests {
		if got := Label(tt.branch, tt.wt); got != tt.want {
			t.Errorf("Label(%q,%q) = %q, want %q", tt.branch, tt.wt, got, tt.want)
		}
	}
}

func TestLabelFromHost(t *testing.T) {
	tests := []struct {
		host, want string
		ok         bool
	}{
		{"issue10.wt.localhost", "issue10", true},
		{"main.wt.localhost:8088", "main", true},
		{"example.com", "", false},
	}
	for _, tt := range tests {
		got, ok := labelFromHost(tt.host)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("labelFromHost(%q) = (%q,%v), want (%q,%v)", tt.host, got, ok, tt.want, tt.ok)
		}
	}
}

func TestRoutes_FromMetadataDefault(t *testing.T) {
	// A repo whose domain service lives only in metadata (_config.dev_services),
	// with no committed .wt/dev.toml, must still produce a proxy route.
	home := t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "myrepo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "main", &core.Entry{
		Type: "main", Branch: "main", PortBase: 9000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := core.SaveConfig(container, core.EntryConfig{
		DevServices: []core.DevService{
			{Name: "api", Cmd: "run-api"},
			{Name: "web", Cmd: "run-web", Domain: true},
		},
	}); err != nil {
		t.Fatal(err)
	}

	routes, err := Routes()
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}
	var found *Route
	for i := range routes {
		if routes[i].Repo == "myrepo" {
			found = &routes[i]
		}
	}
	if found == nil {
		t.Fatalf("no route for myrepo; routes=%+v", routes)
	}
	// web is the domain service at declaration index 1 → port base+1 = 9001.
	if found.Label != "main" || found.Port != 9001 {
		t.Errorf("route = {label:%q port:%d}, want {main 9001}", found.Label, found.Port)
	}
}

func TestHandler_RoutesByHost(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()
	u, _ := url.Parse(backend.URL)
	port, _ := strconv.Atoi(u.Port())

	h := Handler(func() ([]Route, error) {
		return []Route{{Label: "issue10", Port: port}}, nil
	})

	// matching host → proxied to backend
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://issue10.wt.localhost/", http.NoBody))
	if rec.Code != http.StatusOK || rec.Body.String() != "hello from backend" {
		t.Errorf("matching host: code=%d body=%q", rec.Code, rec.Body.String())
	}

	// unknown label → 502
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest("GET", "http://nope.wt.localhost/", http.NoBody))
	if rec2.Code != http.StatusBadGateway {
		t.Errorf("unknown label: code=%d, want 502", rec2.Code)
	}

	// non-wt host → 404
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, httptest.NewRequest("GET", "http://example.com/", http.NoBody))
	if rec3.Code != http.StatusNotFound {
		t.Errorf("non-wt host: code=%d, want 404", rec3.Code)
	}
}
