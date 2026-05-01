package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestErrSessionWorkspaceNotReady_ErrorsIs(t *testing.T) {
	// The production code wraps ErrSessionWorkspaceNotReady with fmt.Errorf("%w", ...).
	// The terminal handler uses errors.Is to detect this sentinel and trigger retry logic.
	// This test ensures the wrapping chain stays detectable.

	wrapped := fmt.Errorf("%w: session test-session has no workspace path configured", ErrSessionWorkspaceNotReady)

	if !errors.Is(wrapped, ErrSessionWorkspaceNotReady) {
		t.Errorf("expected errors.Is(wrapped, ErrSessionWorkspaceNotReady) to be true")
	}

	// Double-wrapped (as done in ensurePassthroughExecutionReady timeout path)
	doubleWrapped := fmt.Errorf("%w: timed out after 30s", ErrSessionWorkspaceNotReady)
	if !errors.Is(doubleWrapped, ErrSessionWorkspaceNotReady) {
		t.Errorf("expected errors.Is(doubleWrapped, ErrSessionWorkspaceNotReady) to be true")
	}
}

func TestErrSessionWorkspaceNotReady_UnrelatedError(t *testing.T) {
	unrelated := fmt.Errorf("some other error: %w", errors.New("connection timeout"))

	if errors.Is(unrelated, ErrSessionWorkspaceNotReady) {
		t.Errorf("expected errors.Is to be false for unrelated error")
	}
}

func TestResolveTaskEnvironmentID(t *testing.T) {
	t.Run("returns TaskEnvironmentID when execution carries it", func(t *testing.T) {
		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:                "exec-1",
			SessionID:         "session-A",
			TaskID:            "task-1",
			TaskEnvironmentID: "env-1",
			Status:            v1.AgentStatusRunning,
		})
		mgr := &Manager{executionStore: store, logger: newTestLogger()}

		got, err := mgr.ResolveTaskEnvironmentID(context.Background(), "session-A")
		if err != nil {
			t.Fatalf("ResolveTaskEnvironmentID returned error: %v", err)
		}
		if got != "env-1" {
			t.Errorf("ResolveTaskEnvironmentID = %q, want %q", got, "env-1")
		}
	})

	t.Run("returns TaskEnvironmentID from provider when no execution", func(t *testing.T) {
		provider := &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-X": {SessionID: "session-X", TaskEnvironmentID: "env-X"},
			},
		}
		mgr := &Manager{executionStore: NewExecutionStore(), logger: newTestLogger()}
		mgr.workspaceInfoProvider = provider

		got, err := mgr.ResolveTaskEnvironmentID(context.Background(), "session-X")
		if err != nil {
			t.Fatalf("ResolveTaskEnvironmentID returned error: %v", err)
		}
		if got != "env-X" {
			t.Errorf("ResolveTaskEnvironmentID = %q, want %q", got, "env-X")
		}
	})

	t.Run("errors when no execution and no provider", func(t *testing.T) {
		mgr := &Manager{executionStore: NewExecutionStore(), logger: newTestLogger()}

		_, err := mgr.ResolveTaskEnvironmentID(context.Background(), "session-X")
		if err == nil {
			t.Fatal("expected error")
		}
		if !containsString(err.Error(), "workspace info provider not configured") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("errors when execution has empty env", func(t *testing.T) {
		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID:        "exec-2",
			SessionID: "session-B",
			TaskID:    "task-2",
			Status:    v1.AgentStatusRunning,
		})
		mgr := &Manager{executionStore: store, logger: newTestLogger()}

		_, err := mgr.ResolveTaskEnvironmentID(context.Background(), "session-B")
		if err == nil {
			t.Fatal("expected error")
		}
		if !containsString(err.Error(), "no task environment ID") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("errors when provider returns empty env", func(t *testing.T) {
		provider := &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-C": {SessionID: "session-C"},
			},
		}
		mgr := &Manager{
			executionStore:        NewExecutionStore(),
			workspaceInfoProvider: provider,
			logger:                newTestLogger(),
		}

		_, err := mgr.ResolveTaskEnvironmentID(context.Background(), "session-C")
		if err == nil {
			t.Fatal("expected error")
		}
		if !containsString(err.Error(), "no task environment ID") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("two sessions sharing env resolve to the same scope", func(t *testing.T) {
		store := NewExecutionStore()
		store.Add(&AgentExecution{
			ID: "exec-A", SessionID: "sess-A", TaskID: "task-1",
			TaskEnvironmentID: "env-shared", Status: v1.AgentStatusRunning,
		})
		store.Add(&AgentExecution{
			ID: "exec-B", SessionID: "sess-B", TaskID: "task-1",
			TaskEnvironmentID: "env-shared", Status: v1.AgentStatusRunning,
		})
		mgr := &Manager{executionStore: store, logger: newTestLogger()}

		envA, err := mgr.ResolveTaskEnvironmentID(context.Background(), "sess-A")
		if err != nil {
			t.Fatalf("ResolveTaskEnvironmentID(sess-A): %v", err)
		}
		envB, err := mgr.ResolveTaskEnvironmentID(context.Background(), "sess-B")
		if err != nil {
			t.Fatalf("ResolveTaskEnvironmentID(sess-B): %v", err)
		}
		if envA != envB {
			t.Error("sessions in the same env must resolve to the same scope key")
		}
	})
}

func TestGetOrEnsureExecution(t *testing.T) {
	t.Run("returns existing execution from store", func(t *testing.T) {
		store := NewExecutionStore()
		execution := &AgentExecution{
			ID:        "exec-1",
			SessionID: "session-1",
			TaskID:    "task-1",
			Status:    v1.AgentStatusRunning,
		}
		store.Add(execution)

		providerCalled := false
		provider := &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{},
		}
		// Wrap to detect calls
		mgr := &Manager{
			executionStore:        store,
			workspaceInfoProvider: provider,
			logger:                newTestLogger(),
		}
		// Override provider to track calls
		trackingProvider := &trackingWorkspaceInfoProvider{
			delegate: provider,
			called:   &providerCalled,
		}
		mgr.workspaceInfoProvider = trackingProvider

		got, err := mgr.GetOrEnsureExecution(context.Background(), "session-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "exec-1" {
			t.Errorf("expected execution ID %q, got %q", "exec-1", got.ID)
		}
		if providerCalled {
			t.Error("provider should not be called when execution exists in store")
		}
	})

	t.Run("empty session ID returns error", func(t *testing.T) {
		mgr := &Manager{
			executionStore: NewExecutionStore(),
			logger:         newTestLogger(),
		}

		_, err := mgr.GetOrEnsureExecution(context.Background(), "")
		if err == nil {
			t.Fatal("expected error for empty session ID")
		}
		if err.Error() != "session_id is required" {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("no provider returns error", func(t *testing.T) {
		mgr := &Manager{
			executionStore:        NewExecutionStore(),
			workspaceInfoProvider: nil,
			logger:                newTestLogger(),
		}

		_, err := mgr.GetOrEnsureExecution(context.Background(), "session-1")
		if err == nil {
			t.Fatal("expected error when provider is nil")
		}
	})

	t.Run("provider error is propagated", func(t *testing.T) {
		provider := &mockWorkspaceInfoProvider{
			err: fmt.Errorf("database connection failed"),
		}
		mgr := &Manager{
			executionStore:        NewExecutionStore(),
			workspaceInfoProvider: provider,
			logger:                newTestLogger(),
		}

		_, err := mgr.GetOrEnsureExecution(context.Background(), "session-1")
		if err == nil {
			t.Fatal("expected error from provider")
		}
		if !containsString(err.Error(), "database connection failed") {
			t.Errorf("expected error to contain provider error, got: %v", err)
		}
	})

	t.Run("concurrent calls use singleflight", func(t *testing.T) {
		store := NewExecutionStore()
		var callCount atomic.Int32

		// Slow provider to create a race window
		provider := &slowWorkspaceInfoProvider{
			delay:     50 * time.Millisecond,
			callCount: &callCount,
			info: &WorkspaceInfo{
				TaskID:        "task-1",
				SessionID:     "session-1",
				WorkspacePath: "/tmp/test",
				AgentID:       "auggie",
			},
		}

		mgr := &Manager{
			executionStore:        store,
			workspaceInfoProvider: provider,
			logger:                newTestLogger(),
		}

		// Both calls will fail at createExecution (no executor backend),
		// but singleflight should ensure the provider is called at most once.
		var wg sync.WaitGroup
		wg.Add(2)
		for range 2 {
			go func() {
				defer wg.Done()
				_, _ = mgr.GetOrEnsureExecution(context.Background(), "session-1")
			}()
		}
		wg.Wait()

		if callCount.Load() > 1 {
			t.Errorf("expected provider to be called at most once (singleflight), got %d calls", callCount.Load())
		}
	})
}

func TestGetOrEnsureExecutionForEnvironment(t *testing.T) {
	t.Run("returns existing execution by environment", func(t *testing.T) {
		store := NewExecutionStore()
		execution := &AgentExecution{
			ID:                "exec-1",
			SessionID:         "session-1",
			TaskID:            "task-1",
			TaskEnvironmentID: "env-1",
			Status:            v1.AgentStatusRunning,
		}
		store.Add(execution)
		mgr := &Manager{executionStore: store, logger: newTestLogger()}

		got, err := mgr.GetOrEnsureExecutionForEnvironment(context.Background(), "env-1")
		if err != nil {
			t.Fatalf("GetOrEnsureExecutionForEnvironment returned error: %v", err)
		}
		if got.ID != "exec-1" {
			t.Errorf("execution ID = %q, want exec-1", got.ID)
		}
	})

	t.Run("empty environment ID returns error", func(t *testing.T) {
		mgr := &Manager{executionStore: NewExecutionStore(), logger: newTestLogger()}

		_, err := mgr.GetOrEnsureExecutionForEnvironment(context.Background(), "")
		if err == nil {
			t.Fatal("expected error")
		}
		if err.Error() != "task_environment_id is required" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing provider returns error instead of session fallback", func(t *testing.T) {
		mgr := &Manager{executionStore: NewExecutionStore(), logger: newTestLogger()}

		_, err := mgr.GetOrEnsureExecutionForEnvironment(context.Background(), "env-missing")
		if err == nil {
			t.Fatal("expected error")
		}
		if !containsString(err.Error(), "workspace info provider not configured") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("provider must return matching environment ID", func(t *testing.T) {
		provider := &mockWorkspaceInfoProvider{
			envInfos: map[string]*WorkspaceInfo{
				"env-want": {
					TaskID:            "task-1",
					SessionID:         "session-1",
					TaskEnvironmentID: "env-other",
					WorkspacePath:     "/tmp/test",
				},
			},
		}
		mgr := &Manager{
			executionStore:        NewExecutionStore(),
			workspaceInfoProvider: provider,
			logger:                newTestLogger(),
		}

		_, err := mgr.GetOrEnsureExecutionForEnvironment(context.Background(), "env-want")
		if err == nil {
			t.Fatal("expected error")
		}
		if !containsString(err.Error(), "workspace info resolved environment env-other") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("provider must return a workspace path", func(t *testing.T) {
		provider := &mockWorkspaceInfoProvider{
			envInfos: map[string]*WorkspaceInfo{
				"env-1": {
					TaskID:            "task-1",
					SessionID:         "session-1",
					TaskEnvironmentID: "env-1",
				},
			},
		}
		mgr := &Manager{
			executionStore:        NewExecutionStore(),
			workspaceInfoProvider: provider,
			logger:                newTestLogger(),
		}

		_, err := mgr.GetOrEnsureExecutionForEnvironment(context.Background(), "env-1")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrSessionWorkspaceNotReady) {
			t.Errorf("expected ErrSessionWorkspaceNotReady, got %v", err)
		}
	})
}

func TestEnsureWorkspaceExecutionForSession_EmptyTaskID(t *testing.T) {
	t.Run("resolves taskID from provider when empty", func(t *testing.T) {
		provider := &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-1": {
					TaskID:        "resolved-task-id",
					SessionID:     "session-1",
					WorkspacePath: "/tmp/test",
					AgentID:       "auggie",
				},
			},
		}

		mgr := &Manager{
			executionStore:        NewExecutionStore(),
			workspaceInfoProvider: provider,
			logger:                newTestLogger(),
		}

		// This will fail at createExecution (no executor backend),
		// but we can verify the taskID resolution by checking the error path.
		// The error should NOT be about empty taskID.
		_, err := mgr.EnsureWorkspaceExecutionForSession(context.Background(), "", "session-1")
		if err == nil {
			t.Fatal("expected error (no executor backend)")
		}
		// Should fail at createExecution, not at taskID validation
		if containsString(err.Error(), "task_id") || containsString(err.Error(), "taskID") {
			t.Errorf("unexpected taskID-related error: %v", err)
		}
	})

	t.Run("uses provided taskID when not empty", func(t *testing.T) {
		provider := &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-1": {
					TaskID:        "provider-task-id",
					SessionID:     "session-1",
					WorkspacePath: "/tmp/test",
					AgentID:       "auggie",
				},
			},
		}

		mgr := &Manager{
			executionStore:        NewExecutionStore(),
			workspaceInfoProvider: provider,
			logger:                newTestLogger(),
		}

		// This will fail at createExecution (no executor backend),
		// but the explicit taskID should be passed through.
		_, err := mgr.EnsureWorkspaceExecutionForSession(context.Background(), "explicit-task-id", "session-1")
		if err == nil {
			t.Fatal("expected error (no executor backend)")
		}
		// Should fail at createExecution, not at taskID
		if containsString(err.Error(), "task_id") || containsString(err.Error(), "taskID") {
			t.Errorf("unexpected taskID-related error: %v", err)
		}
	})
}

// --- test helpers ---

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// trackingWorkspaceInfoProvider wraps a provider and tracks whether it was called.
type trackingWorkspaceInfoProvider struct {
	delegate WorkspaceInfoProvider
	called   *bool
}

func (p *trackingWorkspaceInfoProvider) GetWorkspaceInfoForSession(ctx context.Context, taskID, sessionID string) (*WorkspaceInfo, error) {
	*p.called = true
	return p.delegate.GetWorkspaceInfoForSession(ctx, taskID, sessionID)
}

func (p *trackingWorkspaceInfoProvider) GetWorkspaceInfoForEnvironment(ctx context.Context, taskEnvironmentID string) (*WorkspaceInfo, error) {
	*p.called = true
	return p.delegate.GetWorkspaceInfoForEnvironment(ctx, taskEnvironmentID)
}

// slowWorkspaceInfoProvider adds a delay to simulate slow DB lookups for concurrency tests.
type slowWorkspaceInfoProvider struct {
	delay     time.Duration
	callCount *atomic.Int32
	info      *WorkspaceInfo
	err       error
}

func (p *slowWorkspaceInfoProvider) GetWorkspaceInfoForSession(_ context.Context, _, _ string) (*WorkspaceInfo, error) {
	p.callCount.Add(1)
	time.Sleep(p.delay)
	if p.err != nil {
		return nil, p.err
	}
	return p.info, nil
}

func (p *slowWorkspaceInfoProvider) GetWorkspaceInfoForEnvironment(_ context.Context, _ string) (*WorkspaceInfo, error) {
	p.callCount.Add(1)
	time.Sleep(p.delay)
	if p.err != nil {
		return nil, p.err
	}
	return p.info, nil
}
