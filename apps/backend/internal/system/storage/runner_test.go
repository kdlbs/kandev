package storage

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
)

func TestManualRunnerIgnoresQuietPeriodWithoutActiveTaskWork(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	coordinator := activity.NewCoordinator(activity.Options{Now: func() time.Time { return now }})
	taskLease, err := coordinator.AcquireTask(context.Background(), activity.KindTestCommand)
	if err != nil {
		t.Fatalf("AcquireTask: %v", err)
	}
	taskLease.Release()
	provider := &recordingCleanupProvider{}
	runner := NewRunner(RunnerConfig{
		Activity: coordinator, Store: &recordingRunStore{}, Providers: []CleanupProvider{provider},
		NewID: func() string { return "manual-recent-activity" },
	})
	settings := DefaultSettings()
	settings.IdleForMinutes = 10

	run, err := runner.Run(context.Background(), RunTriggerManual, settings)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.State != RunStateSucceeded || provider.calls != 1 {
		t.Fatalf("manual run state=%q provider calls=%d, want succeeded/1", run.State, provider.calls)
	}
}

func TestManualRunnerRejectsCurrentTaskActivity(t *testing.T) {
	coordinator := activity.NewCoordinator(activity.Options{})
	taskLease, err := coordinator.AcquireTask(context.Background(), activity.KindTestCommand)
	if err != nil {
		t.Fatalf("AcquireTask: %v", err)
	}
	defer taskLease.Release()
	provider := &recordingCleanupProvider{}
	runner := NewRunner(RunnerConfig{
		Activity: coordinator, Store: &recordingRunStore{}, Providers: []CleanupProvider{provider},
		NewID: func() string { return "manual-active-task" },
	})

	run, err := runner.Run(context.Background(), RunTriggerManual, DefaultSettings())
	var busyErr *BusyError
	if !errors.As(err, &busyErr) {
		t.Fatalf("Run error = %v, want BusyError", err)
	}
	if run.State != RunStateSkippedBusy || provider.calls != 0 {
		t.Fatalf("manual run state=%q provider calls=%d, want skipped_busy/0", run.State, provider.calls)
	}
}

func TestScheduledRunnerEnforcesQuietPeriodWithoutActiveTaskWork(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	coordinator := activity.NewCoordinator(activity.Options{Now: func() time.Time { return now }})
	provider := &recordingCleanupProvider{}
	runner := NewRunner(RunnerConfig{
		Activity: coordinator, Store: &recordingRunStore{}, Providers: []CleanupProvider{provider},
		NewID: func() string { return "scheduled-quiet-period" },
	})
	settings := DefaultSettings()
	settings.IdleForMinutes = 10

	run, err := runner.Run(context.Background(), RunTriggerScheduled, settings)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.State != RunStateSkippedBusy || provider.calls != 0 {
		t.Fatalf("scheduled run state=%q provider calls=%d, want skipped_busy/0", run.State, provider.calls)
	}
}

func TestScheduledRunnerPersistsSkippedBusy(t *testing.T) {
	coordinator := activity.NewCoordinator(activity.Options{})
	taskLease, err := coordinator.AcquireTask(context.Background(), activity.KindTestCommand)
	if err != nil {
		t.Fatalf("AcquireTask: %v", err)
	}
	defer taskLease.Release()
	store := &recordingRunStore{}
	runner := NewRunner(RunnerConfig{
		Activity: coordinator, Store: store,
		NewID: func() string { return "scheduled-busy" },
	})
	settings := DefaultSettings()
	settings.Enabled = true
	settings.IdleForMinutes = 10

	run, err := runner.Run(context.Background(), RunTriggerScheduled, settings)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.State != RunStateSkippedBusy {
		t.Fatalf("run state = %q, want skipped_busy", run.State)
	}
	if len(store.created) != 1 || len(store.transitions) != 1 || store.transitions[0] != RunStateSkippedBusy {
		t.Fatalf("persistence created=%d transitions=%v", len(store.created), store.transitions)
	}
}

func TestTaskAdmissionCancelsProviderBeforeProceeding(t *testing.T) {
	coordinator := activity.NewCoordinator(activity.Options{})
	provider := &cancellableProvider{started: make(chan struct{}), stopped: make(chan struct{})}
	store := &recordingRunStore{}
	runner := NewRunner(RunnerConfig{
		Activity: coordinator, Store: store, Providers: []CleanupProvider{provider},
		NewID: func() string { return "cancelled-run" },
	})
	settings := DefaultSettings()
	settings.IdleForMinutes = 0

	runDone := make(chan MaintenanceRun, 1)
	go func() {
		run, _ := runner.Run(context.Background(), RunTriggerScheduled, settings)
		runDone <- run
	}()
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("provider did not start")
	}

	taskAdmitted := make(chan *activity.TaskLease, 1)
	go func() {
		lease, err := coordinator.AcquireTask(context.Background(), activity.KindExecutionStarting)
		if err == nil {
			taskAdmitted <- lease
		}
	}()
	select {
	case <-provider.stopped:
	case <-time.After(time.Second):
		t.Fatal("provider did not stop after task arrival")
	}
	select {
	case lease := <-taskAdmitted:
		lease.Release()
	case <-time.After(time.Second):
		t.Fatal("task was not admitted after provider drained")
	}
	select {
	case run := <-runDone:
		if run.State != RunStateCancelled {
			t.Fatalf("run state = %q, want cancelled", run.State)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled run did not finish")
	}
}

func TestRunnerPersistsCancelledRunAfterCallerCancellation(t *testing.T) {
	coordinator := activity.NewCoordinator(activity.Options{})
	provider := &cancellableProvider{started: make(chan struct{}), stopped: make(chan struct{})}
	store := &cancellationAwareRunStore{}
	runner := NewRunner(RunnerConfig{
		Activity: coordinator, Store: store, Providers: []CleanupProvider{provider},
		NewID: func() string { return "caller-cancelled-run" },
	})
	settings := DefaultSettings()
	settings.IdleForMinutes = 0

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan runnerResult, 1)
	go func() {
		run, err := runner.Run(ctx, RunTriggerScheduled, settings)
		runDone <- runnerResult{run: run, err: err}
	}()
	<-provider.started
	cancel()
	<-provider.stopped

	result := <-runDone
	if !errors.Is(result.err, context.Canceled) {
		t.Fatalf("Run error = %v, want context cancellation", result.err)
	}
	if result.run.State != RunStateCancelled {
		t.Fatalf("run state = %q, want cancelled", result.run.State)
	}
	if terminal := store.terminalState(); terminal != RunStateCancelled {
		t.Fatalf("persisted terminal state = %q, want cancelled", terminal)
	}
}

type runnerResult struct {
	run MaintenanceRun
	err error
}

type recordingRunStore struct {
	created     []MaintenanceRun
	transitions []RunState
}

type cancellationAwareRunStore struct {
	mu          sync.Mutex
	transitions []RunState
}

type cancellableProvider struct {
	started chan struct{}
	stopped chan struct{}
}

type recordingCleanupProvider struct {
	calls int
}

func (p *recordingCleanupProvider) Name() string { return "recording" }

func (p *recordingCleanupProvider) Cleanup(context.Context) (map[string]any, error) {
	p.calls++
	return map[string]any{"cleaned": true}, nil
}

func (p *cancellableProvider) Name() string { return "cancellable" }

func (p *cancellableProvider) Cleanup(ctx context.Context) (map[string]any, error) {
	close(p.started)
	<-ctx.Done()
	close(p.stopped)
	return nil, ctx.Err()
}

func (s *recordingRunStore) CreateRun(_ context.Context, run *MaintenanceRun) error {
	s.created = append(s.created, *run)
	return nil
}

func (s *recordingRunStore) TransitionRun(
	_ context.Context,
	id string,
	state RunState,
	result json.RawMessage,
	message string,
) (MaintenanceRun, error) {
	s.transitions = append(s.transitions, state)
	now := time.Now().UTC()
	return MaintenanceRun{ID: id, State: state, Result: result, Message: message, CompletedAt: &now}, nil
}

func (s *cancellationAwareRunStore) CreateRun(ctx context.Context, _ *MaintenanceRun) error {
	return ctx.Err()
}

func (s *cancellationAwareRunStore) TransitionRun(
	ctx context.Context,
	id string,
	state RunState,
	result json.RawMessage,
	message string,
) (MaintenanceRun, error) {
	if err := ctx.Err(); err != nil {
		return MaintenanceRun{}, err
	}
	s.mu.Lock()
	s.transitions = append(s.transitions, state)
	s.mu.Unlock()
	completedAt := time.Now().UTC()
	if state == RunStateRunning {
		completedAt = time.Time{}
		return MaintenanceRun{ID: id, State: state, Result: result, Message: message}, nil
	}
	return MaintenanceRun{
		ID: id, State: state, Result: result, Message: message, CompletedAt: &completedAt,
	}, nil
}

func (s *cancellationAwareRunStore) terminalState() RunState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.transitions) == 0 {
		return ""
	}
	return s.transitions[len(s.transitions)-1]
}
