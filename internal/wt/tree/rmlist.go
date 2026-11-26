package tree

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"wt/internal/wt/core"
)

// RmEntry describes a single worktree row produced by RmEntries.
type RmEntry struct {
	Repo      string
	WtName    string
	WtPath    string
	MainDir   string
	Container string
	TypeStr   string
	Desc      string
}

// RmEntries enumerates removable worktrees across containers (excluding main/master).
func RmEntries() ([]RmEntry, error) {
	var items []RmEntry

	for _, base := range core.WorkspaceDirs() {
		containers, _ := filepath.Glob(filepath.Join(base, "*"))
		sort.Strings(containers)
		for _, container := range containers {
			repo := filepath.Base(container)
			mainDir, _ := core.ResolveMain(container)
			if mainDir == "" {
				continue
			}
			mainName := filepath.Base(mainDir)

			out, err := core.GitOutput(mainDir, "worktree", "list")
			if err != nil {
				continue
			}

			entries, _ := core.LoadEntries(container)

			for _, line := range strings.Split(out, "\n") {
				if line == "" {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) == 0 {
					continue
				}
				wtPath := fields[0]
				wtName := filepath.Base(wtPath)

				if wtName == mainName {
					continue
				}

				typeStr := "unknown"
				desc := ""
				if e, ok := entries[wtName]; ok {
					if e.Type != "" {
						typeStr = e.Type
					}
					desc = e.Description
				}

				items = append(items, RmEntry{
					Repo:      repo,
					WtName:    wtName,
					WtPath:    wtPath,
					MainDir:   mainDir,
					Container: container,
					TypeStr:   typeStr,
					Desc:      desc,
				})
			}
		}
	}

	if len(items) == 0 {
		return nil, errors.New("削除可能なworktreeがありません")
	}
	return items, nil
}
