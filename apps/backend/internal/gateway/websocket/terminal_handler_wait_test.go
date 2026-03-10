package websocket

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// mockEventBus satisfies bus.EventBus for test setup.
type mockEventBus struct{}

func (m *mockEventBus) Publish(_ context.Context, _ string, _ *bus.Event) error { return nil }
func (m *mockEventBus) Subscribe(_ string, _ bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (m *mockEventBus) QueueSubscribe(_, _ string, _ bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (m *mockEventBus) Request(_ context.Context, _ string, _ *bus.Event, _ time.Duration) (*bus.Event, error) {
	return nil, nil
}
func (m *mockEventBus) Close()            {}
func (m *mockEventBus) IsConnected() bool { return true }

// mockProfileResolver satisfies lifecycle.ProfileResolver.
type mockProfileResolver struct{}

func (m *mockProfileResolver) ResolveProfile(_ context.Context, profileID string) (*lifecycle.AgentProfileInfo, error) {
	return &lifecycle.AgentProfileInfo{ProfileID: profileID}, nil
}

// mockCredsMgr satisfies lifecycle.CredentialsManager.
type mockCredsMgr struct{}

func (m *mockCredsMgr) GetCredentialValue(_ context.Context, _ string) (string, error) {
	return "", nil
}

func newTestTerminalHandler(t *testing.T) (*TerminalHandler, *lifecycle.ExecutionStore) {
	t.Helper()

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()

	mgr := lifecycle.NewManager(
		reg, &mockEventBus{}, nil, &mockCredsMgr{}, &mockProfileResolver{}, nil,
		lifecycle.ExecutorFallbackWarn, "", log,
	)

	handler := &TerminalHandler{
		lifecycleMgr: mgr,
		logger:       log,
	}

	return handler, mgr.ExecutionStoreForTesting()
}

func TestWaitForRemoteExecutionWithTimeout_FastPath(t *testing.T) {
	handler, store := newTestTerminalHandler(t)

	store.Add(&lifecycle.AgentExecution{
		ID:        "exec-1",
		SessionID: "session-1",
		Status:    v1.AgentStatusRunning,
	})

	got, ok := handler.waitForRemoteExecutionWithTimeout(context.Background(), "session-1", 1*time.Second)
	if !ok {
		t.Fatal("expected fast path to find execution")
	}
	if got.ID != "exec-1" {
		t.Errorf("expected exec-1, got %s", got.ID)
	}
}

func TestWaitForRemoteExecutionWithTimeout_PollSuccess(t *testing.T) {
	handler, store := newTestTerminalHandler(t)

	// Add execution after a short delay (simulates async workspace setup)
	go func() {
		time.Sleep(200 * time.Millisecond)
		store.Add(&lifecycle.AgentExecution{
			ID:        "exec-delayed",
			SessionID: "session-1",
			Status:    v1.AgentStatusRunning,
		})
	}()

	got, ok := handler.waitForRemoteExecutionWithTimeout(context.Background(), "session-1", 5*time.Second)
	if !ok {
		t.Fatal("expected polling to find execution")
	}
	if got.ID != "exec-delayed" {
		t.Errorf("expected exec-delayed, got %s", got.ID)
	}
}

func TestWaitForRemoteExecutionWithTimeout_Timeout(t *testing.T) {
	handler, _ := newTestTerminalHandler(t)

	_, ok := handler.waitForRemoteExecutionWithTimeout(context.Background(), "session-missing", 600*time.Millisecond)
	if ok {
		t.Fatal("expected timeout, got execution")
	}
}

func TestWaitForRemoteExecutionWithTimeout_ContextCancelled(t *testing.T) {
	handler, _ := newTestTerminalHandler(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, ok := handler.waitForRemoteExecutionWithTimeout(ctx, "session-1", 5*time.Second)
	if ok {
		t.Fatal("expected context cancellation to return false")
	}
}
