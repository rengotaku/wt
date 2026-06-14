package handler

import (
	"net/http"

	"wt/internal/wt/devserver"
)

// devConfigResponse is the GET payload: the worktree's services plus whether a
// dev.toml currently exists (false → the client shows a "create" flow).
type devConfigResponse struct {
	HasConfig bool                `json:"has_config"`
	Services  []devserver.Service `json:"services"`
}

// GetDevConfig returns the worktree's .wt/dev.toml as structured services. When
// no config exists yet it returns has_config=false with an empty service list.
func (h *Handler) GetDevConfig(w http.ResponseWriter, r *http.Request) {
	worktree, _, _, ok := h.resolveWorktree(w, r)
	if !ok {
		return
	}
	if !devserver.HasConfig(worktree) {
		jsonOK(w, devConfigResponse{HasConfig: false, Services: []devserver.Service{}})
		return
	}
	cfg, err := devserver.Load(worktree)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cfg.Services == nil {
		cfg.Services = []devserver.Service{}
	}
	jsonOK(w, devConfigResponse{HasConfig: true, Services: cfg.Services})
}

// PutDevConfig validates and writes the worktree's .wt/dev.toml from the posted
// service list, making the worktree serve-able from the Web UI.
func (h *Handler) PutDevConfig(w http.ResponseWriter, r *http.Request) {
	worktree, _, _, ok := h.resolveWorktree(w, r)
	if !ok {
		return
	}
	var body struct {
		Services []devserver.Service `json:"services"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonErr(w, http.StatusBadRequest, "リクエストの解析に失敗しました: "+err.Error())
		return
	}
	cfg := devserver.Config{Services: body.Services}
	if err := devserver.Save(worktree, cfg); err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, devConfigResponse{HasConfig: true, Services: cfg.Services})
}
