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

// errChildStallNoticePersist means a re-homed notice was admitted to the new
// primary but the candidate could not record it; the next pass finds the
// notice there by its candidate and records it.
var errChildStallNoticePersist = errors.New("child stall notice move was not recorded")

// reconcileDelivered keeps a delivered notice bound to the parent's current
// promptable primary. It resolves the candidate once its notice is gone, and
// otherwise re-homes, holds, or withdraws the notice. Only child-attention
// alert entries are ever touched, so user, agent, workflow, and non-alert
// server entries are left in place. Any read failure leaves the candidate
// pending for the next pass.
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
	parentTask, err := p.store.GetTask(ctx, start.ParentTaskID)
	if err != nil && !isTaskNotFound(err) {
		stats.observePending(turn)
		return
	}
	parentGone := parentTask == nil || parentTask.ArchivedAt != nil || models.IsTerminalTaskState(parentTask.State) ||
		parentTask.WorkspaceID != start.WorkspaceID
	switch status, _ := p.assessSource(ctx, turn.ID); status {
	case childStallSourceUnreadable:
		stats.observePending(turn)
		return
	case childStallSourceStale:
		p.reconcileStaleSource(ctx, turn, start, state, parentGone, now, stats)
		return
	}
	if parentGone {
		p.withdrawDeliveredNotice(ctx, turn, start, state, noticeOutcomeParentGone, now, stats)
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
		p.log.Debug("child stall notice re-home deferred; retrying next pass",
			zap.String("turn_id", turn.ID), zap.Error(err))
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

// reconcileStaleSource handles a delivered candidate whose source stopped
// qualifying: its own notice is withdrawn unless a folded sibling in the same
// entry still needs it.
func (p *ChildStallProducer) reconcileStaleSource(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	state models.ChildStallState,
	parentGone bool,
	now time.Time,
	stats *childStallPassStats,
) {
	if !parentGone {
		shared, err := p.noticeServesOtherLiveCandidate(ctx, state.QueueID, turn.ID)
		if err != nil {
			stats.observePending(turn)
			return
		}
		if shared {
			// Sibling candidates folded into the same entry still need it;
			// keep the shared notice and let the dispatch boundary drop only
			// this stale candidate. Removing the entry by id would drop every
			// sibling's notice.
			p.resolveDeliveredCandidate(ctx, turn, state, now)
			return
		}
	}
	p.withdrawDeliveredNotice(ctx, turn, start, state, noticeOutcomeSourceDone, now, stats)
}

// noticeServesOtherLiveCandidate reports whether the shared folded notice still
// carries a descriptor for another candidate that still needs it. A candidate
// that cannot be read keeps the notice, and a failed entry lookup is returned
// so the caller retries instead of withdrawing a notice siblings still need.
func (p *ChildStallProducer) noticeServesOtherLiveCandidate(ctx context.Context, queueID, exceptTurnID string) (bool, error) {
	queue := p.messageQueue()
	if queue == nil || queueID == "" {
		return false, nil
	}
	entry, err := queue.FindEntryByID(ctx, queueID)
	if errors.Is(err, messagequeue.ErrEntryNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if entry == nil || !messagequeue.IsChildStallAlert(entry) {
		return false, nil
	}
	for _, descriptor := range messagequeue.ChildStallAlertDescriptors(entry) {
		if exceptTurnID != "" && descriptor.TurnID == exceptTurnID {
			continue
		}
		// A candidate with no turn identity has unclear provenance, so it stays
		// eligible and keeps the shared notice alive.
		if descriptor.TurnID == "" {
			return true, nil
		}
		if status, _ := p.assessSource(ctx, descriptor.TurnID); status != childStallSourceStale {
			return true, nil
		}
	}
	return false, nil
}

// resolveOrKeepDelivered keeps a notice already on the promptable primary, and
// resolves the candidate once that notice is no longer pending. A kept notice
// asks the primary's queue to drain, so a dispatch that was deferred by a read
// failure is retried on the producer's interval under the queue's own policy.
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
		adopted, adoptErr := p.adoptNoticeForCandidate(ctx, turn, state, start.ParentTaskID, primary.ID, now)
		if adoptErr != nil || adopted {
			stats.observePending(turn)
			return
		}
		p.resolveDeliveredCandidate(ctx, turn, state, now)
		return
	}
	stats.observePending(turn)
	if queue := p.messageQueue(); queue != nil && p.svc != nil {
		if identity, identityErr := queue.ResolveSessionIdentity(ctx, start.ParentTaskID, primary.ID); identityErr == nil {
			p.svc.CheckQueueAdmissionReadiness(ctx, identity)
		}
	}
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
	stats *childStallPassStats,
) {
	if removed, err := p.takeNoticeEntry(ctx, start.ParentTaskID, state.QueueID); err != nil {
		p.log.Warn("failed to withdraw delivered child stall notice",
			zap.String("turn_id", turn.ID), zap.Error(err))
		stats.observePending(turn)
		return
	} else if removed != nil {
		recordChildStallOutcome(outcome)
	}
	p.resolveDeliveredCandidate(ctx, turn, state, now)
}

// holdDeliveredNotice withdraws the notice from the queue and returns the
// candidate to waiting_parent, so it waits on the source turn instead of
// occupying a permanent session queue row. The operator notification is
// delivered at most once by waitForParent. The candidate keeps its QueueID, so
// its next delivery is admitted under a re-admission key.
func (p *ChildStallProducer) holdDeliveredNotice(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	state models.ChildStallState,
	now time.Time,
) {
	if removed, err := p.takeNoticeEntry(ctx, start.ParentTaskID, state.QueueID); err != nil {
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
// parent's promptable primary. The notice is first admitted to the primary
// under a re-admission key derived from the entry being moved, and only then
// removed from its holder, so a failure at any point leaves the notice on at
// least one session and the candidate pending; nothing is restored after the
// fact. A retry of a partial move replays the same keyed admission. It reports
// whether a notice was moved; a nil error with moved=false means no notice
// remained.
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
	entry, err := p.findPendingNotice(ctx, start.ParentTaskID, state.QueueID)
	if err != nil {
		return false, err
	}
	if entry == nil {
		// The entry this candidate tracked is gone. A sibling candidate may
		// have moved it under its own key; the descriptor still finds it.
		return p.adoptNoticeForCandidate(ctx, turn, state, start.ParentTaskID, primary.ID, now)
	}
	if entry.SessionID == primary.ID {
		// A session transfer already moved the notice to the primary.
		state.ParentSessionID = primary.ID
		if !p.persist(ctx, turn, state, false, now) {
			return false, errChildStallNoticePersist
		}
		return true, nil
	}
	primaryIdentity, err := queue.ResolveSessionIdentity(ctx, start.ParentTaskID, primary.ID)
	if err != nil {
		return false, err
	}
	// The key covers the moved text: when a sibling folded into the holder
	// after an interrupted attempt, the current text is admitted as a new
	// item and the older partial copy is collapsed by the prune as a
	// superseded alert for the same child.
	key := childStallReadmissionKey(turn, start, primary.ID, entry.ID, entry.Content)
	queued, _, err := queue.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, primaryIdentity, key, entry.Content, entry.Model,
		messagequeue.QueuedByServer, entry.PlanMode, entry.Attachments, entry.Metadata, nil,
	)
	if err != nil {
		return false, err
	}
	holderIdentity, err := queue.ResolveSessionIdentity(ctx, start.ParentTaskID, entry.SessionID)
	if err != nil {
		return false, err
	}
	if _, err := p.removePendingNotice(ctx, queue, holderIdentity, entry.ID, entry); err != nil &&
		!errors.Is(err, messagequeue.ErrEntryNotFound) {
		return false, err
	}
	state.ParentSessionID = primary.ID
	if queued != nil {
		state.QueueID = queued.ID
	}
	if !p.persist(ctx, turn, state, false, now) {
		return false, errChildStallNoticePersist
	}
	recordChildStallOutcome(noticeOutcomeRehomed)
	p.svc.publishQueueStatusEventForIdentity(ctx, primaryIdentity)
	p.svc.CheckQueueAdmissionReadiness(ctx, primaryIdentity)
	return true, nil
}

// adoptNoticeForCandidate records the pending alert on sessionID that carries
// this candidate's descriptor, when the entry it tracked was moved or folded
// under another identity. It reports whether such a notice was found.
func (p *ChildStallProducer) adoptNoticeForCandidate(
	ctx context.Context,
	turn *models.Turn,
	state models.ChildStallState,
	parentTaskID, sessionID string,
	now time.Time,
) (bool, error) {
	queue := p.messageQueue()
	if queue == nil || sessionID == "" {
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
		entry := &entries[i]
		if !messagequeue.IsChildStallAlert(entry) {
			continue
		}
		for _, descriptor := range messagequeue.ChildStallAlertDescriptors(entry) {
			if descriptor.TurnID != turn.ID {
				continue
			}
			state.ParentSessionID, state.QueueID = sessionID, entry.ID
			if !p.persist(ctx, turn, state, false, now) {
				return false, errChildStallNoticePersist
			}
			return true, nil
		}
	}
	return false, nil
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

// findPendingNotice looks the notice up by id wherever it now lives, so a
// notice moved by a session transfer is still reached. A nil, nil result means
// no entry remains. An entry that belongs to another task, is not an alert, or
// is already reserved or attempted is reported as unsettled.
func (p *ChildStallProducer) findPendingNotice(
	ctx context.Context,
	parentTaskID, queueID string,
) (*messagequeue.QueuedMessage, error) {
	queue := p.messageQueue()
	if queue == nil || queueID == "" {
		return nil, nil
	}
	entry, err := queue.FindEntryByID(ctx, queueID)
	if errors.Is(err, messagequeue.ErrEntryNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}
	if entry.TaskID != parentTaskID || !messagequeue.IsChildStallAlert(entry) ||
		entry.IsReservedInFlight() || entry.IsDeliveryAttempted() || entry.IsDurableDelivery() {
		return nil, errChildStallNoticeUnsettled
	}
	return entry, nil
}

// takeNoticeEntry removes the pending child-attention notice wherever it now
// lives and returns it. A nil, nil result means no entry remains; every read
// failure is returned so the candidate is retried instead of resolved.
func (p *ChildStallProducer) takeNoticeEntry(
	ctx context.Context,
	parentTaskID, queueID string,
) (*messagequeue.QueuedMessage, error) {
	queue := p.messageQueue()
	entry, err := p.findPendingNotice(ctx, parentTaskID, queueID)
	if err != nil || entry == nil {
		return nil, err
	}
	identity, err := queue.ResolveSessionIdentity(ctx, parentTaskID, entry.SessionID)
	if err != nil {
		return nil, err
	}
	return p.removePendingNotice(ctx, queue, identity, queueID, entry)
}

// removePendingNotice removes one pending child-attention entry from the
// identified session under its admission lock. The entry is re-read under the
// lock; when expect is given and the entry changed since it was assessed (for
// example a sibling alert folded into it), nothing is removed and the change is
// reported as unsettled so the caller assesses it again.
func (p *ChildStallProducer) removePendingNotice(
	ctx context.Context,
	queue *messagequeue.Service,
	identity messagequeue.QueueSessionIdentity,
	queueID string,
	expect *messagequeue.QueuedMessage,
) (*messagequeue.QueuedMessage, error) {
	var removed *messagequeue.QueuedMessage
	err := queue.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		entry, err := queue.FindEntryByID(admittedCtx, queueID)
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return nil
		}
		if err != nil || entry == nil {
			return err
		}
		if !childStallEntryRemovable(entry, identity, expect) {
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

// childStallEntryRemovable reports whether a re-read entry is still a pending,
// undispatched child-attention entry of the identified session, unchanged
// since it was assessed when expect is given.
func childStallEntryRemovable(
	entry *messagequeue.QueuedMessage,
	identity messagequeue.QueueSessionIdentity,
	expect *messagequeue.QueuedMessage,
) bool {
	if entry.TaskID != identity.TaskID || entry.SessionID != identity.SessionID {
		return false
	}
	if !messagequeue.IsChildStallAlert(entry) && !messagequeue.IsChildStallNudge(entry) {
		return false
	}
	if entry.IsReservedInFlight() || entry.IsDeliveryAttempted() || entry.IsDurableDelivery() {
		return false
	}
	return expect == nil || entry.Content == expect.Content
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
