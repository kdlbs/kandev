package orchestrator

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/childstall"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// Bounded outcomes recorded when a delivered notice is reconciled. They are
// labels on the existing child-attention outcome counter and never carry a
// task, session, or turn identifier.
const (
	noticeOutcomeRehomed    = "rehomed_notice"
	noticeOutcomeHeld       = "held_waiting_parent"
	noticeOutcomeParentGone = "withdrawn_parent_gone"
	noticeOutcomeSourceDone = "withdrawn_source_resolved"
)

var errChildStallNoticeUnsettled = errors.New("child stall notice delivery remains unsettled")

// reconcileDelivered keeps a delivered notice bound to the parent's current
// promptable primary. It resolves the candidate once its notice is gone, and
// otherwise re-homes, holds, or withdraws the notice. Only child-attention
// alert entries are ever touched, so user, agent, workflow, and non-alert
// server entries are left in place.
func (p *ChildStallProducer) reconcileDelivered(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	state models.ChildStallState,
	now time.Time,
	stats *childStallPassStats,
) {
	if state.QueueID == "" {
		p.resolveDeliveredCandidate(ctx, turn, state, now)
		return
	}
	if !p.sourceStillStalled(ctx, turn.ID) {
		p.withdrawDeliveredNotice(ctx, turn, start, state, noticeOutcomeSourceDone, now)
		return
	}
	parentTask, err := p.store.GetTask(ctx, start.ParentTaskID)
	if err != nil {
		stats.observePending(turn)
		return
	}
	if parentTask == nil || parentTask.ArchivedAt != nil || models.IsTerminalTaskState(parentTask.State) {
		p.withdrawDeliveredNotice(ctx, turn, start, state, noticeOutcomeParentGone, now)
		return
	}
	primary, unavailable, err := p.resolveParentSession(ctx, start.ParentTaskID)
	if err != nil {
		stats.observePending(turn)
		return
	}
	if unavailable {
		p.holdDeliveredNotice(ctx, turn, start, state, now)
		stats.observePending(turn)
		return
	}
	if state.ParentSessionID == primary.ID {
		p.resolveOrKeepDelivered(ctx, turn, start, state, primary, now, stats)
		return
	}
	moved, err := p.rehomeDeliveredNotice(ctx, turn, start, state, primary, now)
	if err != nil {
		stats.observePending(turn)
		return
	}
	if moved {
		stats.observePending(turn)
		return
	}
	// Re-home found no entry to move, so the notice is already gone.
	p.resolveDeliveredCandidate(ctx, turn, state, now)
}

// resolveOrKeepDelivered keeps a notice already on the promptable primary, and
// resolves the candidate once that notice is no longer pending.
func (p *ChildStallProducer) resolveOrKeepDelivered(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	state models.ChildStallState,
	primary *models.TaskSession,
	now time.Time,
	stats *childStallPassStats,
) {
	present, err := p.noticePresent(ctx, start.ParentTaskID, primary.ID, state.QueueID)
	if err != nil {
		stats.observePending(turn)
		return
	}
	if !present {
		p.resolveDeliveredCandidate(ctx, turn, state, now)
		return
	}
	stats.observePending(turn)
}

// withdrawDeliveredNotice removes the notice and resolves the candidate, for a
// cause that no longer needs delivery.
func (p *ChildStallProducer) withdrawDeliveredNotice(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	state models.ChildStallState,
	outcome string,
	now time.Time,
) {
	if removed, err := p.takeNoticeEntry(ctx, start.ParentTaskID, state.ParentSessionID, state.QueueID); err != nil {
		p.log.Warn("failed to withdraw delivered child stall notice",
			zap.String("turn_id", turn.ID), zap.Error(err))
		return
	} else if removed != nil {
		recordChildStallOutcome(outcome)
	}
	p.resolveDeliveredCandidate(ctx, turn, state, now)
}

// holdDeliveredNotice withdraws the notice from the queue and returns the
// candidate to waiting_parent, so it waits on the source turn instead of
// occupying a permanent session queue row. The operator notification is
// delivered at most once by waitForParent.
func (p *ChildStallProducer) holdDeliveredNotice(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	state models.ChildStallState,
	now time.Time,
) {
	if removed, err := p.takeNoticeEntry(ctx, start.ParentTaskID, state.ParentSessionID, state.QueueID); err != nil {
		p.log.Warn("failed to withdraw held child stall notice",
			zap.String("turn_id", turn.ID), zap.Error(err))
		return
	} else if removed != nil {
		recordChildStallOutcome(noticeOutcomeHeld)
	}
	decision := childstall.Decision{Cause: childstall.Cause(state.Cause), QuestionID: state.QuestionID}
	p.waitForParent(ctx, turn, start, state, decision, now)
}

// rehomeDeliveredNotice moves a notice off a session that is no longer the
// parent's promptable primary. The notice is removed from its holder first (the
// queue rejects two entries with one admission id), then admitted to the
// primary; if that admission fails the notice is restored to its holder so a
// transient error never drops it. It reports whether a notice was moved; a nil
// error with moved=false means no notice remained.
func (p *ChildStallProducer) rehomeDeliveredNotice(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	state models.ChildStallState,
	primary *models.TaskSession,
	now time.Time,
) (bool, error) {
	queue := p.messageQueue()
	if queue == nil {
		return false, nil
	}
	moved, err := p.takeNoticeEntry(ctx, start.ParentTaskID, state.ParentSessionID, state.QueueID)
	if err != nil {
		return false, err
	}
	if moved == nil {
		return false, nil
	}
	primaryIdentity, resolveErr := queue.ResolveSessionIdentity(ctx, start.ParentTaskID, primary.ID)
	if resolveErr == nil {
		queued, _, admitErr := queue.QueueMessageWithMetadataForSessionWithClientQueueID(
			ctx, primaryIdentity, moved.ID, moved.Content, moved.Model, messagequeue.QueuedByServer,
			moved.PlanMode, moved.Attachments, moved.Metadata, nil,
		)
		if admitErr == nil {
			state.ParentSessionID = primary.ID
			if queued != nil {
				state.QueueID = queued.ID
			}
			p.persist(ctx, turn, state, false, now)
			recordChildStallOutcome(noticeOutcomeRehomed)
			p.svc.publishQueueStatusEventForIdentity(ctx, primaryIdentity)
			p.svc.CheckQueueAdmissionReadiness(ctx, primaryIdentity)
			return true, nil
		}
		resolveErr = admitErr
	}
	p.restoreNoticeToHolder(ctx, start.ParentTaskID, moved, resolveErr)
	return false, resolveErr
}

// restoreNoticeToHolder puts a notice back on the session it came from after a
// failed re-home. A failure here is logged only: the next pass retries from the
// still-scannable candidate.
func (p *ChildStallProducer) restoreNoticeToHolder(
	ctx context.Context,
	parentTaskID string,
	moved *messagequeue.QueuedMessage,
	cause error,
) {
	queue := p.messageQueue()
	if queue == nil || moved == nil || moved.SessionID == "" {
		return
	}
	holderIdentity, err := queue.ResolveSessionIdentity(ctx, parentTaskID, moved.SessionID)
	if err != nil {
		return
	}
	if _, restoreErr := queue.RestoreMessageForSession(ctx, holderIdentity, moved); restoreErr != nil {
		p.log.Warn("failed to restore child stall notice after re-home failure",
			zap.String("queue_id", moved.ID), zap.Error(cause), zap.Error(restoreErr))
		return
	}
	p.svc.publishQueueStatusEventForIdentity(ctx, holderIdentity)
}

// resolveDeliveredCandidate marks a delivered candidate that no longer has a
// pending notice as resolved, so it leaves the scan.
func (p *ChildStallProducer) resolveDeliveredCandidate(
	ctx context.Context,
	turn *models.Turn,
	state models.ChildStallState,
	now time.Time,
) {
	p.persist(ctx, turn, state, true, now)
}

// noticePresent reports whether the identified entry is still pending on the
// exact session. A read failure is returned so the caller keeps the candidate
// pending rather than resolving it.
func (p *ChildStallProducer) noticePresent(
	ctx context.Context,
	parentTaskID, sessionID, queueID string,
) (bool, error) {
	queue := p.messageQueue()
	if queue == nil || queueID == "" || sessionID == "" {
		return false, nil
	}
	identity, err := queue.ResolveSessionIdentity(ctx, parentTaskID, sessionID)
	if err != nil {
		return false, err
	}
	entries, _, err := queue.SnapshotSessionForIdentity(ctx, identity)
	if err != nil {
		return false, err
	}
	for i := range entries {
		if entries[i].ID == queueID {
			return true, nil
		}
	}
	entry, err := queue.FindEntryByID(ctx, queueID)
	if errors.Is(err, messagequeue.ErrEntryNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if entry != nil && (entry.TaskID != parentTaskID || entry.SessionID != sessionID || !messagequeue.IsChildStallAlert(entry)) {
		return false, errChildStallNoticeUnsettled
	}
	return entry != nil, nil
}

// takeNoticeEntry removes the pending child-attention notice wherever it now
// lives and returns it. It first tries the recorded holder session, then falls
// back to a by-id lookup so a notice moved by a session transfer is still
// reached. A nil, nil result means no entry remains.
func (p *ChildStallProducer) takeNoticeEntry(
	ctx context.Context,
	parentTaskID, holderSessionID, queueID string,
) (*messagequeue.QueuedMessage, error) {
	queue := p.messageQueue()
	if queue == nil || queueID == "" {
		return nil, nil
	}
	if holderSessionID != "" {
		removed, err := p.removeNoticeFromSession(ctx, queue, parentTaskID, holderSessionID, queueID)
		if err != nil || removed != nil {
			return removed, err
		}
	}
	return p.removeNoticeFromCurrentHolder(ctx, queue, parentTaskID, holderSessionID, queueID)
}

// removeNoticeFromSession removes queueID from one resolved session, returning
// the removed entry or nil when that session does not hold it.
func (p *ChildStallProducer) removeNoticeFromSession(
	ctx context.Context,
	queue *messagequeue.Service,
	parentTaskID, sessionID, queueID string,
) (*messagequeue.QueuedMessage, error) {
	identity, err := queue.ResolveSessionIdentity(ctx, parentTaskID, sessionID)
	if err != nil {
		return nil, nil
	}
	return p.removePendingNotice(ctx, queue, identity, queueID)
}

func (p *ChildStallProducer) removePendingNotice(ctx context.Context, queue *messagequeue.Service, identity messagequeue.QueueSessionIdentity, queueID string) (*messagequeue.QueuedMessage, error) {
	var removed *messagequeue.QueuedMessage
	err := queue.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		entry, err := queue.FindEntryByID(admittedCtx, queueID)
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return nil
		}
		if err != nil || entry == nil {
			return err
		}
		if entry.TaskID != identity.TaskID || entry.SessionID != identity.SessionID || !messagequeue.IsChildStallAlert(entry) ||
			entry.IsReservedInFlight() || entry.IsDeliveryAttempted() || entry.IsDurableDelivery() {
			return errChildStallNoticeUnsettled
		}
		result, err := queue.RemoveEntryForSession(admittedCtx, identity, queueID)
		if err != nil {
			return err
		}
		if result != nil && len(result.Removed) > 0 {
			copy := result.Removed[0]
			removed = &copy
		}
		return nil
	})
	if removed != nil {
		p.svc.publishQueueStatusEventForIdentity(ctx, identity)
	}
	return removed, err
}

// removeNoticeFromCurrentHolder finds the entry by id and removes it from
// whichever session now holds it, so a notice moved by a session transfer is
// still reached.
func (p *ChildStallProducer) removeNoticeFromCurrentHolder(
	ctx context.Context,
	queue *messagequeue.Service,
	parentTaskID, holderSessionID, queueID string,
) (*messagequeue.QueuedMessage, error) {
	entry, err := queue.FindEntryByID(ctx, queueID)
	if err != nil {
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if entry == nil || entry.SessionID == holderSessionID {
		return nil, nil
	}
	return p.removeNoticeFromSession(ctx, queue, parentTaskID, entry.SessionID, queueID)
}

// pruneNoticeSession runs the stale/superseded prune for one session that held
// a pending child-attention notice, so equivalent notices for the same child
// collapse even when the session never drains.
func (p *ChildStallProducer) pruneNoticeSession(ctx context.Context, parentTaskID, sessionID string) {
	queue := p.messageQueue()
	if queue == nil || sessionID == "" {
		return
	}
	identity, err := queue.ResolveSessionIdentity(ctx, parentTaskID, sessionID)
	if err != nil {
		return
	}
	p.svc.pruneStaleChildStallAlerts(ctx, identity)
}
