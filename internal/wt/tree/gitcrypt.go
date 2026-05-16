package tree

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"wt/internal/wt/core"
)

// isGitCryptRepo returns true when the repo at mainDir uses git-crypt encryption.
func isGitCryptRepo(mainDir string) bool {
	if out, err := core.GitOutput(mainDir, "config", "--get", "filter.git-crypt.smudge"); err == nil && strings.TrimSpace(out) != "" {
		return true
	}
	data, err := os.ReadFile(filepath.Join(mainDir, ".gitattributes"))
	if err == nil && strings.Contains(string(data), "filter=git-crypt") {
		return true
	}
	return false
}

// findGitCryptKey returns the first available git-crypt key path in priority order:
//  1. _config.git_crypt_key in .worktrees.json (container registry)
//  2. git config wt.gitCryptKey (repo-local git config in mainDir)
//  3. ~/.git-crypt-key (home default)
//
// Returns "" if no valid key file is found.
func findGitCryptKey(containerDir, mainDir string) string {
	if cfg, err := core.LoadConfig(containerDir); err == nil && cfg.GitCryptKey != "" {
		if _, err := os.Stat(cfg.GitCryptKey); err == nil {
			return cfg.GitCryptKey
		}
	}
	if out, err := core.GitOutput(mainDir, "config", "--get", "wt.gitCryptKey"); err == nil {
		if key := strings.TrimSpace(out); key != "" {
			if _, err := os.Stat(key); err == nil {
				return key
			}
		}
	}
	defaultKey := filepath.Join(os.Getenv("HOME"), ".git-crypt-key")
	if _, err := os.Stat(defaultKey); err == nil {
		return defaultKey
	}
	return ""
}

// isSmudgeError returns true when output contains a git-crypt smudge filter failure.
func isSmudgeError(output string) bool {
	return strings.Contains(output, "smudge filter git-crypt failed") ||
		strings.Contains(output, "filter 'git-crypt' failed") ||
		strings.Contains(output, "git-crypt: Error")
}

// unlockWorktree runs git-crypt unlock <keyFile> inside the given worktree directory.
func unlockWorktree(out io.Writer, worktreePath, keyFile string) error {
	cmd := exec.Command("git-crypt", "unlock", keyFile)
	cmd.Dir = worktreePath
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// logAndUnlock finds the git-crypt key and runs unlock, logging the outcome.
// Failures are non-fatal: a warning is printed and execution continues.
func logAndUnlock(out io.Writer, worktreePath, containerDir, mainDir string) {
	keyFile := findGitCryptKey(containerDir, mainDir)
	if keyFile == "" {
		_, _ = fmt.Fprintln(out, "⚠️  git-crypt 対象だが鍵が見つかりません。手動で unlock してください: git-crypt unlock <key>")
		return
	}
	_, _ = fmt.Fprintf(out, "🔐 git-crypt 対象 repo を検出。%s で unlock 中...\n", keyFile)
	if err := unlockWorktree(out, worktreePath, keyFile); err != nil {
		_, _ = fmt.Fprintln(out, "⚠️  git-crypt unlock に失敗しました。手動で unlock してください: git-crypt unlock <key>")
		return
	}
	_, _ = fmt.Fprintln(out, "✅ git-crypt unlock 成功")
}

// recoverSmudge handles the recovery path after a git-crypt smudge filter failure
// during git worktree add. It re-creates the worktree with --no-checkout, unlocks
// git-crypt, then runs git reset --hard HEAD.
// startPoint is empty when recovering an existing-branch checkout.
func recoverSmudge(out io.Writer, mainDir, worktreePath, branchName, startPoint, containerDir string) error {
	_, _ = fmt.Fprintln(out, "🔐 git-crypt smudge エラーを検出。--no-checkout でリカバリを試みます...")

	_ = exec.Command("git", "-C", mainDir, "worktree", "prune").Run()
	_ = os.RemoveAll(worktreePath)

	branchExists := exec.Command("git", "-C", mainDir, "rev-parse", "--verify", "--quiet", branchName).Run() == nil

	var ncArgs []string
	if branchExists || startPoint == "" {
		ncArgs = []string{"-C", mainDir, "worktree", "add", "--no-checkout", worktreePath, branchName}
	} else {
		ncArgs = []string{"-C", mainDir, "worktree", "add", "--no-checkout", worktreePath, "-b", branchName, startPoint}
	}
	if out2, err := exec.Command("git", ncArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("worktree作成に失敗しました: %s", strings.TrimSpace(string(out2)))
	}

	keyFile := findGitCryptKey(containerDir, mainDir)
	if keyFile == "" {
		_, _ = fmt.Fprintln(out, "⚠️  git-crypt 対象だが鍵が見つかりません。手動で unlock してください: git-crypt unlock <key>")
		return nil
	}

	_, _ = fmt.Fprintf(out, "🔐 git-crypt 対象 repo を検出。%s で unlock 中...\n", keyFile)
	if err := unlockWorktree(out, worktreePath, keyFile); err != nil {
		_, _ = fmt.Fprintln(out, "⚠️  git-crypt unlock に失敗しました。手動で unlock してください: git-crypt unlock <key>")
		return nil
	}
	_, _ = fmt.Fprintln(out, "✅ git-crypt unlock 成功")

	_, _ = fmt.Fprintln(out, "📥 ファイルを checkout 中...")
	if out2, err := exec.Command("git", "-C", worktreePath, "reset", "--hard", "HEAD").CombinedOutput(); err != nil {
		return fmt.Errorf("checkout に失敗しました: %s", strings.TrimSpace(string(out2)))
	}

	return nil
}

// addWorktreeNewBranch runs git worktree add with -b for a new branch, with
// git-crypt smudge failure recovery. branchName is the new branch; startPoint
// is the ref to branch from.
func addWorktreeNewBranch(out io.Writer, mainDir, worktreePath, branchName, startPoint, containerDir string) error {
	var errBuf bytes.Buffer
	cmd := exec.Command("git", "-C", mainDir, "worktree", "add", worktreePath, "-b", branchName, startPoint)
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)
	if err := cmd.Run(); err == nil {
		if isGitCryptRepo(mainDir) {
			logAndUnlock(out, worktreePath, containerDir, mainDir)
		}
		return nil
	}
	if isSmudgeError(errBuf.String()) {
		return recoverSmudge(out, mainDir, worktreePath, branchName, startPoint, containerDir)
	}
	return fmt.Errorf("worktree作成に失敗しました")
}

// addWorktreeExistingBranch runs git worktree add to check out an existing branch,
// with git-crypt smudge failure recovery.
func addWorktreeExistingBranch(out io.Writer, mainDir, worktreePath, branchName, containerDir string) error {
	var errBuf bytes.Buffer
	cmd := exec.Command("git", "-C", mainDir, "worktree", "add", worktreePath, branchName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)
	if err := cmd.Run(); err == nil {
		if isGitCryptRepo(mainDir) {
			logAndUnlock(out, worktreePath, containerDir, mainDir)
		}
		return nil
	}
	if isSmudgeError(errBuf.String()) {
		return recoverSmudge(out, mainDir, worktreePath, branchName, "", containerDir)
	}
	return fmt.Errorf("既存ローカルブランチからの worktree 作成に失敗: %s", strings.TrimSpace(errBuf.String()))
}
