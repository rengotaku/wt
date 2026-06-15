package handler

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"wt/internal/wt/proxy"
)

// proxyPort is the fixed port the built-in reverse proxy listens on. The
// <label>.wt.localhost links in the UI point at this port.
const proxyPort = 8088

// proxyController runs wt's reverse proxy as a goroutine inside the wt web
// process so it can be started/stopped from the Web UI without a separate
// `wt proxy` command. It is safe for concurrent use.
type proxyController struct {
	mu  sync.Mutex
	srv *http.Server
}

func (p *proxyController) isRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.srv != nil
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
	addr := fmt.Sprintf("127.0.0.1:%d", proxyPort)
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
	return proxyStatus{Running: h.prx.isRunning(), Port: proxyPort, Suffix: proxy.DomainSuffix}
}

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
