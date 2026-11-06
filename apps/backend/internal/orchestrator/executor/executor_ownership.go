package executor

import (
	"context"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (e *Executor) SetExecutorAdmission(admit func(context.Context, string, string, string) (context.Context, error)) {
	e.executorAdmission = admit
}

func (e *Executor) SetAutonomousExecutorContext(resolve func(context.Context, string, string) (context.Context, error)) {
	e.autonomousExecutorContext = resolve
}

func (e *Executor) prepareExecutorAdmission(ctx context.Context, task *v1.Task, session *models.TaskSession, config executorConfig) (context.Context, error) {
	if e.executorAdmission == nil {
		return ctx, nil
	}
	selectedProfile, _ := config.Metadata["executor_profile_id"].(string)
	if session.ExecutorProfileID != "" {
		selectedProfile = session.ExecutorProfileID
	}
	ctx, err := e.prepareAutonomousExecutorActor(ctx, task, selectedProfile)
	if err != nil {
		return ctx, err
	}
	environment, err := e.resolveEnvironmentForAdmission(ctx, task.ID, session.TaskEnvironmentID)
	if err != nil {
		return ctx, err
	}
	if environment != nil {
		ctx, err = e.executorAdmission(ctx, "", environment.ExecutorID, environment.ExecutorProfileID)
		if err != nil {
			return ctx, err
		}
	}
	session.ExecutorProfileID = selectedProfile
	return e.executorAdmission(ctx, "", config.ExecutorID, selectedProfile)
}

func (e *Executor) prepareAutonomousExecutorActor(ctx context.Context, task *v1.Task, profileID string) (context.Context, error) {
	if _, interactive := authn.IdentityFromContext(ctx); interactive {
		return ctx, nil
	}
	if _, attributed := models.ExecutorActorFromContext(ctx); attributed || e.autonomousExecutorContext == nil {
		return ctx, nil
	}
	creator := ""
	if reader, ok := e.repo.(interface {
		GetTaskExecutorActor(context.Context, string) (string, error)
	}); ok {
		var err error
		creator, err = reader.GetTaskExecutorActor(ctx, task.ID)
		if err != nil {
			return ctx, err
		}
	}
	return e.autonomousExecutorContext(models.WithExecutorWorkspace(ctx, task.WorkspaceID), creator, profileID)
}
