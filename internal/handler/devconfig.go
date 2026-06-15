package handler

import (
	"net/http"

	"wt/internal/wt/core"
	"wt/internal/wt/devserver"
)

// devConfigResponse is the GET payload: the worktree's effective services plus
// where they come from ("worktree" override / "repo" default / "file" / "" none).
type devConfigResponse struct {
	HasConfig bool                `json:"has_config"`
	Source    string              `json:"source"`
	Services  []devserver.Service `json:"services"`
}

// GetDevConfig returns the worktree's effective dev config (per-worktree
// override > repo default > committed .wt/dev.toml).
func (h *Handler) GetDevConfig(w http.ResponseWriter, r *http.Request) {
	worktree, _, _, ok := h.resolveWorktree(w, r)
	if !ok {
		return
	}
	cfg, source, err := devserver.EffectiveConfig(worktree)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	services := cfg.Services
	if services == nil {
		services = []devserver.Service{}
	}
	jsonOK(w, devConfigResponse{
		HasConfig: source != devserver.SourceNone,
		Source:    source,
		Services:  services,
	})
}

// PutDevConfig stores a per-worktree dev config override in metadata (never a
// committed file). An empty service list clears the override (falls back to the
// repo default).
func (h *Handler) PutDevConfig(w http.ResponseWriter, r *http.Request) {
	_, container, wtName, ok := h.resolveWorktree(w, r)
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
	// A non-empty override must be valid; empty means "clear override".
	if len(body.Services) > 0 {
		if err := (devserver.Config{Services: body.Services}).Validate(); err != nil {
			jsonErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	entries, err := core.LoadEntries(container)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	entry := entries[wtName]
	entry.DevServices = toCoreServices(body.Services)
	if err := core.PutEntry(container, wtName, &entry); err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.GetDevConfig(w, r)
}

// toCoreServices converts API services to the metadata representation.
func toCoreServices(in []devserver.Service) []core.DevService {
	if len(in) == 0 {
		return nil
	}
	out := make([]core.DevService, 0, len(in))
	for _, s := range in {
		out = append(out, core.DevService{Name: s.Name, Cmd: s.Cmd, Domain: s.Domain})
	}
	return out
}
