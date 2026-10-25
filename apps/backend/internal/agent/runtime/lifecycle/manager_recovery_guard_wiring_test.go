package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/task/models"
)

// guardObservingExecutor is a minimal ExecutorBackend whose RecoverInstances
// calls back into the test so it can observe recovery-guard state from
// inside the same synchronous call Manager.Start makes, before
// ReleaseAllExceptRetained runs.
type guardObservingExecutor struct {
	name               executor.Name
	onRecoverInstances func(ctx context.Context, records []*models.ExecutorRunning)
	stopInstanceCalls  int
}

func (e *guardObservingExecutor) Name() executor.Name               { return e.name }
func (e *guardObservingExecutor) HealthCheck(context.Context) error { return nil }
func (e *guardObservingExecutor) CreateInstance(context.Context, *ExecutorCreateRequest) (*ExecutorInstance, error) {
	return nil, nil
}
func (e *guardObservingExecutor) StopInstance(context.Context, *ExecutorInstance, bool) error {
	e.stopInstanceCalls++
	return nil
}
func (e *guardObservingExecutor) RecoverInstances(ctx context.Context, records []*models.ExecutorRunning) ([]*ExecutorInstance, error) {
	if e.onRecoverInstances != nil {
		e.onRecoverInstances(ctx, records)
	}
	return nil, nil
}
func (e *guardObservingExecutor) GetInteractiveRunner() *process.InteractiveRunner { return nil }
func (e *guardObservingExecutor) RequiresCloneURL() bool                           { return false }
func (e *guardObservingExecutor) ShouldApplyPreferredShell() bool                  { return false }
func (e *guardObservingExecutor) IsAlwaysResumable() bool                          { return false }

// TestManagerStartGuardsLiveStandaloneSessionsExceptPassthroughAndReleasesAfterRecovery
// pins AC-EXECUTORS-SURVIVAL-002.8/005.3: every session named by a live
// standalone recovery-inventory record is guarded before any runtime's
// RecoverInstances is called, a confirmed-passthrough session is excluded,
// and every non-retained guard is released once recovery completes.
func TestManagerStartGuardsLiveStandaloneSessionsExceptPassthroughAndReleasesAfterRecovery(t *testing.T) {
	log := newTestLogger()
	execRegistry := NewExecutorRegistry(log)

	var mgr *Manager
	var guardedErr, passthroughErr error
	mock := &guardObservingExecutor{
		name: executor.NameStandalone,
		onRecoverInstances: func(context.Context, []*models.ExecutorRunning) {
			guardedErr = mgr.recoveryGuard.CheckLaunchAllowed("session-guarded")
			passthroughErr = mgr.recoveryGuard.CheckLaunchAllowed("session-passthrough")
		},
	}
	execRegistry.Register(mock)

	mgr = NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)

	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{
		{SessionID: "session-guarded"},
		{SessionID: "session-passthrough"},
	}})
	mgr.SetPassthroughLookup(func(_ context.Context, sessionID string) (bool, bool) {
		return sessionID == "session-passthrough", true
	})

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if guardedErr == nil {
		t.Fatal("expected session-guarded to be guarded during recovery")
	}
	if !errors.Is(guardedErr, ErrSessionRecoveryGuarded) {
		t.Fatalf("session-guarded guard check = %v, want ErrSessionRecoveryGuarded", guardedErr)
	}
	if passthroughErr != nil {
		t.Fatalf("expected passthrough session to be excluded from guarding, got %v", passthroughErr)
	}

	if err := mgr.recoveryGuard.CheckLaunchAllowed("session-guarded"); err != nil {
		t.Fatalf("expected guard released once recovery finished, got %v", err)
	}
}

// TestManagerStartGuardsSessionWhenPassthroughLookupUnconfigured pins the
// documented default: with no SetPassthroughLookup call, every live
// standalone session is guarded rather than excluded.
func TestManagerStartGuardsSessionWhenPassthroughLookupUnconfigured(t *testing.T) {
	log := newTestLogger()
	execRegistry := NewExecutorRegistry(log)

	var mgr *Manager
	var guardedErr error
	mock := &guardObservingExecutor{
		name: executor.NameStandalone,
		onRecoverInstances: func(context.Context, []*models.ExecutorRunning) {
			guardedErr = mgr.recoveryGuard.CheckLaunchAllowed("session-no-lookup")
		},
	}
	execRegistry.Register(mock)

	mgr = NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)

	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{
		{SessionID: "session-no-lookup"},
	}})

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !errors.Is(guardedErr, ErrSessionRecoveryGuarded) {
		t.Fatalf("session-no-lookup guard check = %v, want ErrSessionRecoveryGuarded", guardedErr)
	}
}

// TestManagerLaunchRefusedForGuardedSession pins that Launch checks the
// recovery guard before doing anything else (AC-EXECUTORS-SURVIVAL-002.8):
// a session whose guard is currently held is refused immediately with the
// retryable sentinel, not queued behind recovery.
func TestManagerLaunchRefusedForGuardedSession(t *testing.T) {
	mgr := newTestManager(t)
	mgr.recoveryGuard.AcquireOrObserve("session-guarded")

	execution, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:    "task-guarded",
		SessionID: "session-guarded",
	})
	if execution != nil {
		t.Fatalf("expected nil execution, got %+v", execution)
	}
	if !errors.Is(err, ErrSessionRecoveryGuarded) {
		t.Fatalf("Launch error = %v, want ErrSessionRecoveryGuarded", err)
	}
}

// TestManagerLaunchRefusedForRetainedUnstoppableSession pins the
// AC-EXECUTORS-SURVIVAL-002.16 non-retryable refusal.
func TestManagerLaunchRefusedForRetainedUnstoppableSession(t *testing.T) {
	mgr := newTestManager(t)
	mgr.recoveryGuard.RetainAsUnstoppable("session-unstoppable")

	execution, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:    "task-unstoppable",
		SessionID: "session-unstoppable",
	})
	if execution != nil {
		t.Fatalf("expected nil execution, got %+v", execution)
	}
	if !errors.Is(err, ErrSessionUnstoppableAgent) {
		t.Fatalf("Launch error = %v, want ErrSessionUnstoppableAgent", err)
	}
}

// TestManagerLaunchProceedsForUnguardedSession pins that the new guard check
// does not interfere with an ordinary launch for a session that was never
// guarded.
func TestManagerLaunchProceedsForUnguardedSession(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, nil)

	_, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:         "task-unguarded",
		SessionID:      "session-unguarded",
		AgentProfileID: "profile-unguarded",
		ExecutorType:   string(models.ExecutorTypeLocal),
		IsEphemeral:    true,
	})
	if err != nil {
		t.Fatalf("Manager.Launch: %v", err)
	}
	if backend.lastRequest == nil {
		t.Fatal("expected executor CreateInstance to be called for an unguarded session")
	}
}
