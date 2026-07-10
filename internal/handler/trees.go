package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"wt/internal/wt/core"
	"wt/internal/wt/gc"
	"wt/internal/wt/tree"
)

// branchRe allows alphanumerics, hyphens, forward slashes, and dots.
// Rejects shell metacharacters, spaces, backticks, etc.
var branchRe = regexp.MustCompile(`^[a-zA-Z0-9/_.\-]+$`)

// issueRe allows GitHub issue URLs.
var issueRe = regexp.MustCompile(`^https://github\.com/[a-zA-Z0-9_.\-]+/[a-zA-Z0-9_.\-]+/issues/\d+$`)

const cacheTTL = 30 * time.Second

type treeItem struct {
	WtName    string `json:"wt_name"`
	Repo      string `json:"repo"`
	Label     string `json:"label"`
	Path      string `json:"path"`
	CreatedAt string `json:"created_at"`
	DiffCount int    `json:"diff_count"`
	HasTmux   bool   `json:"has_tmux"`
	IsMain    bool   `json:"is_main"`
	Branch    string `json:"branch"`
	Issue     string `json:"issue,omitempty"`
	Pinned    bool   `json:"pinned"`
	AutoStart bool   `json:"auto_start"`
}

func (h *Handler) getTmuxSessions() map[string]bool {
	const key = "tmux_sessions"
	if v, ok := h.cache.get(key); ok {
		return v.(map[string]bool)
	}
	sessions := map[string]bool{}
	out, err := exec.Command("tmux", "ls", "-F", "#{session_name}").Output()
	if err == nil {
		for _, s := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if s != "" {
				sessions[s] = true
			}
		}
	}
	h.cache.set(key, sessions, cacheTTL)
	return sessions
}

func (h *Handler) getDiffCount(path string) int {
	key := "diff_count:" + path
	if v, ok := h.cache.get(key); ok {
		return v.(int)
	}
	out, _ := exec.Command("git", "-C", path, "status", "--porcelain").Output()
	count := 0
	for _, line := range strings.Split(string(out), "\n") {
		if line != "" && !strings.HasPrefix(line, "??") {
			count++
		}
	}
	h.cache.set(key, count, cacheTTL)
	return count
}

func (h *Handler) getBranch(path string) string {
	key := "branch:" + path
	if v, ok := h.cache.get(key); ok {
		return v.(string)
	}
	out, _ := exec.Command("git", "-C", path, "branch", "--show-current").Output()
	branch := strings.TrimSpace(string(out))
	h.cache.set(key, branch, cacheTTL)
	return branch
}

func (h *Handler) ListTrees(w http.ResponseWriter, _ *http.Request) {
	entries := tree.Entries()
	tmuxSessions := h.getTmuxSessions()

	hiddenRepos := make(map[string]bool)
	for _, c := range core.ListContainers() {
		cfg, _ := core.LoadConfig(c)
		if cfg.Hidden {
			hiddenRepos[filepath.Base(c)] = true
		}
	}

	items := make([]treeItem, 0, len(entries))
	for i := range entries {
		if hiddenRepos[entries[i].Repo] {
			continue
		}
		items = append(items, treeItem{
			WtName:    entries[i].WtName,
			Repo:      entries[i].Repo,
			Label:     entries[i].Label,
			Path:      entries[i].Path,
			CreatedAt: entries[i].Created,
			IsMain:    entries[i].IsMain,
			Issue:     entries[i].Issue,
			Pinned:    entries[i].Pinned,
			AutoStart: entries[i].AutoStart,
		})
	}

	var wg sync.WaitGroup
	for i := range items {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			items[idx].DiffCount = h.getDiffCount(items[idx].Path)
			items[idx].HasTmux = tmuxSessions[items[idx].WtName]
			items[idx].Branch = h.getBranch(items[idx].Path)
		}(i)
	}
	wg.Wait()

	jsonOK(w, items)
}

type pinTreeRequest struct {
	Pinned bool `json:"pinned"`
}

// SetTreePin sets or clears the pinned flag on a worktree in .worktrees.json.
// Pinned worktrees float to the top of the list. Pinning has no effect on
// auto-serve or the idle reaper; see SetTreeAutoStart for that. The body is
// {"pinned": bool}; the call is idempotent.
func (h *Handler) SetTreePin(w http.ResponseWriter, r *http.Request) {
	repo := r.PathValue("repo")
	wtName := r.PathValue("wt")
	if !isKnownRepo(repo) {
		jsonErr(w, http.StatusBadRequest, "unknown repo: "+repo)
		return
	}
	var req pinTreeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	container, err := core.FindContainer(repo)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := core.LoadEntries(container)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	entry, ok := entries[wtName]
	if !ok {
		jsonErr(w, http.StatusNotFound, "worktree が見つかりません: "+wtName)
		return
	}
	entry.Pinned = req.Pinned
	if err := core.PutEntry(container, wtName, &entry); err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"pinned": req.Pinned})
}

type autoStartTreeRequest struct {
	AutoStart bool `json:"auto_start"`
}

// SetTreeAutoStart sets or clears the AutoStart flag on a worktree in
// .worktrees.json. AutoStart worktrees are auto-served when `wt web` starts
// and are candidates for the idle reaper; it is independent of Pinned. The
// body is {"auto_start": bool}; the call is idempotent.
func (h *Handler) SetTreeAutoStart(w http.ResponseWriter, r *http.Request) {
	repo := r.PathValue("repo")
	wtName := r.PathValue("wt")
	if !isKnownRepo(repo) {
		jsonErr(w, http.StatusBadRequest, "unknown repo: "+repo)
		return
	}
	var req autoStartTreeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	container, err := core.FindContainer(repo)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := core.LoadEntries(container)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	entry, ok := entries[wtName]
	if !ok {
		jsonErr(w, http.StatusNotFound, "worktree が見つかりません: "+wtName)
		return
	}
	entry.AutoStart = req.AutoStart
	if err := core.PutEntry(container, wtName, &entry); err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"auto_start": req.AutoStart})
}

type addTreeRequest struct {
	Repo       string `json:"repo"`
	Branch     string `json:"branch"`
	BranchType string `json:"type"`
	Dir        string `json:"dir"`
	IssueURL   string `json:"issue_url"`
}

func (h *Handler) AddTree(w http.ResponseWriter, r *http.Request) {
	var req addTreeRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.IssueURL != "" {
		// issue URL モード: issueRe のみ検証
		if !issueRe.MatchString(req.IssueURL) {
			jsonErr(w, http.StatusBadRequest, "invalid issue URL format")
			return
		}
		// repo は省略可(tree.Add が URL から補完)
	} else {
		// branch モード
		if req.Repo == "" || req.Branch == "" {
			jsonErr(w, http.StatusBadRequest, "repo and branch are required")
			return
		}
		if !isKnownRepo(req.Repo) {
			jsonErr(w, http.StatusBadRequest, "unknown repo: "+req.Repo)
			return
		}
		if !branchRe.MatchString(req.Branch) {
			jsonErr(w, http.StatusBadRequest, "invalid branch name")
			return
		}
		if req.BranchType != "" && !isKnownBranchType(req.BranchType) {
			jsonErr(w, http.StatusBadRequest, "invalid branch type: "+req.BranchType)
			return
		}
		if req.Dir != "" && !branchRe.MatchString(req.Dir) {
			jsonErr(w, http.StatusBadRequest, "invalid dir name")
			return
		}
	}

	opts := &tree.AddOptions{
		Repo:       req.Repo,
		Branch:     req.Branch,
		BranchType: req.BranchType,
		IssueURL:   req.IssueURL,
		Yes:        true,
		Symlink:    req.IssueURL != "",
	}
	if req.Dir != "" {
		opts.PositionDir = req.Dir
	}

	var buf bytes.Buffer
	result, err := tree.Add(strings.NewReader(""), &buf, opts)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	type addResp struct {
		Path   string `json:"path"`
		Output string `json:"output"`
	}
	resp := addResp{Output: buf.String()}
	if result != nil {
		resp.Path = result.WorktreePath
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, resp)
}

type deleteTreeRequest struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Force  bool   `json:"force"`
}

func (h *Handler) DeleteTree(w http.ResponseWriter, r *http.Request) {
	var req deleteTreeRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Repo == "" || req.Branch == "" {
		jsonErr(w, http.StatusBadRequest, "repo and branch are required")
		return
	}
	if !isKnownRepo(req.Repo) {
		jsonErr(w, http.StatusBadRequest, "unknown repo: "+req.Repo)
		return
	}
	if !branchRe.MatchString(req.Branch) {
		jsonErr(w, http.StatusBadRequest, "invalid branch name")
		return
	}

	var buf bytes.Buffer
	opts := tree.RmOptions{
		Repo:   req.Repo,
		Branch: req.Branch,
		Force:  req.Force,
		DryRun: false,
	}
	if err := tree.Rm(&buf, opts); err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"output": buf.String()})
}

// UpdateTree fast-forwards a single worktree to its branch's latest remote
// commit via `git pull --ff-only`. 未コミット変更がある場合はエラーになる。
func (h *Handler) UpdateTree(w http.ResponseWriter, r *http.Request) {
	worktree, container, wtName, ok := h.resolveWorktree(w, r)
	if !ok {
		return
	}
	out, err := tree.Update(worktree)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	restarted := false
	if out != "Already up to date" {
		if ok, err := restartDevIfRunning(container, wtName, worktree); err != nil {
			out += fmt.Sprintf("（dev 再起動に失敗: %v）", err)
		} else {
			restarted = ok
		}
	}

	jsonOK(w, map[string]any{"output": out, "restarted": restarted})
}

type gcRequest struct {
	Merged       bool   `json:"merged"`
	Closed       bool   `json:"closed"`
	IncludeDirty bool   `json:"include_dirty"`
	OlderThan    string `json:"older_than"`
	NoTmux       bool   `json:"no_tmux"`
	DryRun       bool   `json:"dry_run"`
	Yes          bool   `json:"yes"`
}

func (h *Handler) GcTrees(w http.ResponseWriter, r *http.Request) {
	var req gcRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	// dry_run と yes は排他: yes のみ実行、それ以外は dry_run
	if !req.Yes {
		req.DryRun = true
	}
	// older_than: "30d" or "24h" のみ許可
	if req.OlderThan != "" {
		if ok, _ := regexp.MatchString(`^\d+[dh]$`, req.OlderThan); !ok {
			jsonErr(w, http.StatusBadRequest, "invalid older_than format (e.g. 30d, 24h)")
			return
		}
	}

	var buf bytes.Buffer
	opts := gc.Options{
		Merged:       req.Merged,
		Closed:       req.Closed,
		IncludeDirty: req.IncludeDirty,
		OlderThan:    req.OlderThan,
		NoTmux:       req.NoTmux,
		DryRun:       req.DryRun,
		Yes:          req.Yes,
	}
	if err := gc.Run(&buf, opts); err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"output": buf.String()})
}

type mergedPRInfo struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"head_ref_name"`
	MergedAt    string `json:"merged_at"`
	State       string `json:"state"`
}

func (h *Handler) GetMergedPRs(w http.ResponseWriter, r *http.Request) {
	repoName := r.URL.Query().Get("repo")
	if repoName == "" {
		jsonErr(w, http.StatusBadRequest, "repo is required")
		return
	}
	if !isKnownRepo(repoName) {
		jsonErr(w, http.StatusBadRequest, "unknown repo: "+repoName)
		return
	}

	cacheKey := "merged_prs:" + repoName
	if v, ok := h.cache.get(cacheKey); ok {
		jsonOK(w, v)
		return
	}

	container, err := core.FindContainer(repoName)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	mainDir, _ := core.ResolveMain(container)
	if mainDir == "" {
		jsonErr(w, http.StatusInternalServerError, "main worktree not found")
		return
	}

	prs := fetchMergedPRsForHandler(mainDir)
	h.cache.set(cacheKey, prs, 60*time.Second)
	jsonOK(w, prs)
}

// githubSlug extracts "owner/repo" from a git remote URL. Returns "" if not GitHub.
func githubSlug(mainDir string) string {
	remote, err := core.GitOutput(mainDir, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	s := remote
	if i := strings.Index(s, "github.com"); i >= 0 {
		s = s[i+len("github.com"):]
		if s != "" && (s[0] == ':' || s[0] == '/') {
			s = s[1:]
		}
	}
	return strings.TrimSuffix(s, ".git")
}

func fetchMergedPRsForHandler(mainDir string) []mergedPRInfo {
	slug := githubSlug(mainDir)
	if slug == "" {
		return []mergedPRInfo{}
	}

	type ghPR struct {
		Number      int    `json:"number"`
		HeadRefName string `json:"headRefName"`
		MergedAt    string `json:"mergedAt"`
		State       string `json:"state"`
	}
	out, err := exec.Command("gh", "pr", "list",
		"--state", "all",
		"--json", "number,headRefName,mergedAt,state",
		"--limit", "200",
		"--repo", slug).Output()
	if err != nil {
		return []mergedPRInfo{}
	}
	var prs []ghPR
	if err := json.Unmarshal(out, &prs); err != nil {
		return []mergedPRInfo{}
	}
	items := make([]mergedPRInfo, 0, len(prs))
	for _, p := range prs {
		items = append(items, mergedPRInfo(p))
	}
	return items
}

type issueDetail struct {
	Number       int    `json:"number"`
	State        string `json:"state"`
	ParentNumber int    `json:"parent_number,omitempty"`
	ParentURL    string `json:"parent_url,omitempty"`
}

func (h *Handler) GetIssueDetails(w http.ResponseWriter, r *http.Request) {
	repoName := r.URL.Query().Get("repo")
	if repoName == "" || !isKnownRepo(repoName) {
		jsonErr(w, http.StatusBadRequest, "repo is required")
		return
	}

	cacheKey := "issue_details:" + repoName
	if v, ok := h.cache.get(cacheKey); ok {
		jsonOK(w, v)
		return
	}

	container, err := core.FindContainer(repoName)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := core.LoadEntries(container)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	var issueNums []int
	seen := map[int]bool{}
	for k := range entries {
		issue := entries[k].Issue
		if issue == "" {
			continue
		}
		var num int
		if n, _ := fmt.Sscan(strings.TrimPrefix(issue, "#"), &num); n == 1 && num > 0 && !seen[num] {
			issueNums = append(issueNums, num)
			seen[num] = true
		}
	}
	if len(issueNums) == 0 {
		h.cache.set(cacheKey, []issueDetail{}, 60*time.Second)
		jsonOK(w, []issueDetail{})
		return
	}

	mainDir, _ := core.ResolveMain(container)
	if mainDir == "" {
		jsonErr(w, http.StatusInternalServerError, "main worktree not found")
		return
	}

	details := fetchIssueDetails(mainDir, issueNums)
	h.cache.set(cacheKey, details, 60*time.Second)
	jsonOK(w, details)
}

func fetchIssueDetails(mainDir string, issueNums []int) []issueDetail {
	slug := githubSlug(mainDir)
	if slug == "" {
		return []issueDetail{}
	}
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) != 2 {
		return []issueDetail{}
	}
	owner, repoName := parts[0], parts[1]

	// GraphQL batch query using aliases (i<N>: issue(number: N) { ... })
	var sb strings.Builder
	fmt.Fprintf(&sb, `{ repository(owner: "%s", name: "%s") {`, owner, repoName)
	for _, num := range issueNums {
		fmt.Fprintf(&sb, ` i%d: issue(number: %d) { number state parent { number url } }`, num, num)
	}
	sb.WriteString(` } }`)

	type graphqlReq struct {
		Query string `json:"query"`
	}
	body, err := json.Marshal(graphqlReq{Query: sb.String()})
	if err != nil {
		return []issueDetail{}
	}

	cmd := exec.Command("gh", "api", "graphql", "--input", "-")
	cmd.Stdin = bytes.NewReader(body)
	out, err := cmd.Output()
	if err != nil {
		return []issueDetail{}
	}

	var resp struct {
		Data struct {
			Repository map[string]json.RawMessage `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return []issueDetail{}
	}

	type issueNode struct {
		Number int    `json:"number"`
		State  string `json:"state"`
		Parent *struct {
			Number int    `json:"number"`
			URL    string `json:"url"`
		} `json:"parent"`
	}

	var details []issueDetail
	for _, raw := range resp.Data.Repository {
		var node issueNode
		if err := json.Unmarshal(raw, &node); err != nil || node.Number == 0 {
			continue
		}
		d := issueDetail{Number: node.Number, State: node.State}
		if node.Parent != nil {
			d.ParentNumber = node.Parent.Number
			d.ParentURL = node.Parent.URL
		}
		details = append(details, d)
	}
	return details
}

// isKnownRepo checks that name matches an existing wt container basename.
func isKnownRepo(name string) bool {
	for _, c := range core.ListContainers() {
		if filepath.Base(c) == name {
			return true
		}
	}
	return false
}

var knownBranchTypes = map[string]bool{
	"feature":  true,
	"fix":      true,
	"chore":    true,
	"docs":     true,
	"refactor": true,
	"test":     true,
	"ci":       true,
}

func isKnownBranchType(t string) bool { return knownBranchTypes[t] }
