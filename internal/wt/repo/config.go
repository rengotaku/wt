package repo

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"wt/internal/wt/core"
)

// SetBranchPattern stores the branch_pattern of a container. An empty pattern
// removes the constraint.
func SetBranchPattern(container, pattern string) error {
	if err := core.ValidateBranchPattern(pattern); err != nil {
		return err
	}
	cfg, err := core.LoadConfig(container)
	if err != nil {
		return err
	}
	cfg.BranchPattern = pattern
	return core.SaveConfig(container, &cfg)
}

// ValidateSymlinkPath rejects candidates that would link outside the worktree:
// symlink candidates are paths inside the main worktree.
func ValidateSymlinkPath(p string) error {
	if strings.TrimSpace(p) == "" {
		return fmt.Errorf("symlink 候補のパスが空です")
	}
	if filepath.IsAbs(p) {
		return fmt.Errorf("symlink 候補は worktree からの相対パスで指定してください: %s", p)
	}
	for _, part := range strings.Split(filepath.ToSlash(p), "/") {
		if part == ".." {
			return fmt.Errorf("symlink 候補に .. は使えません: %s", p)
		}
	}
	return nil
}

// AddSymlink adds a symlink candidate. Adding an existing one is a no-op.
func AddSymlink(container, p string) error {
	if err := ValidateSymlinkPath(p); err != nil {
		return err
	}
	cfg, err := core.LoadConfig(container)
	if err != nil {
		return err
	}
	if slices.Contains(cfg.SymlinkCandidates, p) {
		return nil
	}
	cfg.SymlinkCandidates = append(slices.Clone(cfg.SymlinkCandidates), p)
	sort.Strings(cfg.SymlinkCandidates)
	return core.SaveConfig(container, &cfg)
}

// RmSymlink removes a symlink candidate and reports whether it existed.
func RmSymlink(container, p string) (bool, error) {
	cfg, err := core.LoadConfig(container)
	if err != nil {
		return false, err
	}
	kept := make([]string, 0, len(cfg.SymlinkCandidates))
	for _, c := range cfg.SymlinkCandidates {
		if c != p {
			kept = append(kept, c)
		}
	}
	if len(kept) == len(cfg.SymlinkCandidates) {
		return false, nil
	}
	cfg.SymlinkCandidates = kept
	return true, core.SaveConfig(container, &cfg)
}

// ClearSymlinks removes every symlink candidate.
func ClearSymlinks(container string) error {
	cfg, err := core.LoadConfig(container)
	if err != nil {
		return err
	}
	cfg.SymlinkCandidates = []string{}
	return core.SaveConfig(container, &cfg)
}
