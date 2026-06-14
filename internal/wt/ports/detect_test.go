package ports

import "testing"

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

	got := parseSS(out)

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
