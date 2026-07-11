// Package procstats reads per-process-group memory usage from /proc.
package procstats

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// GroupStat aggregates the live processes of one process group.
type GroupStat struct {
	Procs     int    // number of live processes in the group
	RSSBytes  uint64 // sum of resident set sizes (shared pages double-counted)
	UptimeSec int64  // seconds since the oldest process in the group started
}

// parseStat parses a single /proc/<pid>/stat line.
func parseStat(line string) (pgid int, rssPages, starttimeTicks uint64, ok bool) {
	// comm field can contain spaces and parentheses, e.g. "123 (my proc (old)) S ..."
	// We must find the *last* ')' and split everything after it.
	idx := strings.LastIndexByte(line, ')')
	if idx == -1 || idx+2 >= len(line) {
		return 0, 0, 0, false
	}
	// remainder starts after ") "
	remainder := line[idx+2:]
	fields := strings.Fields(remainder)
	// After "(comm) ", the first field is state (index 0).
	// pgid is the 3rd field in remainder -> index 2.
	// starttime is the 20th field in remainder -> index 19.
	// rss is the 22nd field in remainder -> index 21.
	if len(fields) < 22 {
		return 0, 0, 0, false
	}
	var err error
	pgid, err = strconv.Atoi(fields[2])
	if err != nil {
		return 0, 0, 0, false
	}
	starttimeTicks, err = strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	rssPages, err = strconv.ParseUint(fields[21], 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	return pgid, rssPages, starttimeTicks, true
}

// Snapshot scans procRoot (normally "/proc") once and returns stats keyed by
// process group ID. Unreadable entries are skipped (processes may vanish
// mid-scan).
func Snapshot(procRoot string) (map[int]GroupStat, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, err
	}
	uptimeData, err := os.ReadFile(filepath.Join(procRoot, "uptime"))
	if err != nil {
		return nil, err
	}
	uptimeFields := strings.Fields(string(uptimeData))
	if len(uptimeFields) < 1 {
		return nil, fmt.Errorf("invalid uptime file")
	}
	sysUptime, err := strconv.ParseFloat(uptimeFields[0], 64)
	if err != nil {
		return nil, err
	}

	pageSize := uint64(os.Getpagesize())
	stats := make(map[int]GroupStat)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		statData, err := os.ReadFile(filepath.Join(procRoot, entry.Name(), "stat"))
		if err != nil {
			continue
		}
		pgid, rssPages, starttimeTicks, ok := parseStat(string(statData))
		if !ok {
			continue
		}

		uptimeSec := int64(sysUptime) - int64(starttimeTicks/100)
		if uptimeSec < 0 {
			uptimeSec = 0
		}
		rssBytes := rssPages * pageSize

		gs := stats[pgid]
		gs.Procs++
		gs.RSSBytes += rssBytes
		// Keep the oldest uptime
		if gs.Procs == 1 || uptimeSec > gs.UptimeSec {
			gs.UptimeSec = uptimeSec
		}
		stats[pgid] = gs
	}
	return stats, nil
}

// InotifyInstances は uid のプロセスが保持する inotify インスタンス数と
// システム上限（max_user_instances）を返す。上限が読めない場合 maxInstances は 0 のまま。
func InotifyInstances(procRoot string, uid int) (used, maxInstances int) {
	if b, err := os.ReadFile(filepath.Join(procRoot, "sys", "fs", "inotify", "max_user_instances")); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
			maxInstances = n
		}
	}
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue // pid ディレクトリのみ
		}
		pdir := filepath.Join(procRoot, entry.Name())
		var st syscall.Stat_t
		if err := syscall.Stat(pdir, &st); err != nil {
			continue // プロセスがスキャン中に消えた場合など
		}
		if int(st.Uid) != uid {
			continue
		}
		fds, err := os.ReadDir(filepath.Join(pdir, "fd"))
		if err != nil {
			continue // 他ユーザー所有プロセスの fd は権限で読めない
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(pdir, "fd", fd.Name()))
			if err == nil && target == "anon_inode:inotify" {
				used++
			}
		}
	}
	return
}
