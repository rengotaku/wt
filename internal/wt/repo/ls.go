package repo

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"wt/internal/wt/core"
)

// Ls prints registered repositories with their worktree count.
func Ls(out io.Writer) error {
	type row struct {
		name      string
		container string
		count     int
	}
	var rows []row
	maxName := 0

	for _, base := range core.WorkspaceDirs() {
		matches, _ := filepath.Glob(filepath.Join(base, "*", ".worktrees.json"))
		for _, m := range matches {
			container := filepath.Dir(m)
			name := filepath.Base(container)
			entries, err := core.LoadEntries(container)
			if err != nil {
				continue
			}
			r := row{name: name, container: container, count: len(entries)}
			rows = append(rows, r)
			if len(name) > maxName {
				maxName = len(name)
			}
		}
	}

	if len(rows) == 0 {
		return errors.New("登録リポジトリがありません (wt repo add で追加してください)")
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	for _, r := range rows {
		_, _ = fmt.Fprintf(out, "%-*s  %s  (%d worktrees)\n", maxName, r.name, r.container, r.count)
	}
	return nil
}
