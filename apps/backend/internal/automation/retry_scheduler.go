package automation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// RetryScheduler promotes leased rows asynchronously and never runs inline.
type RetryScheduler struct {
	svc                *Service
	logger             *logger.Logger
	cancel             context.CancelFunc
	wg                 sync.WaitGroup
	mu                 sync.Mutex
	started            bool
	cursorAutomationID string
}

const retrySchedulerBatchBudget = 32

const retryOutboxReplayInterval = time.Second

func NewRetryScheduler(svc *Service, log *logger.Logger) *RetryScheduler {
	return &RetryScheduler{svc: svc, logger: log}
}

func (rs *RetryScheduler) Start(ctx context.Context) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.started {
		return
	}
	rs.started = true
	ctx, rs.cancel = context.WithCancel(ctx)
	rs.wg.Add(1)
	go rs.loop(ctx)
}

func (rs *RetryScheduler) Stop() {
	rs.mu.Lock()
	if !rs.started {
		rs.mu.Unlock()
		return
	}
	cancel := rs.cancel
	rs.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	rs.wg.Wait()
	rs.mu.Lock()
	rs.started = false
	rs.mu.Unlock()
}

func (rs *RetryScheduler) loop(ctx context.Context) {
	if rs.svc == nil {
		return
	}
	defer rs.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var lastOutboxReplay time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			now = now.UTC()
			rs.replayPendingOutbox(ctx, now, &lastOutboxReplay)
			if err := rs.svc.Store().RecoverRetryClaims(ctx, now); err != nil {
				rs.logClaimError("retry claim recovery failed", err)
				continue
			}
			automationIDs, err := rs.svc.Store().ListDueRetryAutomationIDs(ctx, now)
			if err != nil {
				rs.logClaimError("retry automation selection failed", err)
				continue
			}
			orderedAutomationIDs := rs.roundRobinAutomationIDs(automationIDs)
			if len(orderedAutomationIDs) > retrySchedulerBatchBudget {
				orderedAutomationIDs = orderedAutomationIDs[:retrySchedulerBatchBudget]
			}
			for _, automationID := range orderedAutomationIDs {
				if err := ctx.Err(); err != nil {
					return
				}
				run, token, claimErr := rs.svc.Store().ClaimDueRetryForAutomation(ctx, now, 30*time.Second, automationID)
				if errors.Is(claimErr, ErrNoDueRetry) {
					continue
				}
				if claimErr != nil {
					rs.logClaimError("retry claim failed", claimErr)
					continue
				}
				rs.cursorAutomationID = automationID
				rs.publishClaim(ctx, run, token)
			}
		}
	}
}

func (rs *RetryScheduler) replayPendingOutbox(ctx context.Context, now time.Time, lastReplay *time.Time) {
	if !lastReplay.IsZero() && now.Sub(*lastReplay) < retryOutboxReplayInterval {
		return
	}
	*lastReplay = now
	if err := rs.svc.ReplayPendingRetryEvents(ctx); err != nil {
		rs.logClaimError("retry outbox replay failed", err)
	}
}

func (rs *RetryScheduler) roundRobinAutomationIDs(ids []string) []string {
	if len(ids) < 2 || rs.cursorAutomationID == "" {
		return ids
	}
	start := 0
	for i, id := range ids {
		if id > rs.cursorAutomationID {
			start = i
			break
		}
		start = (i + 1) % len(ids)
	}
	return append(append([]string(nil), ids[start:]...), ids[:start]...)
}

func (rs *RetryScheduler) publishClaim(ctx context.Context, run *AutomationRun, token string) {
	run.RetryClaimToken = token
	if rs.svc.eventBus == nil {
		_ = rs.svc.Store().ReleaseRetryClaim(context.Background(), run.ID, token, run.RetryGroupGeneration)
		return
	}
	evt := &AutomationTriggeredEvent{
		RunID: run.ID, AutomationID: run.AutomationID, TriggerID: run.TriggerID,
		TriggerType: run.TriggerType, RetryClaimToken: token,
		RetryGroupGeneration: run.RetryGroupGeneration,
		SnapshotVersion:      run.RetryLaunchConfigVersion,
	}
	eventID := fmt.Sprintf("%s:%d", run.ID, run.RetryLaunchConfigVersion)
	outbox, err := rs.svc.Store().ClaimRetryOutbox(ctx, eventID, time.Now().UTC(), 30*time.Second)
	switch {
	case err == nil:
		evt.RetryOutboxEventID = outbox.EventID
		evt.RetryOutboxLeaseToken = outbox.LeaseToken
	case errors.Is(err, ErrRetryOutboxLeaseHeld):
		// The outbox owner publishes the event carrying this run claim.
		return
	case errors.Is(err, sql.ErrNoRows):
		// Legacy/manual retry runs do not own an outbox row.
	default:
		rs.logClaimError("retry outbox claim failed", err)
		_ = rs.svc.Store().ReleaseRetryClaim(context.Background(), run.ID, token, run.RetryGroupGeneration)
		return
	}
	if err := rs.svc.eventBus.Publish(ctx, events.AutomationTriggered,
		bus.NewEvent(events.AutomationTriggered, "automation_retry_scheduler", evt)); err != nil {
		if evt.RetryOutboxEventID != "" {
			_ = rs.svc.Store().ReleaseRetryOutbox(context.Background(), evt.RetryOutboxEventID, evt.RetryOutboxLeaseToken)
		}
		_ = rs.svc.Store().ReleaseRetryClaim(context.Background(), run.ID, token, run.RetryGroupGeneration)
	}
}

func (rs *RetryScheduler) logClaimError(message string, err error) {
	if rs.logger != nil {
		rs.logger.Warn(message, zap.Error(err))
	}
}

// RecoverRetryClaims requeues expired claims without creating a new attempt.
func (s *Store) RecoverRetryClaims(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_runs SET retry_state = ?, retry_claimed_at = NULL, retry_claim_expires_at = NULL, retry_claim_token = '' WHERE retry_state = ? AND (retry_claim_expires_at IS NULL OR retry_claim_expires_at <= ?)`), RetryStateScheduled, RetryStateClaimed, now)
	return err
}
