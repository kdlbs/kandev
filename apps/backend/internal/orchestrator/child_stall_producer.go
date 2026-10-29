package orchestrator

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/childstall"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// ChildStallStore is the task storage the child-turn stalled producer reads
// and annotates. The task SQL repository implements it.
type ChildStallStore interface {
	ListUnresolvedChildStallTurns(ctx context.Context, since time.Time, limit int) ([]*models.Turn, error)
	GetLatestCompletedTurnBySessionID(ctx context.Context, sessionID string) (*models.Turn, error)
	GetActiveTurnBySessionID(ctx context.Context, sessionID string) (*models.Turn, error)
	GetTurn(ctx context.Context, id string) (*models.Turn, error)
	PatchTurnMetadata(ctx context.Context, sessionID, turnID string, updates map[string]interface{}) (bool, time.Time, error)
	SetSessionMetadataKey(ctx context.Context, sessionID, key string, value interface{}) error
	SetTaskMetadataKeyIfNotArchived(ctx context.Context, taskID, key string, value interface{}) (bool, error)
	GetTask(ctx context.Context, id string) (*models.Task, error)
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error)
	GetLatestTaskStepTransitionID(ctx context.Context, taskID string) (int64, error)
	ListTaskStepTransitions(ctx context.Context, taskID string, beforeID int64, limit int) ([]models.StepTransition, error)
	ListPendingInteractions(ctx context.Context, filter models.PendingInteractionFilter) ([]*models.Message, error)
	GetMessage(ctx context.Context, id string) (*models.Message, error)
}

// Producer states persisted under models.TurnMetaKeyChildStall beyond the
// classifier outcomes.
const (
	childStallStateWaitingParent = "waiting_parent"
	childStallStateDelivered     = "delivered"
	childStallStateFailed        = "failed"
	// childStallStateNudgedChild is a resolved candidate that reminded the
	// child's own session instead of alerting the parent.
	childStallStateNudgedChild = "nudged_child"
)

const (
	childStallScanInterval = 30 * time.Second
	childStallScanWindow   = 7 * 24 * time.Hour
	childStallScanLimit    = 200
	childStallSettleGrace  = 20 * time.Second
	childStallSignalWait   = 10 * time.Minute
	childStallMaxAttempts  = 5
	childStallErrorLimit   = 200
)

// childStallRetryWaits are the waits after attempts 1-4; attempt 5 fails.
var childStallRetryWaits = []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute}

// ChildStallProducer classifies settled child-task turns and queues an
// attributed alert for the parent's primary session when a child stalls.
// The source turn row is the durable record; turn.completed only wakes the
// producer early.
type ChildStallProducer struct {
	svc    *Service
	store  ChildStallStore
	log    *logger.Logger
	now    func() time.Time
	policy childstall.Policy
	// nudgePolicy bounds how many consecutive missing-signal stalls precede a
	// parent alert.
	nudgePolicy childstall.NudgePolicy
	kick        chan struct{}

	mu     sync.Mutex
	cancel context.CancelFunc
	sub    bus.Subscription
	wg     sync.WaitGroup
	passMu sync.Mutex
}

// NewChildStallProducer builds a producer bound to the orchestrator's queue.
func NewChildStallProducer(svc *Service, store ChildStallStore, log *logger.Logger) *ChildStallProducer {
	return &ChildStallProducer{
		svc:    svc,
		store:  store,
		log:    log.WithFields(zap.String("component", "child-stall-producer")),
		now:    func() time.Time { return time.Now().UTC() },
		policy: childstall.Policy{SettleGrace: childStallSettleGrace, SignalWait: childStallSignalWait},
		nudgePolicy: childstall.NudgePolicy{
			EscalateAfter: childstall.DefaultNudgeEscalateAfter,
		},
		kick: make(chan struct{}, 1),
	}
}

// Start runs the producer loop until Stop or ctx cancellation. It is
// idempotent while running.
func (p *ChildStallProducer) Start(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	if p.svc != nil && p.svc.eventBus != nil {
		sub, err := p.svc.eventBus.Subscribe(events.TurnCompleted, p.onTurnCompleted)
		if err != nil {
			p.log.Warn("child stall producer could not subscribe to turn completion; relying on the scan interval",
				zap.Error(err))
		} else {
			p.sub = sub
		}
	}
	p.wg.Add(1)
	go p.run(runCtx)
}

// Stop cancels the loop and waits for it to drain. It is idempotent.
func (p *ChildStallProducer) Stop() {
	p.mu.Lock()
	cancel, sub := p.cancel, p.sub
	p.cancel, p.sub = nil, nil
	p.mu.Unlock()
	if sub != nil {
		_ = sub.Unsubscribe()
	}
	if cancel != nil {
		cancel()
	}
	p.wg.Wait()
}

func (p *ChildStallProducer) onTurnCompleted(_ context.Context, _ *bus.Event) error {
	p.wake()
	return nil
}

// wake schedules a pass after the settle grace without blocking.
func (p *ChildStallProducer) wake() {
	select {
	case p.kick <- struct{}{}:
	default:
	}
}

func (p *ChildStallProducer) run(ctx context.Context) {
	defer p.wg.Done()
	p.runPass(ctx)
	ticker := time.NewTicker(childStallScanInterval)
	defer ticker.Stop()
	var delayed <-chan time.Time
	var timer *time.Timer
	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-ticker.C:
			p.runPass(ctx)
		case <-p.kick:
			if timer == nil {
				timer = time.NewTimer(p.policy.SettleGrace + time.Second)
				delayed = timer.C
			}
		case <-delayed:
			timer, delayed = nil, nil
			p.runPass(ctx)
		}
	}
}

// runPass classifies and delivers every unresolved child-turn settlement in
// the scan window. Passes never overlap.
func (p *ChildStallProducer) runPass(ctx context.Context) {
	p.passMu.Lock()
	defer p.passMu.Unlock()
	now := p.now()
	turns, err := p.store.ListUnresolvedChildStallTurns(ctx, now.Add(-childStallScanWindow), childStallScanLimit)
	if err != nil {
		recordChildStallOutcome("scan_failed")
		p.log.Warn("child stall scan failed", zap.Error(err))
		return
	}
	stats := childStallPassStats{}
	holders := map[string]string{}
	for _, turn := range turns {
		if ctx.Err() != nil {
			return
		}
		p.processTurn(ctx, turn, now, &stats)
		if start, ok := models.LoadChildStallStart(turn.Metadata); ok {
			if state, ok := models.LoadChildStallState(turn.Metadata); ok &&
				state.QueueID != "" && state.ParentSessionID != "" {
				holders[state.ParentSessionID] = start.ParentTaskID
			}
		}
	}
	for sessionID, parentTaskID := range holders {
		if ctx.Err() != nil {
			return
		}
		p.pruneNoticeSession(ctx, parentTaskID, sessionID)
	}
	stats.publish(now)
}

func (p *ChildStallProducer) processTurn(ctx context.Context, turn *models.Turn, now time.Time, stats *childStallPassStats) {
	start, ok := models.LoadChildStallStart(turn.Metadata)
	if !ok {
		return
	}
	state, _ := models.LoadChildStallState(turn.Metadata)
	if state.State == childStallStateDelivered {
		p.reconcileDelivered(ctx, turn, start, state, now, stats)
		return
	}
	if state.State == childStallStateFailed ||
		(state.NextAttemptAt != nil && now.Before(*state.NextAttemptAt)) {
		stats.observePending(turn)
		return
	}
	evidence, err := p.gatherEvidence(ctx, turn, start, now)
	if err != nil {
		stats.observePending(turn)
		recordChildStallOutcome("read_failed")
		p.log.Debug("child stall evidence read failed; retrying next pass",
			zap.String("turn_id", turn.ID), zap.Error(err))
		return
	}
	decision := childstall.Classify(evidence, p.policy)
	if decision.Outcome == childstall.OutcomeHeld {
		decision = p.promoteHeld(ctx, decision, now)
	}
	p.apply(ctx, turn, start, state, decision, now, stats)
}

func (p *ChildStallProducer) apply(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	state models.ChildStallState,
	decision childstall.Decision,
	now time.Time,
	stats *childStallPassStats,
) {
	switch decision.Outcome {
	case childstall.OutcomeSettling:
		stats.observePending(turn)
	case childstall.OutcomeUnknown, childstall.OutcomeSuppressed:
		state.State, state.Reason, state.Cause = string(decision.Outcome), decision.Reason, ""
		p.persist(ctx, turn, state, true, now)
		recordChildStallOutcome(string(decision.Outcome))
		recordChildStallReason(decision.Reason)
	case childstall.OutcomeHeld:
		stats.observePending(turn)
		if state.State != string(childstall.OutcomeHeld) || state.QuestionID != decision.QuestionID {
			state.State, state.Cause, state.QuestionID = string(childstall.OutcomeHeld), string(decision.Cause), decision.QuestionID
			p.persist(ctx, turn, state, false, now)
			recordChildStallOutcome(string(childstall.OutcomeHeld))
		}
	case childstall.OutcomeQualified:
		stats.observePending(turn)
		if decision.QuestionID == "" {
			decision.QuestionID = state.QuestionID
		}
		p.deliver(ctx, turn, start, state, decision, now)
	}
}

// persist writes the producer state onto the source turn. Resolved turns
// leave the scan.
func (p *ChildStallProducer) persist(
	ctx context.Context,
	turn *models.Turn,
	state models.ChildStallState,
	resolved bool,
	now time.Time,
) bool {
	state.UpdatedAt = now
	if state.FirstSeenAt == nil {
		first := now
		state.FirstSeenAt = &first
	}
	updates := map[string]interface{}{models.TurnMetaKeyChildStall: state.ToMap()}
	if resolved {
		updates[models.TurnMetaKeyChildStallResolved] = true
	}
	updated, _, err := p.store.PatchTurnMetadata(ctx, turn.TaskSessionID, turn.ID, updates)
	if err != nil || !updated {
		p.log.Warn("failed to persist child stall state",
			zap.String("turn_id", turn.ID), zap.String("state", state.State), zap.Error(err))
		return false
	}
	// Mirror the committed state onto the in-memory turn so the same pass can
	// collect the session holding the notice without a second read.
	if turn.Metadata == nil {
		turn.Metadata = map[string]interface{}{}
	}
	turn.Metadata[models.TurnMetaKeyChildStall] = state.ToMap()
	if resolved {
		turn.Metadata[models.TurnMetaKeyChildStallResolved] = true
	}
	return true
}

// retryLater records a transient delivery failure with bounded backoff. After
// the last attempt the candidate stays visibly failed until an operator
// retries it.
func (p *ChildStallProducer) retryLater(
	ctx context.Context,
	turn *models.Turn,
	state models.ChildStallState,
	decision childstall.Decision,
	cause error,
	now time.Time,
) {
	state.Attempts++
	state.Cause, state.QuestionID = string(decision.Cause), decision.QuestionID
	state.LastError = boundedChildStallError(cause)
	if state.Attempts >= childStallMaxAttempts {
		state.State, state.NextAttemptAt = childStallStateFailed, nil
		recordChildStallOutcome(childStallStateFailed)
		p.log.Warn("child stall alert delivery failed; operator retry required",
			zap.String("turn_id", turn.ID), zap.String("task_id", turn.TaskID), zap.Error(cause))
	} else {
		next := now.Add(childStallRetryWaits[state.Attempts-1])
		state.State, state.NextAttemptAt = string(childstall.OutcomeQualified), &next
		recordChildStallOutcome("retry_scheduled")
	}
	p.persist(ctx, turn, state, false, now)
}

func boundedChildStallError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > childStallErrorLimit {
		return message[:childStallErrorLimit]
	}
	return message
}

// RetryFailed makes a failed candidate eligible again under its original
// identity. It reports whether a failed candidate was reset.
func (p *ChildStallProducer) RetryFailed(ctx context.Context, sessionID, turnID string) (bool, error) {
	turn, err := p.store.GetTurn(ctx, turnID)
	if err != nil || turn == nil || turn.TaskSessionID != sessionID {
		return false, err
	}
	state, ok := models.LoadChildStallState(turn.Metadata)
	if !ok || state.State != childStallStateFailed {
		return false, nil
	}
	state.State, state.Attempts, state.NextAttemptAt, state.LastError = string(childstall.OutcomeQualified), 0, nil, ""
	if !p.persist(ctx, turn, state, false, p.now()) {
		return false, nil
	}
	recordChildStallOutcome("operator_retry")
	p.wake()
	return true, nil
}
