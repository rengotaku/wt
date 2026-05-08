package tree

import (
	"errors"
	"fmt"
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

// Display returns the human-readable name shown in pickers.
func (r *RmEntry) Display() string {
	return r.Repo + "/" + r.WtName
}

// Info returns the right-hand label (type / description).
func (r *RmEntry) Info() string {
	out := "[" + r.TypeStr + "]"
	if r.Desc != "" {
		out += " " + r.Desc
	}
	return out
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

// RmList prints removable worktrees in the legacy format used by wt-tree-rm.sh.
// Each line: "%-*s <info>|<wtPath>|<mainDir>|<wtName>|<container>".
func RmList() error {
	items, err := RmEntries()
	if err != nil {
		return err
	}

	maxWidth := 0
	for i := range items {
		if n := len(items[i].Display()); n > maxWidth {
			maxWidth = n
		}
	}
	maxWidth += 2
	for i := range items {
		it := &items[i]
		fmt.Printf("%-*s %s|%s|%s|%s|%s\n", maxWidth, it.Display(), it.Info(), it.WtPath, it.MainDir, it.WtName, it.Container)
	}
	return nil
}
