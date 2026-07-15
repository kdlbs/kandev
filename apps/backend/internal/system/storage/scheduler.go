package storage

import (
	"context"
	"sync"
	"time"
)

type SchedulerSettings interface {
	GetSettings(ctx context.Context) (StorageMaintenanceSettings, error)
}

type ScheduledRunner interface {
	Run(ctx context.Context, trigger RunTrigger, settings StorageMaintenanceSettings) (MaintenanceRun, error)
}

type SchedulerOptions struct {
	After func(time.Duration) <-chan time.Time
}

type Scheduler struct {
	settings SchedulerSettings
	runner   ScheduledRunner
	after    func(time.Duration) <-chan time.Time

	lifecycleMu sync.Mutex
	mu          sync.Mutex
	cancel      context.CancelFunc
	wake        chan struct{}
	latest      StorageMaintenanceSettings
	wg          sync.WaitGroup
}

func NewScheduler(settings SchedulerSettings, runner ScheduledRunner, options SchedulerOptions) *Scheduler {
	after := options.After
	if after == nil {
		after = time.After
	}
	return &Scheduler{settings: settings, runner: runner, after: after}
}

func (s *Scheduler) Start(ctx context.Context) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	running := s.cancel != nil
	s.mu.Unlock()
	if running {
		return nil
	}
	settings, err := s.settings.GetSettings(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	workerCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.wake = make(chan struct{}, 1)
	s.latest = settings
	s.wg.Add(1)
	wake := s.wake
	s.mu.Unlock()
	go s.run(workerCtx, settings, wake)
	return nil
}

func (s *Scheduler) ApplySettings(settings StorageMaintenanceSettings) {
	s.mu.Lock()
	if s.cancel == nil || s.wake == nil {
		s.mu.Unlock()
		return
	}
	s.latest = settings
	wake := s.wake
	s.mu.Unlock()
	select {
	case wake <- struct{}{}:
	default:
	}
}

func (s *Scheduler) Stop() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.wake = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		s.wg.Wait()
	}
}

func (s *Scheduler) run(
	ctx context.Context,
	settings StorageMaintenanceSettings,
	wake <-chan struct{},
) {
	defer s.wg.Done()
	var interval <-chan time.Time
	if settings.Enabled {
		interval = s.after(settingsInterval(settings))
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
			settings = s.latestSettings()
			interval = nil
			if settings.Enabled {
				interval = s.after(settingsInterval(settings))
			}
		case <-interval:
			_, _ = s.runner.Run(ctx, RunTriggerScheduled, settings)
			interval = s.after(settingsInterval(settings))
		}
	}
}

func (s *Scheduler) latestSettings() StorageMaintenanceSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest
}

func settingsInterval(settings StorageMaintenanceSettings) time.Duration {
	return time.Duration(settings.CheckIntervalHours) * time.Hour
}
