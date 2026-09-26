package backendapp

import (
	"context"
	"errors"

	cursorcloudruntime "github.com/kandev/kandev/internal/agent/runtime/cursorcloud"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type cursorCloudSubmissionResolution struct {
	OperationID string                                   `json:"operationId"`
	State       models.ManagedAgentSubmissionState       `json:"state"`
	Candidates  []cursorcloudruntime.SubmissionCandidate `json:"candidates"`
}

func (m *cursorCloudAgentManager) getSubmissionResolution(
	ctx context.Context,
	taskID, sessionID string,
) (*cursorCloudSubmissionResolution, error) {
	binding, err := m.authorizedCloudBinding(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	latest, err := m.repo.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil {
		return nil, err
	}
	if !needsSubmissionCandidateScan(latest) {
		return &cursorCloudSubmissionResolution{
			OperationID: latest.ID, State: latest.State, Candidates: []cursorcloudruntime.SubmissionCandidate{},
		}, nil
	}
	operation, candidates, err := m.managedRuntime.ListSubmissionCandidates(ctx, binding.ExecutionID)
	if err != nil {
		return nil, err
	}
	return &cursorCloudSubmissionResolution{
		OperationID: operation.ID, State: operation.State, Candidates: candidates,
	}, nil
}

func needsSubmissionCandidateScan(operation *models.ManagedAgentOperation) bool {
	return operation != nil && operation.Kind == models.ManagedAgentOperationFollowup &&
		(operation.State == models.ManagedAgentSubmissionUnknown ||
			operation.State == models.ManagedAgentSubmissionSubmitting)
}

func (m *cursorCloudAgentManager) bindSubmissionCandidate(
	ctx context.Context,
	taskID, sessionID, runID string,
) (*models.ManagedAgentOperation, error) {
	binding, err := m.authorizedCloudBindingWithScope(ctx, taskID, sessionID, authz.ScopeSessionControl)
	if err != nil {
		return nil, err
	}
	operation, err := m.managedRuntime.BindSubmissionCandidate(ctx, binding.ExecutionID, runID)
	if err != nil {
		return nil, err
	}
	task, err := m.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return operation, errors.New("cursor cloud task is unavailable")
	}
	if task.ArchivedAt != nil {
		if err := m.stopCloud(ctx, binding.ExecutionID, "task_archived", false); err != nil {
			return operation, err
		}
		return m.repo.GetManagedAgentLatestOperation(ctx, binding.ID)
	}
	m.startManagedObserver(binding.ExecutionID, "disconnect")
	return operation, nil
}

func (m *cursorCloudAgentManager) retryUnknownSubmission(
	ctx context.Context,
	taskID, sessionID, resolutionID string,
	acknowledgeDuplicateWork bool,
) (*models.ManagedAgentOperation, error) {
	binding, err := m.authorizedCloudBindingWithScope(ctx, taskID, sessionID, authz.ScopeSessionPrompt)
	if err != nil {
		return nil, err
	}
	if !m.enabled() {
		return nil, errors.New("cursor cloud is disabled for new dispatches")
	}
	task, err := m.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return nil, errors.New("cursor cloud task is unavailable")
	}
	if task.ArchivedAt != nil {
		return nil, errors.New("cursor cloud retry is unavailable for an archived task")
	}
	operation, retryErr := m.managedRuntime.RetryUnknownSubmission(ctx, binding.ExecutionID, resolutionID, acknowledgeDuplicateWork)
	if operation != nil && models.ManagedAgentOperationActive(operation.State) && operation.RemoteRunID != "" {
		m.startManagedObserver(binding.ExecutionID, "disconnect")
	}
	return operation, retryErr
}

func (m *cursorCloudAgentManager) authorizedCloudBinding(ctx context.Context, taskID, sessionID string) (*models.ManagedAgentBinding, error) {
	if err := m.taskService.AuthorizeTaskAccess(ctx, taskID); err != nil {
		return nil, err
	}
	return m.loadCloudBindingForTask(ctx, taskID, sessionID)
}

func (m *cursorCloudAgentManager) authorizedCloudBindingWithScope(
	ctx context.Context,
	taskID, sessionID string,
	scope authz.Scope,
) (*models.ManagedAgentBinding, error) {
	if err := m.taskService.AuthorizeTaskScope(ctx, taskID, scope); err != nil {
		return nil, err
	}
	return m.loadCloudBindingForTask(ctx, taskID, sessionID)
}

func (m *cursorCloudAgentManager) loadCloudBindingForTask(ctx context.Context, taskID, sessionID string) (*models.ManagedAgentBinding, error) {
	if taskID == "" || sessionID == "" {
		return nil, errors.New("cursor cloud task and session identity are required")
	}
	binding, err := m.repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if errors.Is(err, repository.ErrManagedAgentBindingNotFound) || err == nil && binding.TaskID != taskID {
		return nil, repository.ErrManagedAgentBindingNotFound
	}
	if err != nil {
		return nil, err
	}
	return binding, nil
}
