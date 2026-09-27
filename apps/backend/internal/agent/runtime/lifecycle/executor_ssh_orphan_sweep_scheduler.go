package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// sshOrphanSweepDefaultIntervalSeconds is the slow per-executor backstop
// cadence (AC-EXECUTORS-SSH-EXECUTOR-001.13): the reachability-change event
// is the primary trigger, this only catches an executor that stays
// reachable without ever toggling again.
const sshOrphanSweepDefaultIntervalSeconds = 30 * 60

// sshOrphanSweepDialTimeout bounds the SSH dial the scheduler opens for each
// sweep, mirroring ProbeSSHHost's own bound on the same connection.
const sshOrphanSweepDialTimeout = 30 * time.Second

// sshOrphanSweepExecutorStore is the narrow executor-read surface the
// scheduler needs. GetExecutor resolves the single id an
// events.ExecutorReachabilityChanged event names; the other two mirror
// reachability.Poller's own eligibility listing and state read, so the
// interval backstop sweeps exactly the executors currently known reachable.
type sshOrphanSweepExecutorStore interface {
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
	ListSSHExecutorsForReachability(ctx context.Context) ([]*models.Executor, error)
	GetExecutorReachability(ctx context.Context, executorID string) (*models.ExecutorReachability, error)
}

// sshOrphanReachabilityEvent is the subset of reachability.RecordDTO this
// package needs. Decoded independently rather than importing the
// reachability package, which would create an import cycle back through
// internal/agent/runtime.
type sshOrphanReachabilityEvent struct {
	ExecutorID string `json:"executor_id"`
	State      string `json:"state"`
}

// decodeSSHOrphanReachabilityEvent accepts both the in-process MemoryEventBus
// shape (the exact Go value handed to Publish) and the wire shape a NATS
// subscriber sees after JSON round-tripping (event.Data decoded generically),
// matching the normalizeTaskPR precedent in internal/automation.
func decodeSSHOrphanReachabilityEvent(data interface{}) (sshOrphanReachabilityEvent, bool) {
	if v, ok := data.(sshOrphanReachabilityEvent); ok {
		return v, true
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return sshOrphanReachabilityEvent{}, false
	}
	var decoded sshOrphanReachabilityEvent
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return sshOrphanReachabilityEvent{}, false
	}
	return decoded, true
}

// OrphanSweepScheduler runs sweepSSHExecutorOrphans against SSH executors:
// once off-cycle whenever events.ExecutorReachabilityChanged reports a
// transition into the reachable state, and on a slow interval as a backstop
// for every executor currently known reachable. A trigger for an executor
// already mid-sweep is dropped rather than queued — see trySweep.
//
// Start and Stop are idempotent. Stop cancels every in-flight sweep and
// waits for all of them (plus the loop) to drain before returning.
type OrphanSweepScheduler struct {
	executors       sshOrphanSweepExecutorStore
	tasks           sshOrphanSweepStore
	log             *logger.Logger
	intervalSeconds int

	mu           sync.Mutex
	started      bool
	stopping     bool
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	subscription bus.Subscription

	sweepingMu sync.Mutex
	sweeping   map[string]bool
}

// NewOrphanSweepScheduler builds a scheduler. An intervalSeconds of 0 or
// less falls back to sshOrphanSweepDefaultIntervalSeconds: unlike
// reachability.Poller, there is no user-facing "disabled" sentinel here —
// this sweep only ever stops orphaned processes, so there is no reason to
// turn the backstop off.
func NewOrphanSweepScheduler(executors sshOrphanSweepExecutorStore, tasks sshOrphanSweepStore, intervalSeconds int, log *logger.Logger) *OrphanSweepScheduler {
	if intervalSeconds <= 0 {
		intervalSeconds = sshOrphanSweepDefaultIntervalSeconds
	}
	return &OrphanSweepScheduler{
		executors:       executors,
		tasks:           tasks,
		log:             log,
		intervalSeconds: intervalSeconds,
		sweeping:        map[string]bool{},
	}
}

// Start subscribes to reachability-change events and launches the interval
// backstop loop, which also runs one pass immediately. Calling Start more
// than once without an intervening Stop is a no-op.
func (s *OrphanSweepScheduler) Start(ctx context.Context, eventBus bus.EventBus) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.started = true
	s.stopping = false
	s.ctx = runCtx
	s.cancel = cancel
	s.wg.Add(1)
	s.mu.Unlock()

	if eventBus != nil {
		subscription, err := eventBus.Subscribe(events.ExecutorReachabilityChanged, s.handleReachabilityChanged)
		if err != nil {
			if s.log != nil {
				s.log.Warn("ssh orphan sweep: reachability subscription failed", zap.Error(err))
			}
		} else {
			s.mu.Lock()
			s.subscription = subscription
			s.mu.Unlock()
		}
	}

	go s.loop(runCtx)
}

// Stop cancels the loop and every in-flight sweep, then waits for all of
// them to drain. Calling Stop before Start, or twice in a row, is a no-op.
func (s *OrphanSweepScheduler) Stop() {
	s.mu.Lock()
	if !s.started || s.stopping {
		s.mu.Unlock()
		return
	}
	s.stopping = true
	cancel := s.cancel
	subscription := s.subscription
	s.mu.Unlock()

	if subscription != nil {
		_ = subscription.Unsubscribe()
	}
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()

	s.mu.Lock()
	s.started = false
	s.stopping = false
	s.ctx = nil
	s.cancel = nil
	s.subscription = nil
	s.mu.Unlock()
}

// acquire registers one unit of in-flight work on wg, gated by mu so it can
// never race a concurrent Stop's wg.Wait() — mirrors reachability.Poller's
// own acquire().
func (s *OrphanSweepScheduler) acquire() (context.Context, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.stopping {
		return nil, false
	}
	s.wg.Add(1)
	return s.ctx, true
}

func (s *OrphanSweepScheduler) loop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Duration(s.intervalSeconds) * time.Second)
	defer ticker.Stop()

	s.sweepAllReachable(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepAllReachable(ctx)
		}
	}
}

// sweepAllReachable is the interval backstop: every eligible SSH executor
// currently recorded reachable gets a coalesced sweep. An executor that has
// gone unreachable since its last event is skipped, so a host that is down
// for an extended period is not repeatedly dialed.
func (s *OrphanSweepScheduler) sweepAllReachable(ctx context.Context) {
	executors, err := s.executors.ListSSHExecutorsForReachability(ctx)
	if err != nil {
		if s.log != nil {
			s.log.Warn("ssh orphan sweep: list executors failed", zap.Error(err))
		}
		return
	}
	for _, executor := range executors {
		if ctx.Err() != nil {
			return
		}
		record, err := s.executors.GetExecutorReachability(ctx, executor.ID)
		if err != nil || record == nil || record.State != models.ExecutorReachabilityStateReachable {
			continue
		}
		s.trySweep(executor)
	}
}

func (s *OrphanSweepScheduler) handleReachabilityChanged(ctx context.Context, event *bus.Event) error {
	if event == nil {
		return nil
	}
	evt, ok := decodeSSHOrphanReachabilityEvent(event.Data)
	if !ok || evt.ExecutorID == "" {
		return nil
	}
	if models.ExecutorReachabilityState(evt.State) != models.ExecutorReachabilityStateReachable {
		return nil
	}
	executor, err := s.executors.GetExecutor(ctx, evt.ExecutorID)
	if err != nil || executor == nil {
		return nil
	}
	s.trySweep(executor)
	return nil
}

// trySweep starts one sweep of executor unless one is already running for
// it, in which case the trigger is dropped — "one sweep per executor at a
// time; a trigger that arrives during a sweep is coalesced."
func (s *OrphanSweepScheduler) trySweep(executor *models.Executor) {
	if executor == nil || executor.Type != models.ExecutorTypeSSH {
		return
	}

	s.sweepingMu.Lock()
	if s.sweeping[executor.ID] {
		s.sweepingMu.Unlock()
		return
	}
	s.sweeping[executor.ID] = true
	s.sweepingMu.Unlock()

	ctx, ok := s.acquire()
	if !ok {
		s.sweepingMu.Lock()
		delete(s.sweeping, executor.ID)
		s.sweepingMu.Unlock()
		return
	}

	go func() {
		defer s.wg.Done()
		defer func() {
			s.sweepingMu.Lock()
			delete(s.sweeping, executor.ID)
			s.sweepingMu.Unlock()
		}()
		s.runSweep(ctx, executor)
	}()
}

func (s *OrphanSweepScheduler) runSweep(ctx context.Context, executor *models.Executor) {
	target, err := SSHTargetFromExecutorConfig(executor.Config)
	if err != nil {
		s.warn(executor.ID, "resolve ssh target", err)
		return
	}

	dialCtx, cancel := context.WithTimeout(ctx, sshOrphanSweepDialTimeout)
	defer cancel()
	client, err := dialSSH(dialCtx, target)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		s.warn(executor.ID, "dial failed", err)
		return
	}
	defer func() { _ = client.Close() }()

	if _, err := sweepSSHExecutorOrphans(ctx, client, s.tasks, executor.ID, executor.Config, s.log); err != nil {
		s.warn(executor.ID, "sweep failed", err)
	}
}

func (s *OrphanSweepScheduler) warn(executorID, message string, err error) {
	if s.log == nil {
		return
	}
	s.log.Warn("ssh orphan sweep: "+message, zap.String("executor_id", executorID), zap.Error(err))
}
