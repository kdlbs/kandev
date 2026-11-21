package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/childstall"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// SetChildStallProducer installs the producer whose candidates the queue
// drain rechecks before dispatching a child stall alert.
func (s *Service) SetChildStallProducer(producer *ChildStallProducer) {
	s.childStallProducer = producer
}

// pruneStaleChildStallAlerts removes pending child stall alerts that no longer
// need delivery: alerts whose every candidate stopped qualifying, and alerts
// whose every candidate is superseded by a newer pending alert for the same
// child task. Alerts with at least one still-needed candidate stay queued. A
// child reminder whose source turn no longer misses its completion signal is
// removed the same way. Only child stall alert and reminder entries are
// considered, so user, agent, and other server entries are never touched, and
// entries are only removed — never reordered. Each removal rechecks the entry
// under the admission lock and skips it when it changed since it was assessed.
func (s *Service) pruneStaleChildStallAlerts(ctx context.Context, identity messagequeue.QueueSessionIdentity) {
	producer := s.childStallProducer
	if producer == nil || s.messageQueue == nil {
		return
	}
	entries, _, err := s.messageQueue.SnapshotSessionForIdentity(ctx, identity)
	if err != nil {
		return
	}
	newest := producer.newestEligibleAlertKeysByChild(ctx, entries)
	for i := range entries {
		entry := &entries[i]
		keep, reason := producer.pruneDisposition(ctx, entry, newest)
		if keep {
			continue
		}
		removed, removeErr := producer.removePendingNotice(ctx, s.messageQueue, identity, entry.ID, entry)
		switch {
		case removeErr != nil && !errors.Is(removeErr, messagequeue.ErrEntryNotFound) &&
			!errors.Is(removeErr, errChildStallNoticeUnsettled):
			s.logger.Warn("failed to remove stale child stall alert",
				zap.String("session_id", identity.SessionID), zap.String("queue_id", entry.ID), zap.Error(removeErr))
		case removed != nil:
			recordChildStallOutcome(reason)
			s.publishQueueStatusEventForIdentity(ctx, identity)
		}
	}
}

// pruneDisposition reports whether the prune keeps one queue entry, and the
// bounded outcome recorded when it is removed. Entries that are not pending
// child-attention alerts or reminders are always kept.
func (p *ChildStallProducer) pruneDisposition(
	ctx context.Context,
	entry *messagequeue.QueuedMessage,
	newest map[string]alertOrderKey,
) (bool, string) {
	if entry.IsReservedInFlight() || entry.IsDeliveryAttempted() || entry.IsDurableDelivery() {
		return true, ""
	}
	switch {
	case messagequeue.IsChildStallAlert(entry):
		return p.alertEntryDisposition(ctx, entry, newest)
	case messagequeue.IsChildStallNudge(entry):
		return p.nudgeStatus(ctx, entry) != childStallSourceStale, "stale_nudge_at_dispatch"
	default:
		return true, ""
	}
}

// alertOrderKey orders pending alert entries by enqueue time, with the FIFO
// position as a tie-break, so one alert can be identified as newer than another.
type alertOrderKey struct {
	queuedAt time.Time
	position int64
}

func (k alertOrderKey) after(other alertOrderKey) bool {
	if k.queuedAt.Equal(other.queuedAt) {
		return k.position > other.position
	}
	return k.queuedAt.After(other.queuedAt)
}

func alertOrderKeyOf(entry *messagequeue.QueuedMessage) alertOrderKey {
	return alertOrderKey{queuedAt: entry.QueuedAt, position: entry.Position}
}

// newestEligibleAlertKeysByChild maps each child task to the order key of the
// newest pending alert whose candidate for that child still qualifies. Only a
// candidate positively read as still stalled can supersede an older one, so an
// eligible alert is never dropped in favor of a stale or unreadable sibling.
func (p *ChildStallProducer) newestEligibleAlertKeysByChild(
	ctx context.Context,
	entries []messagequeue.QueuedMessage,
) map[string]alertOrderKey {
	newest := make(map[string]alertOrderKey)
	for i := range entries {
		entry := &entries[i]
		if !messagequeue.IsChildStallAlert(entry) {
			continue
		}
		key := alertOrderKeyOf(entry)
		for _, descriptor := range messagequeue.ChildStallAlertDescriptors(entry) {
			if descriptor.ChildTaskID == "" || descriptor.TurnID == "" {
				continue
			}
			if status, _ := p.assessSource(ctx, descriptor.TurnID); status != childStallSourceLive {
				continue
			}
			if current, ok := newest[descriptor.ChildTaskID]; !ok || key.after(current) {
				newest[descriptor.ChildTaskID] = key
			}
		}
	}
	return newest
}

// alertEntryDisposition reports whether a pending alert entry must stay queued,
// and why it is removed when it must not. An entry is kept when at least one of
// its candidates still needs delivery (or cannot be read) and is not superseded
// by a newer eligible alert for the same child. A candidate with no child or
// turn identity is treated as eligible and never superseded, so unclear
// provenance is preserved.
func (p *ChildStallProducer) alertEntryDisposition(
	ctx context.Context,
	entry *messagequeue.QueuedMessage,
	newest map[string]alertOrderKey,
) (keep bool, reason string) {
	descriptors := messagequeue.ChildStallAlertDescriptors(entry)
	if len(descriptors) == 0 {
		return true, ""
	}
	key := alertOrderKeyOf(entry)
	retained := 0
	for _, descriptor := range descriptors {
		if descriptor.TurnID != "" {
			if status, _ := p.assessSource(ctx, descriptor.TurnID); status == childStallSourceStale {
				continue
			}
		}
		retained++
		if descriptor.ChildTaskID == "" || descriptor.TurnID == "" || !newest[descriptor.ChildTaskID].after(key) {
			return true, ""
		}
	}
	if retained == 0 {
		return false, "stale_at_dispatch"
	}
	return false, "superseded_at_dispatch"
}

// childStallSourceStatus is what a delivery boundary knows about one folded
// candidate's source turn.
type childStallSourceStatus int

const (
	// childStallSourceStale means the source no longer qualifies.
	childStallSourceStale childStallSourceStatus = iota
	// childStallSourceLive means the source still needs attention.
	childStallSourceLive
	// childStallSourceUnreadable means the source could not be read; the
	// notice is kept but must not be dispatched until a read succeeds.
	childStallSourceUnreadable
)

// assessSource rechecks one candidate's source turn. A live source returns its
// current decision so the notice can name the current cause and question. A
// read failure is reported as unreadable, never as live, so it can keep a
// notice queued without authorizing its dispatch. A source turn, child
// session, or child task that no longer exists is stale.
func (p *ChildStallProducer) assessSource(ctx context.Context, turnID string) (childStallSourceStatus, childstall.Decision) {
	turn, err := p.store.GetTurn(ctx, turnID)
	if errors.Is(err, sql.ErrNoRows) {
		return childStallSourceStale, childstall.Decision{}
	}
	if err != nil {
		return childStallSourceUnreadable, childstall.Decision{}
	}
	if turn == nil {
		return childStallSourceStale, childstall.Decision{}
	}
	start, ok := models.LoadChildStallStart(turn.Metadata)
	if !ok {
		return childStallSourceStale, childstall.Decision{}
	}
	decision, err := p.recheck(ctx, turn, start)
	if errors.Is(err, models.ErrTaskSessionNotFound) || errors.Is(err, errChildStallSessionOwner) {
		return childStallSourceStale, childstall.Decision{}
	}
	if err != nil {
		return childStallSourceUnreadable, childstall.Decision{}
	}
	switch decision.Outcome {
	case childstall.OutcomeQualified, childstall.OutcomeHeld, childstall.OutcomeSettling:
		return childStallSourceLive, decision
	default:
		return childStallSourceStale, decision
	}
}

// nudgeStatus rechecks the source turn of a queued child reminder. The
// reminder is live only while that turn still stops without the completion
// signal its step requires; any other outcome makes the reminder stale.
func (p *ChildStallProducer) nudgeStatus(ctx context.Context, entry *messagequeue.QueuedMessage) childStallSourceStatus {
	turnID := messagequeue.ChildStallNudgeTurnID(entry)
	if turnID == "" {
		return childStallSourceLive
	}
	status, decision := p.assessSource(ctx, turnID)
	if status != childStallSourceLive {
		return status
	}
	if decision.Outcome == childstall.OutcomeQualified && decision.Cause == childstall.CauseMissingCompletionSignal {
		return childStallSourceLive
	}
	return childStallSourceStale
}

// childStallDispatchVerdict is the dispatch boundary's decision for one queued
// alert or reminder.
type childStallDispatchVerdict int

const (
	// childStallDispatchSend dispatches the entry with the planned content.
	childStallDispatchSend childStallDispatchVerdict = iota
	// childStallDispatchDiscard drops the entry without a model turn.
	childStallDispatchDiscard
	// childStallDispatchDefer keeps the entry queued because a source could
	// not be read; it is retried by a later drain.
	childStallDispatchDefer
)

// childStallDispatchPlan is what the dispatch boundary sends for one entry:
// the prompt base text and the metadata recorded with the history item. Nil
// metadata keeps the entry's own.
type childStallDispatchPlan struct {
	verdict  childStallDispatchVerdict
	content  string
	metadata map[string]interface{}
}

// planChildStallDispatch rechecks every folded candidate of a queued alert, or
// the source of a queued child reminder, immediately before dispatch. Stale
// candidates are dropped from both the prompt text and the recorded metadata;
// a live candidate is refreshed to its current cause and question; an
// unreadable source defers the whole entry instead of dispatching unchecked
// text. An entry without candidate provenance is sent as queued.
func (s *Service) planChildStallDispatch(
	ctx context.Context,
	queuedMsg *messagequeue.QueuedMessage,
) childStallDispatchPlan {
	plan := childStallDispatchPlan{verdict: childStallDispatchSend, content: queuedMsg.Content}
	producer := s.childStallProducer
	if producer == nil {
		// Without the producer no candidate can be rechecked, so the entry is
		// delivered exactly as it was admitted.
		return plan
	}
	if messagequeue.IsChildStallNudge(queuedMsg) {
		switch producer.nudgeStatus(ctx, queuedMsg) {
		case childStallSourceStale:
			plan.verdict = childStallDispatchDiscard
		case childStallSourceUnreadable:
			plan.verdict = childStallDispatchDefer
		}
		return plan
	}
	descriptors := messagequeue.ChildStallAlertDescriptors(queuedMsg)
	if len(descriptors) == 0 {
		return plan
	}
	kept := make([]messagequeue.ChildStallAlertDescriptor, 0, len(descriptors))
	changed := false
	for _, descriptor := range descriptors {
		if descriptor.TurnID == "" {
			kept = append(kept, descriptor)
			continue
		}
		status, decision := producer.assessSource(ctx, descriptor.TurnID)
		switch status {
		case childStallSourceUnreadable:
			plan.verdict = childStallDispatchDefer
			return plan
		case childStallSourceStale:
			changed = true
			continue
		}
		if refreshed, ok := refreshChildStallDescriptor(descriptor, decision); ok {
			descriptor, changed = refreshed, true
		}
		kept = append(kept, descriptor)
	}
	if len(kept) == 0 {
		plan.verdict = childStallDispatchDiscard
		return plan
	}
	if !changed {
		return plan
	}
	plan.content = formatChildStallDescriptorsText(kept)
	plan.metadata = messagequeue.WithChildStallAlertDescriptors(queuedMsg.Metadata, kept)
	return plan
}

// refreshChildStallDescriptor moves a still-qualified candidate to the cause
// and question its source currently reports, so a parent is never told to
// answer a question that was already replaced. Held and settling candidates
// keep their queued text.
func refreshChildStallDescriptor(
	descriptor messagequeue.ChildStallAlertDescriptor,
	decision childstall.Decision,
) (messagequeue.ChildStallAlertDescriptor, bool) {
	if decision.Outcome != childstall.OutcomeQualified {
		return descriptor, false
	}
	cause := string(decision.Cause)
	questionID := descriptor.QuestionID
	if decision.QuestionID != "" || cause != descriptor.Cause {
		questionID = decision.QuestionID
	}
	if cause == descriptor.Cause && questionID == descriptor.QuestionID {
		return descriptor, false
	}
	descriptor.Cause, descriptor.QuestionID = cause, questionID
	return descriptor, true
}

// ErrChildStallUnavailable means the child-turn stalled producer is not running.
var ErrChildStallUnavailable = errors.New("child stall producer is not available")

// RetryChildStall makes a failed child stall alert eligible again under its
// original identity. The caller must be authorized for the child task/session.
func (s *Service) RetryChildStall(ctx context.Context, taskID, sessionID, turnID string) (bool, error) {
	if err := s.authorizeSessionControl(ctx, sessionID); err != nil {
		return false, err
	}
	if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
		return false, err
	}
	producer := s.childStallProducer
	if producer == nil {
		return false, ErrChildStallUnavailable
	}
	return producer.RetryFailed(ctx, sessionID, turnID)
}
