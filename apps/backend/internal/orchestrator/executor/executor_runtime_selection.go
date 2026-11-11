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
// selection. An empty selection keeps the existing default behavior. A
// concrete selection without runtime admission fails closed: preparation must
// not perform side effects before lifecycle rejects an unavailable runtime.
func (e *Executor) admitExecutorRuntime(executorType string) error {
	trimmed := strings.TrimSpace(executorType)
	if trimmed == "" {
		return nil
	}
	if e.capabilities == nil {
		return ErrExecutorRuntimeAdmissionUnavailable
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
	config, environment, err := e.resolvePreparedLaunchConfig(ctx, task, session, executorID)
	if err != nil {
		return err
	}
	executorType := strings.TrimSpace(config.ExecutorType)
	if executorType == "" && environment != nil {
		executorType = strings.TrimSpace(environment.ExecutorType)
	}
	if err := e.admitConcreteExecutorRuntime(config); err != nil {
		return err
	}
	if err := e.admitAdditionalExecutorRuntime(config.ExecutorType, executorType); err != nil {
		return err
	}
	return e.admitRetainedPreparedSelection(ctx, task, session, executorType)
}

func (e *Executor) resolvePreparedLaunchConfig(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	executorID string,
) (executorConfig, *models.TaskEnvironment, error) {
	metadata := cloneMetadata(task.Metadata)
	if session.ExecutorProfileID != "" {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["executor_profile_id"] = session.ExecutorProfileID
	}
	config := e.resolveExecutorConfig(ctx, executorID, task.WorkspaceID, metadata)
	if config.ResolutionErr != nil {
		return executorConfig{}, nil, fmt.Errorf("%w: %v", ErrExecutorRuntimeSelectionUnresolved, config.ResolutionErr)
	}
	var environment *models.TaskEnvironment
	if session.TaskEnvironmentID != "" {
		resolved, err := e.repo.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
		if err != nil && taskUsesInheritedWorkspace(task) {
			return executorConfig{}, nil, fmt.Errorf("%w: load inherited environment executor identity", models.ErrWorkspaceReuseUnsafe)
		}
		if err == nil {
			environment = resolved
		}
	}
	if config.ImplicitSelection {
		var err error
		config, err = e.resolveImplicitExecutorForEnvironment(ctx, environment, config)
		if err != nil {
			return executorConfig{}, nil, err
		}
	}
	return config, environment, nil
}

func (e *Executor) admitAdditionalExecutorRuntime(configuredType, effectiveType string) error {
	if effectiveType == "" || effectiveType == configuredType {
		return nil
	}
	return e.admitExecutorRuntime(effectiveType)
}

// admitRetainedPreparedSelection skips retained-runtime comparison for an
// authorized inherited-environment transition. The predecessor is not a
// reuse candidate for the selected environment; lifecycle admission still
// validates the selected runtime after the transition.
func (e *Executor) admitRetainedPreparedSelection(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	executorType string,
) error {
	if inheritedTransitionClaimed(ctx) {
		return nil
	}
	if taskUsesInheritedWorkspace(task) {
		willRebind, err := e.inheritedTransitionWillRebind(ctx, task, session, executorType)
		if err != nil {
			return err
		}
		if willRebind {
			return nil
		}
	}
	return e.admitRetainedExecutionRuntime(ctx, session, executorType)
}

// admitRetainedExecutionRuntime compares a retained execution's recorded
// runtime with the admitted selection before reuse, promotion, resume, or new
// authenticated effects. An empty resolved type is still forwarded so a
// retained binding's own type can be validated by lifecycle.
func (e *Executor) admitRetainedExecutionRuntime(ctx context.Context, session *models.TaskSession, executorType string) error {
	if session == nil {
		return nil
	}
	trimmed := strings.TrimSpace(executorType)
	if trimmed != "" && e.capabilities == nil {
		return ErrExecutorRuntimeAdmissionUnavailable
	}
	admission, ok := e.capabilities.(ExecutorExecutionRuntimeAdmission)
	if !ok {
		if trimmed != "" {
			return ErrExecutorRuntimeAdmissionUnavailable
		}
		return nil
	}
	return admission.ValidateExecutionRuntime(ctx, session.ID, session.TaskEnvironmentID, trimmed)
}
