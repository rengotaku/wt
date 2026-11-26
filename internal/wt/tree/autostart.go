package tree

import (
	"fmt"

	"wt/internal/wt/core"
)

// SetAutoStart sets the AutoStart flag of worktree wtName in repo's
// .worktrees.json. AutoStart worktrees are served when `wt daemon` starts and
// are candidates for the idle reaper.
func SetAutoStart(repo, wtName string, on bool) error {
	container, err := core.FindContainer(repo)
	if err != nil {
		return err
	}
	entries, err := core.LoadEntries(container)
	if err != nil {
		return err
	}
	entry, ok := entries[wtName]
	if !ok {
		return fmt.Errorf("worktree が見つかりません: %s/%s", repo, wtName)
	}
	entry.AutoStart = on
	return core.PutEntry(container, wtName, &entry)
}
