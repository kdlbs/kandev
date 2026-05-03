package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
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

	t.Run("creates execution from provider and caches it", func(t *testing.T) {
		mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
			envInfos: map[string]*WorkspaceInfo{
				"env-new": {
					TaskID:            "task-1",
					SessionID:         "session-1",
					TaskEnvironmentID: "env-new",
					WorkspacePath:     "/workspace/task-1",
					AgentID:           "auggie",
				},
			},
		})

		got, err := mgr.GetOrEnsureExecutionForEnvironment(context.Background(), "env-new")
		if err != nil {
			t.Fatalf("GetOrEnsureExecutionForEnvironment returned error: %v", err)
		}
		if got.TaskEnvironmentID != "env-new" {
			t.Errorf("TaskEnvironmentID = %q, want env-new", got.TaskEnvironmentID)
		}
		if got.WorkspacePath != "/workspace/task-1" {
			t.Errorf("WorkspacePath = %q, want /workspace/task-1", got.WorkspacePath)
		}

		got2, err := mgr.GetOrEnsureExecutionForEnvironment(context.Background(), "env-new")
		if err != nil {
			t.Fatalf("second GetOrEnsureExecutionForEnvironment returned error: %v", err)
		}
		if got2.ID != got.ID {
			t.Errorf("cached execution ID = %q, want %q", got2.ID, got.ID)
		}
		if backend.createCount.Load() != 1 {
			t.Errorf("CreateInstance calls = %d, want 1", backend.createCount.Load())
		}
	})

	t.Run("concurrent creates use singleflight", func(t *testing.T) {
		mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
			envInfos: map[string]*WorkspaceInfo{
				"env-new": {
					TaskID:            "task-1",
					SessionID:         "session-1",
					TaskEnvironmentID: "env-new",
					WorkspacePath:     "/workspace/task-1",
					AgentID:           "auggie",
				},
			},
		})
		backend.entered = make(chan struct{}, 1)
		backend.barrier = make(chan struct{})

		type result struct {
			id  string
			err error
		}
		results := make(chan result, 2)
		for range 2 {
			go func() {
				execution, err := mgr.GetOrEnsureExecutionForEnvironment(context.Background(), "env-new")
				if err != nil {
					results <- result{"", err}
					return
				}
				results <- result{execution.ID, nil}
			}()
		}

		select {
		case <-backend.entered:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for CreateInstance to start")
		}
		runtime.Gosched()
		close(backend.barrier)

		var firstID string
		for range 2 {
			r := <-results
			if r.err != nil {
				t.Fatalf("GetOrEnsureExecutionForEnvironment returned error: %v", r.err)
			}
			if firstID == "" {
				firstID = r.id
			} else if r.id != firstID {
				t.Errorf("execution ID = %q, want %q (singleflight must return same execution)", r.id, firstID)
			}
		}
		if backend.createCount.Load() != 1 {
			t.Errorf("CreateInstance calls = %d, want 1", backend.createCount.Load())
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

func TestEnsureWorkspaceExecutionForSession_ReusesExistingTaskEnvironmentExecution(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		infos: map[string]*WorkspaceInfo{
			"session-1": {
				TaskID:            "task-1",
				SessionID:         "session-1",
				TaskEnvironmentID: "env-1",
				WorkspacePath:     "/workspace/task-1",
				AgentID:           "auggie",
			},
			"session-2": {
				TaskID:            "task-1",
				SessionID:         "session-2",
				TaskEnvironmentID: "env-1",
				WorkspacePath:     "/workspace/task-1",
				AgentID:           "auggie",
			},
		},
	})
	existing := &AgentExecution{
		ID:                "exec-existing",
		SessionID:         "session-1",
		TaskID:            "task-1",
		TaskEnvironmentID: "env-1",
		Status:            v1.AgentStatusRunning,
		agentctl:          newReadyAgentctlClient(t, newTestLogger()),
	}
	if err := mgr.executionStore.Add(existing); err != nil {
		t.Fatalf("add existing execution: %v", err)
	}

	got, err := mgr.EnsureWorkspaceExecutionForSession(context.Background(), "task-1", "session-2")
	if err != nil {
		t.Fatalf("EnsureWorkspaceExecutionForSession returned error: %v", err)
	}
	if got.ID != existing.ID {
		t.Fatalf("execution ID = %q, want existing environment execution %q", got.ID, existing.ID)
	}
	if got.SessionID != "session-1" {
		t.Fatalf("execution session ID = %q, want original owner session", got.SessionID)
	}
	if backend.createCount.Load() != 0 {
		t.Fatalf("CreateInstance calls = %d, want 0", backend.createCount.Load())
	}
}

// --- test helpers ---

type createInstanceExecutor struct {
	MockExecutor
	client       *agentctl.Client
	createCount  atomic.Int32
	stopCount    atomic.Int32
	authToken    string
	nonce        string
	delay        time.Duration
	progressStep string
	// Barrier-based deterministic synchronization for race tests.
	// Set entered (buffered 1) to receive a signal when CreateInstance begins.
	// Set barrier (unbuffered, closed to release) to block until the test is ready.
	entered chan struct{}
	barrier chan struct{}
}

func (e *createInstanceExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	var progress *PrepareStep
	if e.progressStep != "" && req.OnProgress != nil {
		step := beginStep(e.progressStep)
		progress = &step
		reportProgress(req.OnProgress, step, 0, 1)
	}
	if e.entered != nil {
		select {
		case e.entered <- struct{}{}:
		default:
		}
	}
	if e.barrier != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-e.barrier:
		}
	} else if e.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(e.delay):
		}
	}
	e.createCount.Add(1)
	if progress != nil {
		completeStepSuccess(progress)
		reportProgress(req.OnProgress, *progress, 0, 1)
	}
	return &ExecutorInstance{
		InstanceID:     req.InstanceID,
		TaskID:         req.TaskID,
		SessionID:      req.SessionID,
		RuntimeName:    string(e.Name()),
		Client:         e.client,
		WorkspacePath:  req.WorkspacePath,
		AuthToken:      e.authToken,
		BootstrapNonce: e.nonce,
	}, nil
}

func (e *createInstanceExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error {
	e.stopCount.Add(1)
	return nil
}

func newEnvironmentExecutionTestManager(t *testing.T, provider WorkspaceInfoProvider) (*Manager, *createInstanceExecutor) {
	t.Helper()
	log := newTestLogger()
	execRegistry := NewExecutorRegistry(log)
	backend := &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		client:       newReadyAgentctlClient(t, log),
	}
	execRegistry.Register(backend)

	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log,
	)
	mgr.workspaceInfoProvider = provider
	t.Cleanup(func() { close(mgr.stopCh) })
	return mgr, backend
}

func newReadyAgentctlClient(t *testing.T, log *logger.Logger) *agentctl.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	host, portString, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("split test server host: %v", err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return agentctl.NewClient(host, port, log)
}

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

// TestGetOrEnsureExecution_DedupAcrossEnvAndSessionPaths is the regression
// test for the orphaned-claude-acp leak introduced by PR #758, which
// keyed GetOrEnsureExecutionForEnvironment by `"env:" + envID` instead of
// the sessionID. Two concurrent paths for the same session each saw
// "no execution exists" for their own key, both called createExecution,
// and ExecutionStore.Add silently overwrote the bySession index — orphaning
// the first execution's claude-agent-acp subprocess.
//
// After the fix, both paths share the sessionID-keyed singleflight bucket,
// so concurrent callers must observe the same execution and CreateInstance
// must be invoked exactly once.
func TestGetOrEnsureExecution_DedupAcrossEnvAndSessionPaths(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		infos: map[string]*WorkspaceInfo{
			"session-1": {
				TaskID:            "task-1",
				SessionID:         "session-1",
				TaskEnvironmentID: "env-1",
				WorkspacePath:     "/workspace/task-1",
				AgentID:           "auggie",
			},
		},
		envInfos: map[string]*WorkspaceInfo{
			"env-1": {
				TaskID:            "task-1",
				SessionID:         "session-1",
				TaskEnvironmentID: "env-1",
				WorkspacePath:     "/workspace/task-1",
				AgentID:           "auggie",
			},
		},
	})
	// Use a barrier channel so that the test is deterministic: CreateInstance
	// blocks until we explicitly release it, giving the env-path goroutine
	// time to join the same singleflight flight before we let it complete.
	backend.entered = make(chan struct{}, 1)
	backend.barrier = make(chan struct{})

	type result struct {
		exec *AgentExecution
		err  error
	}
	results := make(chan result, 2)

	go func() {
		exec, err := mgr.GetOrEnsureExecution(context.Background(), "session-1")
		results <- result{exec, err}
	}()
	go func() {
		exec, err := mgr.GetOrEnsureExecutionForEnvironment(context.Background(), "env-1")
		results <- result{exec, err}
	}()

	// Wait for the singleflight winner to enter CreateInstance, then yield so
	// the other goroutine can join the same flight before we release the barrier.
	select {
	case <-backend.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for CreateInstance to start")
	}
	runtime.Gosched()
	close(backend.barrier)

	r1 := <-results
	r2 := <-results
	if r1.err != nil || r2.err != nil {
		t.Fatalf("unexpected errors: %v / %v", r1.err, r2.err)
	}
	if r1.exec.ID != r2.exec.ID {
		t.Errorf("execution IDs differ: session-path=%s env-path=%s — duplicate executions created (the leak bug)",
			r1.exec.ID, r2.exec.ID)
	}
	if got := backend.createCount.Load(); got != 1 {
		t.Errorf("CreateInstance called %d times, want 1 (singleflight should deduplicate)", got)
	}
}
