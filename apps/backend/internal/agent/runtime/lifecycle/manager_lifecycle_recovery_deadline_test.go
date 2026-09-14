package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/task/models"
)

// deadlineRecoveringExecutor is a minimal ExecutorBackend that returns a
// fixed set of recovered instances from RecoverInstances (unlike
// guardObservingExecutor, which always returns none) and also implements
// the optional recoveryStopper capability, so a test can observe whether a
// recovered instance's not-re-tracked stop used the bounded-retry path.
type deadlineRecoveringExecutor struct {
	name               executor.Name
	recovered          []*ExecutorInstance
	stopWithRetryCalls []string
}

func (e *deadlineRecoveringExecutor) Name() executor.Name               { return e.name }
func (e *deadlineRecoveringExecutor) HealthCheck(context.Context) error { return nil }
func (e *deadlineRecoveringExecutor) CreateInstance(context.Context, *ExecutorCreateRequest) (*ExecutorInstance, error) {
	return nil, nil
}
func (e *deadlineRecoveringExecutor) StopInstance(context.Context, *ExecutorInstance, bool) error {
	return nil
}
func (e *deadlineRecoveringExecutor) RecoverInstances(context.Context, []*models.ExecutorRunning) ([]*ExecutorInstance, error) {
	return e.recovered, nil
}
func (e *deadlineRecoveringExecutor) GetInteractiveRunner() *process.InteractiveRunner { return nil }
func (e *deadlineRecoveringExecutor) RequiresCloneURL() bool                           { return false }
func (e *deadlineRecoveringExecutor) ShouldApplyPreferredShell() bool                  { return false }
func (e *deadlineRecoveringExecutor) IsAlwaysResumable() bool                          { return false }
func (e *deadlineRecoveringExecutor) stopWithRetry(_ context.Context, instanceID string) error {
	e.stopWithRetryCalls = append(e.stopWithRetryCalls, instanceID)
	return nil
}

// TestManagerStartTreatsRecordsAsNotRetrackedOnceRecoveryDeadlineElapses
// pins Review round 1 finding 2 (AC-EXECUTORS-SURVIVAL-003.7): once the
// recovery deadline has elapsed, a record the consumer loop has not yet
// reconstructed must be treated as not re-tracked -- stopped via the bounded
// retry path and never added to the execution store or published running --
// rather than reconstructed anyway on the ambient unbounded context.
func TestManagerStartTreatsRecordsAsNotRetrackedOnceRecoveryDeadlineElapses(t *testing.T) {
	log := newTestLogger()
	execRegistry := NewExecutorRegistry(log)
	backend := &deadlineRecoveringExecutor{
		name: executor.NameStandalone,
		recovered: []*ExecutorInstance{
			{
				InstanceID:           "exec-1",
				TaskID:               "task-1",
				SessionID:            "session-1",
				AgentProfileID:       "profile-1",
				RuntimeName:          executor.NameStandalone,
				StandaloneInstanceID: "standalone-1",
			},
		},
	}
	execRegistry.Register(backend)

	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{{SessionID: "session-1"}}})
	// The recovery deadline already elapsed before Start() is even called:
	// its clock started an hour ago and the configured bound is far shorter.
	mgr.SetRecoveryDeadlineStart(time.Now().Add(-time.Hour))
	mgr.SetRecoveryDeadline(time.Millisecond)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if _, exists := mgr.executionStore.GetBySessionID("session-1"); exists {
		t.Fatal("session-1 was added to the execution store despite the recovery deadline having elapsed")
	}
	if len(backend.stopWithRetryCalls) != 1 || backend.stopWithRetryCalls[0] != "standalone-1" {
		t.Fatalf("stopWithRetry calls = %v, want exactly one call for standalone-1", backend.stopWithRetryCalls)
	}
	// The guard must not be left held forever: ReleaseAllExceptRetained
	// still runs after the loop even though this session was never
	// individually released inside it.
	if err := mgr.recoveryGuard.CheckLaunchAllowed("session-1"); err != nil {
		t.Fatalf("expected session-1's guard released once Start finished, got %v", err)
	}
}

// guardObservingProfileResolver wraps a ProfileResolver and records the
// recovery guard's state for one session at the moment ResolveProfile is
// called -- a point squarely inside the consumer loop's per-instance
// reconstruction (reDeriveRecoveredAgentIdentity), well before that
// session's outcome is added to the execution store or published.
type guardObservingProfileResolver struct {
	mgr                   *Manager
	sessionID             string
	guardErrDuringResolve error
	observed              bool
}

func (r *guardObservingProfileResolver) ResolveProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error) {
	r.guardErrDuringResolve = r.mgr.recoveryGuard.CheckLaunchAllowed(r.sessionID)
	r.observed = true
	return &AgentProfileInfo{
		ProfileID:   profileID,
		ProfileName: "Test Profile",
		AgentID:     "augment-agent",
		AgentName:   "auggie",
		Model:       "claude-sonnet-4-20250514",
	}, nil
}

// TestManagerStartHoldsGuardThroughReconstructionAndReleasesAfterPublish
// pins Review round 1 finding 3: a recovered session's guard must still be
// held while that session's instance is being reconstructed (identity
// re-derivation, turn-outcome read) and is released only once its outcome
// is actually published -- not released in bulk immediately after RecoverAll
// returns, before any reconstruction for it has even started. Under the
// pre-fix code the bulk release ran before this loop began at all, so the
// guard-observing hook below would have found it already released.
func TestManagerStartHoldsGuardThroughReconstructionAndReleasesAfterPublish(t *testing.T) {
	log := newTestLogger()
	execRegistry := NewExecutorRegistry(log)
	backend := &deadlineRecoveringExecutor{
		name: executor.NameStandalone,
		recovered: []*ExecutorInstance{
			{
				InstanceID:           "exec-1",
				TaskID:               "task-1",
				SessionID:            "session-1",
				AgentProfileID:       "profile-1",
				RuntimeName:          executor.NameStandalone,
				StandaloneInstanceID: "standalone-1",
			},
		},
	}
	execRegistry.Register(backend)

	resolver := &guardObservingProfileResolver{sessionID: "session-1"}
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, resolver, nil, ExecutorFallbackWarn, "", log)
	resolver.mgr = mgr
	cleanupManagerStopCh(t, mgr)
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{{SessionID: "session-1"}}})

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !resolver.observed {
		t.Fatal("ResolveProfile was never called; the guard-observing hook did not fire")
	}
	if !errors.Is(resolver.guardErrDuringResolve, ErrSessionRecoveryGuarded) {
		t.Fatalf("guard state mid-reconstruction = %v, want ErrSessionRecoveryGuarded (guard must still be held)", resolver.guardErrDuringResolve)
	}
	if _, exists := mgr.executionStore.GetBySessionID("session-1"); !exists {
		t.Fatal("session-1 was not re-tracked despite recovering well within the deadline")
	}
	if err := mgr.recoveryGuard.CheckLaunchAllowed("session-1"); err != nil {
		t.Fatalf("expected session-1's guard released once its outcome was published, got %v", err)
	}
}
