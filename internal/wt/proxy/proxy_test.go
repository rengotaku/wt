package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func TestHostParts(t *testing.T) {
	tests := []struct {
		host, label, repo string
		ok                bool
	}{
		{"issue10.myrepo.wt.localhost", "issue10", "myrepo", true},
		{"main.wt.wt.localhost:8088", "main", "wt", true},
		{"main.my.app.wt.localhost", "main", "my.app", true}, // repo name with dots
		{"main.wt.localhost", "", "", false},                 // missing repo segment
		{"example.com", "", "", false},
	}
	for _, tt := range tests {
		label, repo, ok := hostParts(tt.host)
		if ok != tt.ok || (ok && (label != tt.label || repo != tt.repo)) {
			t.Errorf("hostParts(%q) = (%q,%q,%v), want (%q,%q,%v)", tt.host, label, repo, ok, tt.label, tt.repo, tt.ok)
		}
	}
}

func TestRouteDomain(t *testing.T) {
	r := Route{Label: "issue10", Repo: "myrepo"}
	if got, want := r.Domain(), "issue10.myrepo.wt.localhost"; got != want {
		t.Errorf("Domain() = %q, want %q", got, want)
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
		return []Route{{Label: "issue10", Repo: "myrepo", Port: port}}, nil
	})

	// matching host (label + repo) → proxied to backend
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://issue10.myrepo.wt.localhost/", http.NoBody))
	if rec.Code != http.StatusOK || rec.Body.String() != "hello from backend" {
		t.Errorf("matching host: code=%d body=%q", rec.Code, rec.Body.String())
	}

	// same label but different repo → 502 (no collision across repos)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest("GET", "http://issue10.otherrepo.wt.localhost/", http.NoBody))
	if rec2.Code != http.StatusBadGateway {
		t.Errorf("repo mismatch: code=%d, want 502", rec2.Code)
	}

	// unknown label → 502
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, httptest.NewRequest("GET", "http://nope.myrepo.wt.localhost/", http.NoBody))
	if rec3.Code != http.StatusBadGateway {
		t.Errorf("unknown label: code=%d, want 502", rec3.Code)
	}

	// missing repo segment (old single-level form) → not proxied.
	// Since LAN access landed here too, this now renders the worktree index
	// instead of 404. The guarantee kept from the old assertion is that it must
	// NOT reach a backend.
	rec4 := httptest.NewRecorder()
	h.ServeHTTP(rec4, httptest.NewRequest("GET", "http://issue10.wt.localhost/", http.NoBody))
	if rec4.Body.String() == "hello from backend" {
		t.Errorf("missing repo: must not be proxied to a backend, got %q", rec4.Body.String())
	}

	// non-wt host (e.g. http://<host-ip>:8088/ from a phone) → not proxied,
	// and must not be a dead end: the index lists each worktree's direct URL.
	rec5 := httptest.NewRecorder()
	h.ServeHTTP(rec5, httptest.NewRequest("GET", "http://192.168.10.50:8088/", http.NoBody))
	if rec5.Body.String() == "hello from backend" {
		t.Errorf("non-wt host: must not be proxied to a backend, got %q", rec5.Body.String())
	}
	if rec5.Code != http.StatusOK {
		t.Errorf("non-wt host: code=%d, want 200 (worktree index)", rec5.Code)
	}
	body := rec5.Body.String()
	// The link must point at the address the client actually used, not loopback,
	// or it is unreachable from the very device that asked for it.
	wantLink := "http://192.168.10.50:" + strconv.Itoa(port) + "/"
	if !strings.Contains(body, wantLink) {
		t.Errorf("non-wt host: index missing direct link %q, body=%q", wantLink, body)
	}
	if !strings.Contains(body, "myrepo") || !strings.Contains(body, "issue10") {
		t.Errorf("non-wt host: index missing worktree name, body=%q", body)
	}
}

// TestHandler_LANIndex_NoRoutes covers the empty case: reaching the proxy by IP
// with nothing running must still explain what to do rather than render a bare
// empty list.
func TestHandler_LANIndex_NoRoutes(t *testing.T) {
	h := Handler(func() ([]Route, error) { return nil, nil })

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://192.168.10.50:8088/", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Errorf("code=%d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "wt serve") {
		t.Errorf("empty index should mention how to start a dev server, body=%q", rec.Body.String())
	}
}
