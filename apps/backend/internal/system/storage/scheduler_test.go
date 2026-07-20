package storage

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestSchedulerFirstEnabledRunWaitsFullInterval(t *testing.T) {
	ticks := make(chan time.Time, 1)
	runner := &recordingScheduledRunner{calls: make(chan struct{}, 1)}
	settings := DefaultSettings()
	settings.Enabled = true
	scheduler := NewScheduler(staticSchedulerSettings{settings: settings}, runner, SchedulerOptions{
		After: func(time.Duration) <-chan time.Time { return ticks },
	})
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer scheduler.Stop()
	select {
	case <-runner.calls:
		t.Fatal("scheduler ran before the first full interval")
	default:
	}
	ticks <- time.Now()
	select {
	case <-runner.calls:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not run after the interval")
	}
}

func TestDisabledSchedulerStartsNoDestructiveLoop(t *testing.T) {
	runner := &recordingScheduledRunner{calls: make(chan struct{}, 1)}
	scheduler := NewScheduler(staticSchedulerSettings{settings: DefaultSettings()}, runner, SchedulerOptions{
		After: func(time.Duration) <-chan time.Time {
			t.Fatal("disabled scheduler created a destructive timer")
			return nil
		},
	})
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	scheduler.Stop()
	select {
	case <-runner.calls:
		t.Fatal("disabled scheduler ran maintenance")
	default:
	}
}

func TestSchedulerConcurrentApplySettingsAndStopDoNotBlock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		scheduler := NewScheduler(
			staticSchedulerSettings{settings: DefaultSettings()},
			&recordingScheduledRunner{calls: make(chan struct{}, 1)},
			SchedulerOptions{},
		)
		workerCtx, cancel := context.WithCancel(context.Background())
		wake := make(chan struct{})
		scheduler.mu.Lock()
		scheduler.cancel = cancel
		scheduler.wake = wake
		scheduler.wg.Add(1)
		scheduler.mu.Unlock()
		go func() {
			defer scheduler.wg.Done()
			<-workerCtx.Done()
		}()

		applyDone := make(chan struct{})
		go func() {
			scheduler.ApplySettings(DefaultSettings())
			close(applyDone)
		}()
		synctest.Wait()

		scheduler.Stop()
		synctest.Wait()
		select {
		case <-applyDone:
			return
		default:
			// Drain the old implementation's blocking send before failing so the
			// regression test itself does not leak a goroutine.
			<-wake
			synctest.Wait()
			t.Fatal("ApplySettings remained blocked after concurrent Stop")
		}
	})
}

type staticSchedulerSettings struct {
	settings StorageMaintenanceSettings
}

func (s staticSchedulerSettings) GetSettings(context.Context) (StorageMaintenanceSettings, error) {
	return s.settings, nil
}

type recordingScheduledRunner struct {
	mu    sync.Mutex
	calls chan struct{}
}

func (r *recordingScheduledRunner) Run(context.Context, RunTrigger, StorageMaintenanceSettings) (MaintenanceRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls <- struct{}{}
	return MaintenanceRun{}, nil
}
