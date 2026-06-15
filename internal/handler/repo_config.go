package handler

import (
	"net/http"
	"path/filepath"
	"strings"

	"wt/internal/wt/core"
	"wt/internal/wt/devserver"
)

type repoConfigResponse struct {
	SymlinkCandidates []string          `json:"symlink_candidates"`
	DevServices       []core.DevService `json:"dev_services"`
}

type repoConfigRequest struct {
	SymlinkCandidates []string          `json:"symlink_candidates"`
	DevServices       []core.DevService `json:"dev_services"`
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
	dev := cfg.DevServices
	if dev == nil {
		dev = []core.DevService{}
	}
	jsonOK(w, repoConfigResponse{SymlinkCandidates: candidates, DevServices: dev})
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
	if len(req.DevServices) > 0 {
		if err := (devserver.Config{Services: toDevServices(req.DevServices)}).Validate(); err != nil {
			jsonErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	// Preserve other _config fields (e.g. git_crypt_key) by loading first.
	cfg, err := core.LoadConfig(container)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg.SymlinkCandidates = cleaned
	cfg.DevServices = req.DevServices
	if err := core.SaveConfig(container, cfg); err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	dev := cfg.DevServices
	if dev == nil {
		dev = []core.DevService{}
	}
	jsonOK(w, repoConfigResponse{SymlinkCandidates: cleaned, DevServices: dev})
}

// toDevServices converts metadata services to runtime services for validation.
func toDevServices(in []core.DevService) []devserver.Service {
	out := make([]devserver.Service, 0, len(in))
	for _, s := range in {
		out = append(out, devserver.Service{Name: s.Name, Cmd: s.Cmd, Domain: s.Domain})
	}
	return out
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
