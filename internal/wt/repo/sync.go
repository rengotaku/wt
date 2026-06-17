package repo

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Sync the canonical main/master worktree of every container with its remote.

type syncEntry struct {
	Type   string `json:"type"`
	Branch string `json:"branch"`
}

type syncResult struct {
	name   string
	branch string
	ok     bool
	warn   bool
	msg    string
}

// loadSyncSkipList reads ~/.config/wt/sync-skip and returns the set of repo
// directory names to exclude from `wt repo sync`. One name per line; lines
// starting with `#` and blank lines are ignored. Missing file = empty set.
func loadSyncSkipList(home string) map[string]struct{} {
	skip := map[string]struct{}{}
	f, err := os.Open(filepath.Join(home, ".config", "wt", "sync-skip"))
	if err != nil {
		return skip
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		skip[line] = struct{}{}
	}
	return skip
}

func resolveMain(container string) (path, name string) {
	for _, candidate := range []string{"main", "master"} {
		p := filepath.Join(container, candidate)
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			return p, candidate
		}
	}
	return "", ""
}

// Sync runs git pull --ff-only on every main/master worktree in parallel.
func Sync() {
	home := os.Getenv("HOME")
	dirs := []string{
		filepath.Join(home, "Workspace"),
		filepath.Join(home, "MyWorkspace"),
	}

	skipSet := loadSyncSkipList(home)

	type target struct {
		name   string
		branch string
		path   string
	}
	var targets []target
	var skipped []string

	for _, base := range dirs {
		containers, _ := filepath.Glob(filepath.Join(base, "*"))
		for _, container := range containers {
			info, err := os.Stat(container)
			if err != nil || !info.IsDir() {
				continue
			}
			mainPath, mainName := resolveMain(container)
			if mainPath == "" {
				continue
			}

			repo := filepath.Base(container)
			if _, ok := skipSet[repo]; ok {
				skipped = append(skipped, repo)
				continue
			}

			metaFile := filepath.Join(container, ".worktrees.json")
			branchLabel := mainName
			if data, err := os.ReadFile(metaFile); err == nil {
				var entries map[string]json.RawMessage
				if json.Unmarshal(data, &entries) == nil {
					if raw, ok := entries[mainName]; ok {
						var e syncEntry
						if json.Unmarshal(raw, &e) == nil && e.Branch != "" {
							branchLabel = e.Branch
						}
					}
				}
			}

			targets = append(targets, target{name: repo, branch: branchLabel, path: mainPath})
		}
	}

	if len(targets) == 0 {
		if len(skipped) > 0 {
			fmt.Printf("対象リポなし（⏭ %d 件スキップ: %s）\n", len(skipped), strings.Join(skipped, ", "))
		} else {
			fmt.Println("対象リポなし")
		}
		return
	}

	if len(skipped) > 0 {
		fmt.Printf("⏭ %d 件スキップ: %s\n", len(skipped), strings.Join(skipped, ", "))
	}
	fmt.Printf("⏳ %d リポを同期中...\n", len(targets))

	ch := make(chan syncResult, len(targets))

	for _, t := range targets {
		go func(t target) {
			// フォルダ名（main/master）と実際に checkout されているブランチを照合する。
			// worktree-first 運用を破って main フォルダで feat ブランチに切り替えていると、
			// --ff-only pull が意図しないブランチを進めてしまうため、pull せず警告する。
			folderName := filepath.Base(t.path)
			curBranchOut, _ := exec.Command("git", "-C", t.path, "rev-parse", "--abbrev-ref", "HEAD").Output()
			curBranch := strings.TrimSpace(string(curBranchOut))
			if curBranch != "" && curBranch != folderName {
				ch <- syncResult{
					name:   t.name,
					branch: curBranch,
					warn:   true,
					msg:    fmt.Sprintf("フォルダ %q に別ブランチ %q が checkout されています → pull skip", folderName, curBranch),
				}
				return
			}

			oldHead, _ := exec.Command("git", "-C", t.path, "rev-parse", "HEAD").Output()

			cmd := exec.Command("git", "-C", t.path, "pull", "--ff-only")
			out, err := cmd.CombinedOutput()
			output := strings.TrimSpace(string(out))

			switch {
			case err != nil:
				firstLine := output
				if idx := strings.IndexByte(output, '\n'); idx != -1 {
					firstLine = output[:idx]
				}
				ch <- syncResult{name: t.name, branch: t.branch, ok: false, msg: firstLine}
			case strings.Contains(output, "Already up to date"):
				ch <- syncResult{name: t.name, branch: t.branch, ok: true, msg: "Already up to date"}
			default:
				old := strings.TrimSpace(string(oldHead))
				countOut, _ := exec.Command("git", "-C", t.path, "rev-list", "--count", old+"..HEAD").Output()
				count := strings.TrimSpace(string(countOut))
				ch <- syncResult{name: t.name, branch: t.branch, ok: true, msg: count + " commits pulled"}
			}
		}(t)
	}

	success, fail, warn := 0, 0, 0
	for range targets {
		r := <-ch
		switch {
		case r.warn:
			fmt.Printf("⚠️  %s (%s) %s\n", r.name, r.branch, r.msg)
			warn++
		case r.ok:
			fmt.Printf("✅ %s (%s) %s\n", r.name, r.branch, r.msg)
			success++
		default:
			fmt.Printf("❌ %s (%s) %s\n", r.name, r.branch, r.msg)
			fail++
		}
	}

	fmt.Println()
	summary := fmt.Sprintf("✅ %d/%d 完了", success, len(targets))
	if warn > 0 {
		summary += fmt.Sprintf(", ⚠️ %d 警告", warn)
	}
	if fail > 0 {
		summary += fmt.Sprintf(", ❌ %d 失敗", fail)
	}
	fmt.Println(summary)
}
