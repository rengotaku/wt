// Package doctor finds and repairs inconsistencies that span repositories,
// worktrees and port blocks: a main folder on another branch, registry entries
// whose worktree is gone, worktrees without a port block, and broken repo-wide
// dev service settings.
package doctor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"wt/internal/wt/core"
	"wt/internal/wt/devserver"
	"wt/internal/wt/ports"
	"wt/internal/wt/repo"
)

// Checks. Fixed words so callers can match on them.
const (
	CheckMetadata        = "metadata"
	CheckMainBranch      = "main_branch"
	CheckStalePort       = "stale_port"
	CheckUnallocatedPort = "unallocated_port"
	CheckServiceConfig   = "service_config"
)

// Statuses. found: detected, not repaired (no --fix). fixed: repaired.
// fix_failed: repair attempted and failed. manual: no automatic repair.
const (
	StatusFound     = "found"
	StatusFixed     = "fixed"
	StatusFixFailed = "fix_failed"
	StatusManual    = "manual"
)

// Finding is one inconsistency.
type Finding struct {
	Check    string `json:"check"`
	Repo     string `json:"repo"`
	Worktree string `json:"worktree"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
}

// Unresolved reports whether the finding still needs attention.
func (f *Finding) Unresolved() bool { return f.Status != StatusFixed }

// deps are the operations doctor performs, replaceable in tests.
type deps struct {
	currentBranch func(mainDir string) (string, error)
	rescueMain    func(out io.Writer, repoName string) error
	stale         func() ([]ports.Allocation, error)
	deleteEntry   func(container, name string) error
	ensureBase    func(container, name string) (int, error)
}

var defaultDeps = deps{
	currentBranch: func(mainDir string) (string, error) {
		return core.GitOutput(mainDir, "rev-parse", "--abbrev-ref", "HEAD")
	},
	rescueMain:  repo.RescueMain,
	stale:       ports.Stale,
	deleteEntry: core.DeleteEntry,
	ensureBase:  ports.EnsureBase,
}

// Run checks every container, or only containers when it is not empty. With
// fix, repairable findings are repaired; repair progress goes to progress.
func Run(progress io.Writer, containers []string, fix bool) ([]Finding, error) {
	return run(&defaultDeps, progress, containers, fix)
}

func run(d *deps, progress io.Writer, containers []string, fix bool) ([]Finding, error) {
	stale, err := d.stale()
	if err != nil {
		return nil, fmt.Errorf("ポート割当を読めません: %w", err)
	}
	var all []Finding
	for _, c := range containers {
		all = append(all, checkContainer(d, progress, c, stale, fix)...)
	}
	return all, nil
}

func checkContainer(d *deps, progress io.Writer, container string, stale []ports.Allocation, fix bool) []Finding {
	name := filepath.Base(container)
	entries, err := core.LoadEntries(container)
	if err != nil {
		return []Finding{{Check: CheckMetadata, Repo: name, Status: StatusManual,
			Detail: core.MetaFile(container) + " を読めない: " + err.Error()}}
	}
	// Stale entries go first: moving a branch off main creates a worktree
	// and an entry, possibly under the name of a stale one.
	fs := checkStale(d, container, stale, fix)
	if f, ok := checkMainBranch(d, progress, container, fix); ok {
		fs = append(fs, f)
	}
	fs = append(fs, checkUnallocated(d, container, entries, fix)...)
	if f, ok := checkServiceConfig(container); ok {
		fs = append(fs, f)
	}
	return fs
}

func checkMainBranch(d *deps, progress io.Writer, container string, fix bool) (Finding, bool) {
	name := filepath.Base(container)
	mainDir, mainName := core.ResolveMain(container)
	if mainDir == "" {
		return Finding{Check: CheckMainBranch, Repo: name, Status: StatusManual,
			Detail: "main / master のフォルダが無い"}, true
	}
	cur, err := d.currentBranch(mainDir)
	if err != nil {
		return Finding{Check: CheckMainBranch, Repo: name, Worktree: mainName, Status: StatusManual,
			Detail: "現在のブランチを取得できない: " + err.Error()}, true
	}
	if cur == mainName {
		return Finding{}, false
	}
	f := Finding{Check: CheckMainBranch, Repo: name, Worktree: mainName, Status: StatusFound,
		Detail: fmt.Sprintf("フォルダ %s にブランチ %s が checkout されている", mainName, cur)}
	if !fix {
		return f, true
	}
	if err := d.rescueMain(progress, name); err != nil {
		f.Status, f.Detail = StatusFixFailed, f.Detail+": "+err.Error()
		return f, true
	}
	f.Status = StatusFixed
	f.Detail = fmt.Sprintf("ブランチ %s を別の worktree に移し、フォルダ %s を %s に戻した", cur, mainName, mainName)
	return f, true
}

func checkStale(d *deps, container string, stale []ports.Allocation, fix bool) []Finding {
	var fs []Finding
	for i := range stale {
		a := &stale[i]
		if filepath.Dir(a.Path) != container {
			continue
		}
		// The list was taken before this run changed anything. An entry that
		// is no longer stale is not an inconsistency any more; never delete it.
		if fix && !stillStale(container, a) {
			continue
		}
		f := Finding{Check: CheckStalePort, Repo: a.Repo, Worktree: a.WtName, Status: StatusFound,
			Detail: "worktree のディレクトリが無いのにポート " + ports.RangeString(a.PortBase) + " を持っている"}
		if fix {
			if err := d.deleteEntry(container, a.WtName); err != nil {
				f.Status, f.Detail = StatusFixFailed, f.Detail+": "+err.Error()
			} else {
				f.Status, f.Detail = StatusFixed, "登録を消し、ポート "+ports.RangeString(a.PortBase)+" を空けた"
			}
		}
		fs = append(fs, f)
	}
	return fs
}

// stillStale reports whether a's entry is still registered with the same port
// block and its directory is still missing.
func stillStale(container string, a *ports.Allocation) bool {
	if isDir(a.Path) {
		return false
	}
	entries, err := core.LoadEntries(container)
	if err != nil {
		return false
	}
	e, ok := entries[a.WtName]
	return ok && e.PortBase == a.PortBase
}

func checkUnallocated(d *deps, container string, entries map[string]core.Entry, fix bool) []Finding {
	// Old entries may lack type "main"; the main folder name decides too.
	_, mainName := core.ResolveMain(container)
	names := make([]string, 0, len(entries))
	for n := range entries {
		if n == mainName {
			continue
		}
		e := entries[n]
		// The main worktree gets ports on demand (`service up`), so having
		// none is normal. `worktree add` allocates for every other worktree;
		// one without ports predates allocation. A directory-less entry is a
		// leftover, not a worktree to give ports to; allocating would only
		// create a stale_port finding.
		if e.PortBase == 0 && e.Type != "main" && isDir(filepath.Join(container, n)) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var fs []Finding
	for _, n := range names {
		f := Finding{Check: CheckUnallocatedPort, Repo: filepath.Base(container), Worktree: n, Status: StatusFound,
			Detail: "ポートが割り当てられていない"}
		if fix {
			if base, err := d.ensureBase(container, n); err != nil {
				f.Status, f.Detail = StatusFixFailed, f.Detail+": "+err.Error()
			} else {
				f.Status, f.Detail = StatusFixed, "ポート "+ports.RangeString(base)+" を割り当てた"
			}
		}
		fs = append(fs, f)
	}
	return fs
}

func checkServiceConfig(container string) (Finding, bool) {
	cfg, err := devserver.LoadRepoDefault(container)
	if err != nil {
		return Finding{Check: CheckServiceConfig, Repo: filepath.Base(container), Status: StatusManual,
			Detail: "dev サービスの設定を読めない: " + err.Error()}, true
	}
	// No services is a valid state (the repo has no dev servers).
	if len(cfg.Services) == 0 {
		return Finding{}, false
	}
	if err := cfg.Validate(); err != nil {
		return Finding{Check: CheckServiceConfig, Repo: filepath.Base(container), Status: StatusManual,
			Detail: err.Error() + "（wt repo config で直す）"}, true
	}
	return Finding{}, false
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
