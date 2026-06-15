// Package ports manages per-worktree dev port-block allocation within the
// machine-wide dev band (configurable via package settings; default 9000-9999).
// Each worktree owns a block of BlockSize consecutive ports; the base port is
// persisted in .worktrees.json so blocks stay stable across processes.
// Allocation is global across every wt-managed container so two worktrees never
// collide.
package ports

import (
	"fmt"
	"path/filepath"
	"sort"

	"wt/internal/wt/core"
	"wt/internal/wt/settings"
)

// BlockSize is how many consecutive ports one worktree owns. The band itself
// (start/end) is user-configurable via package settings.
const BlockSize = 5

// Allocation is a worktree's port assignment.
type Allocation struct {
	Repo     string
	WtName   string
	Branch   string
	Path     string // absolute worktree path
	PortBase int    // 0 when unallocated
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

// blockFree reports whether none of the BlockSize ports owned by base appear in
// the occupied set (occupied is keyed by individual port, not by base).
func blockFree(occupied map[int]bool, base int) bool {
	for _, p := range PortsForBase(base) {
		if occupied[p] {
			return false
		}
	}
	return true
}

// freeBase returns the lowest block base within [start, end] whose whole port
// range is free of occupied ports.
func freeBase(occupied map[int]bool, start, end int) (int, error) {
	for base := start; base+BlockSize-1 <= end; base += BlockSize {
		if blockFree(occupied, base) {
			return base, nil
		}
	}
	return 0, fmt.Errorf("ポート帯 %d-%d に空きブロックがありません", start, end)
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
		for name := range entries {
			e := entries[name]
			out = append(out, Allocation{
				Repo:     repo,
				WtName:   name,
				Branch:   e.Branch,
				Path:     filepath.Join(container, name),
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

// listenersInBand reports the ports currently being LISTENed on within
// [start, end]. It is a package var so tests can stub the live scan. The
// default shells out to `ss`; if ss is unavailable it returns no listeners.
var listenersInBand = func(start, end int) map[int]Listener {
	m, _ := Listeners(start, end)
	return m
}

// occupiedPorts unions two sources of "this port is taken": every port of every
// block already reserved in a .worktrees.json registry, and every port a process
// is currently LISTENing on inside the band. Consulting live listeners (not just
// the registry) prevents handing a new worktree a block that overlaps a foreign
// process or a server that is up but not yet recorded.
func occupiedPorts(start, end int) (map[int]bool, error) {
	allocs, err := Allocations()
	if err != nil {
		return nil, err
	}
	occ := map[int]bool{}
	for _, a := range allocs {
		for _, p := range PortsForBase(a.PortBase) {
			occ[p] = true
		}
	}
	for p := range listenersInBand(start, end) {
		occ[p] = true
	}
	return occ, nil
}

// Allocate returns the lowest free base port within the configured dev band.
// It does not persist anything; the caller stores the returned base on the
// worktree entry. Freeing is implicit: deleting a worktree entry releases its
// base for reuse (unless a live process keeps the block occupied).
func Allocate() (int, error) {
	band := settings.Load().DevPorts
	occupied, err := occupiedPorts(band.Start, band.End)
	if err != nil {
		return 0, err
	}
	return freeBase(occupied, band.Start, band.End)
}

// Assignment records a port base newly allocated to a worktree, for reporting.
type Assignment struct {
	Repo     string
	WtName   string
	PortBase int
}

// EnsureAll allocates and persists a port base for every registered worktree
// that currently has none (base 0), across all containers. Worktrees that
// already own a base are left untouched. It returns the assignments it made.
//
// This is how existing worktrees — created before allocation existed, or before
// they were ever served — get ports without having to `wt serve` each one.
func EnsureAll() ([]Assignment, error) {
	var made []Assignment
	for _, container := range core.ListContainers() {
		entries, err := core.LoadEntries(container)
		if err != nil {
			continue
		}
		// Deterministic order so repeated runs assign the same blocks.
		names := make([]string, 0, len(entries))
		for name := range entries {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			e := entries[name]
			if e.PortBase != 0 {
				continue
			}
			// Allocate re-scans the registry each call, so a base persisted in a
			// previous iteration is already counted as occupied here.
			base, err := Allocate()
			if err != nil {
				return made, err
			}
			e.PortBase = base
			if err := core.PutEntry(container, name, &e); err != nil {
				return made, err
			}
			made = append(made, Assignment{Repo: filepath.Base(container), WtName: name, PortBase: base})
		}
	}
	return made, nil
}

// EnsureBase returns the worktree's allocated port base, allocating and
// persisting one to .worktrees.json when it currently has none (base 0). This
// lets `wt serve` work on worktrees created before allocation existed.
func EnsureBase(container, wtName string) (int, error) {
	entries, err := core.LoadEntries(container)
	if err != nil {
		return 0, err
	}
	e, ok := entries[wtName]
	if !ok {
		return 0, fmt.Errorf("worktree がレジストリに見つかりません: %s", wtName)
	}
	if e.PortBase != 0 {
		return e.PortBase, nil
	}
	base, err := Allocate()
	if err != nil {
		return 0, err
	}
	e.PortBase = base
	if err := core.PutEntry(container, wtName, &e); err != nil {
		return 0, err
	}
	return base, nil
}
