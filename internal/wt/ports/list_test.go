package ports

import "testing"

func TestLiveCell(t *testing.T) {
	tests := []struct {
		name   string
		states []PortState
		want   string
	}{
		{
			name:   "unallocated",
			states: nil,
			want:   "—",
		},
		{
			name: "nothing up",
			states: []PortState{
				{Port: 9160},
				{Port: 9161},
			},
			want: "idle",
		},
		{
			name: "listening service",
			states: []PortState{
				{Port: 9160, Listening: true, Proc: "python", PID: 123},
			},
			want: "9160 python(123)",
		},
		{
			// A declared-headless worker is Running but not Listening: it must
			// appear (not "idle") and be marked with "*" since it binds no port
			// by design.
			name: "headless running worker",
			states: []PortState{
				{Port: 9160, Listening: true, Proc: "python", PID: 123},
				{Port: 9161, Running: true, Headless: true, Proc: "worker", PID: 456},
			},
			want: "9160 python(123), 9161 worker(456)*",
		},
		{
			// A service expected to LISTEN but Running-without-a-socket (not
			// declared headless) is unhealthy — marked "!" to flag it, not "*".
			name: "unhealthy running-but-not-listening service",
			states: []PortState{
				{Port: 9160, Running: true, Proc: "go", PID: 789},
			},
			want: "9160 go(789)!",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := liveCell(tt.states); got != tt.want {
				t.Errorf("liveCell() = %q, want %q", got, tt.want)
			}
		})
	}
}
