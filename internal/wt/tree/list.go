package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"wt/internal/wt/core"
)

// ListEntry is a single worktree row used by list/interactive commands.
type ListEntry struct {
	Repo      string
	WtName    string // 生の worktree 名（main, issue155 など）
	Name      string // "[wtName - repo]" (CLI 表示用)
	Label     string // pre-padded "[type] info" string
	Path      string
	Branch    string // 現在の git ブランチ名
	Created   string // .worktrees.json の Created フィールド値
	Issue     string // .worktrees.json の Issue フィールド値（例: "#168"）
	IsMain    bool   // wtName == "main" || wtName == "master" || entry.Type == "main"
	Pinned    bool   // .worktrees.json の Pinned フラグ（一覧先頭固定のみ）
	AutoStart bool   // .worktrees.json の AutoStart フラグ（起動時 auto-serve + アイドルリーパ対象）
}

// Entries scans all containers and returns every existing worktree entry.
func Entries() []ListEntry {
	var lines []ListEntry

	for _, base := range core.WorkspaceDirs() {
		matches, _ := filepath.Glob(filepath.Join(base, "*", ".worktrees.json"))
		sort.Strings(matches)
		for _, metaFile := range matches {
			container := filepath.Dir(metaFile)
			repo := filepath.Base(container)

			entries, err := core.LoadEntries(container)
			if err != nil {
				continue
			}

			keys := make([]string, 0, len(entries))
			for k := range entries {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for _, wtName := range keys {
				info := entries[wtName]
				wtPath := filepath.Join(container, wtName)
				if fi, err := os.Stat(wtPath); err != nil || !fi.IsDir() {
					continue
				}

				branch, _ := core.GitOutput(wtPath, "branch", "--show-current")
				typeStr := fmt.Sprintf("%-14s", "["+info.Type+"]")
				var parts []string
				if branch != "" {
					parts = append(parts, branch)
				}
				if info.Description != "" {
					parts = append(parts, info.Description)
				}
				if info.Created != "" {
					parts = append(parts, "("+info.Created+")")
				}
				label := typeStr + strings.Join(parts, " ")
				name := "[" + wtName + " - " + repo + "]"
				lines = append(lines, ListEntry{
					Repo:      repo,
					WtName:    wtName,
					Name:      name,
					Label:     label,
					Path:      wtPath,
					Branch:    branch,
					Created:   info.Created,
					Issue:     info.Issue,
					IsMain:    wtName == "main" || wtName == "master" || info.Type == "main",
					Pinned:    info.Pinned,
					AutoStart: info.AutoStart,
				})
			}
		}
	}
	return lines
}

// List prints worktree entries in the legacy format used by wt-tree-ls.sh.
// Each line: "%-*s <label>|<path>".
func List() {
	lines := Entries()
	maxWidth := 0
	for i := range lines {
		if len(lines[i].Name) > maxWidth {
			maxWidth = len(lines[i].Name)
		}
	}
	maxWidth += 2
	for i := range lines {
		fmt.Printf("%-*s %s|%s\n", maxWidth, lines[i].Name, lines[i].Label, lines[i].Path)
	}
}
