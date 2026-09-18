package backendapp

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/authz"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (a *taskCreatorAdapter) ValidateMaintenanceReview(ctx context.Context, workspace, repository string) error {
	if err := a.taskSvc.AuthorizeWorkspaceScope(ctx, workspace, authz.ScopeWorkspaceRead); err != nil {
		return err
	}
	repo, err := a.taskSvc.GetRepository(ctx, repository)
	if err != nil || repo.WorkspaceID != workspace {
		return fmt.Errorf("maintenance repository unavailable")
	}
	return nil
}

func (a *taskCreatorAdapter) ValidateMaintenanceSuccess(ctx context.Context, workspace string, evidence shared.Evidence, prepared time.Time) error {
	if err := a.ValidateAssistantEvidence(ctx, workspace, evidence); err != nil {
		return err
	}
	task, err := a.taskSvc.GetTask(ctx, evidence.TaskID)
	if err != nil || task.WorkspaceID != workspace || task.State != v1.TaskStateCompleted {
		return fmt.Errorf("maintenance recovery task is not complete")
	}
	sessions, err := a.taskSvc.ListTaskSessions(ctx, task.ID)
	if err != nil {
		return err
	}
	if err = a.validateMaintenanceSessions(sessions, evidence, prepared); err != nil {
		return err
	}
	if err = a.validateAssistantTaskSummary(ctx, task.ID); err != nil {
		return err
	}
	if a.workflow == nil {
		return fmt.Errorf("native review verification unavailable")
	}
	if err = validateOrchestratedCompletion(ctx, &Repositories{Workflow: a.workflow}, task.WorkflowStepID, task.ID); err != nil {
		return err
	}
	return a.validateAssistantReviews(ctx, task.ID)
}

func (a *taskCreatorAdapter) validateMaintenanceSessions(sessions []*models.TaskSession, evidence shared.Evidence, prepared time.Time) error {
	if a.orch == nil {
		return fmt.Errorf("native activity verification unavailable")
	}
	var success *models.TaskSession
	for _, s := range sessions {
		if s.ID == evidence.SessionID {
			success = s
		}
	}
	if !validMaintenanceSession(success, evidence.TaskID, prepared) {
		return fmt.Errorf("subsequent completed native session required")
	}
	for _, s := range sessions {
		if s.TaskID != evidence.TaskID || s.StartedAt.After(success.StartedAt) || a.orch.HasOutstandingSessionWork(s.ID) {
			return fmt.Errorf("newer or outstanding native work prevents resolution")
		}
		switch s.State {
		case models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		default:
			return fmt.Errorf("native session is not settled")
		}
	}
	return nil
}

func validMaintenanceSession(s *models.TaskSession, task string, prepared time.Time) bool {
	return s != nil && s.TaskID == task && s.State == models.TaskSessionStateCompleted && s.StartedAt.After(prepared) && s.CompletedAt != nil && !s.CompletedAt.Before(s.StartedAt)
}

func (a *taskCreatorAdapter) FindMaintenanceSuccess(ctx context.Context, workspace, taskID, profile string, prepared time.Time) (*shared.MaintenanceSuccess, error) {
	task, err := a.taskSvc.GetTask(ctx, taskID)
	if err != nil || task.WorkspaceID != workspace || task.State != v1.TaskStateCompleted {
		return nil, nil
	}
	sessions, err := a.taskSvc.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	latest := latestMaintenanceSession(sessions)
	if latest == nil {
		return nil, nil
	}
	usedProfile := latest.ExecutionProfileID
	if usedProfile == "" {
		usedProfile = latest.AgentProfileID
	}
	if usedProfile != profile || latest.CompletedAt == nil || !latest.StartedAt.After(prepared) {
		return nil, nil
	}
	return a.findMaintenanceResult(ctx, task, latest, prepared)
}

func latestMaintenanceSession(sessions []*models.TaskSession) *models.TaskSession {
	var latest *models.TaskSession
	for _, s := range sessions {
		if latest == nil || s.StartedAt.After(latest.StartedAt) {
			latest = s
		}
	}
	return latest
}

func (a *taskCreatorAdapter) findMaintenanceResult(ctx context.Context, task *models.Task, latest *models.TaskSession, prepared time.Time) (*shared.MaintenanceSuccess, error) {
	messages, _, err := a.taskSvc.ListMessagesPaginated(ctx, taskservice.ListMessagesRequest{TaskSessionID: latest.ID, Limit: 20, AuthorType: string(models.MessageAuthorAgent), Sort: workspaceResultSortDescending})
	if err != nil {
		return nil, err
	}
	for _, m := range messages {
		if m.Type != models.MessageTypeMessage || m.Content == "" {
			continue
		}
		e := shared.Evidence{SourceKind: "task_message", TaskID: task.ID, SessionID: latest.ID, SourceID: m.ID}
		if a.ValidateMaintenanceSuccess(ctx, task.WorkspaceID, e, prepared) != nil {
			return nil, nil
		}
		return &shared.MaintenanceSuccess{ID: m.ID, TaskID: task.ID, TaskTitle: task.Title, SessionID: latest.ID, SourceID: m.ID, CompletedAt: *latest.CompletedAt}, nil
	}
	return nil, nil
}
