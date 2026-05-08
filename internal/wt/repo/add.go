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

// httpsToSSH converts a GitHub HTTPS URL to SSH format so that clone works in
// non-TTY environments where interactive credential prompts are unavailable.
// Non-GitHub URLs and already-SSH URLs are returned unchanged.
func httpsToSSH(url string) string {
	// Match https://github.com/<owner>/<repo>[.git]
	const prefix = "https://github.com/"
	if !strings.HasPrefix(url, prefix) {
		return url
	}
	path := strings.TrimPrefix(url, prefix)
	path = strings.TrimSuffix(path, ".git")
	return "git@github.com:" + path + ".git"
}

// Add clones a remote repository into the wt container layout.
// url is the GitHub URL; targetBase is the optional override (default ~/Workspace).
func Add(out io.Writer, url, targetBase string) error {
	if url == "" {
		return errors.New("Usage: wt add <url> [container_base]") //nolint:staticcheck // user-facing usage string
	}
	url = httpsToSSH(url)
	if targetBase == "" {
		targetBase = filepath.Join(os.Getenv("HOME"), "Workspace")
	}
	targetBase = expandHome(targetBase)
	targetBase = strings.TrimRight(targetBase, "/")

	repoName := strings.TrimSuffix(filepath.Base(url), ".git")
	container := filepath.Join(targetBase, repoName)
	if _, err := os.Stat(container); err == nil {
		return fmt.Errorf("コンテナが既に存在します: %s", container)
	}

	_, _ = fmt.Fprintln(out, "ℹ️  デフォルトブランチを確認中...")
	defaultB := lsRemoteDefault(url)
	if defaultB == "" {
		defaultB = "main"
	}
	if defaultB != "main" && defaultB != "master" {
		_, _ = fmt.Fprintf(out, "ℹ️  default branch '%s' は main/master 以外のため main/ に配置します\n", defaultB)
		defaultB = "main"
	}

	cloneDir := filepath.Join(container, defaultB)
	_, _ = fmt.Fprintf(out, "📦 clone: %s → %s\n", url, cloneDir)
	if err := os.MkdirAll(container, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", url, cloneDir)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		_ = os.Remove(container)
		return errors.New("clone に失敗しました")
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
	if err := ensureSymlink(out, link, cloneDir); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintf(out, "✅ 完了: %s\n", cloneDir)
	return nil
}

// lsRemoteDefault asks `git ls-remote --symref <url> HEAD` for the default branch.
func lsRemoteDefault(url string) string {
	out, err := exec.Command("git", "ls-remote", "--symref", url, "HEAD").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "ref:") {
			continue
		}
		s := strings.TrimSpace(strings.TrimPrefix(line, "ref:"))
		s = strings.TrimPrefix(s, "refs/heads/")
		// Trim trailing whitespace + tab + "HEAD" reference.
		if i := strings.IndexAny(s, " \t"); i >= 0 {
			s = s[:i]
		}
		return s
	}
	return ""
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(os.Getenv("HOME"), p[2:])
	}
	if p == "~" {
		return os.Getenv("HOME")
	}
	return p
}

// ensureSymlink mirrors the bash function: handle broken / mismatched / occupied targets.
func ensureSymlink(out io.Writer, link, target string) error {
	fi, err := os.Lstat(link)
	if err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			cur, _ := os.Readlink(link)
			if curAbs, err := filepath.EvalSymlinks(link); err == nil {
				tgtAbs, _ := filepath.EvalSymlinks(target)
				if curAbs == tgtAbs {
					_, _ = fmt.Fprintf(out, "ℹ️  既に正しい symlink: %s\n", link)
					return nil
				}
				_, _ = fmt.Fprintf(out, "ℹ️  別の場所を指す symlink を上書き: %s → %s\n", link, cur)
			} else {
				_, _ = fmt.Fprintf(out, "ℹ️  壊れた symlink を上書き: %s\n", link)
			}
			_ = os.Remove(link)
		} else {
			return fmt.Errorf("%s は既に存在します。手動で除去してから再実行してください", link)
		}
	}
	if err := os.Symlink(target, link); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✅ symlink: %s → %s\n", link, target)
	return nil
}
