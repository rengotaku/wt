package status

import (
	"bytes"
	"encoding/json"
	"strings"
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
		AutoStart: map[string]bool{"app/app--feat": true},
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

func TestWriteTSV(t *testing.T) {
	rep := Build(sampleInput())
	var buf bytes.Buffer
	if err := WriteTSV(&buf, &rep); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if lines[0] != "# daemon=running pid=4242" || lines[1] != "# proxy=enabled addr=127.0.0.1:8088" {
		t.Errorf("header lines: %q", lines[:2])
	}
	if lines[2] != strings.Join(TSVHeader, "\t") {
		t.Errorf("column header: %q", lines[2])
	}
	if got, want := lines[3], "app\tapp--feat\tfeat/x\trunning\t9000-9004\ton\thttp://feat.app.wt.localhost:8088"; got != want {
		t.Errorf("row:\n got %q\nwant %q", got, want)
	}
	if got, want := lines[4], "app\tmain\tmain\t-\t-\toff\t-"; got != want {
		t.Errorf("empty values must be '-':\n got %q\nwant %q", got, want)
	}
	for _, l := range lines[2:] {
		if n := strings.Count(l, "\t"); n != len(TSVHeader)-1 {
			t.Errorf("%d tabs in %q", n, l)
		}
		if strings.Contains(l, "\x1b[") {
			t.Errorf("TSV must not contain ANSI codes: %q", l)
		}
	}
}

func TestWriteTSV_SanitizesTabs(t *testing.T) {
	in := sampleInput()
	in.Rows = in.Rows[:1]
	in.Rows[0].Branch = "a\tb\nc"
	rep := Build(in)
	var buf bytes.Buffer
	_ = WriteTSV(&buf, &rep)
	row := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")[3]
	if strings.Count(row, "\t") != len(TSVHeader)-1 || !strings.Contains(row, "a b c") {
		t.Errorf("row %q", row)
	}
}

func TestWriteTSV_DaemonStopped(t *testing.T) {
	in := sampleInput()
	in.Daemon = Daemon{}
	rep := Build(in)
	var buf bytes.Buffer
	_ = WriteTSV(&buf, &rep)
	if !strings.HasPrefix(buf.String(), "# daemon=stopped\n") {
		t.Errorf("got %q", buf.String())
	}
}

func TestWriteJSON(t *testing.T) {
	rep := Build(sampleInput())
	var buf bytes.Buffer
	if err := WriteJSON(&buf, &rep); err != nil {
		t.Fatal(err)
	}
	var got Report
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(got.Worktrees) != 5 || got.Worktrees[0].Services[0].Proc != "air" || !got.Worktrees[3].Services[0].Unhealthy {
		t.Errorf("round trip: %+v", got.Worktrees)
	}
}

func TestWriteRich(t *testing.T) {
	rep := Build(sampleInput())

	var plain bytes.Buffer
	if err := WriteRich(&plain, &rep, false); err != nil {
		t.Fatal(err)
	}
	s := plain.String()
	if strings.Contains(s, "\x1b[") {
		t.Error("color=false must not emit ANSI codes")
	}
	for _, want := range []string{"wt daemon: running (pid 4242)", "proxy: 127.0.0.1:8088", "\napp\n", "\nlib\n", "● running", "✗ stale", "┌", "└"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	// Every row of a table has the same display width.
	var widths []int
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "  │") || strings.HasPrefix(l, "  ┌") {
			widths = append(widths, displayWidth(l))
		}
		if l == "" || strings.HasPrefix(l, "lib") {
			break
		}
	}
	for _, w := range widths {
		if w != widths[0] {
			t.Errorf("ragged table widths: %v", widths)
			break
		}
	}

	var colored bytes.Buffer
	_ = WriteRich(&colored, &rep, true)
	if !strings.Contains(colored.String(), ansiGreen+"● running"+ansiReset) {
		t.Error("color=true must color the running state")
	}
}

func TestDisplayWidth(t *testing.T) {
	if displayWidth("abc") != 3 || displayWidth("日本") != 4 || displayWidth("● run") != 5 {
		t.Errorf("widths: %d %d %d", displayWidth("abc"), displayWidth("日本"), displayWidth("● run"))
	}
}
