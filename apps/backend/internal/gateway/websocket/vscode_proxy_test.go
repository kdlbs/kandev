package websocket

import (
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestProxyLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

func TestNewVscodeProxyHandler(t *testing.T) {
	log := newTestProxyLogger()
	h := NewVscodeProxyHandler(nil, log)

	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.proxies == nil {
		t.Error("expected proxies map to be initialized")
	}
	if h.lifecycleMgr != nil {
		t.Error("expected nil lifecycleMgr when nil passed")
	}
}

func TestInvalidateProxy(t *testing.T) {
	log := newTestProxyLogger()
	h := NewVscodeProxyHandler(nil, log)

	// Manually insert a cache entry
	h.mu.Lock()
	h.proxies["session-1"] = &proxyEntry{proxy: nil, target: "127.0.0.1:8080"}
	h.proxies["session-2"] = &proxyEntry{proxy: nil, target: "127.0.0.1:8081"}
	h.mu.Unlock()

	// Invalidate one
	h.InvalidateProxy("session-1")

	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.proxies["session-1"]; ok {
		t.Error("expected session-1 proxy to be removed")
	}
	if _, ok := h.proxies["session-2"]; !ok {
		t.Error("expected session-2 proxy to still exist")
	}
}

func TestInvalidateProxy_Idempotent(t *testing.T) {
	log := newTestProxyLogger()
	h := NewVscodeProxyHandler(nil, log)

	// Invalidate a non-existent entry — should not panic
	h.InvalidateProxy("nonexistent")

	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.proxies) != 0 {
		t.Errorf("expected empty proxies map, got %d entries", len(h.proxies))
	}
}

func TestProxyCacheEmpty_OnCreation(t *testing.T) {
	log := newTestProxyLogger()
	h := NewVscodeProxyHandler(nil, log)

	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.proxies) != 0 {
		t.Errorf("expected empty proxies on creation, got %d", len(h.proxies))
	}
}
