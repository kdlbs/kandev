package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
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

func TestPrepareExecutionCreateRequest_ReuseRequiredDockerUsesEnvironmentControlToken(t *testing.T) {
	mgr := newTestManager(t)
	store := newInMemorySecretStore()
	store.store["session-auth"] = &secrets.SecretWithValue{Value: "sibling-session-token"}
	store.store["container-control"] = &secrets.SecretWithValue{Value: "environment-control-token"}
	mgr.runtimeSecretStore = store

	prepared, err := mgr.prepareExecutionCreateRequest(context.Background(), "task-1", &WorkspaceInfo{
		TaskID:            "task-1",
		SessionID:         "session-2",
		TaskEnvironmentID: "environment-1",
		WorkspacePath:     "/workspace",
		AgentID:           "auggie",
		ExecutorType:      string(models.ExecutorTypeLocalDocker),
		Metadata: map[string]interface{}{
			MetadataKeyContainerID:                "container-1",
			MetadataKeyAuthTokenSecret:            "session-auth",
			MetadataKeyContainerControlAuthSecret: "container-control",
		},
	}, "execution-2")
	if err != nil {
		t.Fatalf("prepareExecutionCreateRequest() error = %v", err)
	}

	if got := prepared.request.AuthToken; got != "environment-control-token" {
		t.Fatalf("reconnect auth token = %q, want environment control token", got)
	}
	if !prepared.request.WorkspaceReuseRequired {
		t.Fatal("on-demand execution for a task environment must attach rather than provision a replacement workspace")
	}
}

func TestResolveSessionRuntimeDoesNotCreateUnsupportedExecution(t *testing.T) {
	provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-ssh": {
			SessionID:    "session-ssh",
			ExecutorType: string(models.ExecutorTypeSSH),
		},
	}}
	mgr, backend := newEnvironmentExecutionTestManager(t, provider)

	runtimeName, err := mgr.ResolveSessionRuntime(context.Background(), "session-ssh")
	if err != nil {
		t.Fatalf("ResolveSessionRuntime() error = %v", err)
	}
	if runtimeName != agentruntime.RuntimeSSH {
		t.Fatalf("ResolveSessionRuntime() = %q, want %q", runtimeName, agentruntime.RuntimeSSH)
	}
	if got := backend.createCount.Load(); got != 0 {
		t.Fatalf("CreateInstance calls = %d, want 0", got)
	}
	if _, exists := mgr.GetExecutionBySessionID("session-ssh"); exists {
		t.Fatal("ResolveSessionRuntime() must not create an in-memory execution")
	}
}

func TestResolveSessionRuntimeChecksSessionAccessBeforeLookup(t *testing.T) {
	denied := errors.New("session access denied")
	provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-ssh": {SessionID: "session-ssh", ExecutorType: string(models.ExecutorTypeSSH)},
	}}
	mgr, _ := newEnvironmentExecutionTestManager(t, provider)
	mgr.SetSessionAccessChecker(func(context.Context, string) error { return denied })

	_, err := mgr.ResolveSessionRuntime(context.Background(), "session-ssh")
	if !errors.Is(err, denied) {
		t.Fatalf("ResolveSessionRuntime() error = %v, want access error", err)
	}
	if provider.sessionCalls != 0 {
		t.Fatalf("workspace provider calls = %d, want 0 before authorization", provider.sessionCalls)
	}
}

func TestGetOrEnsureExecutionLeaderCancellationDoesNotAbortLiveWaiter(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		infos: map[string]*WorkspaceInfo{
			"session-shared": {
				TaskID: "task-1", SessionID: "session-shared", TaskEnvironmentID: "env-1",
				WorkspacePath: "/workspace/task-1", AgentID: "auggie",
			},
		},
	})
	backend.entered = make(chan struct{}, 1)
	backend.barrier = make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(backend.barrier)
		}
	}()

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()
	leaderResult := make(chan error, 1)
	go func() {
		_, err := mgr.GetOrEnsureExecution(leaderCtx, "session-shared")
		leaderResult <- err
	}()
	select {
	case <-backend.entered:
	case <-time.After(time.Second):
		t.Fatal("leader did not reach CreateInstance")
	}

	followerCtx := &doneObservedContext{
		Context:  context.Background(),
		doneRead: make(chan struct{}),
	}
	followerResult := make(chan error, 1)
	go func() {
		_, err := mgr.GetOrEnsureExecution(followerCtx, "session-shared")
		followerResult <- err
	}()
	select {
	case <-followerCtx.doneRead:
	case <-time.After(time.Second):
		t.Fatal("follower did not join the coalesced execution")
	}

	cancelLeader()
	if err := <-leaderResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader error = %v, want context cancellation", err)
	}
	close(backend.barrier)
	released = true
	if err := <-followerResult; err != nil {
		t.Fatalf("live follower failed after leader cancellation: %v", err)
	}
}

func TestAwaitCoalescedResultPrefersCanceledContext(t *testing.T) {
	for range 100 {
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan singleflight.Result, 1)
		result <- singleflight.Result{Val: "created"}
		cancel()

		_, err := awaitCoalescedResult(ctx, result)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("await error = %v, want context cancellation", err)
		}
	}
}

func TestShortDeadlineLeaderDoesNotAbortLiveCoalescedWaiter(t *testing.T) {
	provider := &notifyingWorkspaceInfoProvider{
		mockWorkspaceInfoProvider: &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-shared": {
					TaskID: "task-1", SessionID: "session-shared", TaskEnvironmentID: "env-1",
					WorkspacePath: "/workspace/task-1", AgentID: "auggie",
				},
			},
		},
		environmentReached: make(chan struct{}),
	}
	mgr, backend := newEnvironmentExecutionTestManager(t, provider)
	backend.entered = make(chan struct{}, 1)
	backend.barrier = make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(backend.barrier)
		}
	}()

	leaderCtx, cancelLeader := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancelLeader()
	leaderResult := make(chan error, 1)
	go func() {
		_, err := mgr.GetOrEnsureExecution(leaderCtx, "session-shared")
		leaderResult <- err
	}()
	select {
	case <-backend.entered:
	case <-time.After(time.Second):
		t.Fatal("leader did not reach CreateInstance")
	}

	followerResult := make(chan error, 1)
	go func() {
		_, err := mgr.GetOrEnsureExecutionForEnvironment(context.Background(), "env-1")
		followerResult <- err
	}()
	select {
	case <-provider.environmentReached:
	case <-time.After(time.Second):
		t.Fatal("follower did not resolve its environment")
	}
	select {
	case err := <-followerResult:
		t.Fatalf("follower returned before shared creation completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	if err := <-leaderResult; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("leader error = %v, want context deadline", err)
	}
	close(backend.barrier)
	released = true
	if err := <-followerResult; err != nil {
		t.Fatalf("live follower failed after leader deadline: %v", err)
	}
}

func TestCoalescedExecutionStopsWithManager(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		infos: map[string]*WorkspaceInfo{
			"session-shutdown": {
				TaskID: "task-1", SessionID: "session-shutdown", TaskEnvironmentID: "env-1",
				WorkspacePath: "/workspace/task-1", AgentID: "auggie",
			},
		},
	})
	backend.entered = make(chan struct{}, 1)
	backend.barrier = make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(backend.barrier)
		}
	}()

	result := make(chan error, 1)
	go func() {
		_, err := mgr.GetOrEnsureExecution(context.Background(), "session-shutdown")
		result <- err
	}()
	select {
	case <-backend.entered:
	case <-time.After(time.Second):
		t.Fatal("creation did not reach CreateInstance")
	}
	mgr.closeStopCh()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("creation error = %v, want manager cancellation", err)
		}
	case <-time.After(100 * time.Millisecond):
		close(backend.barrier)
		released = true
		<-result
		t.Fatal("manager shutdown did not cancel coalesced creation")
	}
}

func TestCoalescedExecutionCreationHasManagerDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := newTestLogger()
		execRegistry := NewExecutorRegistry(log)
		backend := &createInstanceExecutor{
			MockExecutor: MockExecutor{name: executor.NameStandalone},
			entered:      make(chan struct{}, 1),
			barrier:      make(chan struct{}),
		}
		execRegistry.Register(backend)
		mgr := NewManager(
			newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{},
			&MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log,
		)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{
			infos: map[string]*WorkspaceInfo{
				"session-deadline": {
					TaskID: "task-1", SessionID: "session-deadline", TaskEnvironmentID: "env-1",
					WorkspacePath: "/workspace/task-1", AgentID: "auggie",
				},
			},
		}
		cleanupManagerStopCh(t, mgr)
		coordinator := activity.NewCoordinator(activity.Options{})
		mgr.SetActivityCoordinator(coordinator)

		startedAt := time.Now()
		result := make(chan error, 1)
		go func() {
			_, err := mgr.GetOrEnsureExecution(context.Background(), "session-deadline")
			result <- err
		}()
		<-backend.entered

		select {
		case err := <-result:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("creation error = %v, want manager deadline", err)
			}
			if elapsed := time.Since(startedAt); elapsed != constants.AgentLaunchTimeout {
				t.Fatalf("manager deadline elapsed after %v, want %v", elapsed, constants.AgentLaunchTimeout)
			}
		case <-time.After(constants.AgentLaunchTimeout + time.Second):
			t.Fatal("blocked creation outlived the manager startup deadline")
		}

		maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
		if err != nil {
			t.Fatalf("activity remained held after manager deadline: %v", err)
		}
		maintenance.Release()
	})
}

func TestCreateExecutionRollsBackWhenRegistrationCannotPersist(t *testing.T) {
	const sessionID = "session-create-persist-failure"
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		infos: map[string]*WorkspaceInfo{
			sessionID: {
				TaskID: "task-create-persist-failure", SessionID: sessionID,
				TaskEnvironmentID: "env-create-persist-failure", WorkspacePath: "/workspace/task",
				AgentID: "auggie",
			},
		},
	})
	mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{session: &models.TaskSession{
		ID: sessionID, TaskID: "task-create-persist-failure", State: models.TaskSessionStateStarting,
	}})
	writer := &launchRegistrationWriter{
		upserted:  make(chan struct{}),
		upsertErr: errors.New("database is locked"),
	}
	mgr.SetExecutorRunningWriter(writer)

	_, err := mgr.GetOrEnsureExecution(context.Background(), sessionID)
	if err == nil || !strings.Contains(err.Error(), "persist execution registration") {
		t.Fatalf("GetOrEnsureExecution error = %v, want persistence failure", err)
	}
	if got := backend.stopCount.Load(); got != 1 {
		t.Fatalf("StopInstance calls = %d, want 1", got)
	}
	if _, exists := mgr.executionStore.GetBySessionID(sessionID); exists {
		t.Fatal("workspace execution survived failed durable registration")
	}
}

const (
	terminalSessionID     = "session-terminal-shell"
	terminalEnvironmentID = "env-terminal-shell"
	terminalTaskID        = "task-terminal-shell"
)

func newTerminalSessionManager(t *testing.T, state models.TaskSessionState) (*Manager, *createInstanceExecutor) {
	t.Helper()
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		infos: map[string]*WorkspaceInfo{
			terminalSessionID: {
				TaskID: terminalTaskID, SessionID: terminalSessionID,
				TaskEnvironmentID: terminalEnvironmentID, WorkspacePath: "/workspace/task",
				AgentID: "auggie",
			},
		},
	})
	mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{session: &models.TaskSession{
		ID: terminalSessionID, TaskID: terminalTaskID, State: state,
	}})
	return mgr, backend
}

// A shell terminal left open on a terminal session reconnects on a timer, and
// the file, git, LSP and port panels poll their own session-keyed path. Every
// entry point must be rejected from the session state alone: creating the
// runtime instance first and rolling it back turned an idle panel into a
// spawn/teardown loop for as long as the tab stayed open.
func TestEnsureExecutionRejectsTerminalSessionWithoutCreatingInstance(t *testing.T) {
	entryPoints := []struct {
		name string
		call func(*Manager) error
	}{
		{"GetOrEnsureExecutionForEnvironment", func(m *Manager) error {
			_, err := m.GetOrEnsureExecutionForEnvironment(context.Background(), terminalEnvironmentID)
			return err
		}},
		{"GetOrEnsureExecution", func(m *Manager) error {
			_, err := m.GetOrEnsureExecution(context.Background(), terminalSessionID)
			return err
		}},
		{"EnsureWorkspaceExecutionForSession", func(m *Manager) error {
			_, err := m.EnsureWorkspaceExecutionForSession(context.Background(), terminalTaskID, terminalSessionID)
			return err
		}},
	}
	states := []models.TaskSessionState{
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled,
		models.TaskSessionStateCompleted,
	}
	for _, entryPoint := range entryPoints {
		for _, state := range states {
			t.Run(entryPoint.name+"/"+string(state), func(t *testing.T) {
				mgr, backend := newTerminalSessionManager(t, state)

				if err := entryPoint.call(mgr); !errors.Is(err, ErrSessionTerminal) {
					t.Fatalf("%s error = %v, want ErrSessionTerminal", entryPoint.name, err)
				}
				if got := backend.createCount.Load(); got != 0 {
					t.Fatalf("CreateInstance calls = %d, want 0", got)
				}
				if got := backend.stopCount.Load(); got != 0 {
					t.Fatalf("StopInstance calls = %d, want 0", got)
				}
				if _, exists := mgr.executionStore.GetBySessionID(terminalSessionID); exists {
					t.Fatal("terminal session must not register an execution")
				}
			})
		}
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

func TestGetOrEnsureTaskHostForEnvironment(t *testing.T) {
	t.Run("creates one dedicated host beside multiple session executions", func(t *testing.T) {
		mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
			envInfos: map[string]*WorkspaceInfo{
				"env-1": {
					TaskID: "task-1", SessionID: "session-2", TaskEnvironmentID: "env-1",
					WorkspacePath: "/workspace/task-1", AgentProfileID: "session-profile-2",
				},
			},
		})
		backend.existingOnlyAbsent = true
		for _, execution := range []*AgentExecution{
			{ID: "session-exec-1", TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1"},
			{ID: "session-exec-2", TaskID: "task-1", SessionID: "session-2", TaskEnvironmentID: "env-1"},
		} {
			if err := mgr.executionStore.Add(execution); err != nil {
				t.Fatalf("seed session execution: %v", err)
			}
		}

		host, err := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-1")
		if err != nil {
			t.Fatalf("GetOrEnsureTaskHostForEnvironment: %v", err)
		}
		if !host.IsTaskHost {
			t.Fatal("created execution is not marked as task host")
		}
		if host.SessionID == "session-1" || host.SessionID == "session-2" {
			t.Fatalf("task host runtime session ID = %q, must not inherit a task session", host.SessionID)
		}
		if !host.IsAgentctlReady() {
			t.Fatal("task host returned before agentctl became ready")
		}
		if got := backend.createCount.Load(); got != 2 {
			t.Fatalf("CreateInstance calls = %d, want absence probe plus one launch", got)
		}
		if backend.lastRequest == nil || !backend.lastRequest.IsTaskHost {
			t.Fatalf("executor request = %#v, want dedicated task-host marker", backend.lastRequest)
		}
		if backend.lastRequest.RequireExistingInstance {
			t.Fatalf("last executor request = %#v, want new task-host launch", backend.lastRequest)
		}
		if backend.lastRequest.AgentConfig != nil || backend.lastRequest.Protocol != "" ||
			backend.lastRequest.AgentProfileID != "" || backend.lastRequest.OfficeAgentProfileID != "" {
			t.Fatalf("task host inherited session agent identity: %#v", backend.lastRequest)
		}
		if got := backend.lastRequest.Env["KANDEV_TASK_ID"]; got != "task-1" {
			t.Fatalf("task host KANDEV_TASK_ID = %q, want task-1", got)
		}
		if got := backend.lastRequest.Env["KANDEV_SESSION_ID"]; got != taskHostRuntimeSessionPrefix+"env-1" {
			t.Fatalf("task host KANDEV_SESSION_ID = %q, want synthetic task-host identity", got)
		}
		if _, exists := backend.lastRequest.Env["KANDEV_AGENT_PROFILE_ID"]; exists {
			t.Fatal("task host environment inherited KANDEV_AGENT_PROFILE_ID")
		}
		if got, ok := mgr.executionStore.GetBySessionID("session-2"); !ok || got.ID != "session-exec-2" {
			t.Fatalf("session execution changed: %#v, %v", got, ok)
		}

		again, err := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-1")
		if err != nil {
			t.Fatalf("second GetOrEnsureTaskHostForEnvironment: %v", err)
		}
		if again.ID != host.ID || backend.createCount.Load() != 2 {
			t.Fatalf("second host = %q with %d executor calls; want %q with 2", again.ID, backend.createCount.Load(), host.ID)
		}
	})

	t.Run("concurrent callers create exactly one host", func(t *testing.T) {
		mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
			envInfos: map[string]*WorkspaceInfo{
				"env-1": {
					TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1",
					WorkspacePath: "/workspace/task-1", AgentID: "auggie",
				},
			},
		})
		backend.entered = make(chan struct{}, 1)
		backend.barrier = make(chan struct{})
		results := make(chan *AgentExecution, 2)
		errs := make(chan error, 2)
		for range 2 {
			go func() {
				execution, err := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-1")
				results <- execution
				errs <- err
			}()
		}
		select {
		case <-backend.entered:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for task-host creation")
		}
		runtime.Gosched()
		close(backend.barrier)
		first, second := <-results, <-results
		if err := <-errs; err != nil {
			t.Fatalf("first concurrent caller: %v", err)
		}
		if err := <-errs; err != nil {
			t.Fatalf("second concurrent caller: %v", err)
		}
		if first.ID != second.ID || backend.createCount.Load() != 1 {
			t.Fatalf("hosts = %q/%q, creates = %d; want one", first.ID, second.ID, backend.createCount.Load())
		}
	})

	t.Run("cached host remains private until readiness and credentials commit", func(t *testing.T) {
		mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
			envInfos: map[string]*WorkspaceInfo{
				"env-1": {
					TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1",
					WorkspacePath: "/workspace/task-1",
				},
			},
		})
		healthEntered := make(chan struct{})
		healthRelease := make(chan struct{})
		var healthOnce sync.Once
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/health" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			healthOnce.Do(func() { close(healthEntered) })
			<-healthRelease
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)
		parsed, err := url.Parse(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		host, portString, err := net.SplitHostPort(parsed.Host)
		if err != nil {
			t.Fatal(err)
		}
		port, err := strconv.Atoi(portString)
		if err != nil {
			t.Fatal(err)
		}
		backend.client = agentctl.NewClient(host, port, mgr.logger)

		type result struct {
			execution *AgentExecution
			err       error
		}
		firstDone := make(chan result, 1)
		go func() {
			execution, ensureErr := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-1")
			firstDone <- result{execution: execution, err: ensureErr}
		}()
		<-healthEntered

		_, exposedEarly, lookupErr := mgr.GetTaskHostForEnvironment(context.Background(), "env-1")
		secondDone := make(chan result, 1)
		go func() {
			execution, ensureErr := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-1")
			secondDone <- result{execution: execution, err: ensureErr}
		}()
		var earlyResult *result
		select {
		case got := <-secondDone:
			earlyResult = &got
		case <-time.After(100 * time.Millisecond):
		}
		close(healthRelease)
		first := <-firstDone
		second := result{}
		if earlyResult != nil {
			second = *earlyResult
		} else {
			second = <-secondDone
		}

		if !errors.Is(lookupErr, ErrTaskHostNotReady) {
			t.Fatalf("lookup error = %v, want readiness uncertainty", lookupErr)
		}
		if exposedEarly {
			t.Fatal("GetTaskHostForEnvironment exposed host before readiness committed")
		}
		if earlyResult != nil {
			t.Fatal("second ensure returned cached host before readiness committed")
		}
		if first.err != nil || second.err != nil {
			t.Fatalf("ensure errors = %v / %v", first.err, second.err)
		}
		if first.execution != second.execution || backend.createCount.Load() != 1 {
			t.Fatalf("hosts = %p/%p creates=%d, want one committed host", first.execution, second.execution, backend.createCount.Load())
		}
	})
}

func TestGetTaskHostForEnvironmentReattachesDetachedRuntimeWithoutCreatingReplacement(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		envInfos: map[string]*WorkspaceInfo{
			"env-1": {
				TaskID: "task-1", TaskEnvironmentID: "env-1",
				WorkspacePath: "/workspace/task-1",
			},
		},
	})
	host, err := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-1")
	if err != nil {
		t.Fatal(err)
	}
	mgr.detachTaskHostForBackendShutdown(host)
	backend.client = newReadyAgentctlClient(t, mgr.logger)

	reattached, exists, err := mgr.GetTaskHostForEnvironment(context.Background(), "env-1")
	if err != nil {
		t.Fatal(err)
	}
	if !exists || reattached == nil || reattached.ID != host.ID {
		t.Fatalf("reattached=%#v exists=%v, want stable detached host %q", reattached, exists, host.ID)
	}
	if backend.lastRequest == nil || !backend.lastRequest.RequireExistingInstance {
		t.Fatalf("reattach request=%#v, want existing-only probe", backend.lastRequest)
	}
	if backend.createCount.Load() != 2 {
		t.Fatalf("executor calls=%d, want create plus physical reattach", backend.createCount.Load())
	}
}

func TestStopTaskHostForEnvironmentReattachesDetachedRuntimeBeforeReaping(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		envInfos: map[string]*WorkspaceInfo{
			"env-1": {
				TaskID: "task-1", TaskEnvironmentID: "env-1",
				WorkspacePath: "/workspace/task-1",
			},
		},
	})
	host, err := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-1")
	if err != nil {
		t.Fatal(err)
	}
	mgr.detachTaskHostForBackendShutdown(host)
	backend.client = newReadyAgentctlClient(t, mgr.logger)

	proved, err := mgr.StopTaskHostForEnvironment(context.Background(), "env-1", "task_archived")
	if err != nil {
		t.Fatal(err)
	}
	if !proved || backend.stopCount.Load() != 1 {
		t.Fatalf("proved=%v stops=%d, want detached runtime reaped", proved, backend.stopCount.Load())
	}
	if _, exists := mgr.executionStore.GetTaskHostByEnvironmentID("env-1"); exists {
		t.Fatal("reattached task host remains tracked after stop")
	}
}

func TestTaskHostPhysicalAbsenceIsProvenWithoutCreatingRuntime(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		envInfos: map[string]*WorkspaceInfo{
			"env-1": {
				TaskID: "task-1", TaskEnvironmentID: "env-1",
				WorkspacePath: "/workspace/task-1",
			},
		},
	})
	backend.existingOnlyAbsent = true

	execution, exists, err := mgr.GetTaskHostForEnvironment(context.Background(), "env-1")
	if err != nil {
		t.Fatal(err)
	}
	if exists || execution != nil {
		t.Fatalf("execution=%#v exists=%v, want proven absence", execution, exists)
	}
	if backend.lastRequest == nil || !backend.lastRequest.RequireExistingInstance {
		t.Fatalf("probe request=%#v, want existing-only", backend.lastRequest)
	}
	proved, err := mgr.StopTaskHostForEnvironment(context.Background(), "env-1", "task_archived")
	if err != nil || !proved {
		t.Fatalf("absence proof=%v error=%v", proved, err)
	}
}

func TestFailedTaskHostRollbackRetainsRuntimeOwnership(t *testing.T) {
	cleanupFailure := errors.New("task-host cleanup failed")
	backend := &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		stopErr:      cleanupFailure,
	}
	execution := &AgentExecution{
		ID: "task-host", TaskID: "task-1", SessionID: taskHostRuntimeSessionPrefix + "env-1",
		TaskEnvironmentID: "env-1", RuntimeName: agentruntime.RuntimeStandalone, IsTaskHost: true,
	}
	manager := &Manager{executionStore: NewExecutionStore(), logger: newTestLogger()}
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}

	_, err := manager.finishTaskHostExecution(
		context.Background(),
		"task-1",
		&WorkspaceInfo{TaskEnvironmentID: "env-1"},
		backend,
		&ExecutorInstance{InstanceID: execution.ID},
		execution,
		false,
	)
	if err == nil {
		t.Fatal("task-host readiness failure unexpectedly succeeded")
	}
	if current, exists := manager.executionStore.GetTaskHostByEnvironmentID("env-1"); !exists || current != execution {
		t.Fatalf("task-host cleanup handle = %#v, %v; want original execution", current, exists)
	}
	if backend.stopCount.Load() != 1 {
		t.Fatalf("task-host cleanup attempts = %d, want 1", backend.stopCount.Load())
	}
}

func TestTaskHostCredentialPersistenceFailurePreventsReady(t *testing.T) {
	persistFailure := errors.New("secret database unavailable")
	store := newInMemorySecretStore()
	store.err = persistFailure
	backend := &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameDocker},
	}
	execution := &AgentExecution{
		ID: "task-host", TaskID: "task-1", SessionID: taskHostRuntimeSessionPrefix + "env-1",
		TaskEnvironmentID: "env-1", RuntimeName: agentruntime.RuntimeDocker, IsTaskHost: true,
		agentctl: newReadyAgentctlClient(t, newTestLogger()),
	}
	manager := &Manager{
		executionStore: NewExecutionStore(), logger: newTestLogger(), runtimeSecretStore: store,
		taskEnvironmentRuntimeSecretWriter: &captureTaskEnvironmentRuntimeSecretWriter{},
	}
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}

	_, err := manager.finishTaskHostExecution(
		context.Background(), "task-1", &WorkspaceInfo{TaskEnvironmentID: "env-1"},
		backend, &ExecutorInstance{InstanceID: execution.ID, AuthToken: "rotated-token"}, execution,
		false,
	)
	if !errors.Is(err, persistFailure) {
		t.Fatalf("task-host credential persistence error = %v", err)
	}
	if execution.IsAgentctlReady() {
		t.Fatal("task host became ready before durable credentials committed")
	}
	if backend.stopCount.Load() != 1 {
		t.Fatalf("rollback cleanup attempts = %d, want 1", backend.stopCount.Load())
	}
}

func TestTaskHostExecutionIDIsStablePerTaskEnvironment(t *testing.T) {
	first := taskHostExecutionID("env-1")
	if first == "" || first != taskHostExecutionID("env-1") {
		t.Fatalf("task-host execution ID is not stable: %q", first)
	}
	if first == taskHostExecutionID("env-2") {
		t.Fatalf("distinct task environments share task-host execution ID %q", first)
	}
	if _, err := uuid.Parse(first); err != nil {
		t.Fatalf("task-host execution ID %q is not an opaque UUID: %v", first, err)
	}
}

func TestStopTaskHostForEnvironmentLeavesSessionExecutionsRunning(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		envInfos: map[string]*WorkspaceInfo{
			"env-1": {
				TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1",
				WorkspacePath: "/workspace/task-1", AgentID: "auggie",
			},
		},
	})
	sessionExecution := &AgentExecution{
		ID: "session-exec", TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1",
	}
	if err := mgr.executionStore.Add(sessionExecution); err != nil {
		t.Fatalf("seed session execution: %v", err)
	}
	host, err := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-1")
	if err != nil {
		t.Fatalf("create task host: %v", err)
	}
	proved, err := mgr.StopTaskHostForEnvironment(context.Background(), "env-1", "task_archived")
	if err != nil {
		t.Fatalf("StopTaskHostForEnvironment: %v", err)
	}
	if !proved {
		t.Fatal("successful task-host stop did not prove the process tree gone")
	}
	if _, ok := mgr.executionStore.Get(host.ID); ok {
		t.Fatal("task host remains tracked")
	}
	if got, ok := mgr.executionStore.GetBySessionID("session-1"); !ok || got.ID != sessionExecution.ID {
		t.Fatalf("session execution after task-host stop = %#v, %v", got, ok)
	}
	if got := backend.stopCount.Load(); got != 1 {
		t.Fatalf("StopInstance calls = %d, want task host only", got)
	}
}

func TestStopTaskHostForEnvironmentRetainsOwnershipWhenRuntimeStopFails(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		envInfos: map[string]*WorkspaceInfo{
			"env-1": {
				TaskID: "task-1", TaskEnvironmentID: "env-1",
				WorkspacePath: "/workspace/task-1",
			},
		},
	})
	backend.stopErr = errors.New("runtime teardown failed")

	host, err := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-1")
	if err != nil {
		t.Fatalf("create task host: %v", err)
	}
	proved, err := mgr.StopTaskHostForEnvironment(context.Background(), "env-1", "task_archived")
	if err == nil || !strings.Contains(err.Error(), "runtime teardown failed") {
		t.Fatalf("StopTaskHostForEnvironment error = %v, want runtime teardown failure", err)
	}
	if proved {
		t.Fatal("failed task-host stop claimed the process tree was gone")
	}
	if got, ok := mgr.executionStore.Get(host.ID); !ok || got.ID != host.ID {
		t.Fatalf("task host was untracked after failed teardown: %#v, %v", got, ok)
	}
	if got, ok := mgr.executionStore.GetTaskHostByEnvironmentID("env-1"); !ok || got.ID != host.ID {
		t.Fatalf("task-host ownership index was cleared after failed teardown: %#v, %v", got, ok)
	}
}

func TestRecoverTaskHostForEnvironmentEvictsOnlyProvenDeadHost(t *testing.T) {
	mgr, _ := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		envInfos: map[string]*WorkspaceInfo{
			"env-1": {
				TaskID: "task-1", TaskEnvironmentID: "env-1",
				WorkspacePath: "/workspace/task-1",
			},
		},
	})
	host, err := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-1")
	if err != nil {
		t.Fatal(err)
	}

	recovered, err := mgr.RecoverTaskHostForEnvironment(context.Background(), "env-1")
	if err != nil || recovered {
		t.Fatalf("healthy recovery = %v, %v", recovered, err)
	}
	if _, exists := mgr.executionStore.Get(host.ID); !exists {
		t.Fatal("healthy task host was evicted")
	}

	host.agentctl.Close()
	host.agentctl = agentctl.NewClient("127.0.0.1", 1, mgr.logger)
	recovered, err = mgr.RecoverTaskHostForEnvironment(context.Background(), "env-1")
	if err != nil || !recovered {
		t.Fatalf("dead recovery = %v, %v", recovered, err)
	}
	if _, exists := mgr.executionStore.Get(host.ID); exists {
		t.Fatal("dead task host remains tracked")
	}
}

func TestEnsureReattachesUnhealthyLiveTaskHostWithoutCreatingReplacement(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		envInfos: map[string]*WorkspaceInfo{
			"env-1": {
				TaskID: "task-1", TaskEnvironmentID: "env-1",
				WorkspacePath: "/workspace/task-1",
			},
		},
	})
	host, err := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-1")
	if err != nil {
		t.Fatal(err)
	}
	host.agentctl.Close()
	host.agentctl = agentctl.NewClient("127.0.0.1", 1, mgr.logger)

	recovered, err := mgr.RecoverTaskHostForEnvironment(context.Background(), "env-1")
	if err != nil || !recovered {
		t.Fatalf("unhealthy recovery = %v, %v", recovered, err)
	}
	backend.client = newReadyAgentctlClient(t, mgr.logger)

	reattached, err := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-1")
	if err != nil {
		t.Fatal(err)
	}
	if reattached.ID != host.ID {
		t.Fatalf("reattached ID = %q, want stable physical host %q", reattached.ID, host.ID)
	}
	if backend.lastRequest == nil || !backend.lastRequest.RequireExistingInstance {
		t.Fatalf("recovery request = %#v, want existing-only physical reattachment", backend.lastRequest)
	}
	if backend.createCount.Load() != 2 {
		t.Fatalf("executor calls = %d, want initial create plus physical reattachment", backend.createCount.Load())
	}
}

func TestRecoverTaskHostForEnvironmentReapsUncommittedHealthyHost(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		envInfos: map[string]*WorkspaceInfo{
			"env-1": {
				TaskID: "task-1", TaskEnvironmentID: "env-1",
				WorkspacePath: "/workspace/task-1",
			},
		},
	})
	host, err := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-1")
	if err != nil {
		t.Fatal(err)
	}
	// Model a credential-persistence failure whose first rollback could not
	// prove that this otherwise healthy process was gone.
	host.agentctlReady.Store(false)

	recovered, err := mgr.RecoverTaskHostForEnvironment(context.Background(), "env-1")
	if err != nil || !recovered {
		t.Fatalf("uncommitted recovery = %v, %v", recovered, err)
	}
	if backend.stopCount.Load() != 1 {
		t.Fatalf("uncommitted host stop attempts = %d, want 1", backend.stopCount.Load())
	}
	if _, exists := mgr.executionStore.Get(host.ID); exists {
		t.Fatal("uncommitted task host remains tracked after physical stop")
	}
}

func TestRecoverTaskHostForEnvironmentDoesNotEvictConcurrentReplacement(t *testing.T) {
	healthStarted := make(chan struct{})
	releaseHealth := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		close(healthStarted)
		<-releaseHealth
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	hostName, portString, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatal(err)
	}
	mgr := newTestManager(t)
	old := &AgentExecution{
		ID: "stable-host", TaskID: "task-1", TaskEnvironmentID: "env-1", IsTaskHost: true,
		agentctl: agentctl.NewClient(hostName, port, mgr.logger),
	}
	old.MarkAgentctlReady()
	if err := mgr.executionStore.Add(old); err != nil {
		t.Fatal(err)
	}

	result := make(chan struct {
		recovered bool
		err       error
	}, 1)
	go func() {
		recovered, recoverErr := mgr.RecoverTaskHostForEnvironment(context.Background(), "env-1")
		result <- struct {
			recovered bool
			err       error
		}{recovered: recovered, err: recoverErr}
	}()
	<-healthStarted
	mgr.executionStore.Remove(old.ID)
	replacement := &AgentExecution{
		ID: old.ID, TaskID: old.TaskID, TaskEnvironmentID: old.TaskEnvironmentID, IsTaskHost: true,
		agentctl: newReadyAgentctlClient(t, mgr.logger),
	}
	replacement.MarkAgentctlReady()
	if err := mgr.executionStore.Add(replacement); err != nil {
		t.Fatal(err)
	}
	close(releaseHealth)
	got := <-result
	if got.err != nil || got.recovered {
		t.Fatalf("recovery result = %v, %v; want concurrent replacement retained", got.recovered, got.err)
	}
	if current, exists := mgr.executionStore.GetTaskHostByEnvironmentID("env-1"); !exists || current != replacement {
		t.Fatalf("task host = %#v, %v; want concurrent replacement", current, exists)
	}
}

func TestTaskHostUsesDedicatedInstanceInsideExistingDockerTaskEnvironment(t *testing.T) {
	log := newTestLogger()
	primaryPath := t.TempDir()
	siblingPath := t.TempDir()
	backend := &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameDocker},
		client:       newReadyAgentctlClient(t, log),
	}
	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(backend)
	provider := &mockWorkspaceInfoProvider{envInfos: map[string]*WorkspaceInfo{
		"env-docker": {
			TaskID: "task-docker", TaskEnvironmentID: "env-docker",
			ExecutorType: string(models.ExecutorTypeLocalDocker), WorkspacePath: t.TempDir(),
			WorkspaceRepositories: []WorkspaceRepositorySpec{
				{RepositoryPath: primaryPath, RepoName: "primary", BaseBranch: "main", Position: 0},
				{RepositoryPath: siblingPath, RepoName: "API", CheckoutBranch: "feature/add-source", Position: 1},
			},
			Metadata: map[string]interface{}{
				MetadataKeyContainerID:          "container-task",
				MetadataKeyAuthTokenSecret:      "auth-secret",
				MetadataKeyBootstrapNonceSecret: "nonce-secret",
				"env_secret_id_AGENT_TOKEN":     "agent-secret",
			},
		},
	}}
	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log,
	)
	mgr.workspaceInfoProvider = provider
	mgr.taskEnvironmentRuntimeSecretWriter = &captureTaskEnvironmentRuntimeSecretWriter{}
	mgr.runtimeSecretStore = &inMemorySecretStore{store: map[string]*secrets.SecretWithValue{
		"auth-secret":  {Secret: secrets.Secret{ID: "auth-secret"}, Value: "task-auth-token"},
		"nonce-secret": {Secret: secrets.Secret{ID: "nonce-secret"}, Value: "task-bootstrap-nonce"},
	}}
	cleanupManagerStopCh(t, mgr)

	host, err := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-docker")
	if err != nil {
		t.Fatalf("GetOrEnsureTaskHostForEnvironment: %v", err)
	}
	request := backend.lastRequest
	if request == nil || request.PreviousExecutionID != request.InstanceID {
		t.Fatalf("docker task-host reconnect request = %#v", request)
	}
	if request.AuthToken != "task-auth-token" || request.BootstrapNonce != "task-bootstrap-nonce" {
		t.Fatalf("docker control credentials = %q/%q", request.AuthToken, request.BootstrapNonce)
	}
	if request.AgentConfig != nil || request.AgentProfileID != "" || request.OfficeAgentProfileID != "" {
		t.Fatalf("docker task host inherited agent identity: %#v", request)
	}
	wantRoots := []string{dockerWorkspacePath, path.Join(dockerWorkspacePath, "API-feature-add-source")}
	if !sameStrings(request.WorkspaceSourceRoots, wantRoots) {
		t.Fatalf("docker task-host roots = %v, want runtime roots %v", request.WorkspaceSourceRoots, wantRoots)
	}
	if request.Metadata[MetadataKeyContainerID] != "container-task" || request.Metadata["task_host"] != true {
		t.Fatalf("docker task-host metadata = %#v", request.Metadata)
	}
	for _, key := range []string{MetadataKeyAuthTokenSecret, MetadataKeyBootstrapNonceSecret, "env_secret_id_AGENT_TOKEN"} {
		if _, exists := request.Metadata[key]; exists {
			t.Fatalf("task-host request retained secret reference %q", key)
		}
	}
	if !host.IsTaskHost {
		t.Fatal("docker host is not marked task-owned")
	}
}

func TestTaskHostUsesLiveDockerControlCredentialsBeforeDurableMirror(t *testing.T) {
	log := newTestLogger()
	backend := &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameDocker},
		client:       newReadyAgentctlClient(t, log),
	}
	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(backend)
	provider := &mockWorkspaceInfoProvider{envInfos: map[string]*WorkspaceInfo{
		"env-docker": {
			TaskID: "task-docker", TaskEnvironmentID: "env-docker",
			ExecutorType: string(models.ExecutorTypeLocalDocker), WorkspacePath: t.TempDir(),
			Metadata: map[string]interface{}{MetadataKeyContainerID: "container-task"},
		},
	}}
	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log,
	)
	mgr.workspaceInfoProvider = provider
	mgr.taskEnvironmentRuntimeSecretWriter = &captureTaskEnvironmentRuntimeSecretWriter{}
	mgr.runtimeSecretStore = &inMemorySecretStore{store: map[string]*secrets.SecretWithValue{
		"auth-secret":  {Secret: secrets.Secret{ID: "auth-secret"}, Value: "live-auth-token"},
		"nonce-secret": {Secret: secrets.Secret{ID: "nonce-secret"}, Value: "live-bootstrap-nonce"},
	}}
	cleanupManagerStopCh(t, mgr)
	liveSession := &AgentExecution{
		ID: "session-execution", TaskID: "task-docker", SessionID: "session-1",
		TaskEnvironmentID: "env-docker", ContainerID: "container-task",
	}
	liveSession.setMetadataValue(MetadataKeyAuthTokenSecret, "auth-secret")
	liveSession.setMetadataValue(MetadataKeyBootstrapNonceSecret, "nonce-secret")
	if err := mgr.executionStore.Add(liveSession); err != nil {
		t.Fatal(err)
	}

	if _, err := mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-docker"); err != nil {
		t.Fatalf("GetOrEnsureTaskHostForEnvironment: %v", err)
	}
	request := backend.lastRequest
	if request == nil || request.AuthToken != "live-auth-token" || request.BootstrapNonce != "live-bootstrap-nonce" {
		t.Fatalf("docker control credentials = %#v", request)
	}
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

func TestCreateExecutionResolvesProfileOnceForEnvAndAutoApprove(t *testing.T) {
	profileResolver := &countingProfileResolver{
		info: &AgentProfileInfo{
			ProfileID:   "profile-1",
			AgentID:     "auggie",
			AutoApprove: true,
			EnvVars:     []settingsmodels.ProfileEnvVar{{Key: "CLAUDE_CONFIG_DIR", Value: "/tmp/claude"}},
		},
	}
	mgr, backend := newEnvironmentExecutionTestManagerWithProfileResolver(t, &mockWorkspaceInfoProvider{}, profileResolver)

	_, err := mgr.createExecution(context.Background(), "task-1", &WorkspaceInfo{
		SessionID:      "session-1",
		AgentID:        "auggie",
		AgentProfileID: "profile-1",
		WorkspacePath:  "/workspace/task-1",
	})
	if err != nil {
		t.Fatalf("createExecution returned error: %v", err)
	}

	if got := profileResolver.calls.Load(); got != 1 {
		t.Fatalf("ResolveProfile calls = %d, want 1", got)
	}
	if backend.lastRequest == nil {
		t.Fatal("CreateInstance was not called")
	}
	if !backend.lastRequest.AutoApprovePermissions {
		t.Fatal("AutoApprovePermissions = false, want true")
	}
	if backend.lastRequest.AutoApprovePermissionsOverride == nil || !*backend.lastRequest.AutoApprovePermissionsOverride {
		t.Fatalf("AutoApprovePermissionsOverride = %v, want true", backend.lastRequest.AutoApprovePermissionsOverride)
	}
	if got := backend.lastRequest.Env["CLAUDE_CONFIG_DIR"]; got != "/tmp/claude" {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want %q", got, "/tmp/claude")
	}
}

func TestCreateExecutionRecoversRepositoryEnvironmentAndSSHApprovals(t *testing.T) {
	profileResolver := &countingProfileResolver{info: &AgentProfileInfo{
		ProfileID: "agent-profile", AgentID: "auggie", EnvVars: []settingsmodels.ProfileEnvVar{{Key: "PROFILE_ONLY", Value: "profile-value"}},
	}}
	mgr, backend := newEnvironmentExecutionTestManagerWithProfileResolver(t, &mockWorkspaceInfoProvider{}, profileResolver)
	store := newInMemorySecretStore()
	if err := store.Create(context.Background(), &secrets.SecretWithValue{Secret: secrets.Secret{
		ID: "workspace-token", Scope: secrets.ScopeWorkspace, WorkspaceID: "workspace-1",
	}, Value: "repository-value"}); err != nil {
		t.Fatalf("seed repository secret: %v", err)
	}
	mgr.secretStore = store
	reader := &recoveryEnvironmentReader{
		fakeExecutorProfileReader: fakeExecutorProfileReader{
			session: &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateStarting},
			profiles: map[string]*models.ExecutorProfile{
				"executor-profile": {ID: "executor-profile", EnvVars: []models.ProfileEnvVar{{Key: "EXECUTOR_ONLY", Value: "executor-value"}}},
			},
		},
		taskRepositories: []*models.TaskRepository{{RepositoryID: "repo-1"}},
		repositories: map[string]*models.Repository{
			"repo-1": {ID: "repo-1", WorkspaceID: "workspace-1", Name: "app", SecretBindings: []models.RepositorySecretBinding{{Key: "NPM_TOKEN", SecretID: "workspace-token"}}},
		},
	}
	mgr.SetExecutorProfileReader(reader)

	execution, err := mgr.createExecution(context.Background(), "task-1", &WorkspaceInfo{
		SessionID: "session-1", WorkspaceID: "workspace-1", AgentProfileID: "agent-profile", ExecutionProfileID: "agent-profile",
		ExecutorProfileID: "executor-profile",
		AgentID:           "auggie", WorkspacePath: "/workspace/task-1",
	})
	if err != nil {
		t.Fatalf("createExecution returned error: %v", err)
	}
	if got := backend.lastRequest.Env["NPM_TOKEN"]; got != "repository-value" {
		t.Fatalf("recovered NPM_TOKEN = %q, want repository value", got)
	}
	if got := backend.lastRequest.Env["PROFILE_ONLY"]; got != "profile-value" {
		t.Fatalf("recovered profile env = %q, want profile value", got)
	}
	if got := backend.lastRequest.Env["EXECUTOR_ONLY"]; got != "executor-value" {
		t.Fatalf("recovered executor env = %q, want executor value", got)
	}
	if got := reader.profileArgs; len(got) != 1 || got[0] != "executor-profile" {
		t.Fatalf("executor profile lookups = %v, want [executor-profile]", got)
	}
	if got := backend.lastRequest.ApprovedSecretEnvKeys; len(got) != 1 || got[0] != "NPM_TOKEN" {
		t.Fatalf("recovered SSH approvals = %#v, want NPM_TOKEN", got)
	}
	if got := execution.RuntimeEnvironment()["NPM_TOKEN"]; got != "repository-value" {
		t.Fatalf("execution runtime NPM_TOKEN = %q, want repository value", got)
	}
}

func TestWorkspaceProfileIDsKeepAgentAndExecutorProfilesDistinct(t *testing.T) {
	info := &WorkspaceInfo{
		AgentProfileID:     "office-agent-instance",
		ExecutionProfileID: "cli-profile",
		ExecutorProfileID:  "executor-profile",
	}

	if got := workspaceExecutionProfileID(info); got != "cli-profile" {
		t.Fatalf("workspaceExecutionProfileID = %q, want cli-profile", got)
	}
	if got := workspaceExecutorProfileID(info); got != "executor-profile" {
		t.Fatalf("workspaceExecutorProfileID = %q, want executor-profile", got)
	}
}

type recoveryEnvironmentReader struct {
	fakeExecutorProfileReader
	taskRepositories []*models.TaskRepository
	repositories     map[string]*models.Repository
}

func (r *recoveryEnvironmentReader) ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error) {
	return r.taskRepositories, nil
}

func (r *recoveryEnvironmentReader) GetRepository(_ context.Context, id string) (*models.Repository, error) {
	return r.repositories[id], nil
}

func TestCreateExecutionRunsRemoteResumePreflightBeforeCreatingWorkspaceExecution(t *testing.T) {
	log := newTestLogger()
	backend := &resumeTrackingExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		client:       newReadyAgentctlClient(t, log),
	}
	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(backend)
	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log,
	)
	cleanupManagerStopCh(t, mgr)

	metadata := map[string]interface{}{"ssh_host": "recovery.example", "ssh_port": 2222}
	_, err := mgr.createExecution(context.Background(), "task-1", &WorkspaceInfo{
		SessionID:        "session-1",
		AgentID:          "auggie",
		WorkspacePath:    "/workspace/task-1",
		Metadata:         metadata,
		AgentExecutionID: "previous-execution",
	})
	if err != nil {
		t.Fatalf("createExecution returned error: %v", err)
	}
	if got, want := backend.calls, []string{"resume", "create"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("backend calls = %v, want %v", got, want)
	}
	if backend.createRequest == nil {
		t.Fatal("CreateInstance was not called")
	}
	if got := backend.createRequest.PreviousExecutionID; got != "previous-execution" {
		t.Fatalf("PreviousExecutionID = %q, want %q", got, "previous-execution")
	}
	if got := backend.createRequest.Metadata; !reflect.DeepEqual(got, metadata) {
		t.Fatalf("Metadata = %#v, want %#v", got, metadata)
	}
}

func TestCreateExecutionReturnsRemoteResumePreflightError(t *testing.T) {
	log := newTestLogger()
	backend := &resumeTrackingExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		client:       newReadyAgentctlClient(t, log),
		resumeErr:    errors.New("remote unavailable"),
	}
	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(backend)
	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log,
	)
	cleanupManagerStopCh(t, mgr)

	_, err := mgr.createExecution(context.Background(), "task-1", &WorkspaceInfo{
		SessionID:     "session-1",
		AgentID:       "auggie",
		WorkspacePath: "/workspace/task-1",
	})
	if !errors.Is(err, backend.resumeErr) {
		t.Fatalf("createExecution error = %v, want remote resume error", err)
	}
	if !strings.Contains(err.Error(), "failed remote resume preflight") {
		t.Fatalf("createExecution error = %q, want remote resume preflight context", err)
	}
	if got, want := backend.calls, []string{"resume"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("backend calls = %v, want %v", got, want)
	}
}

// --- test helpers ---

type resumeTrackingExecutor struct {
	MockExecutor
	client        *agentctl.Client
	resumeErr     error
	calls         []string
	createRequest *ExecutorCreateRequest
}

func (e *resumeTrackingExecutor) ResumeRemoteInstance(_ context.Context, _ *ExecutorCreateRequest) error {
	e.calls = append(e.calls, "resume")
	return e.resumeErr
}

func (e *resumeTrackingExecutor) CreateInstance(_ context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	e.calls = append(e.calls, "create")
	e.createRequest = req
	return &ExecutorInstance{
		InstanceID:    req.InstanceID,
		TaskID:        req.TaskID,
		SessionID:     req.SessionID,
		RuntimeName:   e.Name(),
		Client:        e.client,
		WorkspacePath: req.WorkspacePath,
	}, nil
}

type doneObservedContext struct {
	context.Context
	doneRead chan struct{}
	once     sync.Once
}

func (c *doneObservedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.doneRead) })
	return c.Context.Done()
}

type notifyingWorkspaceInfoProvider struct {
	*mockWorkspaceInfoProvider
	environmentReached chan struct{}
}

func (p *notifyingWorkspaceInfoProvider) GetWorkspaceInfoForEnvironment(
	ctx context.Context,
	taskEnvironmentID string,
) (*WorkspaceInfo, error) {
	close(p.environmentReached)
	return p.mockWorkspaceInfoProvider.GetWorkspaceInfoForEnvironment(ctx, taskEnvironmentID)
}

type createInstanceExecutor struct {
	MockExecutor
	client             *agentctl.Client
	createCount        atomic.Int32
	stopCount          atomic.Int32
	stopErr            error
	forceStopped       atomic.Bool
	lastRequest        *ExecutorCreateRequest
	authToken          string
	nonce              string
	delay              time.Duration
	progressStep       string
	existingOnlyAbsent bool
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
	e.lastRequest = req
	e.createCount.Add(1)
	if req.RequireExistingInstance && e.existingOnlyAbsent {
		return nil, errTaskHostRuntimeNotFound
	}
	if progress != nil {
		completeStepSuccess(progress)
		reportProgress(req.OnProgress, *progress, 0, 1)
	}
	return &ExecutorInstance{
		InstanceID:     req.InstanceID,
		TaskID:         req.TaskID,
		SessionID:      req.SessionID,
		RuntimeName:    e.Name(),
		Client:         e.client,
		WorkspacePath:  req.WorkspacePath,
		AuthToken:      e.authToken,
		BootstrapNonce: e.nonce,
	}, nil
}

func (e *createInstanceExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error {
	e.stopCount.Add(1)
	e.forceStopped.Store(force)
	return e.stopErr
}

func newEnvironmentExecutionTestManager(t *testing.T, provider WorkspaceInfoProvider) (*Manager, *createInstanceExecutor) {
	return newEnvironmentExecutionTestManagerWithProfileResolver(t, provider, &MockProfileResolver{})
}

func newEnvironmentExecutionTestManagerWithProfileResolver(
	t *testing.T,
	provider WorkspaceInfoProvider,
	profileResolver ProfileResolver,
) (*Manager, *createInstanceExecutor) {
	t.Helper()
	log := newTestLogger()
	execRegistry := NewExecutorRegistry(log)
	backend := &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		client:       newReadyAgentctlClient(t, log),
	}
	execRegistry.Register(backend)

	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, profileResolver, nil,
		ExecutorFallbackWarn, "", log,
	)
	mgr.workspaceInfoProvider = provider
	cleanupManagerStopCh(t, mgr)
	return mgr, backend
}

type countingProfileResolver struct {
	info  *AgentProfileInfo
	err   error
	calls atomic.Int32
}

func (r *countingProfileResolver) ResolveProfile(_ context.Context, _ string) (*AgentProfileInfo, error) {
	r.calls.Add(1)
	return r.info, r.err
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

func TestWaitForAgentctlReadyCachesSuccessfulHealthCheck(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID:        "exec-prepared",
		TaskID:    "task-1",
		SessionID: "session-1",
		agentctl:  newReadyAgentctlClient(t, mgr.logger),
	}

	mgr.waitForAgentctlReady(execution)

	if !execution.IsAgentctlReady() {
		t.Fatal("expected successful agentctl health check to be cached")
	}
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
