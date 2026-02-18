package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// Workspace operations

// CreateWorkspace creates a new workspace
func (s *Service) CreateWorkspace(ctx context.Context, req *CreateWorkspaceRequest) (*models.Workspace, error) {
	workspace := &models.Workspace{
		ID:                    uuid.New().String(),
		Name:                  req.Name,
		Description:           req.Description,
		OwnerID:               req.OwnerID,
		DefaultExecutorID:     normalizeOptionalID(req.DefaultExecutorID),
		DefaultEnvironmentID:  normalizeOptionalID(req.DefaultEnvironmentID),
		DefaultAgentProfileID: normalizeOptionalID(req.DefaultAgentProfileID),
	}

	if err := s.repo.CreateWorkspace(ctx, workspace); err != nil {
		s.logger.Error("failed to create workspace", zap.Error(err))
		return nil, err
	}

	s.publishWorkspaceEvent(ctx, events.WorkspaceCreated, workspace)
	s.logger.Info("workspace created", zap.String("workspace_id", workspace.ID), zap.String("name", workspace.Name))
	return workspace, nil
}

// GetWorkspace retrieves a workspace by ID
func (s *Service) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
	return s.repo.GetWorkspace(ctx, id)
}

// UpdateWorkspace updates an existing workspace
func (s *Service) UpdateWorkspace(ctx context.Context, id string, req *UpdateWorkspaceRequest) (*models.Workspace, error) {
	workspace, err := s.repo.GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		workspace.Name = *req.Name
	}
	if req.Description != nil {
		workspace.Description = *req.Description
	}
	if req.DefaultExecutorID != nil {
		workspace.DefaultExecutorID = normalizeOptionalID(req.DefaultExecutorID)
	}
	if req.DefaultEnvironmentID != nil {
		workspace.DefaultEnvironmentID = normalizeOptionalID(req.DefaultEnvironmentID)
	}
	if req.DefaultAgentProfileID != nil {
		workspace.DefaultAgentProfileID = normalizeOptionalID(req.DefaultAgentProfileID)
	}
	workspace.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateWorkspace(ctx, workspace); err != nil {
		s.logger.Error("failed to update workspace", zap.String("workspace_id", id), zap.Error(err))
		return nil, err
	}

	s.publishWorkspaceEvent(ctx, events.WorkspaceUpdated, workspace)
	s.logger.Info("workspace updated", zap.String("workspace_id", workspace.ID))
	return workspace, nil
}

// DeleteWorkspace deletes a workspace
func (s *Service) DeleteWorkspace(ctx context.Context, id string) error {
	workspace, err := s.repo.GetWorkspace(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteWorkspace(ctx, id); err != nil {
		s.logger.Error("failed to delete workspace", zap.String("workspace_id", id), zap.Error(err))
		return err
	}
	s.publishWorkspaceEvent(ctx, events.WorkspaceDeleted, workspace)
	s.logger.Info("workspace deleted", zap.String("workspace_id", id))
	return nil
}

func normalizeOptionalID(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// ListWorkspaces returns all workspaces
func (s *Service) ListWorkspaces(ctx context.Context) ([]*models.Workspace, error) {
	return s.repo.ListWorkspaces(ctx)
}

// Workflow operations

// CreateWorkflow creates a new workflow
func (s *Service) CreateWorkflow(ctx context.Context, req *CreateWorkflowRequest) (*models.Workflow, error) {
	workflow := &models.Workflow{
		ID:                 uuid.New().String(),
		WorkspaceID:        req.WorkspaceID,
		Name:               req.Name,
		Description:        req.Description,
		WorkflowTemplateID: req.WorkflowTemplateID,
	}

	if err := s.repo.CreateWorkflow(ctx, workflow); err != nil {
		s.logger.Error("failed to create workflow", zap.Error(err))
		return nil, err
	}

	// Create workflow steps from template if specified
	if req.WorkflowTemplateID != nil && *req.WorkflowTemplateID != "" && s.workflowStepCreator != nil {
		if err := s.workflowStepCreator.CreateStepsFromTemplate(ctx, workflow.ID, *req.WorkflowTemplateID); err != nil {
			s.logger.Error("failed to create workflow steps from template",
				zap.String("workflow_id", workflow.ID),
				zap.String("template_id", *req.WorkflowTemplateID),
				zap.Error(err))
			// Don't fail workflow creation, just log the error
		}
	}

	s.publishWorkflowEvent(ctx, events.WorkflowCreated, workflow)
	s.logger.Info("workflow created", zap.String("workflow_id", workflow.ID), zap.String("name", workflow.Name))
	return workflow, nil
}

// GetWorkflow retrieves a workflow by ID
func (s *Service) GetWorkflow(ctx context.Context, id string) (*models.Workflow, error) {
	return s.repo.GetWorkflow(ctx, id)
}

// UpdateWorkflow updates an existing workflow
func (s *Service) UpdateWorkflow(ctx context.Context, id string, req *UpdateWorkflowRequest) (*models.Workflow, error) {
	workflow, err := s.repo.GetWorkflow(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		workflow.Name = *req.Name
	}
	if req.Description != nil {
		workflow.Description = *req.Description
	}
	workflow.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateWorkflow(ctx, workflow); err != nil {
		s.logger.Error("failed to update workflow", zap.String("workflow_id", id), zap.Error(err))
		return nil, err
	}

	s.publishWorkflowEvent(ctx, events.WorkflowUpdated, workflow)
	s.logger.Info("workflow updated", zap.String("workflow_id", workflow.ID))
	return workflow, nil
}

// DeleteWorkflow deletes a workflow
func (s *Service) DeleteWorkflow(ctx context.Context, id string) error {
	workflow, err := s.repo.GetWorkflow(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteWorkflow(ctx, id); err != nil {
		s.logger.Error("failed to delete workflow", zap.String("workflow_id", id), zap.Error(err))
		return err
	}

	s.publishWorkflowEvent(ctx, events.WorkflowDeleted, workflow)
	s.logger.Info("workflow deleted", zap.String("workflow_id", id))
	return nil
}

// ListWorkflows returns all workflows for a workspace (or all if empty)
func (s *Service) ListWorkflows(ctx context.Context, workspaceID string) ([]*models.Workflow, error) {
	return s.repo.ListWorkflows(ctx, workspaceID)
}

// Repository operations

func (s *Service) CreateRepository(ctx context.Context, req *CreateRepositoryRequest) (*models.Repository, error) {
	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = "local"
	}
	prefix := strings.TrimSpace(req.WorktreeBranchPrefix)
	if err := worktree.ValidateBranchPrefix(prefix); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidRepositorySettings, err)
	}
	if prefix == "" {
		prefix = worktree.DefaultBranchPrefix
	}
	pullBeforeWorktree := true
	if req.PullBeforeWorktree != nil {
		pullBeforeWorktree = *req.PullBeforeWorktree
	}
	repository := &models.Repository{
		ID:                   uuid.New().String(),
		WorkspaceID:          req.WorkspaceID,
		Name:                 req.Name,
		SourceType:           sourceType,
		LocalPath:            req.LocalPath,
		Provider:             req.Provider,
		ProviderRepoID:       req.ProviderRepoID,
		ProviderOwner:        req.ProviderOwner,
		ProviderName:         req.ProviderName,
		DefaultBranch:        req.DefaultBranch,
		WorktreeBranchPrefix: prefix,
		PullBeforeWorktree:   pullBeforeWorktree,
		SetupScript:          req.SetupScript,
		CleanupScript:        req.CleanupScript,
		DevScript:            req.DevScript,
	}

	if err := s.repo.CreateRepository(ctx, repository); err != nil {
		s.logger.Error("failed to create repository", zap.Error(err))
		return nil, err
	}

	s.publishRepositoryEvent(ctx, events.RepositoryCreated, repository)
	s.logger.Info("repository created", zap.String("repository_id", repository.ID))
	return repository, nil
}

func (s *Service) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	return s.repo.GetRepository(ctx, id)
}

func (s *Service) UpdateRepository(ctx context.Context, id string, req *UpdateRepositoryRequest) (*models.Repository, error) {
	repository, err := s.repo.GetRepository(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := applyRepositoryUpdates(repository, req); err != nil {
		return nil, err
	}
	repository.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateRepository(ctx, repository); err != nil {
		s.logger.Error("failed to update repository", zap.String("repository_id", id), zap.Error(err))
		return nil, err
	}

	s.publishRepositoryEvent(ctx, events.RepositoryUpdated, repository)
	s.logger.Info("repository updated", zap.String("repository_id", repository.ID))
	return repository, nil
}

// applyRepositoryUpdates applies the non-nil fields from req onto repository.
// Returns an error if the WorktreeBranchPrefix is invalid.
func applyRepositoryUpdates(repository *models.Repository, req *UpdateRepositoryRequest) error {
	if req.Name != nil {
		repository.Name = *req.Name
	}
	if req.SourceType != nil {
		repository.SourceType = *req.SourceType
	}
	if req.LocalPath != nil {
		repository.LocalPath = *req.LocalPath
	}
	if req.Provider != nil {
		repository.Provider = *req.Provider
	}
	if req.ProviderRepoID != nil {
		repository.ProviderRepoID = *req.ProviderRepoID
	}
	if req.ProviderOwner != nil {
		repository.ProviderOwner = *req.ProviderOwner
	}
	if req.ProviderName != nil {
		repository.ProviderName = *req.ProviderName
	}
	if req.DefaultBranch != nil {
		repository.DefaultBranch = *req.DefaultBranch
	}
	if req.WorktreeBranchPrefix != nil {
		prefix := strings.TrimSpace(*req.WorktreeBranchPrefix)
		if err := worktree.ValidateBranchPrefix(prefix); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidRepositorySettings, err)
		}
		repository.WorktreeBranchPrefix = prefix
	}
	if req.PullBeforeWorktree != nil {
		repository.PullBeforeWorktree = *req.PullBeforeWorktree
	}
	if req.SetupScript != nil {
		repository.SetupScript = *req.SetupScript
	}
	if req.CleanupScript != nil {
		repository.CleanupScript = *req.CleanupScript
	}
	if req.DevScript != nil {
		repository.DevScript = *req.DevScript
	}
	return nil
}

func (s *Service) DeleteRepository(ctx context.Context, id string) error {
	repository, err := s.repo.GetRepository(ctx, id)
	if err != nil {
		return err
	}
	active, err := s.repo.HasActiveTaskSessionsByRepository(ctx, id)
	if err != nil {
		s.logger.Error("failed to check active agent sessions for repository", zap.String("repository_id", id), zap.Error(err))
		return err
	}
	if active {
		return ErrActiveTaskSessions
	}
	if err := s.repo.DeleteRepository(ctx, id); err != nil {
		s.logger.Error("failed to delete repository", zap.String("repository_id", id), zap.Error(err))
		return err
	}
	s.publishRepositoryEvent(ctx, events.RepositoryDeleted, repository)
	s.logger.Info("repository deleted", zap.String("repository_id", id))
	return nil
}

func (s *Service) ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error) {
	return s.repo.ListRepositories(ctx, workspaceID)
}

// Repository script operations

func (s *Service) CreateRepositoryScript(ctx context.Context, req *CreateRepositoryScriptRequest) (*models.RepositoryScript, error) {
	script := &models.RepositoryScript{
		ID:           uuid.New().String(),
		RepositoryID: req.RepositoryID,
		Name:         req.Name,
		Command:      req.Command,
		Position:     req.Position,
	}
	if err := s.repo.CreateRepositoryScript(ctx, script); err != nil {
		s.logger.Error("failed to create repository script", zap.Error(err))
		return nil, err
	}
	s.publishRepositoryScriptEvent(ctx, events.RepositoryScriptCreated, script)
	s.logger.Info("repository script created", zap.String("script_id", script.ID))
	return script, nil
}

func (s *Service) GetRepositoryScript(ctx context.Context, id string) (*models.RepositoryScript, error) {
	return s.repo.GetRepositoryScript(ctx, id)
}

func (s *Service) UpdateRepositoryScript(ctx context.Context, id string, req *UpdateRepositoryScriptRequest) (*models.RepositoryScript, error) {
	script, err := s.repo.GetRepositoryScript(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		script.Name = *req.Name
	}
	if req.Command != nil {
		script.Command = *req.Command
	}
	if req.Position != nil {
		script.Position = *req.Position
	}
	script.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateRepositoryScript(ctx, script); err != nil {
		s.logger.Error("failed to update repository script", zap.String("script_id", id), zap.Error(err))
		return nil, err
	}
	s.publishRepositoryScriptEvent(ctx, events.RepositoryScriptUpdated, script)
	s.logger.Info("repository script updated", zap.String("script_id", script.ID))
	return script, nil
}

func (s *Service) DeleteRepositoryScript(ctx context.Context, id string) error {
	script, err := s.repo.GetRepositoryScript(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteRepositoryScript(ctx, id); err != nil {
		s.logger.Error("failed to delete repository script", zap.String("script_id", id), zap.Error(err))
		return err
	}
	s.publishRepositoryScriptEvent(ctx, events.RepositoryScriptDeleted, script)
	s.logger.Info("repository script deleted", zap.String("script_id", id))
	return nil
}

func (s *Service) ListRepositoryScripts(ctx context.Context, repositoryID string) ([]*models.RepositoryScript, error) {
	return s.repo.ListRepositoryScripts(ctx, repositoryID)
}

// Executor operations

func (s *Service) CreateExecutor(ctx context.Context, req *CreateExecutorRequest) (*models.Executor, error) {
	if err := validateExecutorConfig(req.Config); err != nil {
		return nil, err
	}
	executor := &models.Executor{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Type:      req.Type,
		Status:    req.Status,
		IsSystem:  req.IsSystem,
		Resumable: req.Resumable,
		Config:    req.Config,
	}

	if err := s.repo.CreateExecutor(ctx, executor); err != nil {
		return nil, err
	}
	s.publishExecutorEvent(ctx, events.ExecutorCreated, executor)
	return executor, nil
}

func (s *Service) GetExecutor(ctx context.Context, id string) (*models.Executor, error) {
	return s.repo.GetExecutor(ctx, id)
}

func (s *Service) UpdateExecutor(ctx context.Context, id string, req *UpdateExecutorRequest) (*models.Executor, error) {
	executor, err := s.repo.GetExecutor(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateExecutorUpdateRequest(executor, req); err != nil {
		return nil, err
	}
	applyExecutorUpdates(executor, req)
	executor.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateExecutor(ctx, executor); err != nil {
		return nil, err
	}
	s.publishExecutorEvent(ctx, events.ExecutorUpdated, executor)
	return executor, nil
}

// validateExecutorUpdateRequest validates config and system executor constraints.
func validateExecutorUpdateRequest(executor *models.Executor, req *UpdateExecutorRequest) error {
	if req.Config != nil {
		if err := validateExecutorConfig(req.Config); err != nil {
			return err
		}
	}
	if !executor.IsSystem {
		return nil
	}
	if req.Name != nil && *req.Name != executor.Name {
		return fmt.Errorf("system executors cannot be modified")
	}
	if req.Type != nil && *req.Type != executor.Type {
		return fmt.Errorf("system executors cannot be modified")
	}
	if req.Status != nil && *req.Status != executor.Status {
		return fmt.Errorf("system executors cannot be modified")
	}
	if req.Resumable != nil && *req.Resumable != executor.Resumable {
		return fmt.Errorf("system executors cannot be modified")
	}
	return nil
}

// applyExecutorUpdates copies non-nil request fields onto the executor model.
func applyExecutorUpdates(executor *models.Executor, req *UpdateExecutorRequest) {
	if req.Name != nil {
		executor.Name = *req.Name
	}
	if req.Type != nil {
		executor.Type = *req.Type
	}
	if req.Status != nil {
		executor.Status = *req.Status
	}
	if req.Resumable != nil {
		executor.Resumable = *req.Resumable
	}
	if req.Config != nil {
		executor.Config = req.Config
	}
}

func (s *Service) DeleteExecutor(ctx context.Context, id string) error {
	executor, err := s.repo.GetExecutor(ctx, id)
	if err != nil {
		return err
	}
	if executor.IsSystem {
		return fmt.Errorf("system executors cannot be deleted")
	}
	active, err := s.repo.HasActiveTaskSessionsByExecutor(ctx, id)
	if err != nil {
		s.logger.Error("failed to check active agent sessions for executor", zap.String("executor_id", id), zap.Error(err))
		return err
	}
	if active {
		return ErrActiveTaskSessions
	}
	if err := s.repo.DeleteExecutor(ctx, id); err != nil {
		return err
	}
	s.publishExecutorEvent(ctx, events.ExecutorDeleted, executor)
	return nil
}

func (s *Service) ListExecutors(ctx context.Context) ([]*models.Executor, error) {
	return s.repo.ListExecutors(ctx)
}

// Environment operations

func (s *Service) CreateEnvironment(ctx context.Context, req *CreateEnvironmentRequest) (*models.Environment, error) {
	environment := &models.Environment{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Kind:         req.Kind,
		IsSystem:     false,
		WorktreeRoot: req.WorktreeRoot,
		ImageTag:     req.ImageTag,
		Dockerfile:   req.Dockerfile,
		BuildConfig:  req.BuildConfig,
	}
	if err := s.repo.CreateEnvironment(ctx, environment); err != nil {
		return nil, err
	}
	s.publishEnvironmentEvent(ctx, events.EnvironmentCreated, environment)
	return environment, nil
}

func (s *Service) GetEnvironment(ctx context.Context, id string) (*models.Environment, error) {
	return s.repo.GetEnvironment(ctx, id)
}

func (s *Service) UpdateEnvironment(ctx context.Context, id string, req *UpdateEnvironmentRequest) (*models.Environment, error) {
	environment, err := s.repo.GetEnvironment(ctx, id)
	if err != nil {
		return nil, err
	}
	if environment.IsSystem {
		if req.Name != nil || req.Kind != nil || req.ImageTag != nil || req.Dockerfile != nil || req.BuildConfig != nil {
			return nil, fmt.Errorf("system environments can only update the worktree root")
		}
	}
	if req.Name != nil {
		environment.Name = *req.Name
	}
	if req.Kind != nil {
		environment.Kind = *req.Kind
	}
	if req.WorktreeRoot != nil {
		environment.WorktreeRoot = *req.WorktreeRoot
	}
	if req.ImageTag != nil {
		environment.ImageTag = *req.ImageTag
	}
	if req.Dockerfile != nil {
		environment.Dockerfile = *req.Dockerfile
	}
	if req.BuildConfig != nil {
		environment.BuildConfig = req.BuildConfig
	}
	environment.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateEnvironment(ctx, environment); err != nil {
		return nil, err
	}
	s.publishEnvironmentEvent(ctx, events.EnvironmentUpdated, environment)
	return environment, nil
}

func (s *Service) DeleteEnvironment(ctx context.Context, id string) error {
	environment, err := s.repo.GetEnvironment(ctx, id)
	if err != nil {
		return err
	}
	if environment.IsSystem {
		return fmt.Errorf("system environments cannot be deleted")
	}
	active, err := s.repo.HasActiveTaskSessionsByEnvironment(ctx, id)
	if err != nil {
		s.logger.Error("failed to check active agent sessions for environment", zap.String("environment_id", id), zap.Error(err))
		return err
	}
	if active {
		return ErrActiveTaskSessions
	}
	if err := s.repo.DeleteEnvironment(ctx, id); err != nil {
		return err
	}
	s.publishEnvironmentEvent(ctx, events.EnvironmentDeleted, environment)
	return nil
}

func (s *Service) ListEnvironments(ctx context.Context) ([]*models.Environment, error) {
	return s.repo.ListEnvironments(ctx)
}
