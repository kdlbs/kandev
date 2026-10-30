package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// queuedTurnStartRestartError ends a claimed queued dispatch before any
// visible side effect because on_turn_start may have changed which session
// owns the task. The entry was already returned to its source queue with the
// on_turn_start marker set, so a workflow profile switch moved it (and its
// attachments) to the destination queue through the ordinary queue transfer.
// redrainSessionID names the session whose queue now delivers the entry; it
// is empty when the transition failed and the entry must stay queued on its
// source session.
type queuedTurnStartRestartError struct {
	sourceSessionID  string
	redrainSessionID string
	turnStartErr     error
}

func (e *queuedTurnStartRestartError) Error() string {
	switch {
	case e.redrainSessionID == "":
		return fmt.Sprintf("queued on_turn_start failed; entry retained on session %s: %v",
			e.sourceSessionID, e.turnStartErr)
	case e.redrainSessionID != e.sourceSessionID:
		return fmt.Sprintf("queued on_turn_start switched session %s to %s",
			e.sourceSessionID, e.redrainSessionID)
	default:
		return fmt.Sprintf("queued on_turn_start applied on session %s", e.sourceSessionID)
	}
}

// queuedTurnStartNeedsRequeue reports whether a claimed queued entry must go
// back to its queue before on_turn_start runs. Only a transition can switch
// the task to another profile session; the profile switch transfers queue
// rows and durable dispatch claims, but a claimed prompt that the source
// worker already owns cannot follow that transfer. Lifecycle and managed
// inputs keep their own receipts and stay on the in-place path.
func (s *Service) queuedTurnStartNeedsRequeue(
	ctx context.Context,
	queuedMsg *messagequeue.QueuedMessage,
	lifecyclePrompt bool,
) bool {
	if lifecyclePrompt || queuedMsg == nil || s.messageQueue == nil ||
		turnStartAlreadyProcessed(queuedMsg.Metadata) {
		return false
	}
	if _, managed := managedInputIDFromQueueMessage(queuedMsg); managed {
		return false
	}
	if s.workflowStepGetter == nil {
		return false
	}
	task, err := s.repo.GetTask(ctx, queuedMsg.TaskID)
	if err != nil || task == nil || task.WorkflowStepID == "" || task.IsEphemeral {
		return false
	}
	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil || step == nil {
		// on_turn_start cannot transition from a step it cannot load either.
		return false
	}
	for _, action := range step.Events.OnTurnStart {
		switch action.Type {
		case wfmodels.OnTurnStartMoveToNext, wfmodels.OnTurnStartMoveToPrevious, wfmodels.OnTurnStartMoveToStep:
			return true
		}
	}
	return false
}

// requeueForQueuedTurnStart returns the claimed entry to the head of its
// source queue with the on_turn_start marker, runs on_turn_start while the
// caller still owns the source dispatch guard, and reports where the entry
// must be delivered. Mirrors the direct send path: on_turn_start runs before
// the user message is recorded or prompted, and the prompt goes to the session
// the transition made current.
func (s *Service) requeueForQueuedTurnStart(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	queuedMsg *messagequeue.QueuedMessage,
	session *models.TaskSession,
) error {
	retained := *queuedMsg
	retained.Metadata = make(map[string]interface{}, len(queuedMsg.Metadata)+1)
	for key, value := range queuedMsg.Metadata {
		retained.Metadata[key] = value
	}
	retained.Metadata[MetaKeyTurnStartAlreadyProcessed] = true
	var requeueErr error
	if identity.SessionIncarnationID != "" {
		requeueErr = s.messageQueue.RequeueAtHeadForSession(ctx, identity, &retained)
	} else {
		requeueErr = s.messageQueue.RequeueAtHead(ctx, &retained)
	}
	if requeueErr != nil {
		return fmt.Errorf("return queued entry before on_turn_start: %w", requeueErr)
	}
	s.publishQueueStatusEvent(ctx, queuedMsg.SessionID)

	workflowCtx := withWorkflowProfileSwitchGuardHeld(ctx, session.ID, "")
	_, turnStartErr := s.processOnTurnStartViaEngineResult(workflowCtx, queuedMsg.TaskID, session)
	restart := &queuedTurnStartRestartError{
		sourceSessionID: session.ID,
		turnStartErr:    turnStartErr,
	}
	primaryID, primaryErr := s.queuedTurnStartPrimarySessionID(ctx, queuedMsg.TaskID)
	switch {
	case primaryErr == nil && primaryID != "" && primaryID != session.ID:
		restart.redrainSessionID = primaryID
	case turnStartErr != nil || primaryErr != nil:
		restart.turnStartErr = errors.Join(turnStartErr, primaryErr)
	default:
		restart.redrainSessionID = session.ID
	}
	return restart
}

func (s *Service) queuedTurnStartPrimarySessionID(ctx context.Context, taskID string) (string, error) {
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("load sessions after queued on_turn_start: %w", err)
	}
	for _, candidate := range sessions {
		if candidate != nil && candidate.IsPrimary && !isTerminalSessionState(candidate.State) {
			return candidate.ID, nil
		}
	}
	return "", nil
}

// finishQueuedTurnStartRestart settles the source worker after the entry was
// returned to a queue. The entry is never acknowledged here: it is delivered
// by the drain of the session that now owns it, or it stays queued.
func (s *Service) finishQueuedTurnStartRestart(
	ctx context.Context,
	queuedMsg *messagequeue.QueuedMessage,
	restart *queuedTurnStartRestartError,
) {
	if restart.redrainSessionID == "" {
		s.logger.Warn("queued on_turn_start failed; message stays queued on its session",
			zap.String("task_id", queuedMsg.TaskID),
			zap.String("session_id", restart.sourceSessionID),
			zap.String("queue_id", queuedMsg.ID),
			zap.Error(restart.turnStartErr))
		return
	}
	s.logger.Info("queued message re-dispatched after on_turn_start",
		zap.String("task_id", queuedMsg.TaskID),
		zap.String("source_session_id", restart.sourceSessionID),
		zap.String("delivery_session_id", restart.redrainSessionID),
		zap.String("queue_id", queuedMsg.ID))
	if restart.redrainSessionID != restart.sourceSessionID {
		s.publishQueueStatusEvent(ctx, restart.sourceSessionID)
		s.publishQueueStatusEvent(ctx, restart.redrainSessionID)
	}
	s.drainQueuedMessageForPromptableSession(ctx, restart.redrainSessionID)
}
