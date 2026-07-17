package devserver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	info, err := os.Stat(filepath.Join(xdg, "bus"))
	if err != nil {
		return false
	}
	return !info.IsDir()
}

var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

// sanitizeUnitPart maps an arbitrary string to characters legal in a systemd
// unit name, collapses dash runs, and caps the length so the assembled name
// stays well under UNIT_NAME_MAX (255). keepTail keeps the end of an
// over-long value instead of the start — a worktree path's distinguishing
// part (repo--branch) is its tail, while the head (home/workspace dirs) is
// shared by every worktree and would make all unit names look alike.
func sanitizeUnitPart(s string, maxLen int, keepTail bool) string {
	safe := nonAlphanumeric.ReplaceAllString(s, "-")
	for strings.Contains(safe, "--") {
		safe = strings.ReplaceAll(safe, "--", "-")
	}
	safe = strings.Trim(safe, "-")
	if len(safe) > maxLen {
		if keepTail {
			safe = safe[len(safe)-maxLen:]
		} else {
			safe = safe[:maxLen]
		}
		safe = strings.Trim(safe, "-")
	}
	return safe
}

// scopeUnitName generates a valid systemd unit name for a given worktree and service.
func scopeUnitName(worktree, svc string) string {
	wtSafe := sanitizeUnitPart(worktree, 40, true)
	svcSafe := sanitizeUnitPart(svc, 40, false)
	// Nanosecond timestamp keeps the name unique across restarts of the same
	// service, so a lingering scope from a previous run cannot collide.
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	return fmt.Sprintf("wt-dev-%s-%s-%s", wtSafe, svcSafe, suffix)
}

// systemdRunCmd creates an exec.Cmd for systemd-run.
func systemdRunCmd(unitName, cmdStr string) *exec.Cmd {
	return exec.Command("systemd-run", "--user", "--scope", "--quiet", "--collect", "--unit="+unitName, "--slice=wt-dev.slice", "--", "sh", "-c", cmdStr)
}
