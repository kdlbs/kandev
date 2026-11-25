package service

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

var (
	// ErrOfficeApprovalStage marks an Office approval stage, which is decided
	// by its participants rather than by the session Approve action.
	ErrOfficeApprovalStage = errors.New("office approval stages are not approved through the session")
	// ErrStepNotAwaitingApproval marks a session with nothing to approve.
	ErrStepNotAwaitingApproval = errors.New("step is not awaiting approval")
	// ErrStepApprovalChanged marks an approval whose observed step no longer
	// holds at write time.
	ErrStepApprovalChanged = errors.New("step approval changed")
	// ErrReviewStatusNotRecorded marks an approval whose task move committed
	// but whose review-status write failed. The move is the authoritative
	// effect and must not be repeated.
	ErrReviewStatusNotRecorded = errors.New("approval applied but review status not recorded")
)

// StepApprovalStatus is the approval state of a session's current workflow
// step and the step that approval would move the task to.
type StepApprovalStatus struct {
	Session  *models.TaskSession
	Task     *models.Task
	Step     *wfmodels.WorkflowStep
	NextStep *wfmodels.WorkflowStep
	Awaiting bool
}

// GetStepApprovalStatus reports whether a session's current step awaits the
// Approve action. It awaits when the session is not running or starting,
// review_status is not approved, and either review_status is pending or the
// step has an on_turn_complete transition that requires approval and the
// session's latest turn completed in the current step entry.
func (s *Service) GetStepApprovalStatus(ctx context.Context, sessionID string) (*StepApprovalStatus, error) {
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if err := s.authorizeTaskID(ctx, session.TaskID); err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, session.TaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	status := &StepApprovalStatus{Session: session, Task: task}
	if task.WorkflowStepID != "" && s.workflowStepGetter != nil {
		step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
		if err != nil {
			return nil, fmt.Errorf("failed to get workflow step: %w", err)
		}
		status.Step = step
	}
	if status.Step != nil && status.Step.StageType == wfmodels.StageTypeApproval {
		return nil, ErrOfficeApprovalStage
	}
	status.Awaiting = s.stepAwaitsApproval(ctx, status)
	if status.Awaiting && status.Step != nil {
		if nextID := s.resolveApprovalNextStep(ctx, status.Step); nextID != "" {
			next, err := s.workflowStepGetter.GetStep(ctx, nextID)
			if err != nil {
				return nil, fmt.Errorf("failed to get approval target step: %w", err)
			}
			status.NextStep = next
		}
	}
	return status, nil
}

func (s *Service) stepAwaitsApproval(ctx context.Context, status *StepApprovalStatus) bool {
	switch status.Session.State {
	case models.TaskSessionStateRunning, models.TaskSessionStateStarting:
		return false
	}
	switch status.Session.ReviewStatus {
	case models.ReviewStatusApproved:
		return false
	case models.ReviewStatusPending:
		return true
	}
	if status.Step == nil || !stepRequiresTurnApproval(status.Step) {
		return false
	}
	return s.latestTurnCompletedInCurrentEntry(ctx, status.Session.ID, status.Task)
}

func stepRequiresTurnApproval(step *wfmodels.WorkflowStep) bool {
	for _, action := range step.Events.OnTurnComplete {
		switch action.Type {
		case wfmodels.OnTurnCompleteMoveToNext, wfmodels.OnTurnCompleteMoveToPrevious, wfmodels.OnTurnCompleteMoveToStep:
			if engine.ConfigRequiresApproval(action.Config) {
				return true
			}
		}
	}
	return false
}

// latestTurnCompletedInCurrentEntry reports whether the session's latest
// non-lifecycle turn started in the task's current step entry and completed.
// Turns carry the step and the step-transition entry they started in, so a
// turn from an earlier visit to the same step does not count.
func (s *Service) latestTurnCompletedInCurrentEntry(ctx context.Context, sessionID string, task *models.Task) bool {
	if s.turns == nil {
		return false
	}
	turns, err := s.turns.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		return false
	}
	var latest *models.Turn
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i] != nil && turns[i].Metadata[models.TurnMetaKeyLifecycleOnly] != true {
			latest = turns[i]
			break
		}
	}
	if latest == nil || latest.CompletedAt == nil ||
		models.StringFromAny(latest.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart]) != task.WorkflowStepID {
		return false
	}
	entry, ok := s.currentStepEntry(ctx, task)
	if !ok {
		return false
	}
	turnEntry := int64(0)
	if raw, present := latest.Metadata[models.TurnMetaKeyWorkflowStepTransitionIDAtStart]; present {
		if turnEntry, ok = models.WorkflowEntryIDFromAny(raw); !ok {
			return false
		}
	}
	return turnEntry == entry
}

// currentStepEntry returns the ID of the task's newest step transition, or
// zero when the task has never moved.
func (s *Service) currentStepEntry(ctx context.Context, task *models.Task) (int64, bool) {
	reader, ok := s.tasks.(stepTransitionReader)
	if !ok {
		return 0, false
	}
	transitions, err := reader.ListTaskStepTransitions(ctx, task.ID, 0, 1)
	if err != nil {
		return 0, false
	}
	if len(transitions) == 0 {
		return 0, true
	}
	return transitions[0].ID, true
}

// ApproveAwaitingStep approves the session's step through ApproveSession,
// the native Approve path, after confirming it still awaits approval and the
// caller's guard accepts the observed status.
func (s *Service) ApproveAwaitingStep(ctx context.Context, sessionID string, guard func(*StepApprovalStatus) error) (*StepApprovalStatus, *ApproveSessionResult, error) {
	status, err := s.GetStepApprovalStatus(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	if guard != nil {
		if err := guard(status); err != nil {
			return status, nil, err
		}
	}
	if !status.Awaiting {
		return status, nil, ErrStepNotAwaitingApproval
	}
	result, err := s.approveSession(ctx, sessionID, status.Task.WorkflowStepID)
	if errors.Is(err, ErrReviewStatusNotRecorded) {
		// The move committed. Retry the status write once and report the
		// approval as applied either way, so a replayed idempotency key never
		// stores a failure for a move that already happened.
		if retryErr := s.sessions.UpdateSessionReviewStatus(ctx, sessionID, "approved"); retryErr != nil {
			s.logger.Error("approval moved the task but the review status was not recorded",
				zap.String("session_id", sessionID), zap.Error(retryErr))
		}
		return status, result, nil
	}
	if err != nil {
		return status, nil, err
	}
	return status, result, nil
}
