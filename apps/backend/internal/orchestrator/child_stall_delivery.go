package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/childstall"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"go.uber.org/zap"
)

var errChildStallQueueUnavailable = errors.New("message queue is not configured")

// errChildStallSessionUnavailable means the stalled child's session row could
// not be read, so a nudge cannot be routed.
var errChildStallSessionUnavailable = errors.New("child stall session is unavailable")

// errChildStallTaskUnavailable means the stalled child's task row could not be
// read, so the (task, step) nudge streak cannot be advanced.
var errChildStallTaskUnavailable = errors.New("child stall task is unavailable")

// reasonNudgeEscalated records a candidate suppressed because its streak
// already alerted the parent.
const reasonNudgeEscalated = "nudge_escalated"

// childStallStaleError means the candidate stopped qualifying at the
// admission boundary.
type childStallStaleError struct{ decision childstall.Decision }

func (e *childStallStaleError) Error() string {
	return "child stall candidate is no longer eligible: " + string(e.decision.Outcome) + " " + e.decision.Reason
}

// childStallAlert is one alert's queue payload.
type childStallAlert struct {
	key      string
	content  string
	metadata map[string]interface{}
}

// deliver routes a qualified candidate. A missing completion signal is nudged
// in the child's own session and escalates to the parent only after the child
// stalls on the same workflow entry DefaultNudgeEscalateAfter times in a row.
// Every other cause is delivered to the parent directly.
func (p *ChildStallProducer) deliver(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	state models.ChildStallState,
	decision childstall.Decision,
	now time.Time,
) {
	if decision.Cause == childstall.CauseMissingCompletionSignal {
		p.deliverMissingSignal(ctx, turn, start, state, decision, now)
		return
	}
	p.deliverToParent(ctx, turn, start, state, decision, now)
}

// deliverMissingSignal nudges the child, escalates to the parent at the streak
// threshold, and suppresses further stalls on an already-escalated streak. The
// streak advances only when a delivery succeeds, so a retried candidate does
// not double-count.
func (p *ChildStallProducer) deliverMissingSignal(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	state models.ChildStallState,
	decision childstall.Decision,
	now time.Time,
) {
	childSession, err := p.store.GetTaskSession(ctx, turn.TaskSessionID)
	if err != nil || childSession == nil {
		if err == nil {
			err = errChildStallSessionUnavailable
		}
		p.retryLater(ctx, turn, state, decision, err, now)
		return
	}
	childTask, err := p.store.GetTask(ctx, turn.TaskID)
	if err != nil || childTask == nil {
		if err == nil {
			err = errChildStallTaskUnavailable
		}
		p.retryLater(ctx, turn, state, decision, err, now)
		return
	}
	streak, action := p.nextNudgeStreak(childTask, decision, start.TransitionID)
	switch action {
	case childstall.NudgeActionSuppress:
		state.State, state.Reason = string(childstall.OutcomeSuppressed), reasonNudgeEscalated
		p.persist(ctx, turn, state, true, now)
		recordChildStallReason(reasonNudgeEscalated)
	case childstall.NudgeActionAlertParent:
		if p.deliverToParent(ctx, turn, start, state, decision, now) {
			p.persistNudgeStreak(ctx, turn, streak, now)
		}
	default:
		if p.deliverChildNudge(ctx, turn, start, state, decision, childSession, now) {
			p.persistNudgeStreak(ctx, turn, streak, now)
		}
	}
}

// deliverToParent queues an attributed alert for the parent's primary session.
// It reports whether the alert was delivered, so the nudge escalation persists
// its streak only on a real delivery.
func (p *ChildStallProducer) deliverToParent(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	state models.ChildStallState,
	decision childstall.Decision,
	now time.Time,
) bool {
	parentTask, err := p.store.GetTask(ctx, start.ParentTaskID)
	if err != nil || parentTask == nil || parentTask.ArchivedAt != nil {
		if err != nil && !isTaskNotFound(err) {
			p.retryLater(ctx, turn, state, decision, err, now)
			return false
		}
		state.State, state.Reason = string(childstall.OutcomeSuppressed), "parent_unavailable"
		p.persist(ctx, turn, state, true, now)
		recordChildStallReason(state.Reason)
		return false
	}
	parentSession, unavailable, err := p.resolveParentSession(ctx, start.ParentTaskID)
	if err != nil {
		p.retryLater(ctx, turn, state, decision, err, now)
		return false
	}
	if unavailable {
		p.waitForParent(ctx, turn, start, state, decision, now)
		return false
	}
	alert, err := p.buildAlert(ctx, turn, start, decision)
	if err != nil {
		p.retryLater(ctx, turn, state, decision, err, now)
		return false
	}
	queued, identity, _, err := p.admitAlert(ctx, turn, start, parentSession, alert, "")
	var stale *childStallStaleError
	switch {
	case errors.As(err, &stale):
		p.applyStale(ctx, turn, state, stale.decision, now)
		return false
	case err != nil:
		p.retryLater(ctx, turn, state, decision, err, now)
		return false
	}
	p.markDelivered(ctx, turn, state, decision, queued, parentSession.ID, now)
	p.svc.publishQueueStatusEventForIdentity(ctx, identity)
	p.svc.CheckQueueAdmissionReadiness(ctx, identity)
	// A keyed replay means the item was already admitted; the delivery still
	// succeeded, so the caller persists its progress (for example the nudge
	// streak) instead of skipping it on replay.
	return true
}

func (p *ChildStallProducer) markDelivered(
	ctx context.Context,
	turn *models.Turn,
	state models.ChildStallState,
	decision childstall.Decision,
	queued *messagequeue.QueuedMessage,
	parentSessionID string,
	now time.Time,
) {
	state.State, state.Cause, state.QuestionID = childStallStateDelivered, string(decision.Cause), decision.QuestionID
	state.ParentSessionID, state.NextAttemptAt, state.LastError = parentSessionID, nil, ""
	if queued != nil {
		state.QueueID = queued.ID
	}
	// A delivered candidate stays scannable while its notice is pending, so a
	// later pass can keep the notice on the parent's current promptable primary.
	p.persist(ctx, turn, state, false, now)
	recordChildStallOutcome(childStallStateDelivered)
	recordChildStallCause(string(decision.Cause))
	lag := time.Duration(0)
	if turn.CompletedAt != nil {
		lag = now.Sub(*turn.CompletedAt)
	}
	p.log.Info("queued child stall alert for parent",
		zap.String("child_task_id", turn.TaskID),
		zap.String("turn_id", turn.ID),
		zap.String("parent_session_id", parentSessionID),
		zap.String("queue_id", state.QueueID),
		zap.String("cause", string(decision.Cause)),
		zap.Duration("settlement_to_queue", lag))
}

// markNudged resolves a candidate that reminded the child's own session.
func (p *ChildStallProducer) markNudged(
	ctx context.Context,
	turn *models.Turn,
	state models.ChildStallState,
	decision childstall.Decision,
	queued *messagequeue.QueuedMessage,
	childSessionID string,
	now time.Time,
) {
	state.State, state.Cause, state.QuestionID = childStallStateNudgedChild, string(decision.Cause), ""
	state.ParentSessionID, state.NextAttemptAt, state.LastError = childSessionID, nil, ""
	if queued != nil {
		state.QueueID = queued.ID
	}
	p.persist(ctx, turn, state, true, now)
	recordChildStallOutcome(childStallStateNudgedChild)
	recordChildStallCause(string(decision.Cause))
	lag := time.Duration(0)
	if turn.CompletedAt != nil {
		lag = now.Sub(*turn.CompletedAt)
	}
	p.log.Info("nudged child session for a missing completion signal",
		zap.String("child_task_id", turn.TaskID),
		zap.String("turn_id", turn.ID),
		zap.String("child_session_id", childSessionID),
		zap.String("queue_id", state.QueueID),
		zap.Duration("settlement_to_queue", lag))
}

// deliverChildNudge queues the short reminder for the child's own session. It
// reports whether the reminder was delivered.
func (p *ChildStallProducer) deliverChildNudge(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	state models.ChildStallState,
	decision childstall.Decision,
	childSession *models.TaskSession,
	now time.Time,
) bool {
	alert := p.buildNudge(turn, start, decision.BlockedReason)
	queued, identity, _, err := p.admitAlert(ctx, turn, start, childSession, alert, childstall.CauseMissingCompletionSignal)
	var stale *childStallStaleError
	switch {
	case errors.As(err, &stale):
		p.applyStale(ctx, turn, state, stale.decision, now)
		return false
	case err != nil:
		p.retryLater(ctx, turn, state, decision, err, now)
		return false
	}
	p.markNudged(ctx, turn, state, decision, queued, childSession.ID, now)
	p.svc.publishQueueStatusEventForIdentity(ctx, identity)
	p.svc.CheckQueueAdmissionReadiness(ctx, identity)
	// A keyed replay still delivered the reminder, so the caller persists the
	// advanced nudge streak instead of skipping it.
	return true
}

// nextNudgeStreak folds the candidate into the child task's stored streak and
// returns the routing action. It is keyed to the task, not the settled turn's
// session, so a session substitution does not reset the escalation count; the
// streak still restarts when the workflow entry (transition) changes. The
// stored streak is never mutated here; it is persisted only after a successful
// delivery.
func (p *ChildStallProducer) nextNudgeStreak(
	childTask *models.Task,
	decision childstall.Decision,
	transitionID int64,
) (childstall.NudgeStreak, childstall.NudgeAction) {
	prev := childstall.NudgeStreak{}
	if persisted, ok := models.LoadChildStallNudgeStreak(childTask.Metadata); ok {
		prev = childstall.NudgeStreak{
			Cause:        childstall.Cause(persisted.Cause),
			TransitionID: persisted.TransitionID,
			Count:        persisted.Count,
			Escalated:    persisted.Escalated,
		}
	}
	return childstall.AdvanceNudge(prev, decision.Cause, transitionID, p.nudgePolicy)
}

// persistNudgeStreak stores the advanced streak on the child task. A write
// failure is logged and does not undo the delivered reminder.
func (p *ChildStallProducer) persistNudgeStreak(
	ctx context.Context,
	turn *models.Turn,
	streak childstall.NudgeStreak,
	now time.Time,
) {
	persisted := models.ChildStallNudgeStreak{
		Cause:        string(streak.Cause),
		TransitionID: streak.TransitionID,
		Count:        streak.Count,
		Escalated:    streak.Escalated,
		UpdatedAt:    now,
	}
	stored, err := p.store.SetTaskMetadataKeyIfNotArchived(
		ctx, turn.TaskID, models.TaskMetaKeyChildStallNudge, persisted.ToMap(),
	)
	if err != nil {
		p.log.Warn("failed to persist child stall nudge streak",
			zap.String("turn_id", turn.ID), zap.Error(err))
		return
	}
	if !stored {
		// The task stopped accepting metadata (archiving/archived) before the
		// streak write; the delivery already happened, so only make the missed
		// escalation bookkeeping visible.
		p.log.Debug("child stall nudge streak not persisted: task no longer accepts metadata",
			zap.String("turn_id", turn.ID))
	}
}

// applyStale records a candidate that stopped qualifying at the admission
// boundary. Only a suppressed or unknown outcome resolves it; a held
// candidate stays held and a settling one is classified again next pass.
func (p *ChildStallProducer) applyStale(
	ctx context.Context,
	turn *models.Turn,
	state models.ChildStallState,
	decision childstall.Decision,
	now time.Time,
) {
	switch decision.Outcome {
	case childstall.OutcomeSuppressed, childstall.OutcomeUnknown:
		state.State, state.Reason = string(decision.Outcome), decision.Reason
		p.persist(ctx, turn, state, true, now)
		recordChildStallReason(decision.Reason)
	case childstall.OutcomeHeld:
		state.State, state.Cause, state.QuestionID = string(decision.Outcome), string(decision.Cause), decision.QuestionID
		p.persist(ctx, turn, state, false, now)
	}
}

// waitForParent keeps a qualified candidate until the parent has a promptable
// primary session, notifying the operator once.
func (p *ChildStallProducer) waitForParent(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	state models.ChildStallState,
	decision childstall.Decision,
	now time.Time,
) {
	first := state.State != childStallStateWaitingParent
	state.State, state.Cause, state.QuestionID = childStallStateWaitingParent, string(decision.Cause), decision.QuestionID
	notify := !state.OperatorNotified
	state.OperatorNotified = true
	if !first && !notify {
		return
	}
	if !p.persist(ctx, turn, state, false, now) {
		return
	}
	recordChildStallOutcome(childStallStateWaitingParent)
	if notify {
		p.notifyOperator(ctx, turn, start, decision)
	}
}

func (p *ChildStallProducer) notifyOperator(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	decision childstall.Decision,
) {
	recordChildStallOutcome("operator_notified")
	p.log.Warn("child stalled but its parent cannot receive the alert",
		zap.String("child_task_id", turn.TaskID),
		zap.String("parent_task_id", start.ParentTaskID),
		zap.String("cause", string(decision.Cause)))
	if p.svc == nil || p.svc.eventBus == nil {
		return
	}
	_ = p.svc.eventBus.Publish(ctx, events.TaskChildStallUndeliverable, bus.NewEvent(
		events.TaskChildStallUndeliverable, "orchestrator", map[string]interface{}{
			"task_id":        turn.TaskID,
			"session_id":     turn.TaskSessionID,
			"turn_id":        turn.ID,
			"parent_task_id": start.ParentTaskID,
			"cause":          string(decision.Cause),
			"occurrence_id":  childStallKey(turn, start),
		}))
}

// admitAlert rechecks eligibility and admits the keyed alert under the target
// session's queue admission lock. A non-empty requireCause additionally demands
// the recheck still classifies that cause, so a nudge is never sent for a
// candidate that turned into a different stall.
func (p *ChildStallProducer) admitAlert(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	session *models.TaskSession,
	alert childStallAlert,
	requireCause childstall.Cause,
) (*messagequeue.QueuedMessage, messagequeue.QueueSessionIdentity, bool, error) {
	queue := p.messageQueue()
	if queue == nil {
		return nil, messagequeue.QueueSessionIdentity{}, false, errChildStallQueueUnavailable
	}
	identity, err := queue.ResolveSessionIdentity(ctx, session.TaskID, session.ID)
	if err != nil {
		return nil, identity, false, err
	}
	var queued *messagequeue.QueuedMessage
	var replay bool
	err = queue.WithSessionAdmission(ctx, session.ID, func(admittedCtx context.Context) error {
		decision, recheckErr := p.recheck(admittedCtx, turn, start)
		if recheckErr != nil {
			return fmt.Errorf("recheck child stall candidate: %w", recheckErr)
		}
		if decision.Outcome != childstall.OutcomeQualified || (requireCause != "" && decision.Cause != requireCause) {
			return &childStallStaleError{decision: decision}
		}
		var admitErr error
		queued, replay, admitErr = queue.QueueMessageWithMetadataForSessionWithClientQueueID(
			admittedCtx, identity, alert.key, alert.content, "", messagequeue.QueuedByServer,
			false, nil, alert.metadata, nil,
		)
		if errors.Is(admitErr, messagequeue.ErrQueueIDConflict) {
			// The keyed alert was already admitted with different display
			// text (for example a renamed child); the earlier item stands.
			queued = &messagequeue.QueuedMessage{ID: alert.key}
			replay = true
			return nil
		}
		return admitErr
	})
	return queued, identity, replay, err
}

// recheck re-reads the candidate's evidence at a delivery boundary. A read
// failure is returned so the candidate stays retryable.
func (p *ChildStallProducer) recheck(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
) (childstall.Decision, error) {
	now := p.now()
	evidence, err := p.gatherEvidence(ctx, turn, start, now)
	if err != nil {
		return childstall.Decision{}, err
	}
	decision := childstall.Classify(evidence, p.policy)
	if decision.Outcome == childstall.OutcomeHeld {
		decision = p.promoteHeld(ctx, decision, now)
	}
	return decision, nil
}

// childStallKey is the caller-owned queue admission identity derived from
// (child task, session, turn, start entry).
func childStallKey(turn *models.Turn, start models.ChildStallStart) string {
	return childStallAdmissionKey("child-stall-", turn, start)
}

// childStallNudgeKey is the admission identity for the reminder queued for the
// child's own session. It is distinct from the parent alert key so a nudge and
// an escalation for the same turn never collide.
func childStallNudgeKey(turn *models.Turn, start models.ChildStallStart) string {
	return childStallAdmissionKey("child-stall-nudge-", turn, start)
}

func childStallAdmissionKey(prefix string, turn *models.Turn, start models.ChildStallStart) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d",
		turn.TaskID, turn.TaskSessionID, turn.ID, start.TransitionID)))
	return prefix + hex.EncodeToString(sum[:20])
}

// buildNudge builds the short reminder queued for the child's own session.
func (p *ChildStallProducer) buildNudge(turn *models.Turn, start models.ChildStallStart, blockedReason string) childStallAlert {
	metadata := map[string]interface{}{
		messagequeue.MetadataChildStallNudge: true,
		"turn_id":                            turn.ID,
	}
	return childStallAlert{
		key:      childStallNudgeKey(turn, start),
		content:  formatChildStallNudge(blockedReason),
		metadata: metadata,
	}
}

// formatChildStallNudge builds the reminder text. When a completion signal for
// the current step was refused by the ownership/profile gate, the reminder names
// that refusal instead of implying the agent never signalled.
func formatChildStallNudge(blockedReason string) string {
	var b strings.Builder
	b.WriteString("STEP COMPLETION SIGNAL MISSING\n\n")
	if reason := strings.TrimSpace(blockedReason); reason != "" {
		b.WriteString("Your step_complete_kandev call was refused, so this workflow step did not advance: ")
		b.WriteString(reason)
		b.WriteString("\n\nResolve the refusal and call step_complete_kandev again with a summary. Otherwise, continue the work.")
		return b.String()
	}
	b.WriteString("Your previous turn ended without calling step_complete_kandev, so this workflow step did not advance.\n\n")
	b.WriteString("If you have finished this step, call step_complete_kandev with a summary. Otherwise, continue the work.")
	return b.String()
}

func (p *ChildStallProducer) buildAlert(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	decision childstall.Decision,
) (childStallAlert, error) {
	child, err := p.store.GetTask(ctx, turn.TaskID)
	if err != nil {
		return childStallAlert{}, err
	}
	stepName := ""
	if p.svc != nil && p.svc.workflowStepGetter != nil && child.WorkflowStepID != "" {
		if step, stepErr := p.svc.workflowStepGetter.GetStep(ctx, child.WorkflowStepID); stepErr == nil && step != nil {
			stepName = step.Name
		}
	}
	descriptor := map[string]interface{}{
		"child_task_id":    child.ID,
		"child_task_title": child.Title,
		"child_session_id": turn.TaskSessionID,
		"turn_id":          turn.ID,
		"step_id":          child.WorkflowStepID,
		"step_name":        stepName,
		"cause":            string(decision.Cause),
	}
	if decision.QuestionID != "" {
		descriptor["question_id"] = decision.QuestionID
	}
	metadata := map[string]interface{}{
		messagequeue.MetadataChildStallAlert:  true,
		messagequeue.MetadataChildStallAlerts: []interface{}{descriptor},
		messagequeue.MetadataSenderTaskID:     child.ID,
		"sender_task_title":                   child.Title,
		"sender_session_id":                   turn.TaskSessionID,
	}
	return childStallAlert{
		key:      childStallKey(turn, start),
		content:  formatChildStallPrompt(child, turn, stepName, decision),
		metadata: metadata,
	}, nil
}

func formatChildStallPrompt(child *models.Task, turn *models.Turn, stepName string, decision childstall.Decision) string {
	return formatChildStallPromptText(
		child.Title, child.ID, turn.ID, stepName, string(decision.Cause), decision.QuestionID,
	)
}

// formatChildStallPromptText renders one candidate's parent-facing alert text.
// A folded alert rebuilds the text of the candidates that still qualify from
// these same values, so a dropped candidate leaves no text behind.
func formatChildStallPromptText(childTitle, childTaskID, turnID, stepName, cause, questionID string) string {
	var b strings.Builder
	b.WriteString("CHILD TASK NEEDS ATTENTION\n\n")
	fmt.Fprintf(&b, "Child task: %s (%s)\n", childTitle, childTaskID)
	if stepName != "" {
		fmt.Fprintf(&b, "Workflow step: %s\n", stepName)
	}
	fmt.Fprintf(&b, "Cause: %s\n", cause)
	fmt.Fprintf(&b, "Child turn: %s\n\n", turnID)
	b.WriteString(childStallCauseInstructionText(childTaskID, cause, questionID))
	b.WriteString("\n\nThis notice does not move, answer, or change either task. Review the child and decide the next action.")
	return b.String()
}

func childStallCauseInstructionText(childID, cause, questionID string) string {
	switch childstall.Cause(cause) {
	case childstall.CauseInputRequired:
		if questionID != "" {
			return fmt.Sprintf("The child is still waiting for your answer to its question %s. Answer it with message_task_kandev using task_id=%s and reply_to_question_id=%s.",
				questionID, childID, questionID)
		}
		return "The child is waiting for input in its own conversation (a question or permission request)."
	case childstall.CauseQuota:
		return "The child's agent stopped on a provider usage limit."
	case childstall.CauseExecutionError:
		return "The child's agent session failed."
	default:
		return "The child's turn ended without signalling step completion, so its workflow step did not advance."
	}
}

func isTaskNotFound(err error) bool {
	return errors.Is(err, repoerrors.ErrTaskNotFound)
}
