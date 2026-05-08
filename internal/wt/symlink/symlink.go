// Package symlink edits the _config.symlink_candidates list inside a
// container's .worktrees.json.
package symlink

import (
	"fmt"
	"io"
	"sort"

	"wt/internal/wt/core"
)

// Ls prints the current symlink_candidates with 1-based numbering.
func Ls(out io.Writer, repo string) error {
	container, err := core.FindContainer(repo)
	if err != nil {
		return err
	}
	cfg, err := core.LoadConfig(container)
	if err != nil {
		return err
	}
	for i, p := range cfg.SymlinkCandidates {
		_, _ = fmt.Fprintf(out, "%6d\t%s\n", i+1, p)
	}
	return nil
}

// Add appends a path to the candidates list (deduplicated).
func Add(out io.Writer, repo, path string) error {
	if path == "" {
		return fmt.Errorf("path required")
	}
	container, err := core.FindContainer(repo)
	if err != nil {
		return err
	}
	cfg, err := core.LoadConfig(container)
	if err != nil {
		return err
	}
	for _, p := range cfg.SymlinkCandidates {
		if p == path {
			_, _ = fmt.Fprintf(out, "✅ added: %s\n", path)
			return nil
		}
	}
	cfg.SymlinkCandidates = append(cfg.SymlinkCandidates, path)
	sort.Strings(cfg.SymlinkCandidates)
	if err := core.SaveConfig(container, cfg); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✅ added: %s\n", path)
	return nil
}

// Rm removes a path from the candidates list.
func Rm(out io.Writer, repo, path string) error {
	if path == "" {
		return fmt.Errorf("path required")
	}
	container, err := core.FindContainer(repo)
	if err != nil {
		return err
	}
	cfg, err := core.LoadConfig(container)
	if err != nil {
		return err
	}
	filtered := cfg.SymlinkCandidates[:0]
	for _, p := range cfg.SymlinkCandidates {
		if p != path {
			filtered = append(filtered, p)
		}
	}
	cfg.SymlinkCandidates = filtered
	if err := core.SaveConfig(container, cfg); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✅ removed: %s\n", path)
	return nil
}
