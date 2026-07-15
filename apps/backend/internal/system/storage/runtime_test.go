package storage

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRuntimeStopSerializesWithApplySettings(t *testing.T) {
	settings := DefaultSettings()
	scheduler := NewScheduler(staticRuntimeSettings{settings: settings}, nil, SchedulerOptions{
		After: func(time.Duration) <-chan time.Time { return make(chan time.Time) },
	})
	t.Cleanup(scheduler.Stop)
	worker := &blockingStopWorker{stopStarted: make(chan struct{}), release: make(chan struct{})}
	runtime := NewRuntime(RuntimeConfig{
		Scheduler: scheduler, Settings: staticRuntimeSettings{settings: settings}, Worker: worker,
	})
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stopped := make(chan struct{})
	go func() {
		runtime.Stop()
		close(stopped)
	}()
	<-worker.stopStarted

	enabled := settings
	enabled.Enabled = true
	applied := make(chan struct{})
	go func() {
		runtime.ApplySettings(enabled)
		close(applied)
	}()
	select {
	case <-applied:
		t.Fatal("ApplySettings completed while Stop was still in progress")
	default:
	}
	close(worker.release)
	<-stopped
	<-applied

	scheduler.mu.Lock()
	running := scheduler.cancel != nil
	scheduler.mu.Unlock()
	if running {
		t.Fatal("scheduler restarted after runtime stop")
	}
}

func TestRuntimeHealthIssuesAreCurrentAndUnique(t *testing.T) {
	schedulerSettings := &mutableRuntimeSettings{
		settings: DefaultSettings(), err: errors.New("scheduler unavailable"),
	}
	scheduler := NewScheduler(schedulerSettings, nil, SchedulerOptions{})
	t.Cleanup(scheduler.Stop)
	runtime := NewRuntime(RuntimeConfig{
		Scheduler: scheduler, Settings: staticRuntimeSettings{settings: DefaultSettings()},
	})
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	enabled := DefaultSettings()
	enabled.Enabled = true
	runtime.ApplySettings(enabled)
	runtime.ApplySettings(enabled)
	if issues := runtime.Check(context.Background()); len(issues) != 1 || issues[0].ID != "storage_scheduler" {
		t.Fatalf("issues after repeated failure = %#v, want one scheduler issue", issues)
	}
	schedulerSettings.err = nil
	runtime.ApplySettings(enabled)
	if issues := runtime.Check(context.Background()); len(issues) != 0 {
		t.Fatalf("issues after scheduler recovery = %#v, want none", issues)
	}
}

func TestRuntimeStartClearsRecoveredIssues(t *testing.T) {
	settings := &mutableRuntimeSettings{err: errors.New("settings unavailable")}
	worker := &toggleCleanupWorker{err: errors.New("worker unavailable")}
	reconciler := &toggleReconciler{err: errors.New("reconciler unavailable")}
	runtime := NewRuntime(RuntimeConfig{Settings: settings, Worker: worker, Reconciler: reconciler})
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if issues := runtime.Check(context.Background()); len(issues) != 3 {
		t.Fatalf("issues after failures = %#v, want three", issues)
	}
	runtime.Stop()
	settings.err = nil
	worker.err = nil
	reconciler.err = nil
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if issues := runtime.Check(context.Background()); len(issues) != 0 {
		t.Fatalf("issues after recovery = %#v, want none", issues)
	}
}

func TestRuntimeStartTracksAndClearsSchedulerIssue(t *testing.T) {
	enabled := DefaultSettings()
	enabled.Enabled = true
	schedulerSettings := &mutableRuntimeSettings{settings: enabled, err: errors.New("scheduler unavailable")}
	scheduler := NewScheduler(schedulerSettings, nil, SchedulerOptions{
		After: func(time.Duration) <-chan time.Time { return make(chan time.Time) },
	})
	t.Cleanup(scheduler.Stop)
	runtime := NewRuntime(RuntimeConfig{
		Scheduler: scheduler, Settings: staticRuntimeSettings{settings: enabled},
	})
	if err := runtime.Start(context.Background()); err == nil {
		t.Fatal("first Start returned nil error")
	}
	if issues := runtime.Check(context.Background()); len(issues) != 1 || issues[0].ID != "storage_scheduler" {
		t.Fatalf("issues after scheduler failure = %#v, want scheduler issue", issues)
	}
	runtime.Stop()
	schedulerSettings.err = nil
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if issues := runtime.Check(context.Background()); len(issues) != 0 {
		t.Fatalf("issues after scheduler recovery = %#v, want none", issues)
	}
}

type mutableRuntimeSettings struct {
	settings StorageMaintenanceSettings
	err      error
}

type toggleCleanupWorker struct{ err error }

func (w *toggleCleanupWorker) StartTaskResourceCleanupWorker(context.Context) error { return w.err }
func (*toggleCleanupWorker) StopTaskResourceCleanupWorker()                         {}

type toggleReconciler struct{ err error }

func (r *toggleReconciler) Reconcile(context.Context) error { return r.err }

func (s *mutableRuntimeSettings) GetSettings(context.Context) (StorageMaintenanceSettings, error) {
	return s.settings, s.err
}

type staticRuntimeSettings struct {
	settings StorageMaintenanceSettings
}

func (s staticRuntimeSettings) GetSettings(context.Context) (StorageMaintenanceSettings, error) {
	return s.settings, nil
}

type blockingStopWorker struct {
	stopStarted chan struct{}
	release     chan struct{}
}

func (*blockingStopWorker) StartTaskResourceCleanupWorker(context.Context) error { return nil }

func (w *blockingStopWorker) StopTaskResourceCleanupWorker() {
	close(w.stopStarted)
	<-w.release
}
