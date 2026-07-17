package devserver

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestServe_SystemdScopeDetachesCgroup exercises the real systemd-run path:
// the spawned service must land in a wt-dev-*.scope under wt-dev.slice —
// outside the cgroup of the current process — so that stopping/restarting
// the unit that runs wt (wt-web.service) cannot take dev services with it.
// Skipped where no user systemd bus is available (CI containers).
func TestServe_SystemdScopeDetachesCgroup(t *testing.T) {
	if !systemdRunAvailable() {
		t.Skip("systemd-run user bus not available")
	}
	wt := writeWorktree(t, "[[services]]\nname = \"sleeper\"\ncmd = \"sleep 30\"\n")
	var buf bytes.Buffer
	if err := Serve(&buf, wt, 9800); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer func() { _ = Down(&bytes.Buffer{}, wt) }()

	if !strings.Contains(buf.String(), "systemd-run") {
		t.Fatalf("expected systemd-run mode banner, got:\n%s", buf.String())
	}
	rec := Recorded(wt)
	if len(rec) != 1 {
		t.Fatalf("expected 1 recorded service, got %d", len(rec))
	}
	pid := rec[0].PID

	childCg, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		t.Fatalf("read child cgroup: %v", err)
	}
	if !strings.Contains(string(childCg), "wt-dev.slice") ||
		!strings.Contains(string(childCg), "wt-dev-") {
		t.Errorf("child not in wt-dev slice/scope: %s", childCg)
	}
	selfCg, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		t.Fatalf("read self cgroup: %v", err)
	}
	if bytes.Equal(childCg, selfCg) {
		t.Errorf("child shares the parent cgroup — separation failed: %s", childCg)
	}

	// The scope must be listable — this is the ops surface promised by #122.
	out, err := exec.Command("systemctl", "--user", "list-units", "wt-dev-*", "--no-legend").Output()
	if err != nil {
		t.Fatalf("systemctl list-units: %v", err)
	}
	if !strings.Contains(string(out), ".scope") {
		t.Errorf("no wt-dev-*.scope listed:\n%s", out)
	}

	// Down must still stop a scope-detached service via its process group.
	if err := Down(&bytes.Buffer{}, wt); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if IsRunning(wt) {
		t.Error("service still running after Down")
	}
}
