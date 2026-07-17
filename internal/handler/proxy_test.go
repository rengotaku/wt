package handler

import (
	"fmt"
	"net"
	"net/http"
	"testing"

	"wt/internal/wt/settings"
)

// freeTCPPort asks the kernel for an unused TCP port on 127.0.0.1 so tests
// don't collide with a running wt web on the machine (default proxy port 8088).
func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen 0: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func TestProxyController_StartStop(t *testing.T) {
	port := freeTCPPort(t)
	p := newProxyController(port)
	if got := p.listenPort(); got != port {
		t.Fatalf("listenPort()=%d, want %d", got, port)
	}
	if p.isRunning() {
		t.Fatal("should not be running initially")
	}
	if err := p.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = p.stop() })
	if !p.isRunning() {
		t.Fatal("should be running after start")
	}
	// Reachable on the proxy port; a non-domain Host yields 404 from the handler.
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("proxy not reachable: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("non-domain Host: got %d, want 404", resp.StatusCode)
	}

	// start is idempotent while running.
	if err := p.start(); err != nil {
		t.Errorf("second start should be a no-op, got: %v", err)
	}

	if err := p.stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if p.isRunning() {
		t.Error("should not be running after stop")
	}
	// stop is idempotent.
	if err := p.stop(); err != nil {
		t.Errorf("second stop should be a no-op, got: %v", err)
	}
}

func TestProxyController_DefaultPortFallback(t *testing.T) {
	// port <= 0 must resolve to settings.DefaultProxyPort so callers that
	// haven't threaded a value through still see a stable, documented port.
	p := newProxyController(0)
	if got, want := p.listenPort(), settings.DefaultProxyPort; got != want {
		t.Fatalf("listenPort()=%d, want DefaultProxyPort=%d", got, want)
	}
	p2 := newProxyController(-1)
	if got, want := p2.listenPort(), settings.DefaultProxyPort; got != want {
		t.Fatalf("negative port listenPort()=%d, want DefaultProxyPort=%d", got, want)
	}
}
