package ports

import "testing"

func TestPortStateUnhealthy(t *testing.T) {
	tests := []struct {
		name string
		st   PortState
		want bool
	}{
		{
			// Listening = reachable, healthy.
			name: "listening",
			st:   PortState{Running: true, Listening: true},
			want: false,
		},
		{
			// Declared headless worker binds no port by design — benign.
			name: "headless worker",
			st:   PortState{Running: true, Listening: false, Headless: true},
			want: false,
		},
		{
			// Expected to LISTEN but alive-without-a-socket = build failed / crashed.
			name: "running but not listening, not headless",
			st:   PortState{Running: true, Listening: false, Headless: false},
			want: true,
		},
		{
			// Not running at all is "stopped", not "unhealthy".
			name: "stopped",
			st:   PortState{Running: false, Listening: false},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.st.Unhealthy(); got != tt.want {
				t.Errorf("Unhealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}
