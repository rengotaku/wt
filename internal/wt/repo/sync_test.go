package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSyncSkipList(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, ".config", "wt")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("missing file returns empty set", func(t *testing.T) {
		got := loadSyncSkipList(home)
		if len(got) != 0 {
			t.Fatalf("want empty, got %v", got)
		}
	})

	body := "# comment line\n" +
		"\n" +
		"fundinno-aws\n" +
		"  spaced-name  \n" +
		"# another comment\n" +
		"mutb-fundoor-aws\n"
	if err := os.WriteFile(filepath.Join(cfg, "sync-skip"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("parses entries, trims, skips comments and blanks", func(t *testing.T) {
		got := loadSyncSkipList(home)
		want := []string{"fundinno-aws", "spaced-name", "mutb-fundoor-aws"}
		if len(got) != len(want) {
			t.Fatalf("want %d entries, got %d (%v)", len(want), len(got), got)
		}
		for _, name := range want {
			if _, ok := got[name]; !ok {
				t.Errorf("missing entry %q", name)
			}
		}
	})
}

func TestHasUncommitted(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"空", "", false},
		{"改行だけ", "\n\n", false},
		{"空白だけ", "  \n\t\n", false},
		{"変更あり", " M internal/repo.go\n", true},
		{"追加あり", "A  new.go\n M old.go\n", true},
	}
	for _, c := range cases {
		if got := hasUncommitted(c.in); got != c.want {
			t.Errorf("%s: hasUncommitted(%q) = %v, want %v", c.name, c.in, got, c.want)
		}
	}
}
