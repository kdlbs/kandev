package instance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsIdle_RespectsInflightRequests verifies that an instance with at
// least one in-flight HTTP request is never considered idle, even when
// LastActivity is older than the timeout. This is the WebSocket-stream case
// — a long-lived /agent/stream request keeps the counter above zero so the
// reaper doesn't kill an actively-connected session.
func TestIsIdle_RespectsInflightRequests(t *testing.T) {
	inst := &Instance{CreatedAt: time.Now().Add(-1 * time.Hour)}
	inst.lastActivityNanos.Store(time.Now().Add(-1 * time.Hour).UnixNano())
	inst.inflightRequests.Store(1)

	assert.False(t, inst.IsIdle(time.Now(), 5*time.Minute),
		"instance with in-flight request must never be idle")

	inst.inflightRequests.Store(0)
	assert.True(t, inst.IsIdle(time.Now(), 5*time.Minute),
		"instance with no in-flight requests and stale activity must be idle")
}

// TestIsIdle_FallsBackToCreatedAt covers the "instance was created but
// never received any HTTP traffic" case — the reaper should still kick in
// after timeout based on CreatedAt.
func TestIsIdle_FallsBackToCreatedAt(t *testing.T) {
	inst := &Instance{CreatedAt: time.Now().Add(-2 * time.Hour)}
	// lastActivityNanos stays zero.

	assert.True(t, inst.IsIdle(time.Now(), 1*time.Hour),
		"instance with no activity must fall back to CreatedAt for idle decision")
}

// TestActivityMiddleware_BumpsAndDecrements asserts the middleware
// increments inflightRequests on entry, decrements on exit, and stamps
// lastActivity in both transitions. While the handler is mid-flight the
// counter stays at 1; after it returns the counter is back to 0.
func TestActivityMiddleware_BumpsAndDecrements(t *testing.T) {
	inst := &Instance{CreatedAt: time.Now()}
	started := make(chan struct{})
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	})

	wrapped := activityMiddleware(inst)(handler)
	srv := httptest.NewServer(wrapped)
	t.Cleanup(srv.Close)

	go func() {
		resp, err := http.Get(srv.URL + "/")
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	<-started
	assert.Equal(t, int32(1), inst.inflightRequests.Load(),
		"in-flight counter must be 1 while handler is executing")
	assert.False(t, inst.LastActivity().IsZero(),
		"lastActivity must be stamped on request entry")

	close(release)

	assert.Eventually(t, func() bool {
		return inst.inflightRequests.Load() == 0
	}, time.Second, 10*time.Millisecond,
		"in-flight counter must drop to 0 after handler returns")
}

// TestManager_IdleReaper_StopsIdleInstance is the end-to-end regression
// test for the disconnected-session leak (GH issue #1247). A configured
// idle timeout + reaper interval well under a second drives the reaper
// fast enough for a unit test; once the instance has been idle past the
// timeout the reaper calls StopInstance and the instance disappears from
// the manager's map.
func TestManager_IdleReaper_StopsIdleInstance(t *testing.T) {
	log := newTestLogger(t)
	cfg := &config.Config{
		Ports:              config.PortConfig{Base: 0, Max: 0},
		Defaults:           config.InstanceDefaults{Protocol: agent.ProtocolACP},
		IdleTimeout:        50 * time.Millisecond,
		IdleReaperInterval: 20 * time.Millisecond,
	}
	mgr := NewManager(cfg, log)
	t.Cleanup(func() {
		_ = mgr.Shutdown(context.Background())
	})

	// Server factory that returns a no-op handler — we don't need a real
	// API server to drive the reaper; lastActivity stays at instance
	// creation time and the reaper picks it up after IdleTimeout elapses.
	mgr.SetServerFactory(func(_ *config.InstanceConfig, _ *process.Manager, _ *logger.Logger) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	})

	// Allocate a real free port without going through Manager.CreateInstance
	// (which needs a workspace path / process manager). Use port 0 via
	// allocator by stubbing the manager state instead.
	inst := &Instance{
		ID:        "idle-test",
		Port:      0,
		Status:    "running",
		CreatedAt: time.Now().Add(-time.Second), // already older than IdleTimeout
	}
	mgr.mu.Lock()
	mgr.instances[inst.ID] = inst
	mgr.mu.Unlock()

	require.Eventually(t, func() bool {
		mgr.mu.RLock()
		_, exists := mgr.instances[inst.ID]
		mgr.mu.RUnlock()
		return !exists
	}, 2*time.Second, 20*time.Millisecond,
		"idle reaper should have stopped the instance and removed it from the map")
}

// TestManager_IdleReaper_DisabledByZeroTimeout verifies that setting
// IdleTimeout = 0 disables the reaper entirely — no goroutine should fire,
// no instance should be touched, regardless of how old it is.
func TestManager_IdleReaper_DisabledByZeroTimeout(t *testing.T) {
	log := newTestLogger(t)
	cfg := &config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
		// IdleTimeout zero => disabled.
	}
	mgr := NewManager(cfg, log)
	t.Cleanup(func() {
		_ = mgr.Shutdown(context.Background())
	})

	inst := &Instance{ID: "no-reaper", CreatedAt: time.Now().Add(-24 * time.Hour)}
	mgr.mu.Lock()
	mgr.instances[inst.ID] = inst
	mgr.mu.Unlock()

	time.Sleep(100 * time.Millisecond)

	mgr.mu.RLock()
	_, exists := mgr.instances[inst.ID]
	mgr.mu.RUnlock()
	assert.True(t, exists, "reaper must be disabled when IdleTimeout=0")
}

// TestActivityMiddleware_Concurrent ensures the inflight counter stays
// consistent under concurrent requests (using atomic ops, no false zero
// while another request is still in flight).
func TestActivityMiddleware_Concurrent(t *testing.T) {
	inst := &Instance{CreatedAt: time.Now()}
	var seenZero atomic.Int32

	gate := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-gate
		if inst.inflightRequests.Load() == 0 {
			seenZero.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	})

	wrapped := activityMiddleware(inst)(handler)
	srv := httptest.NewServer(wrapped)
	t.Cleanup(srv.Close)

	for i := 0; i < 5; i++ {
		go func() {
			resp, err := http.Get(srv.URL + "/")
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
	}

	assert.Eventually(t, func() bool {
		return inst.inflightRequests.Load() == 5
	}, time.Second, 10*time.Millisecond,
		"all five concurrent requests should be counted")

	close(gate)
	assert.Eventually(t, func() bool {
		return inst.inflightRequests.Load() == 0
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, int32(0), seenZero.Load(),
		"no handler should ever observe a zero inflight counter while another request is mid-flight")
}
