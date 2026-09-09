package retention

import (
	"context"
	"sync"
	"time"
)

// firstSweepDelay is the fixed delay before the first sweep after Start,
// and after retention transitions from disabled to enabled
// (AC-OFFICE-RUN-HISTORY-RETENTION-002.10, -002.13). Arming at the full
// interval instead — as the census timer does — would mean an install
// restarted more often than the interval never sweeps at all.
const firstSweepDelay = 5 * time.Minute

// SchedulerOptions configures Scheduler construction. After is injectable
// for deterministic tests; production leaves it nil and gets time.After.
type SchedulerOptions struct {
	After func(time.Duration) <-chan time.Time
}

// Scheduler owns two independent timers on one goroutine, modelled on
// internal/system/storage.Scheduler:
//
//   - The census timer always runs, on the configured sweep interval,
//     whether or not retention is enabled (AC-OFFICE-RUN-HISTORY-RETENTION-003.11):
//     a disabled install is exactly the one whose tables grow unattended,
//     and the only one where an unrecognized status would otherwise never
//     be noticed.
//   - The sweep timer only runs while enabled, armed at firstSweepDelay
//     after Start or after a disabled-to-enabled transition, and at the
//     full interval thereafter (AC-OFFICE-RUN-HISTORY-RETENTION-002.10,
//     -002.13).
//
// A settings change wakes the loop and re-arms both timers from the
// moment of the change (fixed-delay, not fixed-rate): the sweep timer at
// firstSweepDelay only when retention just turned on, otherwise at the
// (possibly new) full interval; the census timer always at the full
// interval.
type Scheduler struct {
	settingsStore *SettingsStore
	sweeper       *Sweeper
	after         func(time.Duration) <-chan time.Time

	lifecycleMu sync.Mutex
	mu          sync.Mutex
	cancel      context.CancelFunc
	wake        chan struct{}
	latest      Settings
	wg          sync.WaitGroup
}

// NewScheduler wires the scheduler to its dependencies.
func NewScheduler(settingsStore *SettingsStore, sweeper *Sweeper, options SchedulerOptions) *Scheduler {
	after := options.After
	if after == nil {
		after = time.After
	}
	return &Scheduler{settingsStore: settingsStore, sweeper: sweeper, after: after}
}

// Start begins the scheduler loop. A no-op when already running.
func (s *Scheduler) Start(ctx context.Context) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	running := s.cancel != nil
	s.mu.Unlock()
	if running {
		return nil
	}

	settings, err := s.settingsStore.GetSettings(ctx)
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

// ApplySettings notifies a running scheduler that settings changed,
// re-arming both timers from this moment. A no-op when not running.
func (s *Scheduler) ApplySettings(settings Settings) {
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

// Stop cancels the scheduler loop and joins it. A no-op when not running.
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

func (s *Scheduler) run(ctx context.Context, settings Settings, wake <-chan struct{}) {
	defer s.wg.Done()

	// The first census evaluation runs here, on the scheduler goroutine,
	// not blocking Start's caller (AC-OFFICE-RUN-HISTORY-RETENTION-003.11's
	// "runs at Start, not at the first sweep").
	s.sweeper.RunCensus(ctx)
	census := s.after(sweepInterval(settings))

	var sweep <-chan time.Time
	if settings.Enabled {
		sweep = s.after(firstSweepDelay)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
			wasEnabled := settings.Enabled
			settings = s.latestSettings()

			sweep = nil
			if settings.Enabled {
				if wasEnabled {
					sweep = s.after(sweepInterval(settings))
				} else {
					sweep = s.after(firstSweepDelay)
				}
			}
			census = s.after(sweepInterval(settings))

		case <-census:
			s.sweeper.RunCensus(ctx)
			census = s.after(sweepInterval(settings))

		case <-sweep:
			s.sweeper.RunSweep(ctx)
			sweep = s.after(sweepInterval(settings))
		}
	}
}

func (s *Scheduler) latestSettings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest
}

func sweepInterval(settings Settings) time.Duration {
	return time.Duration(settings.SweepIntervalHours) * time.Hour
}
