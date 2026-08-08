package handler

import (
	"bytes"
	"log/slog"
	"net/http"
	"path/filepath"

	"wt/internal/wt/core"
	"wt/internal/wt/devserver"
	"wt/internal/wt/ports"
)

// resolveWorktree validates repo + wt path params and returns the worktree path
// along with its container and name.
func (h *Handler) resolveWorktree(w http.ResponseWriter, r *http.Request) (worktree, container, wtName string, ok bool) {
	repo := r.PathValue("repo")
	wtName = r.PathValue("wt")
	if !isKnownRepo(repo) {
		jsonErr(w, http.StatusBadRequest, "unknown repo: "+repo)
		return "", "", "", false
	}
	container, err := core.FindContainer(repo)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return "", "", "", false
	}
	entries, err := core.LoadEntries(container)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return "", "", "", false
	}
	if _, exists := entries[wtName]; !exists {
		jsonErr(w, http.StatusNotFound, "worktree が見つかりません: "+wtName)
		return "", "", "", false
	}
	return filepath.Join(container, wtName), container, wtName, true
}

// ServeWorktree starts the worktree's dev services, auto-allocating a port
// block when the worktree has none yet.
func (h *Handler) ServeWorktree(w http.ResponseWriter, r *http.Request) {
	worktree, container, wtName, ok := h.resolveWorktree(w, r)
	if !ok {
		return
	}
	base, err := ports.EnsureBase(container, wtName)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var buf bytes.Buffer
	slog.Info("devserver action", "worktree", worktree, "action", "serve", "trigger", "manual-api")
	if err := devserver.Serve(&buf, worktree, base); err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, map[string]any{"output": buf.String(), "running": true})
}

// DownWorktree stops the worktree's dev services.
func (h *Handler) DownWorktree(w http.ResponseWriter, r *http.Request) {
	worktree, _, _, ok := h.resolveWorktree(w, r)
	if !ok {
		return
	}
	var buf bytes.Buffer
	slog.Info("devserver action", "worktree", worktree, "action", "down", "trigger", "manual-api")
	if err := devserver.Down(&buf, worktree); err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"output": buf.String(), "running": false})
}
