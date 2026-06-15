package handler

import (
	"net/http"
	"testing"
)

func TestProxyController_StartStop(t *testing.T) {
	p := &proxyController{}
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
	resp, err := http.Get("http://127.0.0.1:8088/")
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
