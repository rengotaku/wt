package tree

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Update fast-forwards a single worktree to its branch's latest remote commit
// via `git pull --ff-only`. 未コミットの追跡変更がある場合は更新せずエラーを返す。
//
// 戻り値の文字列は "Already up to date" もしくは "N commits pulled"。
func Update(worktree string) (string, error) {
	// 未コミット変更ガード: UI の diff_count と揃えるため untracked は除外する。
	statusOut, _ := exec.Command("git", "-C", worktree, "status", "--porcelain", "--untracked-files=no").Output()
	if strings.TrimSpace(string(statusOut)) != "" {
		return "", errors.New("未コミットの変更があるため最新化できません")
	}

	oldHead, _ := exec.Command("git", "-C", worktree, "rev-parse", "HEAD").Output()

	cmd := exec.Command("git", "-C", worktree, "pull", "--ff-only")
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	switch {
	case err != nil:
		firstLine := output
		if idx := strings.IndexByte(output, '\n'); idx != -1 {
			firstLine = output[:idx]
		}
		if firstLine == "" {
			return "", err
		}
		return "", errors.New(firstLine)
	case strings.Contains(output, "Already up to date"):
		return "Already up to date", nil
	default:
		old := strings.TrimSpace(string(oldHead))
		countOut, _ := exec.Command("git", "-C", worktree, "rev-list", "--count", old+"..HEAD").Output()
		count := strings.TrimSpace(string(countOut))
		return fmt.Sprintf("%s commits pulled", count), nil
	}
}
