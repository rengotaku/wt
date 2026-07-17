package devserver

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// systemdRunAvailable reports whether systemd-run can be used to launch transient scopes.
func systemdRunAvailable() bool {
	if os.Getenv("WT_NO_SYSTEMD_RUN") == "1" {
		return false
	}
	if _, err := exec.LookPath("systemd-run"); err != nil {
		return false
	}
	xdg := os.Getenv("XDG_RUNTIME_DIR")
	if xdg == "" {
		return false
	}
	info, err := os.Stat(xdg + "/bus")
	if err != nil {
		return false
	}
	return !info.IsDir()
}

var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9\-\_.]`)

// scopeUnitName generates a valid systemd unit name for a given worktree and service.
func scopeUnitName(worktree, svc string) string {
	wtSafe := nonAlphanumeric.ReplaceAllString(worktree, "-")
	for strings.Contains(wtSafe, "--") {
		wtSafe = strings.ReplaceAll(wtSafe, "--", "-")
	}
	wtSafe = strings.Trim(wtSafe, "-")
	// Keep the tail when truncating: the distinguishing part of a worktree
	// path (repo--branch) is at the end, while the head (home/workspace dirs)
	// is shared by every worktree and would make all unit names look alike.
	if len(wtSafe) > 40 {
		wtSafe = strings.Trim(wtSafe[len(wtSafe)-40:], "-")
	}

	svcSafe := nonAlphanumeric.ReplaceAllString(svc, "-")

	suffix := fmt.Sprintf("%x", time.Now().UnixNano()&0xfffff)

	return fmt.Sprintf("wt-dev-%s-%s-%s", wtSafe, svcSafe, suffix)
}

// systemdRunCmd creates an exec.Cmd for systemd-run.
func systemdRunCmd(unitName, cmdStr string) *exec.Cmd {
	return exec.Command("systemd-run", "--user", "--scope", "--quiet", "--collect", "--unit="+unitName, "--slice=wt-dev.slice", "--", "sh", "-c", cmdStr)
}
