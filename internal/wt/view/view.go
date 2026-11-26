// Package view assembles what the read commands (`wt repo ls|show`,
// `wt worktree ls|show`, `wt daemon status`) print, on top of the machine
// state that package status collects.
package view

import (
	"path/filepath"
	"sort"
	"strings"

	"wt/internal/cli/clierr"
	"wt/internal/wt/core"
	"wt/internal/wt/devserver"
	"wt/internal/wt/ref"
	"wt/internal/wt/status"
)

// RepoSummary is one line of `wt repo ls`.
type RepoSummary struct {
	Repo      string `json:"repo"`
	Path      string `json:"path"`
	Worktrees int    `json:"worktrees"`
	Running   int    `json:"running"`
	Unhealthy int    `json:"unhealthy"`
	Idle      int    `json:"idle"`
	Stale     int    `json:"stale"`
	AutoStart int    `json:"autostart"`
}

// RepoSummaries counts each container's worktrees by state. containers lists
// every wt-managed container so repos are shown even with nothing allocated.
func RepoSummaries(containers []string, wts []status.Worktree) []RepoSummary {
	byPath := map[string]*RepoSummary{}
	out := make([]RepoSummary, 0, len(containers))
	for _, c := range containers {
		out = append(out, RepoSummary{Repo: filepath.Base(c), Path: c})
	}
	for i := range out {
		byPath[out[i].Path] = &out[i]
	}
	for i := range wts {
		w := &wts[i]
		s, ok := byPath[filepath.Dir(w.Path)]
		if !ok {
			continue
		}
		s.Worktrees++
		switch w.State {
		case status.StateRunning:
			s.Running++
		case status.StateUnhealthy:
			s.Unhealthy++
		case status.StateIdle:
			s.Idle++
		case status.StateStale:
			s.Stale++
		}
		if w.AutoStart {
			s.AutoStart++
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// RepoDetail is `wt repo show`.
type RepoDetail struct {
	Repo              string   `json:"repo"`
	Path              string   `json:"path"`
	Main              string   `json:"main"`
	DefaultBranch     string   `json:"default_branch"`
	BranchPattern     string   `json:"branch_pattern"`
	SymlinkCandidates []string `json:"symlink_candidates"`
	DevServices       []string `json:"dev_services"`
	AIPolicy          string   `json:"ai_policy"`
	Worktrees         int      `json:"worktrees"`
	// Services are the repo-wide default dev services in full.
	Services []core.DevService `json:"services"`
}

// LoadRepo reads the repository's configuration.
func LoadRepo(repo string) (RepoDetail, error) {
	if err := core.ValidateRepoName(repo); err != nil {
		return RepoDetail{}, clierr.Usagef("%v", err)
	}
	container, err := core.FindContainer(repo)
	if err != nil {
		return RepoDetail{}, clierr.NotFoundf("リポジトリが見つかりません: %s", repo)
	}
	return LoadRepoAt(container)
}

// LoadRepoAt is LoadRepo for a container path.
func LoadRepoAt(container string) (RepoDetail, error) {
	repo := filepath.Base(container)
	cfg, err := core.LoadConfig(container)
	if err != nil {
		return RepoDetail{}, err
	}
	entries, err := core.LoadEntries(container)
	if err != nil {
		return RepoDetail{}, err
	}
	mainDir, _ := core.ResolveMain(container)
	d := RepoDetail{
		Repo:              repo,
		Path:              container,
		Main:              mainDir,
		DefaultBranch:     defaultBranch(mainDir),
		BranchPattern:     cfg.BranchPattern,
		SymlinkCandidates: nonNil(cfg.SymlinkCandidates),
		DevServices:       []string{},
		AIPolicy:          cfg.AIPolicy,
		Worktrees:         len(entries),
		Services:          []core.DevService{},
	}
	for _, s := range cfg.DevServices {
		d.DevServices = append(d.DevServices, s.Name)
		d.Services = append(d.Services, s)
	}
	return d, nil
}

// ServiceDetail is one dev service of `wt worktree show`.
type ServiceDetail struct {
	Name      string `json:"name"`
	Port      int    `json:"port"`
	State     string `json:"state"` // running / unhealthy / stopped
	PID       int    `json:"pid,omitempty"`
	Proc      string `json:"proc,omitempty"`
	Headless  bool   `json:"headless"`
	Domain    bool   `json:"domain"` // reachable by name through the proxy
	Log       string `json:"log"`
	Listening bool   `json:"listening"`
}

// WorktreeDetail is `wt worktree show`.
type WorktreeDetail struct {
	status.Worktree
	Issue    string          `json:"issue"`
	Created  string          `json:"created"`
	Services []ServiceDetail `json:"services"`
	// ConfigError is why the dev service config could not be read ("" when
	// it could). Services then lack the configured ones and domain.
	ConfigError string `json:"config_error"`
}

// LoadWorktree finds r in the collected report and adds the entry metadata
// and per-service details.
func LoadWorktree(r ref.Ref, rep *status.Report) (WorktreeDetail, error) {
	var w *status.Worktree
	for i := range rep.Worktrees {
		if rep.Worktrees[i].Path == r.Path {
			w = &rep.Worktrees[i]
			break
		}
	}
	if w == nil {
		return WorktreeDetail{}, clierr.NotFoundf("worktree が見つかりません: %s/%s", r.Repo, r.Worktree)
	}
	entries, err := core.LoadEntries(r.Container)
	if err != nil {
		return WorktreeDetail{}, err
	}
	e := entries[r.Worktree]
	d := WorktreeDetail{Worktree: *w, Issue: e.Issue, Created: e.Created, Services: []ServiceDetail{}}
	cfg, _, cfgErr := devserver.EffectiveConfig(r.Path)
	if cfgErr != nil {
		d.ConfigError = cfgErr.Error()
	}
	// Names recorded when the services were started win over the current
	// config: the config may have been reordered since. #189
	recorded := map[int]string{}
	for _, rs := range devserver.Recorded(r.Path) {
		recorded[rs.Port] = rs.Name
	}
	for i, s := range w.Services {
		// A port block is wider than the configured services; skip the
		// unused slots unless something is actually on them.
		if i >= len(cfg.Services) && !s.Listening && !s.Running {
			continue
		}
		sd := ServiceDetail{Port: s.Port, PID: s.PID, Proc: s.Proc, Headless: s.Headless, Listening: s.Listening}
		if name, ok := recorded[s.Port]; ok {
			sd.Name = name
		} else if i < len(cfg.Services) {
			sd.Name = cfg.Services[i].Name
		}
		if sd.Name != "" {
			sd.Log = devserver.LogPath(r.Path, sd.Name)
			sd.Domain = domainOf(cfg.Services, sd.Name)
		}
		switch {
		case s.Unhealthy:
			sd.State = status.StateUnhealthy
		case s.Listening || s.Running:
			sd.State = status.StateRunning
		default:
			sd.State = "stopped"
		}
		d.Services = append(d.Services, sd)
	}
	// Configured services without a port yet (the worktree has no port block,
	// as main until its first `service up`) are still part of the worktree.
	listed := map[string]bool{}
	for _, sd := range d.Services {
		listed[sd.Name] = true
	}
	for _, c := range cfg.Services {
		if listed[c.Name] {
			continue
		}
		d.Services = append(d.Services, ServiceDetail{Name: c.Name, State: "stopped", Headless: c.Headless,
			Domain: c.Domain, Log: devserver.LogPath(r.Path, c.Name)})
	}
	return d, nil
}

func domainOf(svcs []devserver.Service, name string) bool {
	for _, s := range svcs {
		if s.Name == name {
			return s.Domain
		}
	}
	return false
}

// defaultBranch reads origin/HEAD ("" when unknown). The main dir's name
// (main / master) is only where it is checked out, not the branch name.
func defaultBranch(mainDir string) string {
	if mainDir == "" {
		return ""
	}
	v, err := core.GitOutput(mainDir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(v), "origin/")
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
