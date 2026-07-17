package devserver

import (
	"strings"
	"testing"
)

func TestSystemdRunAvailable(t *testing.T) {
	t.Run("WT_NO_SYSTEMD_RUN disables", func(t *testing.T) {
		t.Setenv("WT_NO_SYSTEMD_RUN", "1")
		if systemdRunAvailable() {
			t.Error("expected false when WT_NO_SYSTEMD_RUN=1")
		}
	})

	t.Run("No XDG_RUNTIME_DIR disables", func(t *testing.T) {
		t.Setenv("WT_NO_SYSTEMD_RUN", "0")
		t.Setenv("XDG_RUNTIME_DIR", "")
		if systemdRunAvailable() {
			t.Error("expected false when XDG_RUNTIME_DIR is empty")
		}
	})

	t.Run("XDG_RUNTIME_DIR bus missing disables", func(t *testing.T) {
		t.Setenv("WT_NO_SYSTEMD_RUN", "0")
		dir := t.TempDir()
		t.Setenv("XDG_RUNTIME_DIR", dir)
		if systemdRunAvailable() {
			t.Error("expected false when bus socket is missing")
		}
	})
}

func TestScopeUnitName(t *testing.T) {
	tests := []struct {
		worktree string
		svc      string
		wantSub  []string
	}{
		{"/home/user/work/my-repo", "api-server", []string{"wt-dev-home-user-work-my-repo-api-server-"}},
		{"weird!@#path", "svc$1", []string{"wt-dev-weird-path-svc-1-"}},
		{"very/long/path/that/exceeds/forty/characters/in/length", "svc", []string{"wt-dev-that-exceeds-forty-characters-in-length-svc-"}},
		// Over-long service names must be capped so the unit name stays
		// under UNIT_NAME_MAX; dash-wrapped names must not leave dash runs.
		{"/w", strings.Repeat("s", 60), []string{"wt-dev-w-" + strings.Repeat("s", 40) + "-"}},
		{"/w", "-web-", []string{"wt-dev-w-web-"}},
	}

	for _, tt := range tests {
		got := scopeUnitName(tt.worktree, tt.svc)
		if !strings.HasPrefix(got, "wt-dev-") {
			t.Errorf("expected prefix wt-dev-, got %q", got)
		}
		for _, sub := range tt.wantSub {
			if !strings.Contains(got, sub) {
				t.Errorf("scopeUnitName(%q, %q) = %q, want substring %q", tt.worktree, tt.svc, got, sub)
			}
		}
		if strings.ContainsAny(got, "!@#$%^&*()/\\") {
			t.Errorf("scopeUnitName(%q, %q) contains invalid characters: %q", tt.worktree, tt.svc, got)
		}
	}
}

func TestSystemdRunCmd(t *testing.T) {
	cmd := systemdRunCmd("wt-dev-test-123", "echo hello")

	wantArgs := []string{
		"systemd-run",
		"--user",
		"--scope",
		"--quiet",
		"--collect",
		"--unit=wt-dev-test-123",
		"--slice=wt-dev.slice",
		"--",
		"sh",
		"-c",
		"echo hello",
	}

	if len(cmd.Args) != len(wantArgs) {
		t.Fatalf("got args %v, want %v", cmd.Args, wantArgs)
	}

	for i, arg := range cmd.Args {
		if arg != wantArgs[i] {
			t.Errorf("arg %d = %q, want %q", i, arg, wantArgs[i])
		}
	}
}
