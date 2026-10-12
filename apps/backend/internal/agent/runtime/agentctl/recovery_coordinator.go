package client

import (
	"context"
	"expvar"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

const (
	defaultRecoveryMaxStarts  = 3
	defaultRecoveryWindow     = 60 * time.Second
	defaultRecoveryStartWait  = 15 * time.Second
	defaultStableWindow       = 5 * time.Minute
	maxRecoveryRequestIDBytes = 128
	maxRememberedRetryIDs     = 128
)

type RuntimeLossHandler func(epoch uint64, reason string, safeToReplace bool)

type RecoveryAttemptFunc func(context.Context, RuntimeLossHandler) error

// RecoveryClock makes the retry and stability windows deterministic in tests.
type RecoveryClock interface {
	Now() time.Time
	Sleep(context.Context, time.Duration) error
}

type RecoveryCoordinatorOptions struct {
	Clock        RecoveryClock
	Logger       *logger.Logger
	MaxStarts    int
	EpisodeLimit time.Duration
	StartTimeout time.Duration
	StableWindow time.Duration
	Backoffs     []time.Duration
}

type realRecoveryClock struct{}

func (realRecoveryClock) Now() time.Time { return time.Now() }

func (realRecoveryClock) Sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type recoverySignal struct {
	epoch      uint64
	reason     string
	recoveryID string
	safe       bool
	manual     bool
}

type recoveryEpisode struct {
	signal   recoverySignal
	started  time.Time
	deadline time.Time
	ctx      context.Context
	cancel   context.CancelFunc
	losses   chan recoverySignal
}

// RecoveryCoordinator serializes child replacement attempts without stopping
// the backend process. Runtime owner epochs remain the authority for every
// published connection.
type RecoveryCoordinator struct {
	owner   *RuntimeOwner
	attempt RecoveryAttemptFunc
	clock   RecoveryClock
	logger  *logger.Logger
	opts    RecoveryCoordinatorOptions

	mu                sync.Mutex
	parentCtx         context.Context
	parentCancel      context.CancelFunc
	started           bool
	stopping          bool
	active            *recoveryEpisode
	pending           *recoverySignal
	startsSinceStable int
	lastSafeToRetry   bool
	manualRequests    map[string]string
	manualRequestFIFO []string
	wg                sync.WaitGroup
}

var (
	runtimeRecoveryAttemptsTotal = expvar.NewMap("agent_runtime_recovery_attempts_total")
	runtimeRecoveryOutcomesTotal = expvar.NewMap("agent_runtime_recovery_outcomes_total")
	runtimeRecoveryDuration      = expvar.NewMap("agent_runtime_recovery_duration_seconds")
	runtimeRecoveryInProgress    = expvar.NewInt("agent_runtime_recovery_in_progress")
)

func NewRecoveryCoordinator(
	owner *RuntimeOwner,
	attempt RecoveryAttemptFunc,
	opts RecoveryCoordinatorOptions,
) *RecoveryCoordinator {
	if opts.Clock == nil {
		opts.Clock = realRecoveryClock{}
	}
	if opts.MaxStarts <= 0 {
		opts.MaxStarts = defaultRecoveryMaxStarts
	}
	if opts.EpisodeLimit <= 0 {
		opts.EpisodeLimit = defaultRecoveryWindow
	}
	if opts.StartTimeout <= 0 {
		opts.StartTimeout = defaultRecoveryStartWait
	}
	if opts.StableWindow <= 0 {
		opts.StableWindow = defaultStableWindow
	}
	if len(opts.Backoffs) == 0 {
		opts.Backoffs = []time.Duration{time.Second, 2 * time.Second}
	}
	opts.Backoffs = append([]time.Duration(nil), opts.Backoffs...)
	return &RecoveryCoordinator{
		owner: owner, attempt: attempt, clock: opts.Clock, logger: opts.Logger, opts: opts,
		manualRequests: make(map[string]string),
	}
}

// Start enables recovery after the backend has published readiness. A loss
// observed during startup is retained and handled as soon as this is called.
func (coordinator *RecoveryCoordinator) Start(ctx context.Context) {
	if coordinator == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	coordinator.mu.Lock()
	if coordinator.started || coordinator.stopping {
		coordinator.mu.Unlock()
		return
	}
	coordinator.parentCtx, coordinator.parentCancel = context.WithCancel(ctx)
	coordinator.started = true
	if coordinator.pending != nil && coordinator.pending.safe {
		signal := *coordinator.pending
		coordinator.pending = nil
		coordinator.startLocked(signal)
	}
	coordinator.mu.Unlock()
}

// NotifyRuntimeLoss accepts only the currently published unavailable epoch.
// Unverified ownership is surfaced without starting a competing process.
func (coordinator *RecoveryCoordinator) NotifyRuntimeLoss(epoch uint64, reason string, safeToReplace bool) bool {
	if coordinator == nil || coordinator.owner == nil {
		return false
	}
	coordinator.mu.Lock()
	stopping := coordinator.stopping
	coordinator.mu.Unlock()
	if stopping {
		return false
	}
	recoveryID := uuid.NewString()
	if !coordinator.owner.BeginRecovery(epoch, reason, recoveryID) {
		coordinator.mu.Lock()
		if coordinator.active != nil && !safeToReplace {
			snapshot, ok := coordinator.owner.Snapshot()
			if ok && snapshot.Status == AvailabilityStatusRecovering {
				coordinator.queueLossLocked(recoverySignal{
					epoch: snapshot.RuntimeEpoch, reason: AvailabilityReasonOwnershipUnverified,
					recoveryID: snapshot.RecoveryID, safe: false,
				})
				coordinator.mu.Unlock()
				return true
			}
		}
		coordinator.mu.Unlock()
		return false
	}
	signal := recoverySignal{epoch: epoch, reason: reason, recoveryID: recoveryID, safe: safeToReplace}
	coordinator.mu.Lock()
	if coordinator.stopping {
		coordinator.mu.Unlock()
		return true
	}
	coordinator.lastSafeToRetry = safeToReplace
	runtimeRecoveryInProgress.Set(1)
	if !safeToReplace {
		if coordinator.active != nil {
			coordinator.queueLossLocked(signal)
			coordinator.mu.Unlock()
			return true
		}
		coordinator.pending = nil
		coordinator.mu.Unlock()
		coordinator.owner.FinishRecovery(epoch, recoveryID, AvailabilityReasonOwnershipUnverified, false)
		runtimeRecoveryOutcomesTotal.Add("blocked", 1)
		if coordinator.logger != nil {
			coordinator.logger.Warn("agent runtime recovery blocked by ownership evidence",
				zap.String("outcome", "blocked"), zap.String("recovery_id", recoveryID),
				zap.Uint64("runtime_epoch", epoch))
		}
		return true
	}
	if coordinator.active != nil {
		coordinator.queueLossLocked(signal)
		coordinator.mu.Unlock()
		return true
	}
	if coordinator.started {
		coordinator.startLocked(signal)
	} else {
		coordinator.pending = &signal
		runtimeRecoveryInProgress.Set(1)
	}
	coordinator.mu.Unlock()
	return true
}

func (coordinator *RecoveryCoordinator) queueLossLocked(signal recoverySignal) {
	if coordinator.active == nil {
		return
	}
	select {
	case coordinator.active.losses <- signal:
	default:
		select {
		case <-coordinator.active.losses:
		default:
		}
		coordinator.active.losses <- signal
	}
}

func (coordinator *RecoveryCoordinator) startLocked(signal recoverySignal) {
	ctx, cancel := context.WithCancel(coordinator.parentCtx)
	now := coordinator.clock.Now()
	episode := &recoveryEpisode{
		signal:   signal,
		started:  now,
		deadline: now.Add(coordinator.opts.EpisodeLimit),
		ctx:      ctx,
		cancel:   cancel,
		losses:   make(chan recoverySignal, 1),
	}
	coordinator.active = episode
	runtimeRecoveryInProgress.Set(1)
	coordinator.wg.Add(1)
	go coordinator.run(episode)
}

// Retry starts a fresh bounded episode after safe automatic recovery exhausted
// its budget. The caller owns authorization; the observed boot/epoch fence
// rejects stale controls.
func (coordinator *RecoveryCoordinator) Retry(
	ctx context.Context,
	bootID string,
	epoch uint64,
	requestID string,
) (AvailabilitySnapshot, error) {
	return coordinator.RetryAtRevision(ctx, bootID, epoch, 0, requestID)
}

// RetryAtRevision starts a fresh bounded episode only for the availability
// snapshot the administrator observed. A zero expectedRevision preserves the
// legacy internal API behavior for callers that only have an epoch fence.
func (coordinator *RecoveryCoordinator) RetryAtRevision(
	ctx context.Context,
	bootID string,
	epoch uint64,
	expectedRevision uint64,
	requestID string,
) (AvailabilitySnapshot, error) {
	// Accepted recovery runs independently of the HTTP caller's lifetime.
	_ = ctx
	if coordinator == nil || coordinator.owner == nil {
		return AvailabilitySnapshot{}, ErrRuntimeOwnerStopped
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	snapshot, complete, err := coordinator.validateRetryLocked(bootID, epoch, expectedRevision, requestID)
	if complete || err != nil {
		return snapshot, err
	}
	return coordinator.beginManualRetryLocked(snapshot, epoch, requestID)
}

func (coordinator *RecoveryCoordinator) validateRetryLocked(
	bootID string,
	epoch uint64,
	expectedRevision uint64,
	requestID string,
) (AvailabilitySnapshot, bool, error) {
	if coordinator.stopping || !coordinator.started {
		return AvailabilitySnapshot{}, false, ErrRuntimeOwnerStopped
	}
	if requestID == "" || len(requestID) > maxRecoveryRequestIDBytes {
		return AvailabilitySnapshot{}, false, ErrRecoveryRequestID
	}
	snapshot, ok := coordinator.owner.Snapshot()
	if !ok || snapshot.BootID != bootID {
		return AvailabilitySnapshot{}, false, ErrRecoveryConflict
	}
	if _, duplicate := coordinator.manualRequests[requestID]; duplicate {
		return snapshot, true, nil
	}
	if snapshot.RuntimeEpoch != epoch {
		return AvailabilitySnapshot{}, false, ErrRecoveryConflict
	}
	if expectedRevision != 0 && snapshot.Revision != expectedRevision {
		return AvailabilitySnapshot{}, false, ErrRecoveryConflict
	}
	if snapshot.Status == AvailabilityStatusRecovering && coordinator.active != nil {
		coordinator.rememberManualRequestLocked(requestID, snapshot.RecoveryID)
		return snapshot, true, nil
	}
	if !coordinator.isSnapshotRetryable(snapshot) {
		return AvailabilitySnapshot{}, false, ErrRecoveryNotRetryable
	}
	return snapshot, false, nil
}

func (coordinator *RecoveryCoordinator) isSnapshotRetryable(snapshot AvailabilitySnapshot) bool {
	return snapshot.Status == AvailabilityStatusUnavailable && snapshot.RetryAllowed && coordinator.lastSafeToRetry
}

func (coordinator *RecoveryCoordinator) beginManualRetryLocked(
	snapshot AvailabilitySnapshot,
	epoch uint64,
	requestID string,
) (AvailabilitySnapshot, error) {
	recoveryID := uuid.NewString()
	updated, err := coordinator.owner.BeginManualRecovery(snapshot.BootID, epoch, recoveryID)
	if err != nil {
		return updated, err
	}
	coordinator.startsSinceStable = 0
	coordinator.rememberManualRequestLocked(requestID, recoveryID)
	signal := recoverySignal{
		epoch: epoch, reason: AvailabilityReasonAgentctlExited, recoveryID: recoveryID, safe: true, manual: true,
	}
	coordinator.startLocked(signal)
	return updated, nil
}

func (coordinator *RecoveryCoordinator) rememberManualRequestLocked(requestID, recoveryID string) {
	coordinator.manualRequests[requestID] = recoveryID
	coordinator.manualRequestFIFO = append(coordinator.manualRequestFIFO, requestID)
	if len(coordinator.manualRequestFIFO) <= maxRememberedRetryIDs {
		return
	}
	oldest := coordinator.manualRequestFIFO[0]
	coordinator.manualRequestFIFO = coordinator.manualRequestFIFO[1:]
	delete(coordinator.manualRequests, oldest)
}

func (coordinator *RecoveryCoordinator) run(episode *recoveryEpisode) {
	defer coordinator.wg.Done()
	defer coordinator.finishEpisode(episode)
	for {
		if !coordinator.prepareAttempt(episode) {
			return
		}
		startTimeout, ok := coordinator.attemptTimeout(episode)
		if !ok {
			return
		}
		recovered, errorClass, keepRunning := coordinator.replaceOnce(episode, startTimeout)
		if !keepRunning {
			return
		}
		if recovered {
			if !coordinator.waitForStableOrLoss(episode) {
				return
			}
			continue
		}
		coordinator.logAttemptFailure(episode, errorClass)
	}
}

func (coordinator *RecoveryCoordinator) prepareAttempt(episode *recoveryEpisode) bool {
	if !coordinator.applyQueuedLoss(episode) {
		return false
	}
	starts := coordinator.currentStarts()
	if starts >= coordinator.opts.MaxStarts || !coordinator.clock.Now().Before(episode.deadline) {
		coordinator.finishUnavailable(episode, AvailabilityReasonRecoveryExhausted, true)
		return false
	}
	if starts == 0 {
		return true
	}
	backoff := coordinator.backoffFor(starts)
	remaining := episode.deadline.Sub(coordinator.clock.Now())
	if backoff > remaining {
		backoff = remaining
	}
	if coordinator.clock.Sleep(episode.ctx, backoff) != nil {
		return false
	}
	return coordinator.applyQueuedLoss(episode)
}

func (coordinator *RecoveryCoordinator) attemptTimeout(episode *recoveryEpisode) (time.Duration, bool) {
	remaining := episode.deadline.Sub(coordinator.clock.Now())
	if remaining <= 0 || !coordinator.reserveStart() || coordinator.attempt == nil {
		coordinator.finishUnavailable(episode, AvailabilityReasonRecoveryExhausted, true)
		return 0, false
	}
	startTimeout := coordinator.opts.StartTimeout
	if startTimeout > remaining {
		startTimeout = remaining
	}
	runtimeRecoveryAttemptsTotal.Add(recoverySource(episode.signal), 1)
	return startTimeout, true
}

func (coordinator *RecoveryCoordinator) replaceOnce(
	episode *recoveryEpisode,
	startTimeout time.Duration,
) (bool, string, bool) {
	attemptCtx, cancel := context.WithTimeout(episode.ctx, startTimeout)
	attemptErr := coordinator.attempt(attemptCtx, func(epoch uint64, reason string, safeToReplace bool) {
		coordinator.NotifyRuntimeLoss(epoch, reason, safeToReplace)
	})
	cancel()
	if !coordinator.applyQueuedLoss(episode) || episode.ctx.Err() != nil {
		return false, "", false
	}
	snapshot, ok := coordinator.owner.Snapshot()
	if ok && snapshot.Status == AvailabilityStatusAvailable && snapshot.RuntimeEpoch > episode.signal.epoch {
		runtimeRecoveryOutcomesTotal.Add("success", 1)
		observeRecoveryDuration(coordinator.clock.Now().Sub(episode.started))
		runtimeRecoveryInProgress.Set(0)
		coordinator.logEpisode("recovered", episode)
		return true, "", true
	}
	errorClass := "candidate_not_published"
	if attemptErr != nil {
		errorClass = fmt.Sprintf("%T", attemptErr)
	}
	return false, errorClass, true
}

func (coordinator *RecoveryCoordinator) logAttemptFailure(episode *recoveryEpisode, errorClass string) {
	runtimeRecoveryOutcomesTotal.Add("start_failed", 1)
	if coordinator.logger == nil {
		return
	}
	coordinator.logger.Warn("agent runtime replacement attempt did not publish a successor",
		zap.String("outcome", "start_failed"), zap.String("recovery_id", episode.signal.recoveryID),
		zap.Uint64("runtime_epoch", episode.signal.epoch), zap.String("error_class", errorClass))
}

func recoverySource(signal recoverySignal) string {
	if signal.manual {
		return "manual"
	}
	return "automatic"
}

func (coordinator *RecoveryCoordinator) currentStarts() int {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.startsSinceStable
}

func (coordinator *RecoveryCoordinator) reserveStart() bool {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if coordinator.stopping || coordinator.startsSinceStable >= coordinator.opts.MaxStarts {
		return false
	}
	coordinator.startsSinceStable++
	return true
}

func (coordinator *RecoveryCoordinator) backoffFor(starts int) time.Duration {
	index := starts - 1
	if index >= len(coordinator.opts.Backoffs) {
		index = len(coordinator.opts.Backoffs) - 1
	}
	if index < 0 {
		return 0
	}
	return coordinator.opts.Backoffs[index]
}

func (coordinator *RecoveryCoordinator) applyQueuedLoss(episode *recoveryEpisode) bool {
	select {
	case signal := <-episode.losses:
		episode.signal = signal
		episode.started = coordinator.clock.Now()
		episode.deadline = episode.started.Add(coordinator.opts.EpisodeLimit)
		if signal.safe {
			return true
		}
		coordinator.finishUnavailable(episode, AvailabilityReasonOwnershipUnverified, false)
		return false
	default:
		return true
	}
}

func (coordinator *RecoveryCoordinator) waitForStableOrLoss(episode *recoveryEpisode) bool {
	stableCtx, cancel := context.WithCancel(episode.ctx)
	sleepDone := make(chan error, 1)
	go func() { sleepDone <- coordinator.clock.Sleep(stableCtx, coordinator.opts.StableWindow) }()
	select {
	case err := <-sleepDone:
		cancel()
		return coordinator.finishStableWindow(episode, err)
	case signal := <-episode.losses:
		cancel()
		<-sleepDone
		return coordinator.applyLossSignal(episode, signal)
	case <-episode.ctx.Done():
		cancel()
		<-sleepDone
		return false
	}
}

func (coordinator *RecoveryCoordinator) finishStableWindow(episode *recoveryEpisode, sleepErr error) bool {
	if sleepErr != nil || episode.ctx.Err() != nil {
		return false
	}
	select {
	case signal := <-episode.losses:
		return coordinator.applyLossSignal(episode, signal)
	default:
	}
	coordinator.mu.Lock()
	snapshot, ok := coordinator.owner.Snapshot()
	if ok && snapshot.Status == AvailabilityStatusRecovering {
		coordinator.mu.Unlock()
		return coordinator.waitForLoss(episode)
	}
	coordinator.startsSinceStable = 0
	if coordinator.active == episode {
		coordinator.active = nil
	}
	coordinator.mu.Unlock()
	runtimeRecoveryInProgress.Set(0)
	return false
}

func (coordinator *RecoveryCoordinator) waitForLoss(episode *recoveryEpisode) bool {
	select {
	case signal := <-episode.losses:
		return coordinator.applyLossSignal(episode, signal)
	case <-episode.ctx.Done():
		return false
	}
}

func (coordinator *RecoveryCoordinator) applyLossSignal(episode *recoveryEpisode, signal recoverySignal) bool {
	episode.signal = signal
	episode.started = coordinator.clock.Now()
	episode.deadline = episode.started.Add(coordinator.opts.EpisodeLimit)
	if signal.safe {
		return true
	}
	coordinator.finishUnavailable(episode, AvailabilityReasonOwnershipUnverified, false)
	return false
}

func (coordinator *RecoveryCoordinator) finishUnavailable(episode *recoveryEpisode, reason string, retryAllowed bool) {
	if reason == AvailabilityReasonOwnershipUnverified {
		runtimeRecoveryOutcomesTotal.Add("blocked", 1)
	} else {
		runtimeRecoveryOutcomesTotal.Add("exhausted", 1)
	}
	coordinator.owner.FinishRecovery(episode.signal.epoch, episode.signal.recoveryID, reason, retryAllowed)
	runtimeRecoveryInProgress.Set(0)
	observeRecoveryDuration(coordinator.clock.Now().Sub(episode.started))
	coordinator.logEpisode(reason, episode)
}

func (coordinator *RecoveryCoordinator) logEpisode(outcome string, episode *recoveryEpisode) {
	if coordinator.logger == nil {
		return
	}
	coordinator.logger.Info("agent runtime recovery episode completed",
		zap.String("outcome", outcome), zap.String("recovery_id", episode.signal.recoveryID),
		zap.Uint64("runtime_epoch", episode.signal.epoch))
}

func (coordinator *RecoveryCoordinator) finishEpisode(episode *recoveryEpisode) {
	coordinator.mu.Lock()
	if coordinator.active == episode {
		coordinator.active = nil
	}
	coordinator.mu.Unlock()
	episode.cancel()
}

// Stop prevents candidate commits, cancels all timers, and closes the current
// runtime owner. Backend shutdown therefore wins over delayed child callbacks.
func (coordinator *RecoveryCoordinator) Stop() {
	if coordinator == nil {
		return
	}
	coordinator.mu.Lock()
	if coordinator.stopping {
		coordinator.mu.Unlock()
		return
	}
	coordinator.stopping = true
	if coordinator.parentCancel != nil {
		coordinator.parentCancel()
	}
	if coordinator.active != nil {
		coordinator.active.cancel()
	}
	coordinator.pending = nil
	coordinator.mu.Unlock()
	if coordinator.owner != nil {
		coordinator.owner.PreventNewBindings()
		coordinator.owner.Stop()
	}
	coordinator.wg.Wait()
	runtimeRecoveryInProgress.Set(0)
}

func observeRecoveryDuration(duration time.Duration) {
	seconds := duration.Seconds()
	for _, bucket := range []struct {
		limit float64
		key   string
	}{
		{1, "le_1"}, {5, "le_5"}, {15, "le_15"}, {30, "le_30"}, {60, "le_60"},
	} {
		if seconds <= bucket.limit {
			runtimeRecoveryDuration.Add(bucket.key, 1)
		}
	}
	if seconds > 60 {
		runtimeRecoveryDuration.Add("gt_60", 1)
	}
}
