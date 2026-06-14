package ports

import "testing"

func TestClassify(t *testing.T) {
	listeners := map[int]Listener{
		8000: {Port: 8000, PID: 111, Proc: "python"}, // foreign squatter
		9000: {Port: 9000, PID: 222, Proc: "air"},    // wt-managed
		9001: {Port: 9001, PID: 333, Proc: "node"},   // wt-managed
	}
	owners := map[int]string{
		9000: "wt/main",
		9001: "wt/main",
		9005: "tourheat/feat-x", // allocated but not listening → not in result
	}

	rows := classify(listeners, owners)

	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	// sorted by port: 8000, 9000, 9001
	if rows[0].Port != 8000 || rows[1].Port != 9000 || rows[2].Port != 9001 {
		t.Fatalf("not sorted by port: %+v", rows)
	}
	// 8000 is foreign
	if rows[0].Managed || rows[0].Owner != "" {
		t.Errorf("8000 should be foreign, got %+v", rows[0])
	}
	if rows[0].Proc != "python" || rows[0].PID != 111 {
		t.Errorf("8000 proc/pid wrong: %+v", rows[0])
	}
	// 9000 is wt-managed
	if !rows[1].Managed || rows[1].Owner != "wt/main" {
		t.Errorf("9000 should be wt-managed, got %+v", rows[1])
	}
}
