package doctor

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"wt/internal/wt/core"
	"wt/internal/wt/ports"
)

// setup makes <home>/Workspace/app with a main folder (a .git marker is enough
// for ResolveMain) and returns the container.
func setup(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	container := filepath.Join(home, "Workspace", "app")
	if err := os.MkdirAll(filepath.Join(container, "main", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// No ports on main: that is normal and must not be reported, also for an
	// old entry without type "main".
	if err := core.PutEntry(container, "main", &core.Entry{Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	return container
}

type fake struct {
	branch     string
	rescueErr  error
	rescued    []string
	stale      []ports.Allocation
	deleted    []string
	ensured    []string
	ensureBase int
}

func (f *fake) deps() *deps {
	return &deps{
		currentBranch: func(string) (string, error) { return f.branch, nil },
		rescueMain: func(_ io.Writer, repo string) error {
			f.rescued = append(f.rescued, repo)
			return f.rescueErr
		},
		stale:       func() ([]ports.Allocation, error) { return f.stale, nil },
		deleteEntry: func(_, name string) error { f.deleted = append(f.deleted, name); return nil },
		ensureBase: func(_, name string) (int, error) {
			f.ensured = append(f.ensured, name)
			return f.ensureBase, nil
		},
	}
}

func checks(fs []Finding) map[string]Finding {
	m := map[string]Finding{}
	for _, f := range fs {
		m[f.Check+"/"+f.Worktree] = f
	}
	return m
}

func TestClean(t *testing.T) {
	container := setup(t)
	f := &fake{branch: "main"}
	got, err := run(f.deps(), io.Discard, []string{container}, true)
	if err != nil || len(got) != 0 {
		t.Fatalf("clean repo: got %+v, %v", got, err)
	}
}

func TestMainBranch(t *testing.T) {
	container := setup(t)

	f := &fake{branch: "feat/x"}
	got, _ := run(f.deps(), io.Discard, []string{container}, false)
	if c := checks(got)["main_branch/main"]; c.Status != StatusFound || len(f.rescued) != 0 {
		t.Errorf("without --fix: %+v, rescued %v", c, f.rescued)
	}

	got, _ = run(f.deps(), io.Discard, []string{container}, true)
	if c := checks(got)["main_branch/main"]; c.Status != StatusFixed || len(f.rescued) != 1 {
		t.Errorf("with --fix: %+v, rescued %v", c, f.rescued)
	}

	f = &fake{branch: "feat/x", rescueErr: errors.New("未コミット変更あり")}
	got, _ = run(f.deps(), io.Discard, []string{container}, true)
	if c := checks(got)["main_branch/main"]; c.Status != StatusFixFailed || !c.Unresolved() {
		t.Errorf("failed fix: %+v", c)
	}
}

func TestStalePort_OnlyThisContainer(t *testing.T) {
	container := setup(t)
	other := filepath.Join(filepath.Dir(container), "other")
	if err := core.PutEntry(container, "app--gone", &core.Entry{Branch: "g", PortBase: 9100}); err != nil {
		t.Fatal(err)
	}
	f := &fake{branch: "main", stale: []ports.Allocation{
		{Repo: "app", WtName: "app--gone", Path: filepath.Join(container, "app--gone"), PortBase: 9100},
		{Repo: "other", WtName: "other--gone", Path: filepath.Join(other, "other--gone"), PortBase: 9200},
	}}
	got, _ := run(f.deps(), io.Discard, []string{container}, true)
	if c := checks(got)["stale_port/app--gone"]; c.Status != StatusFixed {
		t.Errorf("stale entry: %+v", c)
	}
	if len(f.deleted) != 1 || f.deleted[0] != "app--gone" {
		t.Errorf("deleted %v, want only app--gone", f.deleted)
	}
}

func TestUnallocated_SkipsMissingDirs(t *testing.T) {
	container := setup(t)
	if err := os.MkdirAll(filepath.Join(container, "app--here"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"app--here", "app--gone"} {
		if err := core.PutEntry(container, n, &core.Entry{Branch: n}); err != nil {
			t.Fatal(err)
		}
	}
	f := &fake{branch: "main", ensureBase: 9300}
	got, _ := run(f.deps(), io.Discard, []string{container}, true)
	if c := checks(got)["unallocated_port/app--here"]; c.Status != StatusFixed {
		t.Errorf("app--here: %+v", c)
	}
	if _, ok := checks(got)["unallocated_port/app--gone"]; ok || len(f.ensured) != 1 {
		t.Errorf("must not allocate to a missing directory: ensured %v", f.ensured)
	}
}

func TestServiceConfig(t *testing.T) {
	container := setup(t)
	cfg := core.EntryConfig{DevServices: []core.DevService{{Name: "api", Cmd: ""}}}
	if err := core.SaveConfig(container, &cfg); err != nil {
		t.Fatal(err)
	}
	f := &fake{branch: "main"}
	got, _ := run(f.deps(), io.Discard, []string{container}, true)
	if c := checks(got)["service_config/"]; c.Status != StatusManual {
		t.Errorf("broken service: %+v", c)
	}
}

func TestMetadataUnreadable(t *testing.T) {
	container := setup(t)
	if err := os.WriteFile(core.MetaFile(container), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &fake{branch: "main"}
	got, _ := run(f.deps(), io.Discard, []string{container}, true)
	if c := checks(got)["metadata/"]; c.Status != StatusManual {
		t.Errorf("unreadable metadata: %+v", got)
	}
}

// An entry that stopped being stale after the list was taken is left alone.
func TestStalePort_RecheckBeforeDelete(t *testing.T) {
	container := setup(t)
	path := filepath.Join(container, "app--back")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := core.PutEntry(container, "app--back", &core.Entry{Branch: "b", PortBase: 9100}); err != nil {
		t.Fatal(err)
	}
	f := &fake{branch: "main", stale: []ports.Allocation{{Repo: "app", WtName: "app--back", Path: path, PortBase: 9100}}}
	if _, err := run(f.deps(), io.Discard, []string{container}, true); err != nil {
		t.Fatal(err)
	}
	if len(f.deleted) != 0 {
		t.Errorf("deleted %v although the directory exists again", f.deleted)
	}
}
