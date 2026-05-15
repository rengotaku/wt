package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"wt/internal/wt/core"
	"wt/internal/wt/repo"
)

type repoItem struct {
	Name        string `json:"name"`
	Container   string `json:"container"`
	Count       int    `json:"count"`
	GitHubURL   string `json:"github_url,omitempty"`
	Description string `json:"description,omitempty"`
	MainBranch  string `json:"main_branch,omitempty"`
	MainDirty   bool   `json:"main_dirty"`
	MainAhead   int    `json:"main_ahead"`
	MainBehind  int    `json:"main_behind"`
}

func (h *Handler) ListRepos(w http.ResponseWriter, _ *http.Request) {
	containers := core.ListContainers()
	items := make([]repoItem, 0, len(containers))
	for _, c := range containers {
		entries, err := core.LoadEntries(c)
		if err != nil {
			continue
		}
		name := filepath.Base(c)
		mainDir, mainName := core.ResolveMain(c)

		item := repoItem{
			Name:       name,
			Container:  c,
			Count:      len(entries),
			MainBranch: mainName,
		}

		if mainDir != "" {
			item.GitHubURL = h.getGitHubURL(mainDir)
			item.Description = h.getGitHubDescription(mainDir)
			item.MainDirty = h.getMainDirty(mainDir)
			item.MainAhead, item.MainBehind = h.getAheadBehind(mainDir)
		}

		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	jsonOK(w, items)
}

func (h *Handler) getGitHubURL(mainDir string) string {
	key := "github_url:" + mainDir
	if v, ok := h.cache.get(key); ok {
		return v.(string)
	}
	remote, err := core.GitOutput(mainDir, "remote", "get-url", "origin")
	url := ""
	if err == nil {
		remote = strings.TrimSpace(remote)
		if strings.HasPrefix(remote, "git@github.com:") {
			path := strings.TrimPrefix(remote, "git@github.com:")
			path = strings.TrimSuffix(path, ".git")
			url = "https://github.com/" + path
		} else if strings.HasPrefix(remote, "https://github.com/") {
			url = strings.TrimSuffix(remote, ".git")
		}
	}
	h.cache.set(key, url, 5*time.Minute)
	return url
}

func (h *Handler) getGitHubDescription(mainDir string) string {
	key := "github_desc:" + mainDir
	if v, ok := h.cache.get(key); ok {
		return v.(string)
	}
	ghURL := h.getGitHubURL(mainDir)
	desc := ""
	if strings.HasPrefix(ghURL, "https://github.com/") {
		slug := strings.TrimPrefix(ghURL, "https://github.com/")
		out, err := exec.Command("gh", "api", "repos/"+slug, "--jq", ".description").Output()
		if err == nil {
			v := strings.TrimSpace(string(out))
			if v != "null" {
				desc = v
			}
		}
	}
	h.cache.set(key, desc, time.Hour)
	return desc
}

func (h *Handler) getMainDirty(mainDir string) bool {
	key := "main_dirty:" + mainDir
	if v, ok := h.cache.get(key); ok {
		return v.(bool)
	}
	dirty := core.IsDirty(mainDir)
	h.cache.set(key, dirty, cacheTTL)
	return dirty
}

func (h *Handler) getAheadBehind(mainDir string) (ahead, behind int) {
	key := "ahead_behind:" + mainDir
	if v, ok := h.cache.get(key); ok {
		pair := v.([2]int)
		return pair[0], pair[1]
	}
	type pair = [2]int
	aheadOut, err := exec.Command("git", "-C", mainDir, "rev-list", "--count", "@{upstream}..HEAD").Output()
	if err == nil {
		_, _ = fmt.Sscan(strings.TrimSpace(string(aheadOut)), &ahead)
	}
	behindOut, err := exec.Command("git", "-C", mainDir, "rev-list", "--count", "HEAD..@{upstream}").Output()
	if err == nil {
		_, _ = fmt.Sscan(strings.TrimSpace(string(behindOut)), &behind)
	}
	h.cache.set(key, pair{ahead, behind}, cacheTTL)
	return ahead, behind
}

type addRepoRequest struct {
	URL string `json:"url"`
}

var repoURLRe = regexp.MustCompile(`^(https://github\.com/[a-zA-Z0-9_.\-]+/[a-zA-Z0-9_.\-]+(?:\.git)?|git@github\.com:[a-zA-Z0-9_.\-]+/[a-zA-Z0-9_.\-]+\.git)$`)

func (h *Handler) AddRepo(w http.ResponseWriter, r *http.Request) {
	var req addRepoRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !repoURLRe.MatchString(req.URL) {
		jsonErr(w, http.StatusBadRequest, "invalid GitHub URL")
		return
	}
	var buf bytes.Buffer
	if err := repo.Add(&buf, repo.AddOptions{URL: req.URL}); err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, map[string]string{"output": buf.String()})
}

type deleteRepoRequest struct {
	Name string `json:"name"`
}

var repoNameRe = regexp.MustCompile(`^[a-zA-Z0-9_.\-]+$`)

func (h *Handler) DeleteRepo(w http.ResponseWriter, r *http.Request) {
	var req deleteRepoRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" || !repoNameRe.MatchString(req.Name) {
		jsonErr(w, http.StatusBadRequest, "invalid repo name")
		return
	}
	container, err := core.FindContainer(req.Name)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// main/master のみかチェック
	entries, _ := core.LoadEntries(container)
	for name, entry := range entries {
		if entry.Type == "main" || name == "main" || name == "master" {
			continue
		}
		jsonErr(w, http.StatusConflict, "追加 worktree が残存しているため削除できません: "+name)
		return
	}

	var buf bytes.Buffer
	if err := repo.Rm(&buf, req.Name, repo.RmOptions{Force: true}); err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"output": buf.String()})
}

type refreshRepoRequest struct {
	Name string `json:"name"`
}

func (h *Handler) RefreshRepo(w http.ResponseWriter, r *http.Request) {
	var req refreshRepoRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" || !repoNameRe.MatchString(req.Name) {
		jsonErr(w, http.StatusBadRequest, "invalid repo name")
		return
	}
	container, err := core.FindContainer(req.Name)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	mainDir, _ := core.ResolveMain(container)
	if mainDir == "" {
		jsonErr(w, http.StatusInternalServerError, "main worktree not found")
		return
	}

	out, err := exec.Command("git", "-C", mainDir, "fetch", "origin").CombinedOutput()
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, string(out))
		return
	}

	// キャッシュを無効化
	h.cache.del("ahead_behind:" + mainDir)
	h.cache.del("main_dirty:" + mainDir)

	jsonOK(w, map[string]string{"output": strings.TrimSpace(string(out))})
}

func (h *Handler) SyncRepo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" || !repoNameRe.MatchString(req.Name) {
		jsonErr(w, http.StatusBadRequest, "invalid repo name")
		return
	}
	container, err := core.FindContainer(req.Name)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	mainDir, _ := core.ResolveMain(container)
	if mainDir == "" {
		jsonErr(w, http.StatusInternalServerError, "main worktree not found")
		return
	}
	if core.IsDirty(mainDir) {
		jsonErr(w, http.StatusConflict, "main に未コミットの変更があります")
		return
	}
	if out, err := exec.Command("git", "-C", mainDir, "fetch", "origin").CombinedOutput(); err != nil {
		jsonErr(w, http.StatusInternalServerError, "fetch failed: "+strings.TrimSpace(string(out)))
		return
	}
	out, err := exec.Command("git", "-C", mainDir, "pull", "--ff-only").CombinedOutput()
	if err != nil {
		jsonErr(w, http.StatusConflict, strings.TrimSpace(string(out)))
		return
	}
	h.cache.del("ahead_behind:" + mainDir)
	h.cache.del("main_dirty:" + mainDir)
	jsonOK(w, map[string]string{"output": strings.TrimSpace(string(out))})
}

func (h *Handler) SyncAll(w http.ResponseWriter, _ *http.Request) {
	containers := core.ListContainers()

	go func() {
		for _, c := range containers {
			mainDir, _ := core.ResolveMain(c)
			if mainDir == "" {
				continue
			}
			if core.IsDirty(mainDir) {
				continue
			}
			if _, err := exec.Command("git", "-C", mainDir, "fetch", "origin").CombinedOutput(); err != nil {
				continue
			}
			if _, err := exec.Command("git", "-C", mainDir, "pull", "--ff-only").CombinedOutput(); err != nil {
				continue
			}
			h.cache.del("ahead_behind:" + mainDir)
			h.cache.del("main_dirty:" + mainDir)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	jsonOK(w, map[string]string{"message": "同期を開始しました"})
}
