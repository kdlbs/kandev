package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
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
		return executorMutationDenial(err)
	}
	tasks, err := s.tasks.ListTasks(ctx, workflowID)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if err := s.authorizeTaskExecutorMutation(ctx, task); err != nil {
			return executorMutationDenial(err)
		}
	}
	return nil
}

// executorMutationDenial marks an executor eligibility refusal so callers
// outside the task service can report it as forbidden rather than as a
// missing resource of their own.
func executorMutationDenial(err error) error {
	if errors.Is(err, repoerrors.ErrExecutorProfileNotFound) || errors.Is(err, models.ErrExecutorNotFound) {
		return fmt.Errorf("%w: %w", models.ErrExecutorMutationDenied, err)
	}
	return err
}

// authorizeTaskExecutorMutation checks the bindings a task edit could hand to
// the next launch. A binding that admission refuses for every caller confers
// no compute, so it does not block the edit; a resolvable binding must be
// usable by the caller.
func (s *Service) authorizeTaskExecutorMutation(ctx context.Context, task *models.Task) error {
	if _, interactive := authn.IdentityFromContext(ctx); !interactive {
		return nil
	}
	profileID, _ := task.Metadata[models.MetaKeyExecutorProfileID].(string)
	executorID, _ := task.Metadata[models.MetaKeyExecutorID].(string)
	if err := s.authorizeRetainedExecutor(ctx, executorID, profileID); err != nil {
		return err
	}
	sessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, task.ID)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if err := s.authorizeRetainedSessionExecutor(ctx, session); err != nil {
			return err
		}
	}
	return nil
}

// authorizeTaskExecutorSelection checks newly supplied executor bindings at
// task creation and update, before they can become retained launch selections.
func (s *Service) authorizeTaskExecutorSelection(ctx context.Context, task *models.Task, metadata map[string]interface{}) error {
	if metadata == nil {
		return nil
	}
	if _, interactive := authn.IdentityFromContext(ctx); !interactive {
		return nil
	}
	if profileID, changed := changedTaskExecutorKey(task, metadata, models.MetaKeyExecutorProfileID); changed {
		if err := s.AuthorizeExecutorProfile(ctx, profileID); err != nil {
			return err
		}
	}
	if executorID, changed := changedTaskExecutorKey(task, metadata, models.MetaKeyExecutorID); changed {
		if err := s.AuthorizeExecutorConfiguration(ctx, executorID, ""); err != nil {
			return err
		}
	}
	return nil
}

func changedTaskExecutorKey(task *models.Task, metadata map[string]interface{}, key string) (string, bool) {
	requested, _ := metadata[key].(string)
	current, _ := task.Metadata[key].(string)
	return requested, requested != "" && requested != current
}

func (s *Service) authorizeRetainedSessionExecutor(ctx context.Context, session *models.TaskSession) error {
	if err := s.authorizeRetainedExecutor(ctx, session.ExecutorID, session.ExecutorProfileID); err != nil {
		return err
	}
	if session.TaskEnvironmentID == "" {
		return nil
	}
	env, err := s.taskEnvironments.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
	if errors.Is(err, repoerrors.ErrTaskEnvironmentNotFound) {
		return nil
	}
	if err != nil || env == nil {
		return err
	}
	return s.authorizeRetainedExecutor(ctx, env.ExecutorID, env.ExecutorProfileID)
}

// authorizeRetainedExecutor applies the strict ownership check to every
// stored binding that still resolves. A missing profile falls back to the
// stored executor, and storage failures remain authoritative.
func (s *Service) authorizeRetainedExecutor(ctx context.Context, executorID, profileID string) error {
	if profileID != "" {
		live, err := s.retainedExecutorProfileLive(ctx, profileID)
		if err != nil {
			return err
		}
		if live {
			return s.AuthorizeExecutorProfile(ctx, profileID)
		}
	}
	if executorID == "" {
		return nil
	}
	live, err := s.retainedExecutorLive(ctx, executorID)
	if err != nil || !live {
		return err
	}
	return s.AuthorizeExecutorConfiguration(ctx, executorID, "")
}

func (s *Service) retainedExecutorProfileLive(ctx context.Context, profileID string) (bool, error) {
	profile, err := s.executors.GetExecutorProfile(ctx, profileID)
	if errors.Is(err, repoerrors.ErrExecutorProfileNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return s.retainedExecutorLive(ctx, profile.ExecutorID)
}

func (s *Service) retainedExecutorLive(ctx context.Context, executorID string) (bool, error) {
	resolved, err := s.executorAdmissionID(ctx, executorID)
	if errors.Is(err, models.ErrExecutorNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = s.executors.GetExecutor(ctx, resolved)
	if errors.Is(err, models.ErrExecutorNotFound) {
		return false, nil
	}
	return err == nil, err
}
