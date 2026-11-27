package ports

import (
	"path/filepath"
	"testing"
	"time"

	"wt/internal/wt/listencache"
	"wt/internal/wt/settings"
)

func TestPortStateUnhealthy(t *testing.T) {
	tests := []struct {
		name string
		st   PortState
		want bool
	}{
		{
			// Listening = reachable, healthy.
			name: "listening",
			st:   PortState{Running: true, Listening: true},
			want: false,
		},
		{
			// Declared headless worker binds no port by design — benign.
			name: "headless worker",
			st:   PortState{Running: true, Listening: false, Headless: true},
			want: false,
		},
		{
			// Expected to LISTEN but alive-without-a-socket = build failed / crashed.
			name: "running but not listening, not headless",
			st:   PortState{Running: true, Listening: false, Headless: false},
			want: true,
		},
		{
			// Not running at all is "stopped", not "unhealthy".
			name: "stopped",
			st:   PortState{Running: false, Listening: false},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.st.Unhealthy(); got != tt.want {
				t.Errorf("Unhealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

// stubScan replaces the socket scan and counts how often it runs.
func stubScan(t *testing.T, found map[int]Listener) *int {
	t.Helper()
	calls := 0
	prev := scanListeners
	scanListeners = func(int, int) (map[int]Listener, bool) {
		calls++
		return found, true
	}
	t.Cleanup(func() { scanListeners = prev })
	return &calls
}

func listening(t *testing.T, rows []Row, wt string) bool {
	t.Helper()
	for _, r := range rows {
		if r.WtName == wt {
			return len(r.Ports) > 0 && r.Ports[0].Listening
		}
	}
	t.Fatalf("worktree %s not in rows", wt)
	return false
}

// #205: a fresh cache is used without scanning; Status always scans and
// replaces the cache; a missing or stale cache makes StatusCached scan.
func TestStatusCached(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	restore := listencache.SetPathForTest(filepath.Join(t.TempDir(), "listeners.json"))
	t.Cleanup(restore)
	stubNoListeners(t)
	writeContainer(t, "repo-a", map[string]int{"wt1": 9000})
	band := settings.Load().DevPorts

	// Cache says 9000 listens; the machine says nothing does.
	if err := listencache.Save(band.Start, band.End, []listencache.Entry{{Port: 9000, PID: 1, Proc: "node"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	calls := stubScan(t, map[int]Listener{})

	rows, err := StatusCached(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if *calls != 0 || !listening(t, rows, "wt1") {
		t.Fatalf("fresh cache: scans=%d listening=%v; want 0, true", *calls, listening(t, rows, "wt1"))
	}

	// Status scans and overwrites the cache with "nothing listens".
	rows, err = Status()
	if err != nil {
		t.Fatal(err)
	}
	if *calls != 1 || listening(t, rows, "wt1") {
		t.Fatalf("Status: scans=%d listening=%v; want 1, false", *calls, listening(t, rows, "wt1"))
	}
	rows, _ = StatusCached(time.Minute)
	if *calls != 1 || listening(t, rows, "wt1") {
		t.Fatalf("after Status: scans=%d listening=%v; want 1, false", *calls, listening(t, rows, "wt1"))
	}

	// A cache older than maxAge, or a missing one, makes StatusCached scan.
	if _, err := StatusCached(0); err != nil {
		t.Fatal(err)
	}
	if err := listencache.Invalidate(); err != nil {
		t.Fatal(err)
	}
	if _, err := StatusCached(time.Hour); err != nil {
		t.Fatal(err)
	}
	if *calls != 3 {
		t.Fatalf("stale + missing cache: scans=%d; want 3", *calls)
	}
}

// #205: a failed scan (neither ss nor lsof ran) is "unknown", not "nothing
// listens": it must drop the cache rather than save an empty result.
func TestRefreshListenerCacheScanFailure(t *testing.T) {
	restore := listencache.SetPathForTest(filepath.Join(t.TempDir(), "listeners.json"))
	t.Cleanup(restore)
	if err := listencache.Save(9000, 9999, []listencache.Entry{{Port: 9000}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	prev := scanListeners
	scanListeners = func(int, int) (map[int]Listener, bool) { return map[int]Listener{}, false }
	t.Cleanup(func() { scanListeners = prev })

	RefreshListenerCache(9000, 9999)

	if got, ok := listencache.Load(9000, 9999, time.Hour, time.Now()); ok {
		t.Fatalf("failed scan left a usable cache: %v", got)
	}
}
