package handler

import (
	"net/http"
	"os"

	"wt/internal/wt/devserver"
	"wt/internal/wt/procstats"
	"wt/internal/wt/settings"
	"wt/internal/wt/tree"
)

// procRoot は procstats が読む /proc のルート。実運用では常に "/proc"。
const procRoot = "/proc"

type ProcessStatsResponse struct {
	WarnBytes        uint64                 `json:"warn_bytes"`
	DangerBytes      uint64                 `json:"danger_bytes"`
	TotalRSSBytes    uint64                 `json:"total_rss_bytes"`
	InotifyInstances int                    `json:"inotify_instances"`
	InotifyMax       int                    `json:"inotify_max"`
	Items            []WorktreeProcessStats `json:"items"`
}

type WorktreeProcessStats struct {
	Repo          string               `json:"repo"`
	WtName        string               `json:"wt_name"`
	TotalRSSBytes uint64               `json:"total_rss_bytes"`
	Level         string               `json:"level"`
	Services      []ServiceProcessStat `json:"services"`
}

type ServiceProcessStat struct {
	Name      string `json:"name"`
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	Alive     bool   `json:"alive"`
	Procs     int    `json:"procs"`
	RSSBytes  uint64 `json:"rss_bytes"`
	UptimeSec int64  `json:"uptime_sec"`
}

func calcLevel(total, warn, danger uint64) string {
	if total >= danger {
		return "danger"
	}
	if total >= warn {
		return "warn"
	}
	return "ok"
}

func (h *Handler) GetProcessStats(w http.ResponseWriter, r *http.Request) {
	st := settings.Load()
	warnBytes := uint64(st.ProcessStats.WarnMB) * 1024 * 1024
	dangerBytes := uint64(st.ProcessStats.DangerMB) * 1024 * 1024

	resp := ProcessStatsResponse{
		WarnBytes:   warnBytes,
		DangerBytes: dangerBytes,
		Items:       []WorktreeProcessStats{}, // initialized so it renders as [] not null
	}

	entries := tree.Entries()

	var snapshot map[int]procstats.GroupStat
	snapshotFetched := false

	for i := range entries {
		entry := &entries[i]
		recorded := devserver.Recorded(entry.Path)
		if len(recorded) == 0 {
			continue
		}

		if !snapshotFetched {
			snapshot, _ = procstats.Snapshot(procRoot)
			snapshotFetched = true
		}

		wtStats := WorktreeProcessStats{
			Repo:     entry.Repo,
			WtName:   entry.WtName,
			Services: []ServiceProcessStat{}, // initialize to []
		}

		for _, rec := range recorded {
			svcStat := ServiceProcessStat{
				Name: rec.Name,
				PID:  rec.PID,
				Port: rec.Port,
			}
			if gs, ok := snapshot[rec.PID]; ok && gs.Procs > 0 {
				svcStat.Alive = true
				svcStat.Procs = gs.Procs
				svcStat.RSSBytes = gs.RSSBytes
				svcStat.UptimeSec = gs.UptimeSec
				wtStats.TotalRSSBytes += gs.RSSBytes
			} else {
				svcStat.Alive = false
				svcStat.Procs = 0
				svcStat.RSSBytes = 0
				svcStat.UptimeSec = 0
			}
			wtStats.Services = append(wtStats.Services, svcStat)
		}

		wtStats.Level = calcLevel(wtStats.TotalRSSBytes, warnBytes, dangerBytes)
		resp.Items = append(resp.Items, wtStats)
		resp.TotalRSSBytes += wtStats.TotalRSSBytes
	}

	resp.InotifyInstances, resp.InotifyMax = procstats.InotifyInstances(procRoot, os.Getuid())

	jsonOK(w, resp)
}
