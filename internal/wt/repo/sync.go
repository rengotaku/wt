package repo

import (
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
	msg    string
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

	type target struct {
		name   string
		branch string
		path   string
	}
	var targets []target

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

			repo := filepath.Base(container)
			targets = append(targets, target{name: repo, branch: branchLabel, path: mainPath})
		}
	}

	if len(targets) == 0 {
		fmt.Println("対象リポなし")
		return
	}

	fmt.Printf("⏳ %d リポを同期中...\n", len(targets))

	ch := make(chan syncResult, len(targets))

	for _, t := range targets {
		go func(t target) {
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

	success, fail := 0, 0
	for range targets {
		r := <-ch
		if r.ok {
			fmt.Printf("✅ %s (%s) %s\n", r.name, r.branch, r.msg)
			success++
		} else {
			fmt.Printf("❌ %s (%s) %s\n", r.name, r.branch, r.msg)
			fail++
		}
	}

	fmt.Println()
	if fail == 0 {
		fmt.Printf("✅ %d/%d 完了\n", success, len(targets))
	} else {
		fmt.Printf("✅ %d/%d 完了, ❌ %d 失敗\n", success, len(targets), fail)
	}
}
