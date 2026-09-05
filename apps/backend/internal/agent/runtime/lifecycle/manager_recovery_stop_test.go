package lifecycle

import (
	"context"
	"errors"
	"testing"

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
