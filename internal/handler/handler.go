// Package handler implements the JSON API for wt web.
package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Handler holds shared state for API handlers.
type Handler struct {
	cache *ttlCache
	prx   *proxyController
}

// New returns a ready-to-use Handler.
func New() *Handler { return &Handler{cache: newTTLCache(), prx: &proxyController{}} }

// Routes wires all API endpoints and falls back to staticHandler for SPA assets.
func (h *Handler) Routes(staticHandler http.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/repos", h.ListRepos)
	mux.HandleFunc("POST /api/repos", h.AddRepo)
	mux.HandleFunc("DELETE /api/repos", h.DeleteRepo)
	mux.HandleFunc("POST /api/repos/refresh", h.RefreshRepo)
	mux.HandleFunc("POST /api/repos/sync", h.SyncRepo)
	mux.HandleFunc("POST /api/repos/sync-all", h.SyncAll)
	mux.HandleFunc("GET /api/repos/{name}/config", h.GetRepoConfig)
	mux.HandleFunc("PUT /api/repos/{name}/config", h.UpdateRepoConfig)
	mux.HandleFunc("GET /api/trees", h.ListTrees)
	mux.HandleFunc("POST /api/trees", h.AddTree)
	mux.HandleFunc("DELETE /api/trees", h.DeleteTree)
	mux.HandleFunc("POST /api/trees/{repo}/{wt}/update", h.UpdateTree)
	mux.HandleFunc("PUT /api/trees/{repo}/{wt}/pin", h.SetTreePin)
	mux.HandleFunc("POST /api/trees/gc", h.GcTrees)
	mux.HandleFunc("GET /api/trees/merged-prs", h.GetMergedPRs)
	mux.HandleFunc("GET /api/trees/issue-details", h.GetIssueDetails)
	mux.HandleFunc("GET /api/ports", h.ListPorts)
	mux.HandleFunc("GET /api/ports/listeners", h.ListListeners)
	mux.HandleFunc("GET /api/ports/stale", h.ListStalePorts)
	mux.HandleFunc("POST /api/ports/prune", h.PrunePorts)
	mux.HandleFunc("POST /api/ports/{repo}/{wt}/serve", h.ServeWorktree)
	mux.HandleFunc("POST /api/ports/{repo}/{wt}/down", h.DownWorktree)
	mux.HandleFunc("GET /api/ports/{repo}/{wt}/devconfig", h.GetDevConfig)
	mux.HandleFunc("PUT /api/ports/{repo}/{wt}/devconfig", h.PutDevConfig)
	mux.HandleFunc("GET /api/ports/{repo}/{wt}/logs", h.GetLogs)
	mux.HandleFunc("GET /api/proxy", h.GetProxy)
	mux.HandleFunc("POST /api/proxy/start", h.StartProxy)
	mux.HandleFunc("POST /api/proxy/stop", h.StopProxy)
	mux.HandleFunc("GET /api/settings", h.GetSettings)
	mux.HandleFunc("PUT /api/settings", h.UpdateSettings)

	mux.Handle("/", staticHandler)

	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5174")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("json encode", "err", err)
	}
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
