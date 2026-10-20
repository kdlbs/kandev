package service

import (
	"context"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

// AuthorizeWorkflowExecutorMutation prevents workflow edits from lending a
// source creator's private compute to another workspace member.
func (s *Service) AuthorizeWorkflowExecutorMutation(ctx context.Context, workflowID string) error {
	if _, interactive := authn.IdentityFromContext(ctx); !interactive {
		return nil
	}
	workflow, err := s.workflows.GetWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}
	_, err = s.PrepareAutonomousExecutor(models.WithExecutorWorkspace(ctx, workflow.WorkspaceID), workflow.CreatedByUserID, "")
	if err != nil {
		return err
	}
	tasks, err := s.tasks.ListTasks(ctx, workflowID)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if err := s.authorizeTaskExecutorMutation(ctx, task); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) authorizeTaskExecutorMutation(ctx context.Context, task *models.Task) error {
	if _, interactive := authn.IdentityFromContext(ctx); !interactive {
		return nil
	}
	profileID, _ := task.Metadata[models.MetaKeyExecutorProfileID].(string)
	executorID, _ := task.Metadata["executor_id"].(string)
	if err := s.AuthorizeExecutorConfiguration(ctx, executorID, profileID); err != nil {
		return err
	}
	sessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, task.ID)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if err := s.authorizeSessionExecutor(ctx, session); err != nil {
			return err
		}
	}
	return nil
}
