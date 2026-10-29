package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/childstall"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const parentQuestionStatusPendingValue = "pending"

// gatherEvidence reads the structured state a classification needs. Any read
// failure is returned so the candidate stays retryable.
func (p *ChildStallProducer) gatherEvidence(
	ctx context.Context,
	turn *models.Turn,
	start models.ChildStallStart,
	now time.Time,
) (childstall.Evidence, error) {
	evidence := childstall.Evidence{
		Start:      start,
		Settlement: models.ChildStallSettlement(turn.Metadata),
	}
	if turn.CompletedAt != nil {
		evidence.SettledFor = now.Sub(*turn.CompletedAt)
	}
	child, err := p.store.GetTask(ctx, turn.TaskID)
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return evidence, nil
	}
	if err != nil {
		return evidence, err
	}
	if child == nil {
		return evidence, nil
	}
	latestTransition, err := p.store.GetLatestTaskStepTransitionID(ctx, child.ID)
	if err != nil {
		return evidence, err
	}
	evidence.Child = &childstall.ChildSnapshot{
		ParentID:           child.ParentID,
		WorkspaceID:        child.WorkspaceID,
		State:              child.State,
		Archived:           child.ArchivedAt != nil,
		LatestTransitionID: latestTransition,
	}
	if err := p.gatherSessionEvidence(ctx, turn.TaskSessionID, child, &evidence); err != nil {
		return evidence, err
	}
	return evidence, nil
}

func (p *ChildStallProducer) gatherSessionEvidence(
	ctx context.Context,
	sessionID string,
	child *models.Task,
	evidence *childstall.Evidence,
) error {
	session, err := p.store.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil || session.TaskID != child.ID {
		return fmt.Errorf("child stall session owner unavailable")
	}
	evidence.SessionState = session.State
	evidence.SessionErrorClass = session.RouteErrorClass
	active, err := p.hasActiveTurn(ctx, sessionID)
	if err != nil {
		return err
	}
	evidence.SessionActiveTurn = active
	pending, err := p.store.ListPendingInteractions(ctx, models.PendingInteractionFilter{SessionIDs: []string{sessionID}})
	if err != nil {
		return err
	}
	evidence.PendingInputs = pendingInputsFromMessages(pending)
	if p.svc != nil {
		evidence.StepRequiresSignal = p.svc.WorkflowStepRequiresCompletionSignal(ctx, child.WorkflowStepID)
	}
	if err := p.gatherCompletionSignalState(ctx, session, child, evidence); err != nil {
		return err
	}
	return nil
}

// gatherCompletionSignalState reads whether the child task already holds an
// accepted completion signal for its current step, and whether a completion
// signal for that step was refused by the ownership/profile gate. The completed
// signal is written on whichever session called step_complete, which after a
// session substitution is the task's current driving session, not the settled
// turn's session. Only the sessions that can authoritatively hold it are read —
// the settled turn's session, the step's durable binding, and the task's
// primary — so a stale bag left on a superseded session from an earlier entry of
// the same step cannot suppress a genuine stall.
func (p *ChildStallProducer) gatherCompletionSignalState(
	ctx context.Context,
	turnSession *models.TaskSession,
	child *models.Task,
	evidence *childstall.Evidence,
) error {
	candidates := make([]*models.TaskSession, 0, 3)
	seen := make(map[string]struct{}, 3)
	add := func(session *models.TaskSession) {
		if session == nil || session.ID == "" {
			return
		}
		if _, ok := seen[session.ID]; ok {
			return
		}
		seen[session.ID] = struct{}{}
		candidates = append(candidates, session)
	}
	add(turnSession)
	if p.svc != nil {
		boundID, err := p.svc.boundChildStallStepSession(ctx, child.ID, child.WorkflowStepID)
		if err != nil {
			return err
		}
		if boundID != "" {
			session, err := p.store.GetTaskSession(ctx, boundID)
			switch {
			case err == nil:
				add(session)
			case errors.Is(err, models.ErrTaskSessionNotFound):
				// The durable binding can outlive the session it names after a
				// substitution or cleanup. A missing bound session is not fatal
				// to the remaining candidates, so classify on those.
			default:
				return err
			}
		}
	}
	primary, err := p.store.GetPrimarySessionByTaskID(ctx, child.ID)
	if err != nil && !errors.Is(err, repoerrors.ErrNoPrimarySession) {
		return err
	}
	add(primary)

	entryOccurredAt, entryKnown := p.currentEntryOccurredAt(ctx, child.ID, evidence.Start.TransitionID)
	var newestRejection time.Time
	for _, session := range candidates {
		if err := p.gatherCompletionSessionActivity(ctx, turnSession, session, child.ID, evidence); err != nil {
			return err
		}
		observeCompletionSignalState(session, child.WorkflowStepID, entryOccurredAt, entryKnown, evidence, &newestRejection)
	}
	return nil
}

func (p *ChildStallProducer) gatherCompletionSessionActivity(
	ctx context.Context,
	turnSession, session *models.TaskSession,
	childID string,
	evidence *childstall.Evidence,
) error {
	if session.TaskID != childID {
		return fmt.Errorf("child stall signal session owner changed")
	}
	if session.ID == turnSession.ID {
		return nil
	}
	active, err := p.hasActiveTurn(ctx, session.ID)
	if err != nil {
		return err
	}
	evidence.SessionActiveTurn = evidence.SessionActiveTurn || active || session.State == models.TaskSessionStateRunning || session.State == models.TaskSessionStateStarting
	return nil
}

// currentEntryOccurredAt returns when the task entered the workflow entry the
// candidate turn started in. It is the anchor a stored signal is judged
// against: a signal left on a session by an earlier entry of the same step (an
// auto-advance out of the step does not consume it) must not hide a genuine
// stall in the new entry. The second result is false when the entry time cannot
// be established, in which case the caller trusts any matching signal so an
// accepted signal never lets the reminder resume.
func (p *ChildStallProducer) currentEntryOccurredAt(ctx context.Context, taskID string, transitionID int64) (time.Time, bool) {
	if transitionID == 0 {
		return time.Time{}, false
	}
	transitions, err := p.store.ListTaskStepTransitions(ctx, taskID, 0, 1)
	if err != nil || len(transitions) == 0 {
		return time.Time{}, false
	}
	latest := transitions[0]
	if latest.ID != transitionID || latest.OccurredAt.IsZero() {
		return time.Time{}, false
	}
	return latest.OccurredAt, true
}

// observeCompletionSignalState folds one session's stored completion state for
// the current step into the evidence. A signal for the step marks the task as
// signalled unless it predates the current entry; the newest recorded rejection
// for the step provides the block reason shown in the reminder.
func observeCompletionSignalState(
	session *models.TaskSession,
	stepID string,
	entryOccurredAt time.Time,
	entryKnown bool,
	evidence *childstall.Evidence,
	newestRejection *time.Time,
) {
	if signal, has := models.LoadPendingStepSignal(session.Metadata); has && signal.StepID == stepID {
		if signalBelongsToEntry(signal, entryOccurredAt, entryKnown) {
			evidence.PendingStepSignal = true
		}
	}
	if rejection, has := models.LoadStepCompleteRejection(session.Metadata); has && rejection.StepID == stepID {
		// A zero timestamp (missing/legacy) must still become the block reason;
		// only an older recorded rejection loses to a newer one.
		if newestRejection.IsZero() || rejection.At.After(*newestRejection) {
			*newestRejection = rejection.At
			evidence.BlockedReason = rejection.Reason
		}
	}
}

// signalBelongsToEntry excludes only a positively older accepted signal.
// Missing legacy timing retains the signal rather than inventing a reminder.
func signalBelongsToEntry(signal models.PendingStepCompletionSignal, entryOccurredAt time.Time, entryKnown bool) bool {
	if !entryKnown || signal.SignaledAt.IsZero() {
		return true
	}
	return !signal.SignaledAt.Before(entryOccurredAt)
}

func (s *Service) boundChildStallStepSession(ctx context.Context, taskID, stepID string) (string, error) {
	store, ok := s.repo.(workflowSessionBindingStore)
	if !ok || taskID == "" || stepID == "" {
		return "", nil
	}
	key := workflowSessionBindingTargetKey(stepID)
	binding, err := store.GetWorkflowSessionBinding(ctx, taskID, key)
	if err != nil {
		return "", err
	}
	if binding == nil || binding.TaskID != taskID || binding.TargetKey != key {
		return "", nil
	}
	return binding.SessionID, nil
}

func (p *ChildStallProducer) hasActiveTurn(ctx context.Context, sessionID string) (bool, error) {
	turn, err := p.store.GetActiveTurnBySessionID(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return turn != nil, nil
}

func pendingInputsFromMessages(messages []*models.Message) []childstall.PendingInput {
	inputs := make([]childstall.PendingInput, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		inputs = append(inputs, childstall.PendingInput{
			MessageID:        message.ID,
			ParentQuestionID: pendingParentQuestionID(message),
		})
	}
	return inputs
}

func pendingParentQuestionID(message *models.Message) string {
	if message.Type != models.MessageTypeClarificationRequest || message.Metadata == nil {
		return ""
	}
	isParentQuestion, _ := message.Metadata[models.MetaKeyParentQuestion].(bool)
	status, _ := message.Metadata[models.MetaKeyParentQuestionStatus].(string)
	if !isParentQuestion || status != parentQuestionStatusPendingValue {
		return ""
	}
	questionID, _ := message.Metadata[models.MetaKeyParentQuestionID].(string)
	if questionID == "" {
		questionID = message.ID
	}
	return questionID
}

// promoteHeld decides whether a candidate held on a pending parent question
// becomes an alert. A parent that cannot receive the alert is reported as
// qualified so delivery records the wait and notifies the operator.
func (p *ChildStallProducer) promoteHeld(ctx context.Context, held childstall.Decision, now time.Time) childstall.Decision {
	question, err := p.store.GetMessage(ctx, held.QuestionID)
	if err != nil || question == nil {
		return held
	}
	status, _ := question.Metadata[models.MetaKeyParentQuestionStatus].(string)
	parentID, _ := question.Metadata[models.MetaKeyParentQuestionParentID].(string)
	promotion := childstall.QuestionPromotion{QuestionPending: status == parentQuestionStatusPendingValue}
	if !promotion.QuestionPending || parentID == "" {
		return withQuestion(childstall.PromoteHeld(promotion), held.QuestionID)
	}
	parentSession, unavailable, err := p.resolveParentSession(ctx, parentID)
	if err != nil {
		return held
	}
	if unavailable {
		return withQuestion(childstall.Decision{Outcome: childstall.OutcomeQualified, Cause: childstall.CauseInputRequired}, held.QuestionID)
	}
	if promotion.QuestionQueued, err = p.questionQueuedForParent(ctx, parentSession, held.QuestionID); err != nil {
		return held
	}
	if promotion.ParentIdle, err = p.parentIdle(ctx, parentSession); err != nil {
		return held
	}
	promotion.ParentSettledAfterQuestion, err = p.parentSettledAfter(ctx, parentSession, question.CreatedAt, now)
	if err != nil {
		return held
	}
	return withQuestion(childstall.PromoteHeld(promotion), held.QuestionID)
}

func withQuestion(decision childstall.Decision, questionID string) childstall.Decision {
	if decision.Outcome == childstall.OutcomeQualified || decision.Outcome == childstall.OutcomeHeld {
		decision.QuestionID = questionID
	}
	return decision
}

func (p *ChildStallProducer) questionQueuedForParent(
	ctx context.Context,
	parentSession *models.TaskSession,
	questionID string,
) (bool, error) {
	queue := p.messageQueue()
	if queue == nil {
		return false, errChildStallQueueUnavailable
	}
	identity, err := queue.ResolveSessionIdentity(ctx, parentSession.TaskID, parentSession.ID)
	if err != nil {
		return false, err
	}
	entries, _, err := queue.SnapshotSessionForIdentity(ctx, identity)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if queuedID, _ := entry.Metadata[models.MetaKeyParentQuestionID].(string); queuedID == questionID {
			return true, nil
		}
	}
	return false, nil
}

func (p *ChildStallProducer) parentIdle(ctx context.Context, parentSession *models.TaskSession) (bool, error) {
	if parentSession.State == models.TaskSessionStateRunning || parentSession.State == models.TaskSessionStateStarting {
		return false, nil
	}
	active, err := p.hasActiveTurn(ctx, parentSession.ID)
	return !active, err
}

func (p *ChildStallProducer) parentSettledAfter(
	ctx context.Context,
	parentSession *models.TaskSession,
	questionCreatedAt time.Time,
	now time.Time,
) (bool, error) {
	latest, err := p.store.GetLatestCompletedTurnBySessionID(ctx, parentSession.ID)
	if err != nil {
		return false, err
	}
	if latest == nil || latest.CompletedAt == nil {
		// A primary session started after the question replaced the one that
		// received it, so the question never reached it.
		return parentSession.StartedAt.After(questionCreatedAt), nil
	}
	return latest.StartedAt.After(questionCreatedAt) && now.Sub(*latest.CompletedAt) >= p.policy.SettleGrace, nil
}

// resolveParentSession returns the parent's current primary session and
// whether it cannot receive a prompt (missing, failed, cancelled, completed).
func (p *ChildStallProducer) resolveParentSession(ctx context.Context, parentTaskID string) (*models.TaskSession, bool, error) {
	session, err := p.store.GetPrimarySessionByTaskID(ctx, parentTaskID)
	if errors.Is(err, repoerrors.ErrNoPrimarySession) {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	if session == nil {
		return nil, true, nil
	}
	switch session.State {
	case models.TaskSessionStateFailed, models.TaskSessionStateCancelled, models.TaskSessionStateCompleted:
		return session, true, nil
	}
	return session, false, nil
}

func (p *ChildStallProducer) messageQueue() *messagequeue.Service {
	if p.svc == nil {
		return nil
	}
	return p.svc.messageQueue
}
