package storage

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
)

func TestScheduledGoCleanupRunsWhileTasksAreActiveAndSkipsOtherProviders(t *testing.T) {
	coordinator := activity.NewCoordinator(activity.Options{})
	task, err := coordinator.AcquireTask(context.Background(), activity.KindTestCommand)
	if err != nil {
		t.Fatalf("AcquireTask: %v", err)
	}
	defer task.Release()

	goCache := &testNamedCleanupProvider{name: "go_cache", result: map[string]any{"removed": int64(12)}}
	other := &testNamedCleanupProvider{name: "workspaces"}
	settings := DefaultSettings()
	settings.Enabled = true
	settings.IdleForMinutes = 10
	settings.GoCache.Enabled = true
	settings.GoCache.AllowCleanupWhileBusy = true
	runner := NewRunner(RunnerConfig{
		Activity: coordinator,
		Store:    &resultRunStore{},
		Providers: []CleanupProvider{
			other,
			goCache,
		},
		NewID: func() string { return "go-cache-busy" },
	})

	run, err := runner.Run(context.Background(), RunTriggerScheduled, settings)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.State != RunStateSucceeded || goCache.calls != 1 || other.calls != 0 {
		t.Fatalf("run=%#v go calls=%d other calls=%d", run, goCache.calls, other.calls)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(run.Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result["go_cache"]) == 0 || len(result["skipped_providers"]) == 0 {
		t.Fatalf("run result = %s, want completed Go data and skipped providers", run.Result)
	}
}

func TestBusyGoCleanupDoesNotRunWhenPolicyOrPrerequisitesAreOff(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*StorageMaintenanceSettings)
	}{
		{
			name: "policy disabled",
			configure: func(settings *StorageMaintenanceSettings) {
				settings.Enabled = true
				settings.GoCache.Enabled = true
			},
		},
		{
			name: "scheduled maintenance disabled",
			configure: func(settings *StorageMaintenanceSettings) {
				settings.GoCache.Enabled = true
				settings.GoCache.AllowCleanupWhileBusy = true
			},
		},
		{
			name: "Go cache management disabled",
			configure: func(settings *StorageMaintenanceSettings) {
				settings.Enabled = true
				settings.GoCache.AllowCleanupWhileBusy = true
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coordinator := activity.NewCoordinator(activity.Options{})
			task, err := coordinator.AcquireTask(context.Background(), activity.KindTestCommand)
			if err != nil {
				t.Fatal(err)
			}
			defer task.Release()
			goCache := &testNamedCleanupProvider{name: "go_cache"}
			settings := DefaultSettings()
			settings.IdleForMinutes = 10
			test.configure(&settings)
			runner := NewRunner(RunnerConfig{
				Activity: coordinator,
				Store:    &recordingRunStore{},
				Providers: []CleanupProvider{
					goCache,
				},
			})

			run, err := runner.Run(context.Background(), RunTriggerScheduled, settings)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if run.State != RunStateSkippedBusy || goCache.calls != 0 {
				t.Fatalf("run=%#v Go calls=%d, want skipped_busy/0", run, goCache.calls)
			}
		})
	}
}

func TestNewTaskAdmissionDoesNotCancelBusyGoCleanup(t *testing.T) {
	coordinator := activity.NewCoordinator(activity.Options{})
	goCache := &barrierCleanupProvider{
		name:    "go_cache",
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	settings := DefaultSettings()
	settings.Enabled = true
	settings.IdleForMinutes = 10
	settings.GoCache.Enabled = true
	settings.GoCache.AllowCleanupWhileBusy = true
	runner := NewRunner(RunnerConfig{
		Activity: coordinator,
		Store:    &recordingRunStore{},
		Providers: []CleanupProvider{
			goCache,
		},
	})
	runDone := make(chan runnerResult, 1)
	go func() {
		run, err := runner.Run(context.Background(), RunTriggerScheduled, settings)
		runDone <- runnerResult{run: run, err: err}
	}()
	select {
	case <-goCache.started:
	case <-time.After(time.Second):
		t.Fatal("Go cleanup did not start")
	}

	taskDone := make(chan *activity.TaskLease, 1)
	go func() {
		lease, err := coordinator.AcquireTask(context.Background(), activity.KindExecutionStarting)
		if err == nil {
			taskDone <- lease
		}
	}()
	var task *activity.TaskLease
	select {
	case task = <-taskDone:
	case <-time.After(time.Second):
		t.Fatal("new task was blocked by Go cleanup")
	}
	defer task.Release()
	close(goCache.release)

	select {
	case result := <-runDone:
		if result.err != nil || result.run.State != RunStateSucceeded {
			t.Fatalf("run=%#v error=%v", result.run, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Go cleanup did not finish")
	}
}

func TestBusyGoResultSurvivesOtherProviderFailure(t *testing.T) {
	coordinator := activity.NewCoordinator(activity.Options{})
	goCache := &testNamedCleanupProvider{name: "go_cache", result: map[string]any{"removed": int64(12)}}
	other := &testNamedCleanupProvider{name: "workspaces", err: errors.New("workspace cleanup failed")}
	settings := DefaultSettings()
	settings.Enabled = true
	settings.IdleForMinutes = 0
	settings.GoCache.Enabled = true
	settings.GoCache.AllowCleanupWhileBusy = true
	runner := NewRunner(RunnerConfig{
		Activity:  coordinator,
		Store:     &resultRunStore{},
		Providers: []CleanupProvider{goCache, other},
	})

	run, err := runner.Run(context.Background(), RunTriggerScheduled, settings)
	if err == nil || run.State != RunStateFailed {
		t.Fatalf("Run = (%#v, %v), want failed result", run, err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(run.Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result["go_cache"]) == 0 || len(result["workspaces"]) == 0 {
		t.Fatalf("run result = %s, want both Go and failed provider results", run.Result)
	}
}

func TestBusyGoCleanupRemainsCancellable(t *testing.T) {
	coordinator := activity.NewCoordinator(activity.Options{})
	goCache := &cancellableNamedProvider{started: make(chan struct{}), stopped: make(chan struct{})}
	settings := DefaultSettings()
	settings.Enabled = true
	settings.GoCache.Enabled = true
	settings.GoCache.AllowCleanupWhileBusy = true
	runner := NewRunner(RunnerConfig{
		Activity: coordinator,
		Store:    &recordingRunStore{},
		Providers: []CleanupProvider{
			goCache,
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan runnerResult, 1)
	go func() {
		run, err := runner.Run(ctx, RunTriggerScheduled, settings)
		runDone <- runnerResult{run: run, err: err}
	}()
	<-goCache.started
	cancel()
	<-goCache.stopped
	result := <-runDone
	if !errors.Is(result.err, context.Canceled) || result.run.State != RunStateCancelled {
		t.Fatalf("run=%#v error=%v, want cancelled run", result.run, result.err)
	}
}

type testNamedCleanupProvider struct {
	name   string
	calls  int
	result map[string]any
	err    error
}

func (p *testNamedCleanupProvider) Name() string { return p.name }
func (p *testNamedCleanupProvider) Cleanup(context.Context) (map[string]any, error) {
	p.calls++
	return p.result, p.err
}

type barrierCleanupProvider struct {
	name    string
	started chan struct{}
	release chan struct{}
}

func (p *barrierCleanupProvider) Name() string { return p.name }
func (p *barrierCleanupProvider) Cleanup(context.Context) (map[string]any, error) {
	close(p.started)
	<-p.release
	return map[string]any{"cleaned": true}, nil
}

type cancellableNamedProvider struct {
	started chan struct{}
	stopped chan struct{}
}

func (*cancellableNamedProvider) Name() string { return "go_cache" }
func (p *cancellableNamedProvider) Cleanup(ctx context.Context) (map[string]any, error) {
	close(p.started)
	<-ctx.Done()
	close(p.stopped)
	return nil, ctx.Err()
}

type resultRunStore struct {
	recordingRunStore
	terminal MaintenanceRun
}

func (s *resultRunStore) TransitionRun(
	ctx context.Context,
	id string,
	state RunState,
	result json.RawMessage,
	message string,
) (MaintenanceRun, error) {
	run, err := s.recordingRunStore.TransitionRun(ctx, id, state, result, message)
	s.terminal = run
	return run, err
}
