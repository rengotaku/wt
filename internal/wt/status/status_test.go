package status

import (
	"testing"

	"wt/internal/wt/ports"
	"wt/internal/wt/proxy"
	"wt/internal/wt/settings"
)

func sampleInput() *Input {
	return &Input{
		Rows: []ports.Row{
			{Repo: "app", WtName: "app--feat", Branch: "feat/x", Path: "/w/app/app--feat", PortBase: 9000, Exists: true,
				Ports: []ports.PortState{{Port: 9000, Listening: true, PID: 11, Proc: "air"}, {Port: 9001}}},
			{Repo: "app", WtName: "main", Branch: "main", Path: "/w/app/main", Exists: true},
			{Repo: "app", WtName: "app--idle", Branch: "idle", PortBase: 9005, Exists: true,
				Ports: []ports.PortState{{Port: 9005}}},
			{Repo: "app", WtName: "app--broken", Branch: "broken", PortBase: 9010, Exists: true,
				Ports: []ports.PortState{{Port: 9010, Running: true, PID: 12}}},
			{Repo: "lib", WtName: "lib--gone", Branch: "gone", PortBase: 9015, Exists: false},
		},
		AutoStart: map[string]bool{"/w/app/app--feat": true},
		Routes:    []proxy.Route{{Repo: "app", WtName: "app--feat", Label: "feat", Port: 9000}},
		Daemon:    Daemon{Running: true, PID: 4242},
		Proxy:     settings.Proxy{Enabled: true, Port: 8088, Bind: "127.0.0.1"},
	}
}

func TestBuild_States(t *testing.T) {
	rep := Build(sampleInput())
	want := map[string]string{
		"app--feat":   StateRunning,
		"main":        StateNone,
		"app--idle":   StateIdle,
		"app--broken": StateUnhealthy, // running without a LISTEN socket, not headless
		"lib--gone":   StateStale,
	}
	if len(rep.Worktrees) != len(want) {
		t.Fatalf("got %d worktrees, want %d", len(rep.Worktrees), len(want))
	}
	for _, w := range rep.Worktrees {
		if w.State != want[w.Worktree] {
			t.Errorf("%s: state %q, want %q", w.Worktree, w.State, want[w.Worktree])
		}
	}
	feat := rep.Worktrees[0]
	if !feat.AutoStart || feat.Ports != "9000-9004" || feat.URL != "http://feat.app.wt.localhost:8088" {
		t.Errorf("feat row: %+v", feat)
	}
	if rep.Proxy.Addr != "127.0.0.1:8088" || !rep.Daemon.Running || rep.Daemon.PID != 4242 {
		t.Errorf("header: proxy=%+v daemon=%+v", rep.Proxy, rep.Daemon)
	}
}

func TestBuild_NoURLWhenProxyDisabled(t *testing.T) {
	in := sampleInput()
	in.Proxy.Enabled = false
	if url := Build(in).Worktrees[0].URL; url != "" {
		t.Errorf("proxy disabled: URL %q, want empty", url)
	}
}

// A running daemon's recorded proxy wins over settings (it may have started
// with --proxy-port or --no-proxy).
func TestBuild_DaemonProxyOverridesSettings(t *testing.T) {
	in := sampleInput()
	in.DaemonProxy, in.DaemonProxyKnown = "127.0.0.1:18088", true
	rep := Build(in)
	if rep.Proxy.Addr != "127.0.0.1:18088" || rep.Worktrees[0].URL != "http://feat.app.wt.localhost:18088" {
		t.Errorf("proxy=%+v url=%q", rep.Proxy, rep.Worktrees[0].URL)
	}

	in.DaemonProxy = "" // daemon runs with --no-proxy
	rep = Build(in)
	if rep.Proxy.Enabled || rep.Worktrees[0].URL != "" {
		t.Errorf("--no-proxy daemon: proxy=%+v url=%q", rep.Proxy, rep.Worktrees[0].URL)
	}
}
