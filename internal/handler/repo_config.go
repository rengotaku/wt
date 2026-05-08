package handler

import (
	"net/http"
	"path/filepath"
	"strings"

	"wt/internal/wt/core"
)

type repoConfigResponse struct {
	SymlinkCandidates []string `json:"symlink_candidates"`
}

type repoConfigRequest struct {
	SymlinkCandidates []string `json:"symlink_candidates"`
}

func (h *Handler) GetRepoConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	container, ok := resolveRepoContainer(w, name)
	if !ok {
		return
	}
	cfg, err := core.LoadConfig(container)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	candidates := cfg.SymlinkCandidates
	if candidates == nil {
		candidates = []string{}
	}
	jsonOK(w, repoConfigResponse{SymlinkCandidates: candidates})
}

func (h *Handler) UpdateRepoConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	container, ok := resolveRepoContainer(w, name)
	if !ok {
		return
	}
	var req repoConfigRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	cleaned, err := sanitizeCandidates(req.SymlinkCandidates)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := core.SaveConfig(container, core.EntryConfig{SymlinkCandidates: cleaned}); err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, repoConfigResponse{SymlinkCandidates: cleaned})
}

// resolveRepoContainer validates the repo name and returns its container path.
// Writes a JSON error to w and returns ok=false on validation failure.
func resolveRepoContainer(w http.ResponseWriter, name string) (string, bool) {
	if name == "" || !repoNameRe.MatchString(name) {
		jsonErr(w, http.StatusBadRequest, "invalid repo name")
		return "", false
	}
	container, err := core.FindContainer(name)
	if err != nil {
		jsonErr(w, http.StatusNotFound, err.Error())
		return "", false
	}
	return container, true
}

// sanitizeCandidates trims, validates and de-duplicates candidate paths.
// Rules: relative paths only, no ".." segments, no leading "/".
func sanitizeCandidates(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, raw := range in {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "/") {
			return nil, &validationError{msg: "absolute path is not allowed: " + p}
		}
		clean := filepath.Clean(p)
		if clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
			return nil, &validationError{msg: "path must stay within container: " + p}
		}
		if _, dup := seen[clean]; dup {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out, nil
}

type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }
