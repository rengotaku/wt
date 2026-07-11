package procstats

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestParseStat(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		wantPGID      int
		wantRSS       uint64
		wantStarttime uint64
		wantOK        bool
	}{
		{
			name:          "normal",
			line:          "123 (foo) S 1 200 300 0 -1 4194560 100 0 0 0 10 20 0 0 20 0 1 0 1000 50000 200 18446744073709551615 1 1 0 0 0 0 0 0 0 0 0 0 0 0 0",
			wantPGID:      200,  // pgrp is 5th field: 123=1, (foo)=2, S=3, 1=4, 200=5
			wantRSS:       200,  // rss is 24th field: ... fields[21] in remainder
			wantStarttime: 1000, // starttime is 22nd field: ... fields[19] in remainder
			wantOK:        true,
		},
		{
			name:          "comm with parens and space",
			line:          "123 (my proc (old)) S 1 200 300 0 -1 4194560 100 0 0 0 10 20 0 0 20 0 1 0 1000 50000 200 18446744073709551615 1 1 0 0 0 0 0 0 0 0 0 0 0 0 0",
			wantPGID:      200,
			wantRSS:       200,
			wantStarttime: 1000,
			wantOK:        true,
		},
		{
			name:   "fields short",
			line:   "123 (foo) S 1 200",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pgid, rss, starttime, ok := parseStat(tt.line)
			if ok != tt.wantOK {
				t.Errorf("got ok=%v, want %v", ok, tt.wantOK)
			}
			if ok {
				if pgid != tt.wantPGID {
					t.Errorf("got pgid=%v, want %v", pgid, tt.wantPGID)
				}
				if rss != tt.wantRSS {
					t.Errorf("got rss=%v, want %v", rss, tt.wantRSS)
				}
				if starttime != tt.wantStarttime {
					t.Errorf("got starttime=%v, want %v", starttime, tt.wantStarttime)
				}
			}
		})
	}
}

func TestSnapshot(t *testing.T) {
	dir := t.TempDir()

	// mock uptime
	err := os.WriteFile(filepath.Join(dir, "uptime"), []byte("1000.50 2000.00"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	writeStat := func(pid, stat string) {
		pdir := filepath.Join(dir, pid)
		_ = os.Mkdir(pdir, 0o755)
		_ = os.WriteFile(filepath.Join(pdir, "stat"), []byte(stat), 0o644)
	}

	// pgid 200, starttime 10000 (100sec), rss 100
	writeStat("123", "123 (foo) S 1 200 300 0 -1 0 0 0 0 0 0 0 0 0 0 0 1 0 10000 0 100")
	// pgid 200, starttime 20000 (200sec), rss 50
	writeStat("124", "124 (bar) S 1 200 300 0 -1 0 0 0 0 0 0 0 0 0 0 0 1 0 20000 0 50")
	// pgid 300, starttime 90000 (900sec), rss 10
	writeStat("125", "125 (baz) S 1 300 300 0 -1 0 0 0 0 0 0 0 0 0 0 0 1 0 90000 0 10")

	// Ignore non-pid dir
	_ = os.Mkdir(filepath.Join(dir, "notpid"), 0o755)

	stats, err := Snapshot(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(stats) != 2 {
		t.Fatalf("got %d stats, want 2", len(stats))
	}

	pageSize := uint64(os.Getpagesize())

	s200 := stats[200]
	if s200.Procs != 2 {
		t.Errorf("pgid 200 procs=%v, want 2", s200.Procs)
	}
	if s200.RSSBytes != 150*pageSize {
		t.Errorf("pgid 200 rss=%v, want %v", s200.RSSBytes, 150*pageSize)
	}
	// uptime = 1000 - 10000/100 = 900 (the older one)
	if s200.UptimeSec != 900 {
		t.Errorf("pgid 200 uptime=%v, want 900", s200.UptimeSec)
	}

	s300 := stats[300]
	if s300.Procs != 1 {
		t.Errorf("pgid 300 procs=%v, want 1", s300.Procs)
	}
	if s300.RSSBytes != 10*pageSize {
		t.Errorf("pgid 300 rss=%v, want %v", s300.RSSBytes, 10*pageSize)
	}
	// uptime = 1000 - 90000/100 = 100
	if s300.UptimeSec != 100 {
		t.Errorf("pgid 300 uptime=%v, want 100", s300.UptimeSec)
	}
}

func TestInotifyInstances(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "sys", "fs", "inotify"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sys", "fs", "inotify", "max_user_instances"), []byte("1024\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	writePid := func(pid string, fdTargets []string) {
		fddir := filepath.Join(dir, pid, "fd")
		if err := os.MkdirAll(fddir, 0o755); err != nil {
			t.Fatal(err)
		}
		for i, target := range fdTargets {
			if err := os.Symlink(target, filepath.Join(fddir, strconv.Itoa(i))); err != nil {
				t.Fatal(err)
			}
		}
	}

	// pid 100 (own uid): 2 inotify fd + 1 unrelated fd
	writePid("100", []string{"anon_inode:inotify", "anon_inode:inotify", "socket:[12345]"})
	// pid 200 (own uid): 1 inotify fd
	writePid("200", []string{"anon_inode:inotify"})

	// non-pid dir must be ignored
	if err := os.Mkdir(filepath.Join(dir, "notpid"), 0o755); err != nil {
		t.Fatal(err)
	}

	uid := os.Getuid()

	used, maxInstances := InotifyInstances(dir, uid)
	if maxInstances != 1024 {
		t.Errorf("maxInstances=%v, want 1024", maxInstances)
	}
	if used != 3 {
		t.Errorf("used=%v, want 3", used)
	}

	// different uid must not match any pid dir (all temp dirs are owned by the
	// current user), so the count stays 0.
	usedOther, _ := InotifyInstances(dir, uid+1)
	if usedOther != 0 {
		t.Errorf("usedOther=%v, want 0", usedOther)
	}
}
