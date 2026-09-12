package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// recoveryStoppingExecutor is a minimal ExecutorBackend that also implements
// the optional recoveryStopper capability, so a test can observe which path
// stopUnreconstructableRecoveredInstance took.
type recoveryStoppingExecutor struct {
	name               executor.Name
	stopInstanceCalls  int
	stopWithRetryCalls []string
	stopWithRetryErr   error
}

func (e *recoveryStoppingExecutor) Name() executor.Name               { return e.name }
func (e *recoveryStoppingExecutor) HealthCheck(context.Context) error { return nil }
func (e *recoveryStoppingExecutor) CreateInstance(context.Context, *ExecutorCreateRequest) (*ExecutorInstance, error) {
	return nil, nil
}
func (e *recoveryStoppingExecutor) StopInstance(context.Context, *ExecutorInstance, bool) error {
	e.stopInstanceCalls++
	return nil
}
func (e *recoveryStoppingExecutor) RecoverInstances(context.Context, []*models.ExecutorRunning) ([]*ExecutorInstance, error) {
	return nil, nil
}
func (e *recoveryStoppingExecutor) GetInteractiveRunner() *process.InteractiveRunner { return nil }
func (e *recoveryStoppingExecutor) RequiresCloneURL() bool                           { return false }
func (e *recoveryStoppingExecutor) ShouldApplyPreferredShell() bool                  { return false }
func (e *recoveryStoppingExecutor) IsAlwaysResumable() bool                          { return false }
func (e *recoveryStoppingExecutor) stopWithRetry(_ context.Context, instanceID string) error {
	e.stopWithRetryCalls = append(e.stopWithRetryCalls, instanceID)
	return e.stopWithRetryErr
}

func newRecoveryStopTestManager(t *testing.T, backend ExecutorBackend) *Manager {
	t.Helper()
	log := newTestLogger()
	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	return mgr
}

// TestStopUnreconstructableRecoveredInstanceUsesBoundedRetryWhenSupported
// pins Review round 1 finding 6 (AC-EXECUTORS-SURVIVAL-002.15): a standalone
// backend's bounded-retry stop path must be used for an unreconstructable
// recovered instance, matching the AC-002.6/002.10 loser/orphan stop paths
// in the same package -- not a single unretried StopInstance call.
func TestStopUnreconstructableRecoveredInstanceUsesBoundedRetryWhenSupported(t *testing.T) {
	backend := &recoveryStoppingExecutor{name: executor.NameStandalone}
	mgr := newRecoveryStopTestManager(t, backend)

	ri := &ExecutorInstance{
		InstanceID:           "exec-1",
		RuntimeName:          executor.NameStandalone,
		StandaloneInstanceID: "standalone-1",
	}
	mgr.stopUnreconstructableRecoveredInstance(context.Background(), ri)

	if len(backend.stopWithRetryCalls) != 1 || backend.stopWithRetryCalls[0] != "standalone-1" {
		t.Fatalf("stopWithRetry calls = %v, want exactly one call for standalone-1", backend.stopWithRetryCalls)
	}
	if backend.stopInstanceCalls != 0 {
		t.Fatalf("StopInstance calls = %d, want 0 (bounded-retry path should be used instead)", backend.stopInstanceCalls)
	}
}

// TestStopUnreconstructableRecoveredInstanceLogsRetryExhaustion pins that a
// bounded-retry failure is logged rather than silently dropped
// (AC-EXECUTORS-SURVIVAL-002.4/002.15): the warning must actually be emitted,
// carrying the instance identity and the underlying error, not just leave the
// call completing without panicking and without falling back to StopInstance.
func TestStopUnreconstructableRecoveredInstanceLogsRetryExhaustion(t *testing.T) {
	retryErr := errors.New("boom")
	backend := &recoveryStoppingExecutor{name: executor.NameStandalone, stopWithRetryErr: retryErr}
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)

	ri := &ExecutorInstance{
		InstanceID:           "exec-1",
		RuntimeName:          executor.NameStandalone,
		StandaloneInstanceID: "standalone-1",
	}
	mgr.stopUnreconstructableRecoveredInstance(context.Background(), ri)

	if len(backend.stopWithRetryCalls) != 1 {
		t.Fatalf("stopWithRetry calls = %d, want 1", len(backend.stopWithRetryCalls))
	}
	if backend.stopInstanceCalls != 0 {
		t.Fatalf("StopInstance calls = %d, want 0 even when the retry path fails", backend.stopInstanceCalls)
	}

	entries := logs.FilterMessage("failed to stop unreconstructable recovered instance after exhausting retries").All()
	if len(entries) != 1 {
		t.Fatalf("warn entries = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if got := fields["instance_id"]; got != "exec-1" {
		t.Fatalf("instance_id = %v, want exec-1", got)
	}
	if got, ok := fields["error"].(string); !ok || got != retryErr.Error() {
		t.Fatalf("error = %v, want %q", fields["error"], retryErr.Error())
	}
}

// TestStopUnreconstructableRecoveredInstanceFallsBackWithoutRetrySupport
// pins that a backend without the recoveryStopper capability (Docker,
// Sprites, SSH, Kubernetes) keeps using the plain StopInstance call --
// AC-EXECUTORS-SURVIVAL-002.15's retry contract is standalone-only.
func TestStopUnreconstructableRecoveredInstanceFallsBackWithoutRetrySupport(t *testing.T) {
	mock := &guardObservingExecutor{name: executor.NameDocker}
	mgr := newRecoveryStopTestManager(t, mock)

	ri := &ExecutorInstance{InstanceID: "exec-1", RuntimeName: executor.NameDocker}
	mgr.stopUnreconstructableRecoveredInstance(context.Background(), ri)

	if mock.stopInstanceCalls != 1 {
		t.Fatalf("StopInstance calls = %d, want 1 via the plain fallback path", mock.stopInstanceCalls)
	}
}

// TestStopUnreconstructableRecoveredInstanceRetainsGuardOnBoundedRetryFailure
// pins AC-EXECUTORS-SURVIVAL-002.16: when the bounded-retry stop path
// exhausts its retries, the session's recovery guard must be retained for
// the rest of this backend's lifetime -- not left for ReleaseAllExceptRetained
// to release, which would let a fresh launch race the still-live instance.
func TestStopUnreconstructableRecoveredInstanceRetainsGuardOnBoundedRetryFailure(t *testing.T) {
	backend := &recoveryStoppingExecutor{name: executor.NameStandalone, stopWithRetryErr: errors.New("boom")}
	mgr := newRecoveryStopTestManager(t, backend)
	mgr.recoveryGuard.AcquireOrObserve("session-1")

	ri := &ExecutorInstance{
		InstanceID:           "exec-1",
		SessionID:            "session-1",
		RuntimeName:          executor.NameStandalone,
		StandaloneInstanceID: "standalone-1",
	}
	mgr.stopUnreconstructableRecoveredInstance(context.Background(), ri)

	if err := mgr.recoveryGuard.CheckLaunchAllowed("session-1"); !errors.Is(err, ErrSessionUnstoppableAgent) {
		t.Fatalf("CheckLaunchAllowed = %v, want ErrSessionUnstoppableAgent", err)
	}
	mgr.recoveryGuard.ReleaseAllExceptRetained()
	if err := mgr.recoveryGuard.CheckLaunchAllowed("session-1"); !errors.Is(err, ErrSessionUnstoppableAgent) {
		t.Fatalf("guard must survive ReleaseAllExceptRetained, got %v", err)
	}
}

// TestStopUnreconstructableRecoveredInstanceReleasesGuardOnBoundedRetrySuccess
// pins the success counterpart: once the bounded-retry stop actually
// succeeds, the session's guard must not be left retained -- a session whose
// live instance really did stop is not the AC-EXECUTORS-SURVIVAL-002.16 case.
func TestStopUnreconstructableRecoveredInstanceReleasesGuardOnBoundedRetrySuccess(t *testing.T) {
	backend := &recoveryStoppingExecutor{name: executor.NameStandalone}
	mgr := newRecoveryStopTestManager(t, backend)
	mgr.recoveryGuard.AcquireOrObserve("session-1")

	ri := &ExecutorInstance{
		InstanceID:           "exec-1",
		SessionID:            "session-1",
		RuntimeName:          executor.NameStandalone,
		StandaloneInstanceID: "standalone-1",
	}
	mgr.stopUnreconstructableRecoveredInstance(context.Background(), ri)

	if err := mgr.recoveryGuard.CheckLaunchAllowed("session-1"); err != nil {
		t.Fatalf("CheckLaunchAllowed = %v, want nil (guard released on stop success)", err)
	}
}

// TestStopUnreconstructableRecoveredInstanceRetainsGuardWhenPlainStopInstanceFails
// pins AC-EXECUTORS-SURVIVAL-002.16 on the non-standalone fallback path too:
// a Docker/Sprites/SSH/Kubernetes instance that could not be stopped via the
// plain StopInstance call must retain its guard exactly like the
// bounded-retry path does.
func TestStopUnreconstructableRecoveredInstanceRetainsGuardWhenPlainStopInstanceFails(t *testing.T) {
	mock := &MockExecutor{name: executor.NameDocker, stopInstanceErr: errors.New("boom")}
	mgr := newRecoveryStopTestManager(t, mock)
	mgr.recoveryGuard.AcquireOrObserve("session-1")

	ri := &ExecutorInstance{InstanceID: "exec-1", SessionID: "session-1", RuntimeName: executor.NameDocker}
	mgr.stopUnreconstructableRecoveredInstance(context.Background(), ri)

	if err := mgr.recoveryGuard.CheckLaunchAllowed("session-1"); !errors.Is(err, ErrSessionUnstoppableAgent) {
		t.Fatalf("CheckLaunchAllowed = %v, want ErrSessionUnstoppableAgent", err)
	}
}

// TestStopUnreconstructableRecoveredInstanceRetainsGuardWhenNoBackendForRuntime
// pins AC-EXECUTORS-SURVIVAL-002.16 on the defensive no-backend branch: an
// instance whose runtime has no registered backend at all was never even
// attempted to stop, so it must be treated the same as a failed stop --
// retained, not silently left for a bulk release to clear.
func TestStopUnreconstructableRecoveredInstanceRetainsGuardWhenNoBackendForRuntime(t *testing.T) {
	backend := &recoveryStoppingExecutor{name: executor.NameStandalone}
	mgr := newRecoveryStopTestManager(t, backend)
	mgr.recoveryGuard.AcquireOrObserve("session-1")

	ri := &ExecutorInstance{InstanceID: "exec-1", SessionID: "session-1", RuntimeName: executor.NameDocker}
	mgr.stopUnreconstructableRecoveredInstance(context.Background(), ri)

	if err := mgr.recoveryGuard.CheckLaunchAllowed("session-1"); !errors.Is(err, ErrSessionUnstoppableAgent) {
		t.Fatalf("CheckLaunchAllowed = %v, want ErrSessionUnstoppableAgent", err)
	}
}

// blockingUnreconstructableStopper is a minimal ExecutorBackend + recoveryStopper
// whose stopWithRetry blocks one named instance until the test releases it,
// so a Manager.Start test can observe whether other recovered records are
// reconstructed while that stop is still in flight.
type blockingUnreconstructableStopper struct {
	name        executor.Name
	recovered   []*ExecutorInstance
	blockedInst string
	started     chan struct{}
	unblock     chan struct{}
}

func (e *blockingUnreconstructableStopper) Name() executor.Name               { return e.name }
func (e *blockingUnreconstructableStopper) HealthCheck(context.Context) error { return nil }
func (e *blockingUnreconstructableStopper) CreateInstance(context.Context, *ExecutorCreateRequest) (*ExecutorInstance, error) {
	return nil, nil
}
func (e *blockingUnreconstructableStopper) StopInstance(context.Context, *ExecutorInstance, bool) error {
	return nil
}
func (e *blockingUnreconstructableStopper) RecoverInstances(context.Context, []*models.ExecutorRunning) ([]*ExecutorInstance, error) {
	return e.recovered, nil
}
func (e *blockingUnreconstructableStopper) GetInteractiveRunner() *process.InteractiveRunner {
	return nil
}
func (e *blockingUnreconstructableStopper) RequiresCloneURL() bool          { return false }
func (e *blockingUnreconstructableStopper) ShouldApplyPreferredShell() bool { return false }
func (e *blockingUnreconstructableStopper) IsAlwaysResumable() bool         { return false }
func (e *blockingUnreconstructableStopper) stopWithRetry(_ context.Context, instanceID string) error {
	if instanceID == e.blockedInst {
		close(e.started)
		<-e.unblock
	}
	return nil
}

// TestManagerStartDoesNotBlockOtherRecordsOnOneUnreconstructableStop pins
// AC-EXECUTORS-SURVIVAL-002.11 ("no such instance's outcome shall depend on
// another's"): one recovered record whose not-re-tracked stop is still
// retrying must not prevent a different record in the same pass from being
// fully reconstructed and published. Manager.Start dispatches
// stopUnreconstructableRecoveredInstance on its own goroutine
// (dispatchUnreconstructableStop) rather than blocking the shared consumer
// loop inline.
func TestManagerStartDoesNotBlockOtherRecordsOnOneUnreconstructableStop(t *testing.T) {
	log := newTestRegistryLogger()
	execRegistry := NewExecutorRegistry(log)
	backend := &blockingUnreconstructableStopper{
		name: executor.NameStandalone,
		recovered: []*ExecutorInstance{
			{
				InstanceID:           "exec-blocked",
				TaskID:               "", // empty TaskID triggers the AC-002.4 refusal path
				SessionID:            "session-blocked",
				RuntimeName:          executor.NameStandalone,
				StandaloneInstanceID: "standalone-blocked",
			},
			{
				InstanceID:           "exec-fine",
				TaskID:               "task-fine",
				SessionID:            "session-fine",
				AgentProfileID:       recoveryTestAgentProfileID,
				RuntimeName:          executor.NameStandalone,
				StandaloneInstanceID: "standalone-fine",
			},
		},
		blockedInst: "standalone-blocked",
		started:     make(chan struct{}),
		unblock:     make(chan struct{}),
	}
	execRegistry.Register(backend)

	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	registerRecoveryTestAgentProfile(t, mgr)
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{
		{SessionID: "session-blocked"},
		{SessionID: "session-fine"},
	}})

	done := make(chan error, 1)
	go func() { done <- mgr.Start(context.Background()) }()

	select {
	case <-backend.started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the blocked instance's stop to begin")
	}

	// session-blocked's stop stays in flight (backend.unblock is not closed
	// yet) for this entire poll window: on the pre-fix synchronous dispatch,
	// session-fine could never reach the store until after that stop
	// resolved, so this loop would exhaust its deadline and fail. It only
	// succeeds here because dispatchUnreconstructableStop runs the stop on
	// its own goroutine, letting the shared consumer loop reconstruct
	// session-fine independently.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := mgr.executionStore.GetBySessionID("session-fine"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session-fine was not re-tracked while session-blocked's stop was still in flight")
		}
		time.Sleep(5 * time.Millisecond)
	}

	close(backend.unblock)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Start to finish after unblocking the stop")
	}

	if err := mgr.recoveryGuard.CheckLaunchAllowed("session-blocked"); err != nil {
		t.Fatalf("expected session-blocked's guard released once its stop succeeded, got %v", err)
	}
}
