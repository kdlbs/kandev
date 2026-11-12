package dashboard

import (
	"context"
	"fmt"
)

// FinalStepMoveReasonCompletionGate and FinalStepMoveReasonWorkflowMove
// identify why a human Done move to the final step was refused. A pending
// approver refusal uses ApprovalGateReasonApprovals.
const (
	FinalStepMoveReasonCompletionGate = "completion_gate"
	FinalStepMoveReasonWorkflowMove   = "workflow_move"
)

// TaskFinalStepMover moves a task to its workflow's final step through the
// human step-move path, so that step's completion setting, approver guard,
// completion gate and entry actions apply.
type TaskFinalStepMover interface {
	MoveTaskToFinalStep(ctx context.Context, taskID string) (*FinalStepMoveResult, error)
}

// FinalStepMoveResult is the task after a final-step move. The mover sets
// State (the persisted task state), StepID and StepName;
// UpdateTaskStatusAsHuman fills Status in the Office vocabulary.
type FinalStepMoveResult struct {
	State    string
	Status   string
	StepID   string
	StepName string
	// Queued reports a move that is waiting for room under the step's WIP
	// limit; the task keeps its prior state until the step admits it.
	Queued bool
}

// FinalStepMoveRefusedError reports a final-step move that was refused
// before anything was written.
type FinalStepMoveRefusedError struct {
	Reason  string
	Code    string
	Pending []PendingApproverDTO
	Err     error
}

func (e *FinalStepMoveRefusedError) Error() string {
	return fmt.Sprintf("move to final workflow step refused (%s): %v", e.Reason, e.Err)
}

func (e *FinalStepMoveRefusedError) Unwrap() error { return e.Err }

// SetTaskFinalStepMover wires the final-step move used for human Done
// requests on a task that is not on its workflow's final step.
func (s *DashboardService) SetTaskFinalStepMover(m TaskFinalStepMover) {
	s.finalStepMover = m
}

// UpdateTaskStatusAsHuman applies a user's status change. A Done request on
// a task that is not on its workflow's final step moves the task to that step
// instead of redirecting it to in_review; the move owns every write and event,
// so a refused move changes nothing. Any other request goes through
// UpdateTaskStatus. The result is non-nil only when the move ran.
func (s *DashboardService) UpdateTaskStatusAsHuman(
	ctx context.Context, req TaskStatusUpdateRequest,
) (*FinalStepMoveResult, error) {
	if normaliseStatus(req.NewStatus) != stateCompleted || s.finalStepMover == nil {
		return nil, s.UpdateTaskStatus(ctx, req)
	}
	terminal, hasStep, err := s.repo.IsTaskWorkflowStepTerminal(ctx, req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("human done: resolve workflow step: %w", err)
	}
	if !hasStep || terminal {
		if s.humanDoneCompleter != nil {
			return s.completeHumanDone(ctx, req)
		}
		return nil, s.UpdateTaskStatus(ctx, req)
	}
	result, err := s.finalStepMover.MoveTaskToFinalStep(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	result.Status = dbStateToOfficeStatus(result.State)
	s.maybeCreateStatusComment(ctx, req)
	return result, nil
}

// TaskHumanDoneCompleter applies explicit human Done under task admission.
type TaskHumanDoneCompleter interface {
	CompleteOfficeTaskAsHuman(context.Context, string, string) (*FinalStepMoveResult, error)
}

func (s *DashboardService) SetTaskHumanDoneCompleter(c TaskHumanDoneCompleter) {
	s.humanDoneCompleter = c
}

func (s *DashboardService) completeHumanDone(ctx context.Context, req TaskStatusUpdateRequest) (*FinalStepMoveResult, error) {
	stepID, err := s.repo.GetTaskWorkflowStepID(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	preStatus := ""
	if fields, err := s.repo.GetTaskExecutionFields(ctx, req.TaskID); err == nil && fields != nil {
		preStatus = fields.State
	}
	result, err := s.humanDoneCompleter.CompleteOfficeTaskAsHuman(ctx, req.TaskID, stepID)
	if err != nil {
		return nil, err
	}
	result.Status = dbStateToOfficeStatus(result.State)
	commentID := s.maybeCreateStatusComment(ctx, req)
	if !req.SuppressStatusActivity {
		s.logTaskStatusChangeActivity(ctx, req)
	}
	s.publishTaskStatusChanged(ctx, req)
	s.publishCanonicalTaskUpdated(ctx, req.TaskID)
	s.runReactivityForStatus(ctx, req, commentID, preStatus)
	return result, nil
}
