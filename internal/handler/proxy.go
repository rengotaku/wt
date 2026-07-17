package handler

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"wt/internal/wt/proxy"
	"wt/internal/wt/settings"
)

// proxyController runs wt's reverse proxy as a goroutine inside the wt web
// process so it can be started/stopped from the Web UI without a separate
// `wt proxy` command. It is safe for concurrent use.
type proxyController struct {
	mu   sync.Mutex
	srv  *http.Server
	port int
}

// newProxyController returns a controller that will listen on `port` when
// started. A port <= 0 falls back to the built-in default so callers that
// haven't threaded a value through still get a working proxy.
func newProxyController(port int) *proxyController {
	if port <= 0 {
		port = settings.DefaultProxyPort
	}
	return &proxyController{port: port}
}

func (p *proxyController) isRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.srv != nil
}

// listenPort returns the port the controller will bind to on start().
func (p *proxyController) listenPort() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.port
}

// start binds the proxy port and serves in the background. It is a no-op when
// already running, and returns an error (e.g. address in use) when the port is
// taken — typically by a separate `wt proxy` process.
func (p *proxyController) start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.srv != nil {
		return nil
	}
	addr := fmt.Sprintf("127.0.0.1:%d", p.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           proxy.Handler(proxy.Routes),
		ReadHeaderTimeout: 5 * time.Second,
	}
	p.srv = srv
	go func() { _ = srv.Serve(ln) }()
	return nil
}

// stop shuts the proxy down. It is a no-op when not running.
func (p *proxyController) stop() error {
	p.mu.Lock()
	srv := p.srv
	p.srv = nil
	p.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Close()
}

type proxyStatus struct {
	Running bool   `json:"running"`
	Port    int    `json:"port"`
	Suffix  string `json:"suffix"`
}

func (h *Handler) statusOfProxy() proxyStatus {
	return proxyStatus{Running: h.prx.isRunning(), Port: h.prx.listenPort(), Suffix: proxy.DomainSuffix}
}

// StartBuiltinProxy starts the built-in proxy synchronously so callers can
// distinguish success from bind failure at wt web startup. Idempotent while
// already running.
func (h *Handler) StartBuiltinProxy() error { return h.prx.start() }

// GetProxy returns whether the built-in proxy is currently running.
func (h *Handler) GetProxy(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, h.statusOfProxy())
}

// StartProxy starts the built-in proxy.
func (h *Handler) StartProxy(w http.ResponseWriter, _ *http.Request) {
	if err := h.prx.start(); err != nil {
		jsonErr(w, http.StatusBadRequest, "proxy の起動に失敗しました: "+err.Error())
		return
	}
	jsonOK(w, h.statusOfProxy())
}

// StopProxy stops the built-in proxy.
func (h *Handler) StopProxy(w http.ResponseWriter, _ *http.Request) {
	if err := h.prx.stop(); err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, h.statusOfProxy())
}
