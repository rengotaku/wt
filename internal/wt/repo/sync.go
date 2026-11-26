package repo

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Sync the canonical main/master worktree of every container with its remote.

type syncEntry struct {
	Type   string `json:"type"`
	Branch string `json:"branch"`
}

// Sync results. Fixed words so callers can match on them.
const (
	SyncUpdated     = "updated"
	SyncUpToDate    = "up_to_date"
	SyncSkipped     = "skipped"
	SyncWrongBranch = "wrong_branch"
	SyncFailed      = "failed"
)

// SyncResult is the outcome for one repository.
type SyncResult struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Result string `json:"result"`
	Detail string `json:"detail"`
}

// hasUncommitted は git status --porcelain の出力に、pull を止めるべき
// 未コミットの変更（tracked の変更）があるかを返す。
func hasUncommitted(porcelain string) bool {
	for _, line := range strings.Split(porcelain, "\n") {
		if strings.TrimSpace(line) != "" {
			return true
		}
	}
	return false
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

// Sync runs git pull --ff-only on the main/master worktree of every container
// in parallel, or only on the container at path only when it is not empty. A
// container named explicitly is synced even when it is in the sync-skip list. Progress goes to progress; the
// per-repo outcome is returned sorted by repo name. Failures are reported in
// the results, not as an error: the caller decides the exit code.
func Sync(progress io.Writer, only string) ([]SyncResult, error) {
	home := os.Getenv("HOME")
	skipSet := loadSyncSkipList(home)

	type target struct {
		name   string
		branch string
		path   string
	}
	var targets []target
	var results []SyncResult

	for _, base := range []string{filepath.Join(home, "Workspace"), filepath.Join(home, "MyWorkspace")} {
		containers, _ := filepath.Glob(filepath.Join(base, "*"))
		for _, container := range containers {
			info, err := os.Stat(container)
			if err != nil || !info.IsDir() {
				continue
			}
			name := filepath.Base(container)
			if only != "" && filepath.Clean(container) != filepath.Clean(only) {
				continue
			}
			mainPath, mainName := resolveMain(container)
			if mainPath == "" {
				continue
			}
			if _, ok := skipSet[name]; ok && only == "" {
				results = append(results, SyncResult{Repo: name, Branch: mainName, Result: SyncSkipped, Detail: "sync-skip に載っている"})
				continue
			}
			targets = append(targets, target{name: name, branch: mainBranchLabel(container, mainName), path: mainPath})
		}
	}
	if only != "" && len(targets) == 0 {
		return nil, fmt.Errorf("main / master が見つかりません: %s", only)
	}

	_, _ = fmt.Fprintf(progress, "⏳ %d リポを同期中...\n", len(targets))
	ch := make(chan SyncResult, len(targets))
	for _, t := range targets {
		go func(t target) { ch <- syncOne(t.name, t.branch, t.path) }(t)
	}
	for range targets {
		results = append(results, <-ch)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Repo < results[j].Repo })
	return results, nil
}

// mainBranchLabel reads the main entry's branch from .worktrees.json, falling
// back to the folder name.
func mainBranchLabel(container, mainName string) string {
	data, err := os.ReadFile(filepath.Join(container, ".worktrees.json"))
	if err != nil {
		return mainName
	}
	var entries map[string]json.RawMessage
	if json.Unmarshal(data, &entries) != nil {
		return mainName
	}
	var e syncEntry
	if raw, ok := entries[mainName]; ok && json.Unmarshal(raw, &e) == nil && e.Branch != "" {
		return e.Branch
	}
	return mainName
}

func syncOne(name, branch, path string) SyncResult {
	r := SyncResult{Repo: name, Branch: branch}
	folderName := filepath.Base(path)
	curOut, err := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		r.Result, r.Detail = SyncFailed, "現在のブランチを取得できない: "+err.Error()
		return r
	}
	if cur := strings.TrimSpace(string(curOut)); cur != folderName {
		r.Branch = cur
		r.Result = SyncWrongBranch
		r.Detail = fmt.Sprintf("フォルダ %q に別のブランチ %q が checkout されている（wt doctor --fix で直せる）", folderName, cur)
		return r
	}
	statusOut, err := exec.Command("git", "-C", path, "status", "--porcelain", "--untracked-files=no").Output()
	if err != nil {
		r.Result, r.Detail = SyncFailed, "git status に失敗: "+err.Error()
		return r
	}
	if hasUncommitted(string(statusOut)) {
		r.Result, r.Detail = SyncSkipped, "未コミットの変更がある"
		return r
	}
	oldHead, _ := exec.Command("git", "-C", path, "rev-parse", "HEAD").Output()
	out, err := exec.Command("git", "-C", path, "pull", "--ff-only").CombinedOutput()
	output := strings.Join(strings.Fields(string(out)), " ")
	switch {
	case err != nil:
		if output == "" {
			output = err.Error()
		}
		r.Result, r.Detail = SyncFailed, output
	case strings.Contains(output, "Already up to date"):
		r.Result = SyncUpToDate
	default:
		old := strings.TrimSpace(string(oldHead))
		countOut, _ := exec.Command("git", "-C", path, "rev-list", "--count", old+"..HEAD").Output()
		r.Result, r.Detail = SyncUpdated, strings.TrimSpace(string(countOut))+" commits"
	}
	return r
}
