package executor

import (
	"context"
	"errors"
	"fmt"
	"time"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

type staleExecutionByIdentityCleaner interface {
	CleanupStaleExecutionBySessionIDIfCurrent(context.Context, string, string, time.Time) error
}

func (e *Executor) claimInheritedEnvironmentTransition(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	executorID string,
	running *models.ExecutorRunning,
) error {
	if !taskUsesInheritedWorkspace(task) {
		return nil
	}
	metadata := cloneMetadata(task.Metadata)
	if session.ExecutorProfileID != "" {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["executor_profile_id"] = session.ExecutorProfileID
	}
	selected := e.resolveExecutorConfig(ctx, executorID, task.WorkspaceID, metadata)
	if selected.ImplicitSelection && session.TaskEnvironmentID != "" {
		environment, err := e.repo.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
		if err != nil {
			return fmt.Errorf("%w: load inherited environment executor identity", models.ErrWorkspaceReuseUnsafe)
		}
		selected, err = e.resolveImplicitExecutorForEnvironment(ctx, environment, selected)
		if err != nil {
			return err
		}
	}
	if selected.ExecutorType == "" {
		return fmt.Errorf("%w: selected executor type could not be verified", models.ErrWorkspaceReuseUnsafe)
	}
	if err := e.admitInheritedTransitionRuntime(ctx, task, session, selected.ExecutorType, running); err != nil {
		return err
	}
	claimer, ok := e.repo.(inheritedTaskEnvironmentTransitionClaimer)
	if !ok {
		return fmt.Errorf("%w: inherited workspace transition admission is unavailable", models.ErrWorkspaceReuseUnsafe)
	}
	environment, handled, _, err := claimer.ClaimInheritedTaskEnvironmentTransition(
		ctx, task.ID, session.ID, session.TaskEnvironmentID,
		selected.ExecutorType, selected.ExecutorID, session.ExecutorProfileID,
	)
	if err != nil {
		return err
	}
	if !handled {
		return nil
	}
	if environment == nil {
		return fmt.Errorf("%w: inherited workspace transition returned no canonical environment", models.ErrWorkspaceReuseUnsafe)
	}
	session.TaskEnvironmentID = environment.ID
	session.WorkspacePath = environment.WorkspacePath
	return nil
}

func (e *Executor) admitInheritedTransitionRuntime(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	selectedExecutorType string,
	running *models.ExecutorRunning,
) error {
	willRebind, err := e.inheritedTransitionWillRebind(ctx, task, session, selectedExecutorType)
	if err != nil {
		return err
	}
	if willRebind && e.executorHasActiveAgent(ctx, session, running) {
		return ErrExecutionAlreadyRunning
	}
	return nil
}

func (e *Executor) inheritedTransitionWillRebind(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	selectedExecutorType string,
) (bool, error) {
	if !taskUsesInheritedWorkspace(task) {
		return false, nil
	}
	canonical, err := e.repo.GetTaskEnvironmentByTaskID(ctx, task.ID)
	if err != nil {
		return false, fmt.Errorf("%w: load canonical environment before inherited transition", models.ErrWorkspaceReuseUnsafe)
	}
	if canonical != nil && canonical.ID != session.TaskEnvironmentID {
		return true, nil
	}
	if session.TaskEnvironmentID == "" {
		return false, nil
	}
	source, err := e.repo.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
	if err != nil {
		return false, fmt.Errorf("%w: load inherited environment before transition", models.ErrWorkspaceReuseUnsafe)
	}
	if source == nil {
		return false, fmt.Errorf("%w: inherited environment is unavailable before transition", models.ErrWorkspaceReuseUnsafe)
	}
	sourceExecutorType, known := source.EffectiveExecutorType()
	if !known {
		return false, fmt.Errorf("%w: inherited environment executor could not be verified", models.ErrWorkspaceReuseUnsafe)
	}
	if source.TaskID == task.ID || string(sourceExecutorType) == selectedExecutorType ||
		sourceExecutorType == models.ExecutorTypeKubernetes || selectedExecutorType == string(models.ExecutorTypeKubernetes) {
		return false, nil
	}
	return true, nil
}

func (e *Executor) retireInheritedRuntimeForTransition(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	environment *models.TaskEnvironment,
	selectedExecutorType string,
	running *models.ExecutorRunning,
) error {
	if !taskUsesInheritedWorkspace(task) || environment == nil || environment.TaskID != task.ID {
		return nil
	}
	if err := validateInheritedRuntimeTransitionTarget(environment, selectedExecutorType); err != nil {
		return err
	}
	if running == nil {
		return e.refuseUntrackedInheritedRuntime(ctx, session.ID, models.ErrWorkspaceReuseUnsafe)
	}
	oldEnvironmentID, _ := running.Metadata[runtimeapi.MetadataKeyTaskEnvironmentID].(string)
	if oldEnvironmentID == "" {
		return fmt.Errorf("%w: existing runtime environment identity is unavailable", models.ErrWorkspaceReuseUnsafe)
	}
	if oldEnvironmentID == environment.ID {
		return nil
	}
	if running.AgentExecutionID == "" {
		return e.refuseUntrackedInheritedRuntime(ctx, session.ID, ErrExecutionAlreadyRunning)
	}
	if e.agentManager.IsAgentRunningForSession(ctx, session.ID) {
		return ErrExecutionAlreadyRunning
	}
	return e.cleanupInheritedRuntime(ctx, task, session, environment, running)
}

func validateInheritedRuntimeTransitionTarget(environment *models.TaskEnvironment, selectedExecutorType string) error {
	environmentType, known := environment.EffectiveExecutorType()
	if environment.ID == "" || !known {
		return fmt.Errorf("%w: canonical child runtime environment identity is incomplete", models.ErrWorkspaceReuseUnsafe)
	}
	if string(environmentType) != selectedExecutorType {
		return fmt.Errorf("%w: canonical task environment uses executor %q; selected executor is %q",
			models.ErrWorkspaceExecutorMismatch, environmentType, selectedExecutorType)
	}
	return nil
}

func (e *Executor) refuseUntrackedInheritedRuntime(ctx context.Context, sessionID string, refusal error) error {
	executionID, err := e.currentExecutionIDForSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load inherited runtime identity: %w", err)
	}
	if executionID != "" {
		return fmt.Errorf("%w: inherited runtime %s has no matching durable runtime identity", refusal, executionID)
	}
	return nil
}

func (e *Executor) currentExecutionIDForSession(ctx context.Context, sessionID string) (string, error) {
	executionID, err := e.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if errors.Is(err, runtimeapi.ErrNoExecutionForSession) || runtimeapi.IsNotFound(err) || errors.Is(err, ErrExecutionNotFound) {
		return "", nil
	}
	return executionID, err
}

func (e *Executor) cleanupInheritedRuntime(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	environment *models.TaskEnvironment,
	running *models.ExecutorRunning,
) error {
	cleaner, ok := e.agentManager.(staleExecutionByIdentityCleaner)
	if !ok {
		return fmt.Errorf("%w: identity-aware runtime cleanup is unavailable", models.ErrWorkspaceReuseUnsafe)
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := cleaner.CleanupStaleExecutionBySessionIDIfCurrent(
		cleanupCtx, session.ID, running.AgentExecutionID, running.UpdatedAt,
	); err != nil && !isAbsentInheritedExecutionError(err) {
		return fmt.Errorf("retire inherited session runtime: %w", err)
	}
	if err := e.verifyInheritedRuntimeRetired(cleanupCtx, session.ID); err != nil {
		return err
	}
	e.logger.Info("retired inherited session runtime after executor transition",
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.String("task_environment_id", environment.ID),
		zap.String("executor_type", environment.ExecutorType))
	return nil
}

func isAbsentInheritedExecutionError(err error) bool {
	return runtimeapi.IsNotFound(err) || errors.Is(err, runtimeapi.ErrNoExecutionForSession)
}

func (e *Executor) verifyInheritedRuntimeRetired(ctx context.Context, sessionID string) error {
	executionID, err := e.currentExecutionIDForSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("verify inherited runtime retirement: %w", err)
	}
	if executionID != "" {
		return fmt.Errorf("%w: inherited runtime %s still owns session %s", ErrExecutionAlreadyRunning, executionID, sessionID)
	}
	return nil
}
