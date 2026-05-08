package repo

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"wt/internal/wt/core"
)

// InitOptions captures CLI flags for `wt repo init`.
type InitOptions struct {
	Src    string // 必須: scaffold 元ディレクトリ
	Target string // 任意: コンテナ親（既定: $HOME/Workspace）
	Force  bool   // 開いているプロセスがあっても続行する
}

// Init scans Src for git repos and scaffolds each into the wt container layout.
// Non-interactive — all decisions are driven by InitOptions.
func Init(out io.Writer, opts InitOptions) error {
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("git が必要です")
	}
	if opts.Src == "" {
		return errors.New("Usage: wt repo init <src-dir> [target-dir] [--force]") //nolint:staticcheck // user-facing usage string
	}

	src := strings.TrimRight(expandHome(opts.Src), "/")
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return fmt.Errorf("ディレクトリが存在しません: %s", src)
	}

	target := opts.Target
	if target == "" {
		target = filepath.Join(os.Getenv("HOME"), "Workspace")
	}
	target = strings.TrimRight(expandHome(target), "/")
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "wt repo init - 既存 git リポを wt コンテナ形式に scaffold します")
	_, _ = fmt.Fprintf(out, "WT_MASTER_DIR: %s\n", core.MasterDir())
	_, _ = fmt.Fprintf(out, "📂 src:    %s\n", src)
	_, _ = fmt.Fprintf(out, "🎯 target: %s\n", target)
	_, _ = fmt.Fprintln(out, "")

	entries, _ := os.ReadDir(src)
	var found []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		repoPath := filepath.Join(src, e.Name())
		if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
			continue
		}
		if alreadyContainerized(repoPath) {
			return fmt.Errorf("wt コンテナが既に存在します: %s", repoPath)
		}
		found = append(found, repoPath)
	}

	if len(found) == 0 {
		_, _ = fmt.Fprintf(out, "ℹ️  git 管理下のリポはありません: %s\n", src)
		return nil
	}

	_, _ = fmt.Fprintf(out, "対象 (%d 件):\n", len(found))
	for _, p := range found {
		_, _ = fmt.Fprintf(out, "  - %s → %s/%s\n", p, target, filepath.Base(p))
	}
	_, _ = fmt.Fprintln(out, "")

	if _, err := exec.LookPath("lsof"); err == nil {
		_, _ = fmt.Fprintln(out, "ℹ️  対象パスを開いているプロセスを検出中...")
		hasOpen := false
		for _, p := range found {
			if checkOpenProcesses(out, p) {
				hasOpen = true
			}
		}
		if hasOpen && !opts.Force {
			_, _ = fmt.Fprintln(out, "")
			_, _ = fmt.Fprintln(out, "⚠️  上記プロセスが mv 後の旧パスを掴んだままだと、空ディレクトリやキャッシュを再生成します")
			_, _ = fmt.Fprintln(out, "  → 続行前に dev server (vite/webpack/next 等) と LSP (tsserver/gopls 等) を停止し、--force で再実行してください")
			return errors.New("aborted: open processes detected")
		}
	} else {
		_, _ = fmt.Fprintln(out, "⚠️  lsof が見つからないため開いているプロセスのチェックをスキップします")
	}

	var firstErr error
	for _, p := range found {
		if err := scaffoldRepo(out, p, target); err != nil {
			_, _ = fmt.Fprintf(out, "❌ %s: %v\n", p, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if firstErr != nil {
		return firstErr
	}

	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "✅ 完了")
	return nil
}

// alreadyContainerized matches the bash heuristic: container has main/master
// or legacy <name>/<name> layout.
func alreadyContainerized(p string) bool {
	for _, n := range []string{"main", "master"} {
		if _, err := os.Stat(filepath.Join(p, n, ".git")); err == nil {
			return true
		}
	}
	repo := filepath.Base(p)
	if _, err := os.Stat(filepath.Join(p, repo, ".git")); err == nil {
		return true
	}
	return false
}

// defaultBranchOf finds origin/HEAD or falls back to local main/master.
func defaultBranchOf(repoPath string) string {
	if v, err := core.GitOutput(repoPath, "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil && v != "" {
		return strings.TrimPrefix(v, "refs/remotes/origin/")
	}
	if core.GitCheck(repoPath, "rev-parse", "--verify", "--quiet", "main") {
		return "main"
	}
	if core.GitCheck(repoPath, "rev-parse", "--verify", "--quiet", "master") {
		return "master"
	}
	return "main"
}

// scaffoldRepo moves an existing repo into its container layout.
func scaffoldRepo(out io.Writer, src, targetBase string) error {
	repoName := filepath.Base(src)
	container := filepath.Join(targetBase, repoName)
	if _, err := os.Stat(container); err == nil {
		return fmt.Errorf("コンテナが既に存在します: %s （冪等性なし。手動で確認してください）", container)
	}

	defaultB := defaultBranchOf(src)
	if defaultB != "main" && defaultB != "master" {
		_, _ = fmt.Fprintf(out, "ℹ️  default branch '%s' は main/master ではないため main/ に配置します\n", defaultB)
		defaultB = "main"
	}

	dst := filepath.Join(container, defaultB)
	_, _ = fmt.Fprintf(out, "📦 scaffold: %s → %s\n", src, dst)
	if err := os.MkdirAll(container, 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err != nil {
		return err
	}

	if err := core.PutEntry(container, defaultB, &core.Entry{
		Type:        "main",
		Created:     time.Now().Format("2006-01-02"),
		Branch:      defaultB,
		Description: "main worktree",
	}); err != nil {
		return err
	}
	if err := core.SaveConfig(container, core.EntryConfig{SymlinkCandidates: []string{}}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✅ metadata: %s\n", core.MetaFile(container))

	masterDir := core.MasterDir()
	_ = os.MkdirAll(masterDir, 0o755)
	link := filepath.Join(masterDir, repoName)
	return ensureSymlink(out, link, dst)
}

// checkOpenProcesses reports processes holding files under path. Returns
// true when at least one process was found.
func checkOpenProcesses(out io.Writer, path string) bool {
	cmd := exec.Command("lsof", "+D", path)
	stdout, err := cmd.Output()
	if err != nil || len(stdout) == 0 {
		return false
	}
	lines := strings.Split(string(stdout), "\n")
	if len(lines) <= 1 {
		return false
	}
	_, _ = fmt.Fprintf(out, "⚠️  対象パスを開いているプロセスがあります: %s\n", path)
	seen := map[string]struct{}{}
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := fmt.Sprintf("    PID=%s CMD=%s FILE=%s\n", fields[1], fields[0], fields[len(fields)-1])
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		_, _ = fmt.Fprint(out, key)
	}
	return true
}
