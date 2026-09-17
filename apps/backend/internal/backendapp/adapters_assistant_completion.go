package backendapp

import (
	"context"
	"fmt"

	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
)

func (a *taskCreatorAdapter) validateAssistantEntry(ctx context.Context, workflowID string, spec shared.WorkspaceTaskSpec) error {
	if spec.ExecutionMode == "" && spec.WorkflowStepID == "" {
		return nil
	}
	if spec.ExecutionMode != "" && spec.ExecutionMode != "execute" && spec.ExecutionMode != "design" {
		return fmt.Errorf("answer and inspect require no delivery task")
	}
	if spec.WorkflowStepID == "" {
		return nil
	} // configured core start/plan resolution
	if a.workflow == nil {
		return fmt.Errorf("workflow entry validation unavailable")
	}
	step, err := a.workflow.GetStep(ctx, spec.WorkflowStepID)
	if err != nil || step.WorkflowID != workflowID {
		return fmt.Errorf("entry step must belong to the selected workflow")
	}
	if !step.IsStartStep && !step.AllowManualMove {
		return fmt.Errorf("workflow policy does not permit entry at this step")
	}
	return nil
}

func (a *taskCreatorAdapter) ValidateAssistantEvidence(ctx context.Context, ws string, e shared.Evidence) error {
	task, err := a.taskSvc.GetTask(ctx, e.TaskID)
	if err != nil || task.WorkspaceID != ws {
		return fmt.Errorf("evidence task unavailable")
	}
	m, err := a.taskSvc.GetMessage(ctx, e.SourceID)
	if err != nil || m.TaskID != e.TaskID || m.TaskSessionID != e.SessionID || m.AuthorType != models.MessageAuthorAgent || m.Type != models.MessageTypeMessage || m.Content == "" {
		return fmt.Errorf("evidence must reference a persisted result from the linked task and session")
	}
	return nil
}

func (a *taskCreatorAdapter) ValidateAssistantTaskCompletion(ctx context.Context, ws, id string) error {
	task, err := a.taskSvc.GetTask(ctx, id)
	if err != nil || task.WorkspaceID != ws {
		return fmt.Errorf("task unavailable")
	}
	sessions, err := a.taskSvc.ListTaskSessions(ctx, id)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		return fmt.Errorf("no worker result exists")
	}
	for _, s := range sessions {
		switch s.State {
		case models.TaskSessionStateCreated, models.TaskSessionStateStarting, models.TaskSessionStateRunning:
			return fmt.Errorf("worker session is active")
		case models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
			return fmt.Errorf("worker session needs failure review")
		}
		if s.ReviewStatus == models.ReviewStatusPending {
			return fmt.Errorf("session review is pending")
		}
		if a.orch == nil {
			return fmt.Errorf("live activity verification unavailable")
		}
		if a.orch.HasOutstandingSessionWork(s.ID) {
			return fmt.Errorf("worker background activity is active")
		}
	}
	if err := a.validateAssistantTaskSummary(ctx, id); err != nil {
		return err
	}
	if a.workflow == nil {
		return fmt.Errorf("review gate verification unavailable")
	}
	if err := validateOrchestratedCompletion(ctx, &Repositories{Workflow: a.workflow}, task.WorkflowStepID, id); err != nil {
		return err
	}
	return a.validateAssistantReviews(ctx, id)
}

func (a *taskCreatorAdapter) validateAssistantTaskSummary(ctx context.Context, id string) error {
	if a.taskRepo == nil {
		return fmt.Errorf("activity verification unavailable")
	}
	rows, err := a.taskRepo.LoadTaskStatusSummaries(ctx, []string{id})
	if err != nil {
		return err
	}
	s := rows[id]
	if s == nil {
		return fmt.Errorf("task activity summary unavailable")
	}
	if s.ActiveSubagentCount > 0 || s.QueuedPromptCount > 0 || s.ForegroundActivity != "" || s.PendingAction != "" || s.ActiveError != nil {
		return fmt.Errorf("task has outstanding activity, input or errors")
	}
	if a.orch != nil && a.orch.GetMessageQueue() != nil {
		count, err := a.orch.GetMessageQueue().CountPendingByTask(ctx, id)
		if err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("worker input is queued")
		}
	}
	return nil
}

func (a *taskCreatorAdapter) validateAssistantReviews(ctx context.Context, id string) error {
	active, err := a.taskRepo.ListActiveTaskReviewRuns(ctx, id)
	if err != nil {
		return err
	}
	if len(active) > 0 {
		return fmt.Errorf("code review is active")
	}
	runs, err := a.taskRepo.ListTaskReviewRuns(ctx, id, 1)
	if err != nil {
		return err
	}
	if len(runs) > 0 && (runs[0].Status == models.ReviewRunFailed || runs[0].Status == models.ReviewRunCancelled) {
		return fmt.Errorf("latest code review failed or was cancelled")
	}
	findings, err := a.taskRepo.ListTaskReviewFindings(ctx, id)
	if err != nil {
		return err
	}
	for _, f := range findings {
		if f.Status == models.ReviewFindingOpen && (f.Severity == models.ReviewSeverityBlocker || f.Severity == models.ReviewSeverityMajor) {
			return fmt.Errorf("required code review findings are open")
		}
	}
	return nil
}
