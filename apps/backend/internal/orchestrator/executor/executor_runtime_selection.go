package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ErrExecutorRuntimeAdmissionUnavailable means the concrete executor selection
// could not be validated because runtime admission is not wired. It fails
// closed: an unvalidated concrete selection never starts work.
var ErrExecutorRuntimeAdmissionUnavailable = errors.New("executor runtime admission is unavailable")

// ErrExecutorRuntimeSelectionUnresolved means an executor ID remains selected
// after its type lookup failed. Such a selection must never fall back to the
// unspecified standalone path.
var ErrExecutorRuntimeSelectionUnresolved = errors.New("executor runtime selection is unresolved")

// admitExecutorRuntime validates the registered backend for a concrete
// selection. An empty selection keeps the existing default behavior. When the
// optional capability is not wired (isolated tests), lifecycle launch still
// enforces the required runtime before any effect.
func (e *Executor) admitExecutorRuntime(executorType string) error {
	trimmed := strings.TrimSpace(executorType)
	if trimmed == "" || e.capabilities == nil {
		return nil
	}
	return e.capabilities.ValidateExecutorRuntime(trimmed)
}

// admitConcreteExecutorRuntime rejects an unresolved concrete selection before
// admitting its mapped runtime. It performs no credential, script, workspace,
// or runtime work.
func (e *Executor) admitConcreteExecutorRuntime(config executorConfig) error {
	if config.ResolutionErr != nil {
		return fmt.Errorf("%w: %v", ErrExecutorRuntimeSelectionUnresolved, config.ResolutionErr)
	}
	executorID := strings.TrimSpace(config.ExecutorID)
	executorType := strings.TrimSpace(config.ExecutorType)
	if executorID != "" && executorType == "" {
		return fmt.Errorf("%w: executor %q", ErrExecutorRuntimeSelectionUnresolved, executorID)
	}
	return e.admitExecutorRuntime(executorType)
}

// admitPreparedLaunchRuntime resolves the effective prepared-session
// selection and validates the retained candidate before profile replacement
// cleanup, workspace recovery, credential resolution, or existing-workspace
// success. The incoming request selection is authoritative even when it
// differs from the execution's old type.
func (e *Executor) admitPreparedLaunchRuntime(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	executorID string,
) error {
	metadata := cloneMetadata(task.Metadata)
	if session.ExecutorProfileID != "" {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["executor_profile_id"] = session.ExecutorProfileID
	}
	config := e.resolveExecutorConfig(ctx, executorID, task.WorkspaceID, metadata)
	if config.ResolutionErr != nil {
		return fmt.Errorf("%w: %v", ErrExecutorRuntimeSelectionUnresolved, config.ResolutionErr)
	}
	executorType := strings.TrimSpace(config.ExecutorType)
	if executorType == "" && session.TaskEnvironmentID != "" {
		// Environment resolution is best-effort here: the launch path below
		// owns inherited-environment failure classification and will repeat
		// runtime admission with the resolved environment type.
		if environment, err := e.repo.GetTaskEnvironment(ctx, session.TaskEnvironmentID); err == nil && environment != nil {
			executorType = strings.TrimSpace(environment.ExecutorType)
		}
	}
	if err := e.admitConcreteExecutorRuntime(config); err != nil {
		return err
	}
	if executorType != "" && executorType != config.ExecutorType {
		if err := e.admitExecutorRuntime(executorType); err != nil {
			return err
		}
	}
	return e.admitRetainedExecutionRuntime(ctx, session, executorType)
}

// admitRetainedExecutionRuntime compares a retained execution's recorded
// runtime with the admitted selection before reuse, promotion, resume, or new
// authenticated effects.
func (e *Executor) admitRetainedExecutionRuntime(ctx context.Context, session *models.TaskSession, executorType string) error {
	trimmed := strings.TrimSpace(executorType)
	if session == nil || trimmed == "" {
		return nil
	}
	admission, ok := e.capabilities.(ExecutorExecutionRuntimeAdmission)
	if !ok {
		// Lifecycle's own reuse admission remains the required backstop.
		return nil
	}
	return admission.ValidateExecutionRuntime(ctx, session.ID, session.TaskEnvironmentID, trimmed)
}
