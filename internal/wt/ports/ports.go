// Package ports manages per-worktree dev port-block allocation within the
// machine-wide dev band (9000-9999). Each worktree owns a block of BlockSize
// consecutive ports; the base port is persisted in .worktrees.json so blocks
// stay stable across processes. Allocation is global across every wt-managed
// container so two worktrees never collide.
package ports

import (
	"fmt"
	"path/filepath"
	"sort"

	"wt/internal/wt/core"
)

const (
	// BandStart is the first port of the dev band.
	BandStart = 9000
	// BandEnd is the last port of the dev band (inclusive).
	BandEnd = 9999
	// BlockSize is how many consecutive ports one worktree owns.
	BlockSize = 5
)

// BlockCount is the number of allocatable blocks in the band.
const BlockCount = (BandEnd - BandStart + 1) / BlockSize

// Allocation is a worktree's port assignment.
type Allocation struct {
	Repo     string
	WtName   string
	Branch   string
	PortBase int // 0 when unallocated
}

// PortsForBase returns the BlockSize ports owned by a base port.
func PortsForBase(base int) []int {
	if base == 0 {
		return nil
	}
	out := make([]int, 0, BlockSize)
	for i := 0; i < BlockSize; i++ {
		out = append(out, base+i)
	}
	return out
}

// RangeString renders a base as a "9000-9004" range, or "" when unallocated.
func RangeString(base int) string {
	if base == 0 {
		return ""
	}
	return fmt.Sprintf("%d-%d", base, base+BlockSize-1)
}

// freeBase returns the lowest base in the band not present in used.
func freeBase(used map[int]bool) (int, error) {
	for base := BandStart; base+BlockSize-1 <= BandEnd; base += BlockSize {
		if !used[base] {
			return base, nil
		}
	}
	return 0, fmt.Errorf("ポート帯 %d-%d が満杯です（%d ブロック使用中）", BandStart, BandEnd, len(used))
}

// Allocations scans every wt-managed container and returns one Allocation per
// worktree entry, sorted by repo then worktree name.
func Allocations() ([]Allocation, error) {
	var out []Allocation
	for _, container := range core.ListContainers() {
		repo := filepath.Base(container)
		entries, err := core.LoadEntries(container)
		if err != nil {
			continue
		}
		for name, e := range entries {
			out = append(out, Allocation{
				Repo:     repo,
				WtName:   name,
				Branch:   e.Branch,
				PortBase: e.PortBase,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		return out[i].WtName < out[j].WtName
	})
	return out, nil
}

// usedBases collects every allocated base across all containers.
func usedBases() (map[int]bool, error) {
	allocs, err := Allocations()
	if err != nil {
		return nil, err
	}
	used := make(map[int]bool, len(allocs))
	for _, a := range allocs {
		if a.PortBase != 0 {
			used[a.PortBase] = true
		}
	}
	return used, nil
}

// Allocate returns the lowest free base port in the dev band. It does not
// persist anything; the caller stores the returned base on the worktree entry.
// Freeing is implicit: deleting a worktree entry releases its base for reuse.
func Allocate() (int, error) {
	used, err := usedBases()
	if err != nil {
		return 0, err
	}
	return freeBase(used)
}
