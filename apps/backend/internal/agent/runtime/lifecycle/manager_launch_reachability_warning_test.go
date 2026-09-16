package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

type fakeReachabilityReader struct {
	records map[string]*models.ExecutorReachability
	err     error
	calls   []string
}

func (f *fakeReachabilityReader) GetExecutorReachability(_ context.Context, executorID string) (*models.ExecutorReachability, error) {
	f.calls = append(f.calls, executorID)
	if f.err != nil {
		return nil, f.err
	}
	return f.records[executorID], nil
}

// failOnReadReachabilityReader fails the test the moment it is touched at
// all — used to prove a code path performs no reachability read, rather than
// merely counting reads after the fact.
type failOnReadReachabilityReader struct {
	t *testing.T
}

func (f failOnReadReachabilityReader) GetExecutorReachability(context.Context, string) (*models.ExecutorReachability, error) {
	f.t.Fatal("reachability record was read from a code path that must never read one")
	return nil, nil
}

func newWarningTestManager(t *testing.T, reader ReachabilityReader, probingEnabled bool, windowSeconds int) (*Manager, *bus.MemoryEventBus) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	memBus := bus.NewMemoryEventBus(log)
	mgr := &Manager{
		logger:         log,
		eventPublisher: NewEventPublisher(memBus, log),
	}
	mgr.SetSSHReachabilityWarningPolicy(reader, probingEnabled, windowSeconds)
	return mgr, memBus
}

func subscribeLaunchWarnings(t *testing.T, memBus *bus.MemoryEventBus, sessionID string) <-chan *bus.Event {
	t.Helper()
	received := make(chan *bus.Event, 1)
	sub, err := memBus.Subscribe(events.BuildSessionLaunchWarningSubject(sessionID), func(_ context.Context, ev *bus.Event) error {
		received <- ev
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return received
}

const warningWindowSeconds = 180 // 3x reachability.DefaultIntervalSeconds

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.3
func TestMaybePublishSSHLaunchWarning_UnreachableAndProbingEnabledWarns(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	lastSuccess := old.Add(-time.Hour)
	reader := &fakeReachabilityReader{records: map[string]*models.ExecutorReachability{
		"executor-1": {
			ExecutorID:    "executor-1",
			State:         models.ExecutorReachabilityStateUnreachable,
			Reason:        models.ExecutorReachabilityReasonTimeout,
			Host:          "build.example",
			CheckedAt:     &old,
			LastSuccessAt: &lastSuccess,
		},
	}}
	mgr, memBus := newWarningTestManager(t, reader, true, warningWindowSeconds)
	received := subscribeLaunchWarnings(t, memBus, "session-1")

	req := &LaunchRequest{
		ExecutorType: string(models.ExecutorTypeSSH),
		TaskID:       "task-1",
		SessionID:    "session-1",
	}
	metadata := map[string]interface{}{"executor_id": "executor-1"}
	mgr.maybePublishSSHLaunchWarning(context.Background(), req, metadata, req.SessionID)

	select {
	case ev := <-received:
		payload, ok := ev.Data.(*LaunchWarningEventPayload)
		if !ok {
			t.Fatalf("payload type = %T", ev.Data)
		}
		if payload.ExecutorID != "executor-1" || payload.Host != "build.example" ||
			payload.State != string(models.ExecutorReachabilityStateUnreachable) ||
			payload.Reason != string(models.ExecutorReachabilityReasonTimeout) ||
			payload.LastSuccessAt == nil || !payload.LastSuccessAt.Equal(lastSuccess) {
			t.Fatalf("payload = %+v", payload)
		}
	default:
		t.Fatal("expected a session.launch.warning event, got none")
	}
	if len(reader.calls) != 1 || reader.calls[0] != "executor-1" {
		t.Fatalf("reader.calls = %v", reader.calls)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.3
func TestMaybePublishSSHLaunchWarning_ProbingDisabledButRecentlyCheckedWarns(t *testing.T) {
	recent := time.Now().Add(-time.Minute)
	reader := &fakeReachabilityReader{records: map[string]*models.ExecutorReachability{
		"executor-1": {
			ExecutorID: "executor-1",
			State:      models.ExecutorReachabilityStateUnreachable,
			Reason:     models.ExecutorReachabilityReasonNetwork,
			Host:       "build.example",
			CheckedAt:  &recent,
		},
	}}
	mgr, memBus := newWarningTestManager(t, reader, false, warningWindowSeconds)
	received := subscribeLaunchWarnings(t, memBus, "session-1")

	req := &LaunchRequest{ExecutorType: string(models.ExecutorTypeSSH), SessionID: "session-1"}
	mgr.maybePublishSSHLaunchWarning(context.Background(), req, map[string]interface{}{"executor_id": "executor-1"}, req.SessionID)

	select {
	case <-received:
	default:
		t.Fatal("expected a session.launch.warning event when checked_at is within the warning window")
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.3
func TestMaybePublishSSHLaunchWarning_ProbingDisabledAndStaleCheckedAtDoesNotWarn(t *testing.T) {
	stale := time.Now().Add(-time.Hour)
	reader := &fakeReachabilityReader{records: map[string]*models.ExecutorReachability{
		"executor-1": {
			ExecutorID: "executor-1",
			State:      models.ExecutorReachabilityStateUnreachable,
			Host:       "build.example",
			CheckedAt:  &stale,
		},
	}}
	mgr, memBus := newWarningTestManager(t, reader, false, warningWindowSeconds)
	received := subscribeLaunchWarnings(t, memBus, "session-1")

	req := &LaunchRequest{ExecutorType: string(models.ExecutorTypeSSH), SessionID: "session-1"}
	mgr.maybePublishSSHLaunchWarning(context.Background(), req, map[string]interface{}{"executor_id": "executor-1"}, req.SessionID)

	select {
	case ev := <-received:
		t.Fatalf("expected no warning for a stale checked_at with probing disabled, got %+v", ev)
	default:
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.3
func TestMaybePublishSSHLaunchWarning_ReachableRecordDoesNotWarn(t *testing.T) {
	reader := &fakeReachabilityReader{records: map[string]*models.ExecutorReachability{
		"executor-1": {ExecutorID: "executor-1", State: models.ExecutorReachabilityStateReachable, Host: "build.example"},
	}}
	mgr, memBus := newWarningTestManager(t, reader, true, warningWindowSeconds)
	received := subscribeLaunchWarnings(t, memBus, "session-1")

	req := &LaunchRequest{ExecutorType: string(models.ExecutorTypeSSH), SessionID: "session-1"}
	mgr.maybePublishSSHLaunchWarning(context.Background(), req, map[string]interface{}{"executor_id": "executor-1"}, req.SessionID)

	select {
	case ev := <-received:
		t.Fatalf("expected no warning for a reachable record, got %+v", ev)
	default:
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.3
func TestMaybePublishSSHLaunchWarning_NoRecordDoesNotWarn(t *testing.T) {
	reader := &fakeReachabilityReader{records: map[string]*models.ExecutorReachability{}}
	mgr, memBus := newWarningTestManager(t, reader, true, warningWindowSeconds)
	received := subscribeLaunchWarnings(t, memBus, "session-1")

	req := &LaunchRequest{ExecutorType: string(models.ExecutorTypeSSH), SessionID: "session-1"}
	mgr.maybePublishSSHLaunchWarning(context.Background(), req, map[string]interface{}{"executor_id": "executor-1"}, req.SessionID)

	select {
	case ev := <-received:
		t.Fatalf("expected no warning when no record exists yet, got %+v", ev)
	default:
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.3
func TestMaybePublishSSHLaunchWarning_ReadFailureDoesNotWarnOrPanic(t *testing.T) {
	reader := &fakeReachabilityReader{err: errors.New("boom")}
	mgr, memBus := newWarningTestManager(t, reader, true, warningWindowSeconds)
	received := subscribeLaunchWarnings(t, memBus, "session-1")

	req := &LaunchRequest{ExecutorType: string(models.ExecutorTypeSSH), SessionID: "session-1"}
	mgr.maybePublishSSHLaunchWarning(context.Background(), req, map[string]interface{}{"executor_id": "executor-1"}, req.SessionID)

	select {
	case ev := <-received:
		t.Fatalf("expected no warning on a read failure, got %+v", ev)
	default:
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.1
//
// Non-ssh launches must never trigger a reachability read at all — the
// failOnReadReachabilityReader fails the test the instant it is touched,
// which is what proves the short-circuit rather than merely observing zero
// warnings (a bug that read the record but decided not to warn would still
// pass a call-count-only assertion).
func TestMaybePublishSSHLaunchWarning_NonSSHExecutorNeverReads(t *testing.T) {
	mgr, _ := newWarningTestManager(t, failOnReadReachabilityReader{t: t}, true, warningWindowSeconds)

	req := &LaunchRequest{ExecutorType: "local_docker", SessionID: "session-1"}
	mgr.maybePublishSSHLaunchWarning(context.Background(), req, map[string]interface{}{"executor_id": "executor-1"}, req.SessionID)
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.1
//
// launchBuildExecutorRequest is the single production call site for every
// executor type (verified: it has exactly one caller, launchInternal, which
// every launch entry point — WS-initiated, resume, and workflow/dependency
// launches via orchestrator/executor's LaunchAgent — funnels through). This
// exercises that shared call site directly with a non-ssh backend and proves
// the reachability reader is never touched and CreateInstance still runs.
func TestLaunchBuildExecutorRequestNonSSHExecutorNeverReadsReachability(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	backend := &createInstanceExecutor{MockExecutor: MockExecutor{name: executor.NameDocker}}
	registry.Register(backend)

	memBusLog, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	memBus := bus.NewMemoryEventBus(memBusLog)
	mgr := &Manager{
		executorRegistry:       registry,
		executorFallbackPolicy: ExecutorFallbackDeny,
		logger:                 log,
		eventPublisher:         NewEventPublisher(memBus, memBusLog),
	}
	mgr.SetSSHReachabilityWarningPolicy(failOnReadReachabilityReader{t: t}, true, warningWindowSeconds)

	req := &LaunchRequest{
		ExecutorType:         "local_docker",
		EnvironmentFinalized: true,
		SessionID:            "session-1",
		Metadata:             map[string]interface{}{"executor_id": "executor-1"},
	}

	_, _, _, err := mgr.launchBuildExecutorRequest(
		context.Background(), "instance-1", req, &testAgent{id: "agent"}, &AgentProfileInfo{}, "", "", "", nil,
	)

	if err != nil {
		t.Fatalf("launchBuildExecutorRequest: %v", err)
	}
	if backend.lastRequest == nil {
		t.Fatal("expected CreateInstance to run")
	}
}
