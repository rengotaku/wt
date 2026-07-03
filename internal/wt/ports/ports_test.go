package ports

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"wt/internal/wt/core"
)

func TestPortsForBase(t *testing.T) {
	tests := []struct {
		name string
		base int
		want []int
	}{
		{name: "unallocated", base: 0, want: nil},
		{name: "band start", base: 9000, want: []int{9000, 9001, 9002, 9003, 9004}},
		{name: "second block", base: 9005, want: []int{9005, 9006, 9007, 9008, 9009}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PortsForBase(tt.base)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("port[%d] = %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestRangeString(t *testing.T) {
	tests := []struct {
		base int
		want string
	}{
		{base: 0, want: ""},
		{base: 9000, want: "9000-9004"},
		{base: 9995, want: "9995-9999"},
	}
	for _, tt := range tests {
		if got := RangeString(tt.base); got != tt.want {
			t.Errorf("RangeString(%d) = %q, want %q", tt.base, got, tt.want)
		}
	}
}

// stubNoListeners makes occupiedPorts ignore the machine's real `ss` output so
// allocation tests are deterministic. Restored automatically after the test.
func stubNoListeners(t *testing.T) {
	t.Helper()
	prev := listenersInBand
	listenersInBand = func(int, int) map[int]Listener { return map[int]Listener{} }
	t.Cleanup(func() { listenersInBand = prev })
}

func TestFreeBase(t *testing.T) {
	tests := []struct {
		name    string
		used    map[int]bool
		want    int
		wantErr bool
	}{
		{name: "empty → first", used: map[int]bool{}, want: 9000},
		{name: "first port taken → second block", used: map[int]bool{9000: true}, want: 9005},
		{name: "mid-block port taken → block skipped", used: map[int]bool{9002: true}, want: 9005},
		{name: "gap reused", used: map[int]bool{9000: true, 9010: true}, want: 9005},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := freeBase(tt.used, 9000, 9999)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got base %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("freeBase = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFreeBase_Full(t *testing.T) {
	const start, end = 9000, 9999
	used := map[int]bool{}
	for base := start; base+BlockSize-1 <= end; base += BlockSize {
		used[base] = true
	}
	if _, err := freeBase(used, start, end); err == nil {
		t.Error("expected error when band is full, got nil")
	}
}

func TestFreeBase_RespectsConfiguredBand(t *testing.T) {
	// A custom narrow band starting at 9500 must allocate from there.
	got, err := freeBase(map[int]bool{}, 9500, 9520)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 9500 {
		t.Errorf("freeBase = %d, want 9500 (band start)", got)
	}
}

// writeContainer creates a temp container with the given worktree→base mapping.
// Each worktree's directory is created on disk so allocations count as live
// (dirExists true). Use removeWtDir to simulate a ghost (deleted) worktree.
func writeContainer(t *testing.T, repo string, bases map[string]int) string {
	t.Helper()
	home := os.Getenv("HOME")
	container := filepath.Join(home, "Workspace", repo)
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, base := range bases {
		if err := os.MkdirAll(filepath.Join(container, name), 0o755); err != nil {
			t.Fatal(err)
		}
		e := core.Entry{Branch: name}
		e.PortBase = base
		if err := core.PutEntry(container, name, &e); err != nil {
			t.Fatal(err)
		}
	}
	return container
}

// removeWtDir deletes a worktree's directory while leaving its registry entry
// intact, reproducing a ghost entry (removed outside `wt tree rm`).
func removeWtDir(t *testing.T, container, name string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(container, name)); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureBase_AllocatesWhenUnset_ReusesWhenSet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubNoListeners(t)
	container := writeContainer(t, "repo-a", map[string]int{"wt1": 0, "wt2": 9005})

	got, err := EnsureBase(container, "wt1")
	if err != nil {
		t.Fatalf("EnsureBase wt1: %v", err)
	}
	if got == 0 || got == 9005 {
		t.Errorf("wt1 base = %d, want a fresh non-zero base ≠ 9005", got)
	}
	entries, _ := core.LoadEntries(container)
	if entries["wt1"].PortBase != got {
		t.Errorf("wt1 base not persisted: %d vs %d", entries["wt1"].PortBase, got)
	}
	if again, _ := EnsureBase(container, "wt1"); again != got {
		t.Errorf("EnsureBase reuse = %d, want %d", again, got)
	}
	if b, _ := EnsureBase(container, "wt2"); b != 9005 {
		t.Errorf("wt2 base = %d, want 9005 (unchanged)", b)
	}
}

func TestAllocate_GlobalUniqueAcrossRepos(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubNoListeners(t)
	writeContainer(t, "repo-a", map[string]int{"wt1": 9000})
	writeContainer(t, "repo-b", map[string]int{"wt2": 9005})

	got, err := Allocate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 9010 {
		t.Errorf("Allocate = %d, want 9010 (lowest free across both repos)", got)
	}
}

func TestAllocate_ReusesReleasedBlock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubNoListeners(t)
	container := writeContainer(t, "repo-a", map[string]int{"wt1": 9000, "wt2": 9005})

	// Releasing wt1 (delete entry) frees 9000 for reuse.
	if err := core.DeleteEntry(container, "wt1"); err != nil {
		t.Fatal(err)
	}
	got, err := Allocate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 9000 {
		t.Errorf("Allocate = %d, want 9000 (released block reused)", got)
	}
}

func TestEnsureAll_AssignsMissingOnly_AndIdempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubNoListeners(t)
	// wt2 already has 9005; wt1 and wt3 are unallocated.
	writeContainer(t, "repo-a", map[string]int{"wt1": 0, "wt2": 9005, "wt3": 0})

	made, err := EnsureAll()
	if err != nil {
		t.Fatalf("EnsureAll: %v", err)
	}
	if len(made) != 2 {
		t.Fatalf("got %d assignments, want 2 (the two unallocated)", len(made))
	}

	entries, _ := core.LoadEntries(filepath.Join(os.Getenv("HOME"), "Workspace", "repo-a"))
	if entries["wt2"].PortBase != 9005 {
		t.Errorf("wt2 base changed to %d, want 9005 (untouched)", entries["wt2"].PortBase)
	}
	bases := map[int]int{}
	for _, name := range []string{"wt1", "wt2", "wt3"} {
		b := entries[name].PortBase
		if b == 0 {
			t.Errorf("%s still unallocated", name)
		}
		bases[b]++
	}
	for b, n := range bases {
		if n > 1 {
			t.Errorf("base %d assigned to %d worktrees (collision)", b, n)
		}
	}

	// Second run is a no-op: everything is now allocated.
	again, err := EnsureAll()
	if err != nil {
		t.Fatalf("EnsureAll (2nd): %v", err)
	}
	if len(again) != 0 {
		t.Errorf("2nd EnsureAll made %d assignments, want 0", len(again))
	}
}

func TestAllocate_SkipsBlockWithLiveListener(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Registry is empty, so the registry-only logic would hand out 9000. But a
	// foreign process is LISTENing on 9002, which falls in the 9000-9004 block,
	// so that whole block must be skipped in favour of 9005.
	prev := listenersInBand
	listenersInBand = func(int, int) map[int]Listener {
		return map[int]Listener{9002: {Port: 9002, PID: 1234, Proc: "foreign"}}
	}
	t.Cleanup(func() { listenersInBand = prev })

	got, err := Allocate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 9005 {
		t.Errorf("Allocate = %d, want 9005 (9000 block occupied by live :9002)", got)
	}
}

func TestAllocate_IgnoresGhostEntryBlock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubNoListeners(t)
	// wt1 owns 9000 but its directory is gone (ghost); wt2 owns 9005 and is live.
	container := writeContainer(t, "repo-a", map[string]int{"wt1": 9000, "wt2": 9005})
	removeWtDir(t, container, "wt1")

	got, err := Allocate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 9000 {
		t.Errorf("Allocate = %d, want 9000 (ghost wt1's block reclaimed)", got)
	}
}

func TestAllocate_FullOfGhosts_StillAllocates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubNoListeners(t)
	// Fill the whole band with ghost entries: without reclaiming, this would
	// exhaust 9000-9999 and fail — the real bug this fixes.
	bases := map[string]int{}
	for i, base := 0, 9000; base+BlockSize-1 <= 9999; i, base = i+1, base+BlockSize {
		bases[fmt.Sprintf("ghost%d", i)] = base
	}
	container := writeContainer(t, "repo-a", bases)
	for name := range bases {
		removeWtDir(t, container, name)
	}

	got, err := Allocate()
	if err != nil {
		t.Fatalf("expected allocation to succeed once ghosts are ignored, got: %v", err)
	}
	if got != 9000 {
		t.Errorf("Allocate = %d, want 9000 (all blocks were ghosts)", got)
	}
}

func TestStale_ReportsGhostPortHoldersOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	container := writeContainer(t, "repo-a", map[string]int{"live": 9000, "ghost": 9005, "noport": 0})
	removeWtDir(t, container, "ghost")
	removeWtDir(t, container, "noport") // dir gone but no port → not a reclaim target

	stale, err := Stale()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("got %d stale, want 1 (only the port-holding ghost)", len(stale))
	}
	if stale[0].WtName != "ghost" || stale[0].PortBase != 9005 {
		t.Errorf("stale[0] = %+v, want ghost/9005", stale[0])
	}
}

func TestPrune_DryRunKeepsEntries_YesDeletes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubNoListeners(t)
	container := writeContainer(t, "repo-a", map[string]int{"live": 9000, "ghost": 9005})
	removeWtDir(t, container, "ghost")

	// Dry run: report the ghost but leave the registry untouched.
	preview, err := Prune(true)
	if err != nil {
		t.Fatalf("Prune(dry): %v", err)
	}
	if len(preview) != 1 || preview[0].WtName != "ghost" {
		t.Fatalf("dry-run preview = %+v, want [ghost]", preview)
	}
	if entries, _ := core.LoadEntries(container); len(entries) != 2 {
		t.Errorf("dry-run mutated registry: %d entries, want 2", len(entries))
	}

	// Real prune: the ghost entry is removed, live entry stays.
	removed, err := Prune(false)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(removed) != 1 || removed[0].WtName != "ghost" {
		t.Fatalf("removed = %+v, want [ghost]", removed)
	}
	entries, _ := core.LoadEntries(container)
	if _, ok := entries["ghost"]; ok {
		t.Error("ghost entry still present after prune")
	}
	if _, ok := entries["live"]; !ok {
		t.Error("live entry was wrongly pruned")
	}
}

func TestAllocations_SortedAndCarriesBase(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeContainer(t, "zrepo", map[string]int{"wtz": 9005})
	writeContainer(t, "arepo", map[string]int{"wta": 9000})

	allocs, err := Allocations()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(allocs) != 2 {
		t.Fatalf("got %d allocations, want 2", len(allocs))
	}
	if allocs[0].Repo != "arepo" || allocs[1].Repo != "zrepo" {
		t.Errorf("not sorted by repo: %+v", allocs)
	}
	if allocs[0].PortBase != 9000 {
		t.Errorf("arepo base = %d, want 9000", allocs[0].PortBase)
	}
}
