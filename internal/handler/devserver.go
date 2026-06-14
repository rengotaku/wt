package handler

import (
	"bytes"
	"net/http"
	"path/filepath"

	"wt/internal/wt/core"
	"wt/internal/wt/devserver"
)

// resolveWorktree validates repo + wt path params and returns the worktree path
// and its allocated port base.
func (h *Handler) resolveWorktree(w http.ResponseWriter, r *http.Request) (worktree string, base int, ok bool) {
	repo := r.PathValue("repo")
	wtName := r.PathValue("wt")
	if !isKnownRepo(repo) {
		jsonErr(w, http.StatusBadRequest, "unknown repo: "+repo)
		return "", 0, false
	}
	container, err := core.FindContainer(repo)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return "", 0, false
	}
	entries, err := core.LoadEntries(container)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return "", 0, false
	}
	e, ok := entries[wtName]
	if !ok {
		jsonErr(w, http.StatusNotFound, "worktree が見つかりません: "+wtName)
		return "", 0, false
	}
	return filepath.Join(container, wtName), e.PortBase, true
}

// ServeWorktree starts the worktree's dev services on its allocated ports.
func (h *Handler) ServeWorktree(w http.ResponseWriter, r *http.Request) {
	worktree, base, ok := h.resolveWorktree(w, r)
	if !ok {
		return
	}
	var buf bytes.Buffer
	if err := devserver.Serve(&buf, worktree, base); err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, map[string]any{"output": buf.String(), "running": true})
}

// DownWorktree stops the worktree's dev services.
func (h *Handler) DownWorktree(w http.ResponseWriter, r *http.Request) {
	worktree, _, ok := h.resolveWorktree(w, r)
	if !ok {
		return
	}
	var buf bytes.Buffer
	if err := devserver.Down(&buf, worktree); err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"output": buf.String(), "running": false})
}
