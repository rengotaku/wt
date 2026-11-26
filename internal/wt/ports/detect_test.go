package ports

import (
	"net"
	"os/exec"
	"testing"
	"time"
)

func TestPortFromLocalAddr(t *testing.T) {
	tests := []struct {
		addr     string
		wantPort int
		wantOK   bool
	}{
		{addr: "0.0.0.0:9000", wantPort: 9000, wantOK: true},
		{addr: "127.0.0.1:9001", wantPort: 9001, wantOK: true},
		{addr: "*:9002", wantPort: 9002, wantOK: true},
		{addr: "[::]:9003", wantPort: 9003, wantOK: true},
		{addr: "noport", wantPort: 0, wantOK: false},
		{addr: "1.2.3.4:", wantPort: 0, wantOK: false},
	}
	for _, tt := range tests {
		gotPort, gotOK := portFromLocalAddr(tt.addr)
		if gotPort != tt.wantPort || gotOK != tt.wantOK {
			t.Errorf("portFromLocalAddr(%q) = (%d, %v), want (%d, %v)",
				tt.addr, gotPort, gotOK, tt.wantPort, tt.wantOK)
		}
	}
}

func TestParseSS(t *testing.T) {
	out := `LISTEN 0 2048 0.0.0.0:8000 0.0.0.0:* users:(("python",pid=3000870,fd=4))
LISTEN 0 4096 127.0.0.1:9000 0.0.0.0:* users:(("air",pid=12345,fd=7))
LISTEN 0 511 [::]:9001 [::]:* users:(("node",pid=678,fd=20))
LISTEN 0 128 127.0.0.1:5174 0.0.0.0:* users:(("vite",pid=999,fd=3))`

	got := parseSS(out, 9000, 9999)

	// Out-of-band ports (8000, 5174) must be excluded.
	if _, ok := got[8000]; ok {
		t.Error("8000 is out of band and should be excluded")
	}
	if _, ok := got[5174]; ok {
		t.Error("5174 is out of band and should be excluded")
	}
	if len(got) != 2 {
		t.Fatalf("got %d in-band listeners, want 2: %+v", len(got), got)
	}
	if l := got[9000]; l.PID != 12345 || l.Proc != "air" {
		t.Errorf("9000 = %+v, want pid=12345 proc=air", l)
	}
	if l := got[9001]; l.PID != 678 || l.Proc != "node" {
		t.Errorf("9001 = %+v, want pid=678 proc=node", l)
	}
}

func TestParseEstablished(t *testing.T) {
	out := `0      0      127.0.0.1:9000     127.0.0.1:45321
0      0      127.0.0.1:45321    127.0.0.1:9001
0      0      192.168.1.2:8000   10.0.0.1:54321
0      0      [::1]:9002         [::1]:33333
just_one_column
`
	got := parseEstablished(out, 9000, 9999)

	if len(got) != 3 {
		t.Fatalf("got %d in-band ports, want 3: %+v", len(got), got)
	}
	if !got[9000] {
		t.Error("missing 9000 (local side)")
	}
	if !got[9001] {
		t.Error("missing 9001 (peer side)")
	}
	if !got[9002] {
		t.Error("missing 9002 (IPv6 local)")
	}
	if got[8000] {
		t.Error("included 8000 (out of band)")
	}
}

func TestParseLsof(t *testing.T) {
	out := "p100\ncnode\nf20\nn*:9000\nf21\nn[::1]:9000\np200\ncair\nf3\nn127.0.0.1:9001\np300\ncother\nf4\nn*:8088\n"
	got := parseLsof(out, 9000, 9004)
	if len(got) != 2 {
		t.Fatalf("got %d listeners, want 2: %+v", len(got), got)
	}
	if l := got[9000]; l.PID != 100 || l.Proc != "node" {
		t.Errorf("9000: %+v, want pid 100 node (first socket wins)", l)
	}
	if l := got[9001]; l.PID != 200 || l.Proc != "air" {
		t.Errorf("9001: %+v, want pid 200 air", l)
	}
	if _, ok := got[8088]; ok {
		t.Error("8088 is outside the band")
	}
}

func TestParseLsofEstablished(t *testing.T) {
	out := "p1\nf5\nn127.0.0.1:9000->127.0.0.1:54321\nf6\nn127.0.0.1:54400->127.0.0.1:9003\n" +
		"p2\nf7\nn[::1]:9002->[::1]:60000\nf8\nn192.168.1.2:55314->52.198.188.161:443\nf9\nn*:9004\n"
	got := parseLsofEstablished(out, 9000, 9004)
	for _, p := range []int{9000, 9002, 9003} {
		if !got[p] {
			t.Errorf("port %d: want established", p)
		}
	}
	if got[9004] {
		t.Error("9004 is a LISTEN-style line without a peer, not established")
	}
	if len(got) != 3 {
		t.Errorf("got %v, want exactly 9000/9002/9003", got)
	}
}

// End to end on the real machine: an open TCP connection to a port must show
// up as established (ss on Linux, lsof on macOS). #185
func TestEstablished_RealConnection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			defer func() { _ = c.Close() }()
			_, _ = c.Read(make([]byte, 1)) // hold the connection open
		}
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	port := ln.Addr().(*net.TCPAddr).Port
	if _, err := exec.LookPath("ss"); err != nil {
		if _, err := exec.LookPath("lsof"); err != nil {
			t.Skip("neither ss nor lsof is available")
		}
	}
	var got map[int]bool
	for i := 0; i < 20 && !got[port]; i++ {
		got, _ = Established(port, port)
		time.Sleep(50 * time.Millisecond)
	}
	if !got[port] {
		t.Errorf("port %d has an open connection but was not reported as established", port)
	}
}
