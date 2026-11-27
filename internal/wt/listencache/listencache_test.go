package listencache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func useTempPath(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "wt", "listeners.json")
	old := pathFunc
	pathFunc = func() string { return p }
	t.Cleanup(func() { pathFunc = old })
	return p
}

func TestSaveLoad(t *testing.T) {
	useTempPath(t)
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	want := []Entry{{Port: 9000, PID: 42, Proc: "node"}}
	if err := Save(9000, 9999, want, now); err != nil {
		t.Fatal(err)
	}

	got, ok := Load(9000, 9999, 5*time.Minute, now.Add(time.Minute))
	if !ok || len(got) != 1 || got[0] != want[0] {
		t.Fatalf("Load = %v, %v; want %v, true", got, ok, want)
	}
}

func TestLoadRejects(t *testing.T) {
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		start, end int
		at         time.Time
	}{
		{"too old", 9000, 9999, now.Add(5*time.Minute + time.Second)},
		{"other band", 8000, 8999, now},
		{"taken in the future", 9000, 9999, now.Add(-time.Second)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			useTempPath(t)
			if err := Save(9000, 9999, nil, now); err != nil {
				t.Fatal(err)
			}
			if _, ok := Load(c.start, c.end, 5*time.Minute, c.at); ok {
				t.Fatal("Load accepted the cache")
			}
		})
	}
}

func TestLoadMissingOrBroken(t *testing.T) {
	p := useTempPath(t)
	now := time.Now()
	if _, ok := Load(9000, 9999, time.Hour, now); ok {
		t.Fatal("Load accepted a missing cache")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(9000, 9999, time.Hour, now); ok {
		t.Fatal("Load accepted a broken cache")
	}
}

func TestInvalidate(t *testing.T) {
	p := useTempPath(t)
	if err := Invalidate(); err != nil {
		t.Fatalf("Invalidate on a missing cache: %v", err)
	}
	if err := Save(9000, 9999, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := Invalidate(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("cache still exists: %v", err)
	}
}

// SetPathForTest lets other packages' tests isolate the cache.
func TestSetPathForTest(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	restore := SetPathForTest(p)
	if Path() != p {
		t.Fatalf("Path = %q", Path())
	}
	restore()
	if Path() == p {
		t.Fatal("restore did not reset the path")
	}
}
