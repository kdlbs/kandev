package executor

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

func TestStopSessionDetailed_DoesNotOverwriteTerminalRace(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateCompleted,
	}
	stopCalls := make(chan struct{}, 1)
	agentManager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-1", nil
		},
		stopAgentFunc: func(context.Context, string, bool) error {
			stopCalls <- struct{}{}
			return nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)

	staleRunningSession := &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateRunning,
	}
	result, err := exec.StopSessionDetailed(context.Background(), staleRunningSession, "test stop", false)

	if err != nil {
		t.Errorf("stop error = %v", err)
	}
	if result.Changed || result.FinalState != models.TaskSessionStateCompleted {
		t.Errorf("result = %#v, want unchanged COMPLETED", result)
	}
	if got := repo.sessions["session-1"].State; got != models.TaskSessionStateCompleted {
		t.Errorf("session state = %q, want %q", got, models.TaskSessionStateCompleted)
	}
	select {
	case <-stopCalls:
		t.Error("terminal session runtime was stopped")
	default:
	}
}

func TestStopSessionDetailed_ClassifiesExecutionLookup(t *testing.T) {
	lookupFailure := errors.New("lifecycle store unavailable")
	tests := []struct {
		name      string
		execution string
		lookupErr error
		wantErr   error
	}{
		{
			name:      "exact absence sentinel is idempotent",
			lookupErr: fmt.Errorf("wrapped: %w", lifecycle.ErrNoExecutionForSession),
		},
		{
			name:      "other lookup failure is preserved",
			lookupErr: lookupFailure,
			wantErr:   lookupFailure,
		},
		{
			name:    "empty execution ID is an invariant failure",
			wantErr: errEmptyExecutionID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepository()
			repo.sessions["session-1"] = &models.TaskSession{
				ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateRunning,
			}
			manager := &mockAgentManager{
				getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
					return tt.execution, tt.lookupErr
				},
			}
			exec := newTestExecutor(t, manager, repo)

			result, err := exec.StopSessionDetailed(
				context.Background(), repo.sessions["session-1"], "test stop", false,
			)

			if tt.wantErr == errEmptyExecutionID {
				if err == nil || !errors.Is(err, errEmptyExecutionID) {
					t.Fatalf("error = %v, want empty execution ID error", err)
				}
			} else if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if result.Changed {
				t.Error("lookup-only outcome reported a changed session")
			}
		})
	}
}

// TestStop_MissingSessionReportsRuntimeNotFound covers the boundary contract the
// durable task cleanup depends on: when the task_session row is genuinely gone,
// Stop must report the runtime as not-found (wrapping the public
// runtimeapi.ErrNotFound sentinel) so cleanup can classify a confirmed-dead
// local runtime as already-stopped. An unrelated store error must NOT be
// collapsed into a not-found result.
func TestStop_MissingSessionReportsRuntimeNotFound(t *testing.T) {
	t.Run("missing session row normalizes to runtime not-found", func(t *testing.T) {
		repo := newMockRepository()
		repo.getTaskSessionFunc = func(context.Context, string) (*models.TaskSession, error) {
			return nil, fmt.Errorf("agent session not found: %w", models.ErrTaskSessionNotFound)
		}
		exec := newTestExecutor(t, &mockAgentManager{}, repo)

		err := exec.Stop(context.Background(), "sess-gone", "cleanup", true)
		if !errors.Is(err, ErrExecutionNotFound) {
			t.Fatalf("error = %v, want ErrExecutionNotFound", err)
		}
		if !errors.Is(err, runtimeapi.ErrNotFound) {
			t.Fatalf("missing session must normalize to runtimeapi.ErrNotFound; got %v", err)
		}
	})

	t.Run("unrelated store error is not collapsed to not-found", func(t *testing.T) {
		repo := newMockRepository()
		repo.getTaskSessionFunc = func(context.Context, string) (*models.TaskSession, error) {
			return nil, errors.New("database is unavailable")
		}
		exec := newTestExecutor(t, &mockAgentManager{}, repo)

		err := exec.Stop(context.Background(), "sess-x", "cleanup", true)
		if !errors.Is(err, ErrExecutionNotFound) {
			t.Fatalf("error = %v, want ErrExecutionNotFound", err)
		}
		if errors.Is(err, runtimeapi.ErrNotFound) {
			t.Fatalf("an unrelated store error must not be reclassified as runtime not-found; got %v", err)
		}
	})
}

func TestStopSessionSynchronouslyWaitsForAgentTeardown(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-sync"] = &models.TaskSession{
		ID: "session-sync", TaskID: "task-sync", State: models.TaskSessionStateRunning,
	}
	stopCalls := make(chan string, 1)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-sync", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, _ string, _ bool) error {
			stopCalls <- executionID
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)

	if err := exec.StopSessionSynchronously(context.Background(), "session-sync", "cleanup", true); err != nil {
		t.Fatalf("StopSessionSynchronously: %v", err)
	}
	select {
	case executionID := <-stopCalls:
		if executionID != "execution-sync" {
			t.Fatalf("stopped execution = %q, want execution-sync", executionID)
		}
	default:
		t.Fatal("synchronous stop returned before agent teardown ran")
	}
	if got := repo.sessions["session-sync"].State; got != models.TaskSessionStateCancelled {
		t.Fatalf("session state = %q, want CANCELLED", got)
	}
}

func TestStopSessionSynchronouslyStopsLateExecutionAfterSessionDeletion(t *testing.T) {
	repo := newMockRepository()
	repo.getTaskSessionFunc = func(context.Context, string) (*models.TaskSession, error) {
		return nil, fmt.Errorf("session deleted: %w", models.ErrTaskSessionNotFound)
	}
	stopCalls := make(chan string, 1)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-late", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, _ string, _ bool) error {
			stopCalls <- executionID
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)

	if err := exec.StopSessionSynchronously(context.Background(), "session-deleted", "cleanup", true); err != nil {
		t.Fatalf("StopSessionSynchronously: %v", err)
	}
	if got := <-stopCalls; got != "execution-late" {
		t.Fatalf("stopped execution = %q, want execution-late", got)
	}
}

// TestStopByTaskID_StopsFailedSessionWithRegisteredExecution covers
// @covers AC-TASKS-TASK-STOP-REACHABILITY-001.1-.2: a task whose only session
// is FAILED in the database but still holds a registered execution (an
// orphan left by a failed teardown attempt) must be stopped, and the
// session's terminal state must survive — no CANCELLED transition.
func TestStopByTaskID_StopsFailedSessionWithRegisteredExecution(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID:           "session-1",
		TaskID:       "task-1",
		State:        models.TaskSessionStateFailed,
		ErrorMessage: "agent produced no output since start",
	}
	stopCalls := make(chan string, 1)
	manager := &mockAgentManager{
		listSessionIDsForTaskFunc: func(taskID string) []string {
			if taskID != "task-1" {
				return nil
			}
			return []string{"session-1"}
		},
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-orphan", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, _ string, force bool) error {
			if !force {
				t.Errorf("registry-recovered stop was not forced")
			}
			stopCalls <- executionID
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)

	if err := exec.StopByTaskID(context.Background(), "task-1", "cleanup", true); err != nil {
		t.Fatalf("StopByTaskID: %v", err)
	}
	select {
	case executionID := <-stopCalls:
		if executionID != "execution-orphan" {
			t.Fatalf("stopped execution = %q, want execution-orphan", executionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("registry-recovered execution was not stopped")
	}
	after, err := repo.GetTaskSession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("get session after stop: %v", err)
	}
	if after.State != models.TaskSessionStateFailed {
		t.Fatalf("session state = %q, want unchanged FAILED", after.State)
	}
	if after.ErrorMessage != "agent produced no output since start" {
		t.Fatalf("session error message = %q, want preserved launch-failure message", after.ErrorMessage)
	}
}

// TestStopByTaskID_ActiveSessionKeepsCancellationTransition pins the
// unchanged path: a task with an active session behaves exactly as before —
// the session transitions to CANCELLED and its execution is stopped.
func TestStopByTaskID_ActiveSessionKeepsCancellationTransition(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateRunning,
	}
	repo.listActiveTaskSessionsByTaskIDFunc = func(_ context.Context, taskID string) ([]*models.TaskSession, error) {
		if taskID != "task-1" {
			return nil, nil
		}
		return []*models.TaskSession{repo.sessions["session-1"]}, nil
	}
	stopCalls := make(chan string, 1)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-active", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, _ string, _ bool) error {
			stopCalls <- executionID
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)

	if err := exec.StopByTaskID(context.Background(), "task-1", "cleanup", true); err != nil {
		t.Fatalf("StopByTaskID: %v", err)
	}
	select {
	case executionID := <-stopCalls:
		if executionID != "execution-active" {
			t.Fatalf("stopped execution = %q, want execution-active", executionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("active session's execution was not stopped")
	}
	after, err := repo.GetTaskSession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("get session after stop: %v", err)
	}
	if after.State != models.TaskSessionStateCancelled {
		t.Fatalf("session state = %q, want CANCELLED", after.State)
	}
}

// TestStopByTaskID_NoActiveSessionAndNoRegisteredExecutionReportsNotFound
// pins the unchanged path: a task with neither an active session nor a
// registered execution still returns ErrExecutionNotFound and changes
// nothing; a repeated stop after the executions are gone creates no
// duplicate transition.
func TestStopByTaskID_NoActiveSessionAndNoRegisteredExecutionReportsNotFound(t *testing.T) {
	repo := newMockRepository()
	manager := &mockAgentManager{}
	exec := newTestExecutor(t, manager, repo)

	err := exec.StopByTaskID(context.Background(), "task-empty", "cleanup", true)
	if !errors.Is(err, ErrExecutionNotFound) {
		t.Fatalf("error = %v, want ErrExecutionNotFound", err)
	}

	// Repeated stop after the (nonexistent) executions are gone must behave
	// identically — no duplicate transition to create.
	err = exec.StopByTaskID(context.Background(), "task-empty", "cleanup", true)
	if !errors.Is(err, ErrExecutionNotFound) {
		t.Fatalf("repeated stop error = %v, want ErrExecutionNotFound", err)
	}
}

// TestStopByTaskID_RegistryRecoveryLoadFailureIsNotReportedAsNotFound covers
// the registry-recovery path's own failure mode: a session is registered in
// the execution registry (so a real orphan exists) but its database row
// fails to load (e.g. a transient "database is locked"). The active-session
// query legitimately returns nothing, so if the load failure were silently
// swallowed the same way a missing row is, StopByTaskID would report
// ErrExecutionNotFound — indistinguishable from "there is nothing to stop" —
// which is exactly the reachability defect this recovery path exists to
// eliminate. It must surface the load failure instead.
func TestStopByTaskID_RegistryRecoveryLoadFailureIsNotReportedAsNotFound(t *testing.T) {
	repo := newMockRepository()
	loadFailure := errors.New("database is locked")
	repo.getTaskSessionFunc = func(context.Context, string) (*models.TaskSession, error) {
		return nil, loadFailure
	}
	manager := &mockAgentManager{
		listSessionIDsForTaskFunc: func(taskID string) []string {
			if taskID != "task-1" {
				return nil
			}
			return []string{"session-1"}
		},
	}
	exec := newTestExecutor(t, manager, repo)

	err := exec.StopByTaskID(context.Background(), "task-1", "cleanup", true)
	if err == nil {
		t.Fatal("StopByTaskID: want an error, got nil")
	}
	if errors.Is(err, ErrExecutionNotFound) {
		t.Fatalf("error = %v, want the load failure surfaced instead of ErrExecutionNotFound", err)
	}
	if !errors.Is(err, loadFailure) {
		t.Fatalf("error = %v, want it to wrap %v", err, loadFailure)
	}
}

// TestStopByTaskID_MixedActiveAndUnloadableOrphanSurfacesLoadError covers
// Finding 2 from Review Round 1: a task with one active session (which stops
// cleanly) plus one registry-only orphan whose row fails to load must not
// return a clean nil. The retry contract on the never-started teardown's
// failed-attempt path depends on StopByTaskID surfacing that load failure so
// a later stop keeps retrying instead of reporting a false all-clear.
func TestStopByTaskID_MixedActiveAndUnloadableOrphanSurfacesLoadError(t *testing.T) {
	repo := newMockRepository()
	loadFailure := errors.New("database is locked")
	activeSession := &models.TaskSession{
		ID: "session-active", TaskID: "task-1", State: models.TaskSessionStateRunning,
	}
	repo.sessions[activeSession.ID] = activeSession
	repo.listActiveTaskSessionsByTaskIDFunc = func(_ context.Context, taskID string) ([]*models.TaskSession, error) {
		if taskID != "task-1" {
			return nil, nil
		}
		return []*models.TaskSession{activeSession}, nil
	}
	repo.getTaskSessionFunc = func(_ context.Context, id string) (*models.TaskSession, error) {
		if id == "session-orphan" {
			return nil, loadFailure
		}
		if s, ok := repo.sessions[id]; ok {
			return s, nil
		}
		return nil, nil
	}
	stopCalls := make(chan string, 1)
	manager := &mockAgentManager{
		listSessionIDsForTaskFunc: func(taskID string) []string {
			if taskID != "task-1" {
				return nil
			}
			return []string{"session-active", "session-orphan"}
		},
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-active", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, _ string, _ bool) error {
			stopCalls <- executionID
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)

	err := exec.StopByTaskID(context.Background(), "task-1", "cleanup", true)
	if err == nil {
		t.Fatal("StopByTaskID: want an error, got nil")
	}
	if !errors.Is(err, loadFailure) {
		t.Fatalf("error = %v, want it to wrap %v", err, loadFailure)
	}

	select {
	case executionID := <-stopCalls:
		if executionID != "execution-active" {
			t.Fatalf("stopped execution = %q, want execution-active", executionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("active session's execution was not stopped despite the orphan load failure")
	}
}

func TestStopSessionDetailed_RejectsInvalidSession(t *testing.T) {
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())

	for _, session := range []*models.TaskSession{nil, &models.TaskSession{}} {
		result, err := exec.StopSessionDetailed(context.Background(), session, "test stop", false)
		if err == nil {
			t.Errorf("session %#v: expected validation error", session)
		}
		if result.Changed {
			t.Errorf("session %#v: invalid input reported a change", session)
		}
	}
}

func TestStopSessionDetailed_PersistenceFailurePreventsTeardown(t *testing.T) {
	writeFailure := errors.New("database is read-only")
	repo := newMockRepository()
	session := &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateRunning,
	}
	repo.sessions[session.ID] = session
	repo.updateTaskSessionStateFunc = func(context.Context, string, models.TaskSessionState, string) error {
		return writeFailure
	}
	stopCalls := make(chan struct{}, 1)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-1", nil
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			stopCalls <- struct{}{}
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)

	result, err := exec.StopSessionDetailed(context.Background(), session, "test stop", false)

	if !errors.Is(err, writeFailure) {
		t.Fatalf("error = %v, want %v", err, writeFailure)
	}
	if result.Changed {
		t.Error("failed persistence reported a changed session")
	}
	select {
	case <-stopCalls:
		t.Error("runtime teardown ran after persistence failure")
	default:
	}
}

func TestStopSessionDetailed_PersistsBeforeDetachedTeardown(t *testing.T) {
	type stopCall struct {
		executionID string
		reason      string
		force       bool
		state       models.TaskSessionState
		contextErr  error
	}
	repo := newMockRepository()
	session := &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateRunning,
	}
	repo.sessions[session.ID] = session
	stopCalls := make(chan stopCall, 1)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-1", nil
		},
		stopAgentWithReasonFunc: func(ctx context.Context, executionID, reason string, force bool) error {
			stored, _ := repo.GetTaskSession(context.Background(), session.ID)
			stopCalls <- stopCall{
				executionID: executionID,
				reason:      reason,
				force:       force,
				state:       stored.State,
				contextErr:  ctx.Err(),
			}
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)
	ctx, cancel := context.WithCancel(context.Background())

	result, err := exec.StopSessionDetailed(ctx, session, "coordinator stop", false)
	cancel()

	if err != nil {
		t.Fatalf("StopSessionDetailed: %v", err)
	}
	if !result.Changed || result.FinalState != models.TaskSessionStateCancelled {
		t.Fatalf("result = %#v, want accepted CANCELLED", result)
	}
	select {
	case <-stopCalls:
		t.Fatal("teardown started before caller explicitly scheduled it")
	default:
	}
	if !result.ScheduleTeardown() {
		t.Fatal("accepted stop did not expose teardown scheduling")
	}
	if result.ScheduleTeardown() {
		t.Fatal("teardown was scheduled more than once")
	}
	select {
	case call := <-stopCalls:
		if call.executionID != "execution-1" || call.reason != "coordinator stop" || call.force {
			t.Errorf("stop call = %#v", call)
		}
		if call.state != models.TaskSessionStateCancelled {
			t.Errorf("state at teardown = %q, want CANCELLED", call.state)
		}
		if call.contextErr != nil {
			t.Errorf("detached teardown context error = %v", call.contextErr)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for asynchronous teardown")
	}
}

func TestStopWithSession_PreservesLegacyBestEffortPersistence(t *testing.T) {
	writeFailure := errors.New("database is read-only")
	repo := newMockRepository()
	session := &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateRunning,
	}
	repo.sessions[session.ID] = session
	repo.updateTaskSessionStateFunc = func(context.Context, string, models.TaskSessionState, string) error {
		return writeFailure
	}
	stopCalls := make(chan struct{}, 1)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-1", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, reason string, force bool) error {
			if executionID != "execution-1" || reason != "legacy reason" || !force {
				t.Errorf("legacy stop arguments = %q, %q, %v", executionID, reason, force)
			}
			stopCalls <- struct{}{}
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)

	if err := exec.stopWithSession(context.Background(), session, "legacy reason", true); err != nil {
		t.Fatalf("legacy stop returned persistence error: %v", err)
	}
	select {
	case <-stopCalls:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for legacy teardown")
	}
}

func TestStopWithSession_PreservesLegacyLookupFailureClassification(t *testing.T) {
	lookupFailure := errors.New("lifecycle store unavailable")
	repo := newMockRepository()
	session := &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateRunning,
	}
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "", lookupFailure
		},
	}, repo)

	err := exec.stopWithSession(context.Background(), session, "legacy reason", true)
	if !errors.Is(err, lookupFailure) {
		t.Fatalf("error = %v, want lookup failure", err)
	}
	if !errors.Is(err, ErrExecutionNotFound) {
		t.Fatalf("error = %v, want legacy ErrExecutionNotFound", err)
	}
	if errors.Is(err, runtimeapi.ErrNotFound) {
		t.Fatalf("error = %v, real lookup failure must not look like exact runtime absence", err)
	}
}

func TestStopWithSession_ClaimSuppressesLateCancelledLaunchCleanup(t *testing.T) {
	repo := newMockRepository()
	session := &models.TaskSession{
		ID: "session-legacy-race", TaskID: "task-legacy-race", State: models.TaskSessionStateRunning,
	}
	repo.sessions[session.ID] = session
	stopCalls := make(chan bool, 2)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-legacy-race", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, _ string, _ string, force bool) error {
			stopCalls <- force
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)
	var claimed bool
	exec.SetOnExecutionStopOwnerRegistration(func(sessionID, executionID string, force bool) {
		if sessionID != session.ID || executionID != "execution-legacy-race" || force {
			t.Fatalf("stop claim = (%q, %q, %v)", sessionID, executionID, force)
		}
		claimed = true
	})
	exec.SetOnExecutionCleanupClaim(func(sessionID, executionID string) bool {
		if sessionID != session.ID || executionID != "execution-legacy-race" {
			t.Fatalf("cleanup claim = (%q, %q)", sessionID, executionID)
		}
		return !claimed
	})

	if err := exec.stopWithSession(context.Background(), session, "legacy graceful stop", false); err != nil {
		t.Fatalf("stopWithSession: %v", err)
	}
	exec.handleAgentProcessStartFailure(
		context.Background(), session.TaskID, session.ID, "execution-legacy-race",
		errors.New("start failed after legacy cancellation"), true, false,
	)

	select {
	case force := <-stopCalls:
		if force {
			t.Fatal("legacy owner was replaced by forced launch cleanup")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for legacy graceful stop")
	}
	select {
	case force := <-stopCalls:
		t.Fatalf("duplicate teardown reached runtime (force=%v)", force)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestStopWithSession_RegistersStopOwnerBeforeCancelledWrite is a regression
// test for Finding 1 from Review Round 1: a concurrent StartAgentProcess
// failure claims forced cleanup for itself only when no stop owner is
// registered yet, and it classifies CANCELLED as terminal as soon as the row
// is visible. If the CANCELLED write happened before stop-owner registration,
// that window would let the concurrent failure win the claim and issue its
// own teardown alongside this one's. The owner must be registered first.
func TestStopWithSession_RegistersStopOwnerBeforeCancelledWrite(t *testing.T) {
	repo := newMockRepository()
	session := &models.TaskSession{
		ID: "session-order", TaskID: "task-order", State: models.TaskSessionStateRunning,
	}
	repo.sessions[session.ID] = session

	var order []string
	repo.updateTaskSessionStateFunc = func(_ context.Context, sessionID string, state models.TaskSessionState, errorMessage string) error {
		order = append(order, "cancelled-write")
		if s, ok := repo.sessions[sessionID]; ok {
			s.State = state
			s.ErrorMessage = errorMessage
		}
		return nil
	}

	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-order", nil
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)
	exec.SetOnExecutionStopOwnerRegistration(func(string, string, bool) {
		order = append(order, "register-owner")
	})

	if err := exec.stopWithSession(context.Background(), session, "test stop", false); err != nil {
		t.Fatalf("stopWithSession: %v", err)
	}

	if len(order) != 2 || order[0] != "register-owner" || order[1] != "cancelled-write" {
		t.Fatalf("operation order = %v, want [register-owner cancelled-write]", order)
	}
}

func TestStopExecution_PreservesRuntimeFailureClassification(t *testing.T) {
	runtimeFailure := errors.New("runtime teardown failed")
	tests := []struct {
		name    string
		stopErr error
		wantErr error
	}{
		{
			name:    "exact absence remains classifiable",
			stopErr: fmt.Errorf("missing: %w", lifecycle.ErrExecutionNotFound),
			wantErr: lifecycle.ErrExecutionNotFound,
		},
		{
			name:    "real runtime failure is preserved",
			stopErr: runtimeFailure,
			wantErr: runtimeFailure,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := newTestExecutor(t, &mockAgentManager{
				stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
					return tt.stopErr
				},
			}, newMockRepository())

			err := exec.StopExecution(context.Background(), "execution-1", "cleanup", true)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("StopExecution error = %v, want %v", err, tt.wantErr)
			}
			if !errors.Is(err, ErrExecutionNotFound) {
				t.Fatalf("StopExecution error = %v, want legacy ErrExecutionNotFound", err)
			}
			if errors.Is(tt.stopErr, lifecycle.ErrExecutionNotFound) &&
				!errors.Is(err, runtimeapi.ErrNotFound) {
				t.Fatalf("StopExecution error = %v, want public runtime ErrNotFound", err)
			}
			if !errors.Is(tt.stopErr, lifecycle.ErrExecutionNotFound) &&
				errors.Is(err, runtimeapi.ErrNotFound) {
				t.Fatalf("StopExecution error = %v, real failure must not look like exact runtime absence", err)
			}
		})
	}
}

func TestStopPreparedModelSwitchAgent_ReturnsTeardownFailure(t *testing.T) {
	stopErr := errors.New("runtime teardown failed")
	manager := &mockAgentManager{
		stopAgentFunc: func(_ context.Context, executionID string, force bool) error {
			if executionID != "execution-model-switch" || force {
				t.Fatalf("StopAgent = (%q, %v), want (execution-model-switch, false)", executionID, force)
			}
			return stopErr
		},
	}
	exec := newTestExecutor(t, manager, newMockRepository())

	err := exec.stopPreparedModelSwitchAgent(
		context.Background(), "execution-model-switch",
	)

	if !errors.Is(err, stopErr) {
		t.Fatalf("stopPreparedModelSwitchAgent error = %v, want %v", err, stopErr)
	}
}

func TestLaunchModelSwitchAgent_CleansStartedExecutionAfterTerminalRace(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	session := &models.TaskSession{
		ID: "session-model-race", TaskID: "task-model-race", State: models.TaskSessionStateWaitingForInput,
	}
	repo.sessions[session.ID] = session
	stopCalls := make(chan struct{}, 1)
	manager := &mockAgentManager{
		launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{AgentExecutionID: "execution-model-race"}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error {
			return repo.UpdateTaskSessionState(
				ctx,
				session.ID,
				models.TaskSessionStateCancelled,
				"stopped during model switch",
			)
		},
		stopAgentFunc: func(_ context.Context, executionID string, force bool) error {
			if executionID != "execution-model-race" || !force {
				t.Fatalf("StopAgent = (%q, %v), want (execution-model-race, true)", executionID, force)
			}
			stopCalls <- struct{}{}
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)
	exec.SetOnExecutionCleanupClaim(func(sessionID, executionID string) bool {
		return sessionID == session.ID && executionID == "execution-model-race"
	})

	callerSession := *session
	err := exec.launchModelSwitchAgent(
		ctx,
		session.TaskID,
		session.ID,
		"new-model",
		&callerSession,
		&LaunchAgentRequest{},
		nil,
	)

	if !errors.Is(err, ErrSessionStateSuperseded) {
		t.Fatalf("launchModelSwitchAgent error = %v, want ErrSessionStateSuperseded", err)
	}
	select {
	case <-stopCalls:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for model-switch race cleanup")
	}
}

func TestPersistModelSwitchState_CancellationAfterSnapshotWins(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	stored := &models.TaskSession{
		ID:                   "session-model-persist-race",
		TaskID:               "task-model-persist-race",
		State:                models.TaskSessionStateRunning,
		AgentProfileSnapshot: map[string]interface{}{"model": "old-model"},
	}
	repo.sessions[stored.ID] = stored
	stale := *stored
	stale.AgentProfileSnapshot = map[string]interface{}{"model": "old-model"}
	if err := repo.UpdateTaskSessionState(
		ctx, stored.ID, models.TaskSessionStateCancelled, "stopped via API",
	); err != nil {
		t.Fatalf("cancel session: %v", err)
	}

	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	var gotExpectedState models.TaskSessionState
	exec.SetOnSessionStarting(func(
		ctx context.Context,
		_ string,
		next *models.TaskSession,
		expectedState models.TaskSessionState,
		_ bool,
	) error {
		gotExpectedState = expectedState
		return exec.persistSessionFullRowIfCurrentState(ctx, next, expectedState)
	})

	err := exec.persistModelSwitchState(
		ctx, stale.TaskID, stale.ID, &stale, "new-model",
	)
	if !errors.Is(err, ErrSessionStateSuperseded) {
		t.Fatalf("persistModelSwitchState error = %v, want ErrSessionStateSuperseded", err)
	}
	if gotExpectedState != models.TaskSessionStateRunning {
		t.Fatalf("expected state = %q, want RUNNING", gotExpectedState)
	}
	persisted := repo.sessions[stored.ID]
	if persisted.State != models.TaskSessionStateCancelled {
		t.Fatalf("stored state = %q, want CANCELLED", persisted.State)
	}
	if got, _ := persisted.AgentProfileSnapshot["model"].(string); got != "old-model" {
		t.Fatalf("stored model = %q, want old-model", got)
	}
}

func TestPersistInPlaceModelSwitch_DoesNotRestoreStaleActiveStateAfterCancellation(t *testing.T) {
	repo := newMockRepository()
	session := &models.TaskSession{
		ID:                   "session-model-race",
		TaskID:               "task-model-race",
		State:                models.TaskSessionStateRunning,
		AgentProfileSnapshot: map[string]interface{}{"model": "old-model"},
		Metadata:             map[string]interface{}{},
	}
	repo.sessions[session.ID] = session
	repo.getTaskSessionFunc = func(context.Context, string) (*models.TaskSession, error) {
		stale := *session
		stale.AgentProfileSnapshot = map[string]interface{}{"model": "old-model"}
		stale.Metadata = map[string]interface{}{}

		repo.mu.Lock()
		cancelled := *session
		cancelled.State = models.TaskSessionStateCancelled
		cancelled.ErrorMessage = "stopped by parent task via MCP"
		repo.sessions[session.ID] = &cancelled
		repo.mu.Unlock()
		return &stale, nil
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	exec.persistInPlaceModelSwitch(context.Background(), session.ID, "new-model")

	repo.mu.Lock()
	stored := repo.sessions[session.ID]
	repo.mu.Unlock()
	if stored.State != models.TaskSessionStateCancelled {
		t.Fatalf("session state = %q, want CANCELLED", stored.State)
	}
	if stored.ErrorMessage != "stopped by parent task via MCP" {
		t.Fatalf("error message = %q, want coordinator stop reason", stored.ErrorMessage)
	}
	if got, _ := stored.AgentProfileSnapshot["model"].(string); got != "new-model" {
		t.Fatalf("snapshot model = %q, want new-model", got)
	}
}

func TestSwitchModelFallback_PreservesRestrictedMCPMode(t *testing.T) {
	tests := []struct {
		name         string
		isFromOffice bool
		assignee     string
		metadata     map[string]interface{}
		wantMode     string
	}{
		{name: "unassigned Office task", isFromOffice: true, wantMode: McpModeOffice},
		{name: "assigned Kanban task", assignee: "assigned-agent", wantMode: ""},
		{
			name:         "Config session takes precedence for Office task",
			isFromOffice: true,
			metadata:     map[string]interface{}{"config_mode": true},
			wantMode:     McpModeConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepository()
			repo.tasks["task-office"] = &models.Task{
				ID:                     "task-office",
				WorkspaceID:            "workspace-1",
				Title:                  "Office task",
				IsFromOffice:           tt.isFromOffice,
				AssigneeAgentProfileID: tt.assignee,
			}
			repo.sessions["session-office"] = &models.TaskSession{
				ID:             "session-office",
				TaskID:         "task-office",
				AgentProfileID: "office-agent",
				State:          models.TaskSessionStateRunning,
				Metadata:       tt.metadata,
			}

			var capturedReq *LaunchAgentRequest
			manager := &mockAgentManager{
				getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
					return "execution-old", nil
				},
				launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
					capturedReq = req
					return &LaunchAgentResponse{AgentExecutionID: "execution-new"}, nil
				},
			}
			exec := newTestExecutor(t, manager, repo)
			exec.SetGitLabCredentialResolver(&fakeGitLabCredentialResolver{byWorkspace: map[string]struct{ host, token string }{
				"workspace-1": {host: "https://gitlab.example", token: "switch-token"},
			}})

			if _, err := exec.SwitchModel(
				context.Background(), "task-office", "session-office", "new-model", "continue",
			); err != nil {
				t.Fatalf("SwitchModel: %v", err)
			}
			if capturedReq == nil {
				t.Fatal("fallback did not call LaunchAgent")
			}
			if capturedReq.McpMode != tt.wantMode {
				t.Fatalf("McpMode = %q, want %q", capturedReq.McpMode, tt.wantMode)
			}
			if capturedReq.WorkspaceID != "workspace-1" || capturedReq.Env[envGitLabToken] != "switch-token" {
				t.Fatalf("model-switch credentials not workspace scoped: workspace=%q env=%#v", capturedReq.WorkspaceID, capturedReq.Env)
			}
		})
	}
}
