package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/scriptengine"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

const (
	spritesTokenEnvKey                = "SPRITES_API_TOKEN"
	workspaceDeletePageSize           = 500
	workspaceDeleteCleanupConcurrency = 8
)

var ErrWorkspaceConfirmNameMismatch = errors.New("confirm_name does not match workspace name")

type workspaceDeleteTaskCleanup struct {
	task        *models.Task
	sessions    []*models.TaskSession
	worktrees   []*worktree.Worktree
	stopTargets []taskStopTarget
	taskEnv     *models.TaskEnvironment
}

// Workspace operations

// CreateWorkspace creates a new workspace
func (s *Service) CreateWorkspace(ctx context.Context, req *CreateWorkspaceRequest) (*models.Workspace, error) {
	workspace := &models.Workspace{
		ID:                          uuid.New().String(),
		Name:                        req.Name,
		Description:                 req.Description,
		OwnerID:                     req.OwnerID,
		DefaultExecutorID:           normalizeOptionalID(req.DefaultExecutorID),
		DefaultEnvironmentID:        normalizeOptionalID(req.DefaultEnvironmentID),
		DefaultAgentProfileID:       normalizeOptionalID(req.DefaultAgentProfileID),
		DefaultConfigAgentProfileID: normalizeOptionalID(req.DefaultConfigAgentProfileID),
	}

	if err := s.workspaces.CreateWorkspace(ctx, workspace); err != nil {
		s.logger.Error("failed to create workspace", zap.Error(err))
		return nil, err
	}

	s.publishWorkspaceEvent(ctx, events.WorkspaceCreated, workspace)
	s.logger.Info("workspace created", zap.String("workspace_id", workspace.ID), zap.String("name", workspace.Name))
	return workspace, nil
}

// GetWorkspace retrieves a workspace by ID
func (s *Service) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
	return s.workspaces.GetWorkspace(ctx, id)
}

// UpdateWorkspace updates an existing workspace
func (s *Service) UpdateWorkspace(ctx context.Context, id string, req *UpdateWorkspaceRequest) (*models.Workspace, error) {
	workspace, err := s.workspaces.GetWorkspace(ctx, id)
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
	if req.DefaultConfigAgentProfileID != nil {
		workspace.DefaultConfigAgentProfileID = normalizeOptionalID(req.DefaultConfigAgentProfileID)
	}
	workspace.UpdatedAt = time.Now().UTC()

	if err := s.workspaces.UpdateWorkspace(ctx, workspace); err != nil {
		s.logger.Error("failed to update workspace", zap.String("workspace_id", id), zap.Error(err))
		return nil, err
	}

	s.publishWorkspaceEvent(ctx, events.WorkspaceUpdated, workspace)
	s.logger.Info("workspace updated", zap.String("workspace_id", workspace.ID))
	return workspace, nil
}

// DeleteWorkspace deletes a workspace
func (s *Service) DeleteWorkspace(ctx context.Context, id string) error {
	workspace, err := s.workspaces.GetWorkspace(ctx, id)
	if err != nil {
		return err
	}
	return s.deleteWorkspace(ctx, workspace, nil)
}

// DeleteWorkspaceWithConfirmName deletes a workspace only when confirmName
// matches the workspace name read for the cascade and final row delete.
func (s *Service) DeleteWorkspaceWithConfirmName(ctx context.Context, id, confirmName string) error {
	workspace, err := s.workspaces.GetWorkspace(ctx, id)
	if err != nil {
		return err
	}
	if confirmName != workspace.Name {
		return ErrWorkspaceConfirmNameMismatch
	}
	return s.deleteWorkspace(ctx, workspace, &confirmName)
}

func (s *Service) deleteWorkspace(ctx context.Context, workspace *models.Workspace, confirmedName *string) error {
	tasks, err := s.listAllTasksForWorkspaceDelete(ctx, workspace.ID)
	if err != nil {
		return err
	}
	// Runtime cleanup needs task rows before the cascade removes them.
	cleanups, err := s.prepareWorkspaceDeleteTaskCleanups(ctx, tasks)
	if err != nil {
		return err
	}

	var deletedTasks []*models.Task
	var deletedWorkflows []*models.Workflow
	if confirmedName == nil {
		deletedTasks, deletedWorkflows, err = s.workspaces.DeleteWorkspaceCascade(ctx, workspace.ID)
	} else {
		deletedTasks, deletedWorkflows, err = s.workspaces.DeleteWorkspaceCascadeWithName(ctx, workspace.ID, *confirmedName)
	}
	if err != nil {
		return s.mapWorkspaceDeleteError(workspace.ID, err)
	}
	cleanups = s.appendWorkspaceDeleteMissingTaskCleanups(ctx, cleanups, deletedTasks)
	s.publishWorkspaceDeleteChildEvents(ctx, deletedTasks, deletedWorkflows)
	s.runWorkspaceDeleteTaskCleanups(cleanups, deletedTasks)
	s.publishWorkspaceEvent(ctx, events.WorkspaceDeleted, workspace)
	s.logger.Info("workspace deleted", zap.String("workspace_id", workspace.ID))
	return nil
}

func (s *Service) prepareWorkspaceDeleteTaskCleanups(ctx context.Context, tasks []*models.Task) ([]workspaceDeleteTaskCleanup, error) {
	cleanups := make([]workspaceDeleteTaskCleanup, 0, len(tasks))
	for _, task := range tasks {
		if task == nil || task.ID == "" {
			continue
		}
		cleanup, err := s.prepareWorkspaceDeleteTaskCleanup(ctx, task)
		if err != nil {
			return nil, err
		}
		cleanups = append(cleanups, cleanup)
	}
	return cleanups, nil
}

func (s *Service) appendWorkspaceDeleteMissingTaskCleanups(
	ctx context.Context,
	cleanups []workspaceDeleteTaskCleanup,
	deletedTasks []*models.Task,
) []workspaceDeleteTaskCleanup {
	prepared := make(map[string]struct{}, len(cleanups))
	for _, cleanup := range cleanups {
		if cleanup.task != nil && cleanup.task.ID != "" {
			prepared[cleanup.task.ID] = struct{}{}
		}
	}
	for _, task := range deletedTasks {
		if task == nil || task.ID == "" {
			continue
		}
		if _, ok := prepared[task.ID]; ok {
			continue
		}
		cleanup, err := s.prepareWorkspaceDeleteTaskCleanup(ctx, task)
		if err != nil {
			s.logger.Error("failed to prepare late workspace task cleanup",
				zap.String("task_id", task.ID),
				zap.Error(err))
			continue
		}
		cleanups = append(cleanups, cleanup)
		prepared[task.ID] = struct{}{}
	}
	return cleanups
}

func (s *Service) prepareWorkspaceDeleteTaskCleanup(ctx context.Context, task *models.Task) (workspaceDeleteTaskCleanup, error) {
	cleanup := workspaceDeleteTaskCleanup{
		task:      task,
		worktrees: s.gatherWorktreesForDelete(ctx, task.ID),
		taskEnv:   s.gatherTaskEnvironmentForCleanup(ctx, task.ID),
	}
	var err error
	cleanup.sessions, err = s.sessions.ListTaskSessions(ctx, task.ID)
	if err != nil {
		return workspaceDeleteTaskCleanup{}, fmt.Errorf("list task sessions for workspace delete task %q: %w", task.ID, err)
	}
	if s.executionStopper == nil {
		return cleanup, nil
	}
	activeSessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, task.ID)
	if err != nil {
		s.logger.Warn("failed to list active task sessions for workspace delete",
			zap.String("task_id", task.ID),
			zap.Error(err))
	}
	cleanup.stopTargets, err = s.buildStopTargets(ctx, task.ID, activeSessions)
	if err != nil {
		return workspaceDeleteTaskCleanup{}, fmt.Errorf("list runtime cleanup inventory: %w", err)
	}
	return cleanup, nil
}

func (s *Service) publishWorkspaceDeleteChildEvents(ctx context.Context, tasks []*models.Task, workflows []*models.Workflow) {
	for _, task := range tasks {
		if task == nil || task.ID == "" {
			continue
		}
		s.publishTaskEvent(ctx, events.TaskDeleted, task, nil)
	}
	for _, workflow := range workflows {
		if workflow == nil || workflow.ID == "" {
			continue
		}
		s.publishWorkflowEvent(ctx, events.WorkflowDeleted, workflow)
	}
}

func (s *Service) runWorkspaceDeleteTaskCleanups(cleanups []workspaceDeleteTaskCleanup, deletedTasks []*models.Task) {
	jobs := s.workspaceDeleteTaskCleanupJobs(cleanups, deletedTasks)
	if len(jobs) == 0 {
		return
	}
	go s.runWorkspaceDeleteTaskCleanupJobs(jobs)
}

func (s *Service) workspaceDeleteTaskCleanupJobs(
	cleanups []workspaceDeleteTaskCleanup,
	deletedTasks []*models.Task,
) []workspaceDeleteTaskCleanup {
	deletedTaskIDs := make(map[string]struct{}, len(deletedTasks))
	for _, task := range deletedTasks {
		if task != nil && task.ID != "" {
			deletedTaskIDs[task.ID] = struct{}{}
		}
	}
	jobs := make([]workspaceDeleteTaskCleanup, 0, len(cleanups))
	for _, cleanup := range cleanups {
		if cleanup.task == nil {
			continue
		}
		if _, ok := deletedTaskIDs[cleanup.task.ID]; !ok {
			continue
		}
		hasCleanup := len(cleanup.stopTargets) > 0 || s.worktreeCleanup != nil ||
			len(cleanup.sessions) > 0 || cleanup.task.IsEphemeral || cleanup.taskEnv != nil
		if !hasCleanup {
			continue
		}
		jobs = append(jobs, cleanup)
	}
	return jobs
}

func (s *Service) runWorkspaceDeleteTaskCleanupJobs(jobs []workspaceDeleteTaskCleanup) {
	workers := workspaceDeleteCleanupConcurrency
	if len(jobs) < workers {
		workers = len(jobs)
	}
	jobCh := make(chan workspaceDeleteTaskCleanup)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for cleanup := range jobCh {
				s.runWorkspaceDeleteTaskCleanup(cleanup)
			}
		}()
	}
	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)
	wg.Wait()
}

func (s *Service) runWorkspaceDeleteTaskCleanup(cleanup workspaceDeleteTaskCleanup) {
	envCleanup := taskEnvironmentCleanup{env: cleanup.taskEnv, deleteRow: false}
	s.runTaskCleanup(cleanup.task.ID, cleanup.sessions, cleanup.worktrees, cleanup.stopTargets, envCleanup,
		"task deleted", "failed to stop session on task delete", "task cleanup completed")
}

func (s *Service) mapWorkspaceDeleteError(id string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, taskrepo.ErrWorkspaceNameMismatch) {
		return ErrWorkspaceConfirmNameMismatch
	}
	s.logger.Error("failed to delete workspace", zap.String("workspace_id", id), zap.Error(err))
	return err
}

func (s *Service) listAllTasksForWorkspaceDelete(ctx context.Context, workspaceID string) ([]*models.Task, error) {
	var all []*models.Task
	for page := 1; ; page++ {
		tasks, total, err := s.tasks.ListTasksByWorkspace(
			ctx, workspaceID, "", "", "", page, workspaceDeletePageSize, true, true, false, false,
		)
		if err != nil {
			return nil, fmt.Errorf("list workspace tasks: %w", err)
		}
		all = append(all, tasks...)
		if len(all) >= total || len(tasks) == 0 {
			return all, nil
		}
	}
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
	return s.workspaces.ListWorkspaces(ctx)
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
		Hidden:             req.Hidden,
	}

	if err := s.workflows.CreateWorkflow(ctx, workflow); err != nil {
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
	return s.workflows.GetWorkflow(ctx, id)
}

// UpdateWorkflow updates an existing workflow
func (s *Service) UpdateWorkflow(ctx context.Context, id string, req *UpdateWorkflowRequest) (*models.Workflow, error) {
	workflow, err := s.workflows.GetWorkflow(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		workflow.Name = *req.Name
	}
	if req.Description != nil {
		workflow.Description = *req.Description
	}
	if req.AgentProfileID != nil {
		workflow.AgentProfileID = strings.TrimSpace(*req.AgentProfileID)
	}
	workflow.UpdatedAt = time.Now().UTC()

	if err := s.workflows.UpdateWorkflow(ctx, workflow); err != nil {
		s.logger.Error("failed to update workflow", zap.String("workflow_id", id), zap.Error(err))
		return nil, err
	}

	s.publishWorkflowEvent(ctx, events.WorkflowUpdated, workflow)
	s.logger.Info("workflow updated", zap.String("workflow_id", workflow.ID))
	return workflow, nil
}

// SetWorkflowHidden flips the hidden flag on a workflow. Used by system
// flows (e.g. improve-kandev) to heal records created before Hidden was
// honored on insert.
func (s *Service) SetWorkflowHidden(ctx context.Context, id string, hidden bool) error {
	workflow, err := s.workflows.GetWorkflow(ctx, id)
	if err != nil {
		return err
	}
	if workflow.Hidden == hidden {
		return nil
	}
	workflow.Hidden = hidden
	workflow.UpdatedAt = time.Now().UTC()
	if err := s.workflows.UpdateWorkflow(ctx, workflow); err != nil {
		s.logger.Error("failed to update workflow hidden flag", zap.String("workflow_id", id), zap.Error(err))
		return err
	}
	s.publishWorkflowEvent(ctx, events.WorkflowUpdated, workflow)
	return nil
}

// DeleteWorkflow deletes a workflow, archiving its remaining tasks first so
// they do not linger as orphan rows pointing at a workflow_id that no longer
// exists (the tasks.workflow_id FK was dropped to support empty workflow_id
// on ephemeral tasks, so SQLite cannot cascade for us).
func (s *Service) DeleteWorkflow(ctx context.Context, id string) error {
	workflow, err := s.workflows.GetWorkflow(ctx, id)
	if err != nil {
		return err
	}

	tasks, err := s.tasks.ListTasks(ctx, id)
	if err != nil {
		s.logger.Error("failed to list tasks for workflow delete cascade",
			zap.String("workflow_id", id), zap.Error(err))
		return err
	}
	archived := 0
	for _, task := range tasks {
		if err := s.ArchiveTask(ctx, task.ID); err != nil {
			// Concurrent archive between ListTasks and here is a no-op:
			// the task is already in the desired state, keep cascading.
			if errors.Is(err, ErrTaskAlreadyArchived) {
				continue
			}
			s.logger.Error("failed to archive task during workflow delete cascade",
				zap.String("workflow_id", id),
				zap.String("task_id", task.ID),
				zap.Error(err))
			return err
		}
		archived++
	}

	if err := s.workflows.DeleteWorkflow(ctx, id); err != nil {
		s.logger.Error("failed to delete workflow", zap.String("workflow_id", id), zap.Error(err))
		return err
	}

	s.publishWorkflowEvent(ctx, events.WorkflowDeleted, workflow)
	s.logger.Info("workflow deleted",
		zap.String("workflow_id", id),
		zap.Int("archived_tasks", archived))
	return nil
}

// ListWorkflows returns workflows for a workspace, excluding hidden ones by default.
// Pass includeHidden=true to include system-only flows like Improve Kandev.
func (s *Service) ListWorkflows(ctx context.Context, workspaceID string, includeHidden bool) ([]*models.Workflow, error) {
	return s.workflows.ListWorkflows(ctx, workspaceID, includeHidden)
}

// GetOfficeWorkflowIDs returns the set of workflow IDs that are office workflows
// (referenced by any workspace's office_workflow_id column).
func (s *Service) GetOfficeWorkflowIDs(ctx context.Context) map[string]struct{} {
	workspaces, err := s.workspaces.ListWorkspaces(ctx)
	if err != nil {
		return nil
	}
	ids := make(map[string]struct{})
	for _, ws := range workspaces {
		if ws.OfficeWorkflowID != "" {
			ids[ws.OfficeWorkflowID] = struct{}{}
		}
	}
	return ids
}

// ReorderWorkflows updates sort_order for workflows within a workspace.
func (s *Service) ReorderWorkflows(ctx context.Context, workspaceID string, workflowIDs []string) error {
	if err := s.workflows.ReorderWorkflows(ctx, workspaceID, workflowIDs); err != nil {
		s.logger.Error("failed to reorder workflows", zap.String("workspace_id", workspaceID), zap.Error(err))
		return err
	}
	s.logger.Info("reordered workflows", zap.String("workspace_id", workspaceID), zap.Int("count", len(workflowIDs)))
	return nil
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
		CopyFiles:            req.CopyFiles,
		StartupPrompt:        req.StartupPrompt,
	}

	// Auto-detect GitHub provider info from git remote if not provided
	if repository.Provider == "" && repository.LocalPath != "" {
		if p, o, n := ResolveGitRemoteProvider(repository.LocalPath); p != "" {
			repository.Provider = p
			repository.ProviderOwner = o
			repository.ProviderName = n
		}
	}

	if err := s.repoEntities.CreateRepository(ctx, repository); err != nil {
		s.logger.Error("failed to create repository", zap.Error(err))
		return nil, err
	}

	s.publishRepositoryEvent(ctx, events.RepositoryCreated, repository)
	s.logger.Info("repository created", zap.String("repository_id", repository.ID))
	return repository, nil
}

func (s *Service) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	return s.repoEntities.GetRepository(ctx, id)
}

// GetRepositoryByProviderInfo looks up a repository by workspace and provider identity.
// Returns nil (with nil error) when no matching repository exists.
func (s *Service) GetRepositoryByProviderInfo(ctx context.Context, workspaceID, provider, owner, name string) (*models.Repository, error) {
	return s.repoEntities.GetRepositoryByProviderInfo(ctx, workspaceID, provider, owner, name)
}

// FindOrCreateRepository looks up a repository by provider info, creating one if not found.
// If the repository exists but has no LocalPath and the request provides one, updates LocalPath.
//
// Returns created=true only when CreateRepository was invoked by this call.
// Callers that register cleanup on the new row (e.g. add_branch_to_task's
// orphan-rollback path) must gate it on that flag instead of inferring
// ownership from a workspace snapshot — a concurrent request can win the
// create race between snapshot and lookup, so a snapshot-miss does NOT
// mean this call created the row.
func (s *Service) FindOrCreateRepository(ctx context.Context, req *FindOrCreateRepositoryRequest) (*models.Repository, bool, error) {
	existing, err := s.repoEntities.GetRepositoryByProviderInfo(ctx, req.WorkspaceID, req.Provider, req.ProviderOwner, req.ProviderName)
	if err != nil {
		return nil, false, fmt.Errorf("lookup repository: %w", err)
	}
	if existing != nil {
		dirty := false
		if existing.LocalPath == "" && req.LocalPath != "" {
			existing.LocalPath = req.LocalPath
			dirty = true
		}
		// Backfill default_branch when the caller carries one and the existing
		// row is still empty. Lets the synchronous add_branch probe persist its
		// answer onto a previously-empty Repository row (e.g. one created by
		// an earlier create_task that left default_branch unset).
		if existing.DefaultBranch == "" && req.DefaultBranch != "" {
			existing.DefaultBranch = req.DefaultBranch
			dirty = true
		}
		if dirty {
			if updateErr := s.repoEntities.UpdateRepository(ctx, existing); updateErr != nil {
				s.logger.Warn("failed to backfill repository fields",
					zap.String("repository_id", existing.ID), zap.Error(updateErr))
			}
		}
		return existing, false, nil
	}

	name := fmt.Sprintf("%s/%s", req.ProviderOwner, req.ProviderName)
	created, createErr := s.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:   req.WorkspaceID,
		Name:          name,
		SourceType:    "provider",
		LocalPath:     req.LocalPath,
		Provider:      req.Provider,
		ProviderOwner: req.ProviderOwner,
		ProviderName:  req.ProviderName,
		DefaultBranch: req.DefaultBranch,
	})
	if createErr != nil {
		return nil, false, createErr
	}
	return created, true, nil
}

func (s *Service) UpdateRepository(ctx context.Context, id string, req *UpdateRepositoryRequest) (*models.Repository, error) {
	repository, err := s.repoEntities.GetRepository(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := applyRepositoryUpdates(repository, req); err != nil {
		return nil, err
	}
	repository.UpdatedAt = time.Now().UTC()

	if err := s.repoEntities.UpdateRepository(ctx, repository); err != nil {
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
	if req.CopyFiles != nil {
		repository.CopyFiles = *req.CopyFiles
	}
	if req.StartupPrompt != nil {
		repository.StartupPrompt = *req.StartupPrompt
	}
	return nil
}

func (s *Service) DeleteRepository(ctx context.Context, id string) error {
	repository, err := s.repoEntities.GetRepository(ctx, id)
	if err != nil {
		return err
	}
	active, err := s.sessions.HasActiveTaskSessionsByRepository(ctx, id)
	if err != nil {
		s.logger.Error("failed to check active agent sessions for repository", zap.String("repository_id", id), zap.Error(err))
		return err
	}
	if active {
		return ErrActiveTaskSessions
	}
	if err := s.repoEntities.DeleteRepository(ctx, id); err != nil {
		s.logger.Error("failed to delete repository", zap.String("repository_id", id), zap.Error(err))
		return err
	}
	s.publishRepositoryEvent(ctx, events.RepositoryDeleted, repository)
	s.logger.Info("repository deleted", zap.String("repository_id", id))
	return nil
}

func (s *Service) ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error) {
	return s.repoEntities.ListRepositories(ctx, workspaceID)
}

// ResolveRepositoryStartupPrompt returns the given repository's StartupPrompt
// with {{TICKET_*}} / {{TASK_TITLE}} placeholders substituted from the task's
// title and metadata. Lines whose ticket placeholders never resolve are
// dropped (see scriptengine.ResolveStartupPrompt). Returns empty string when
// the repository has no startup prompt, when the repository lookup fails, or
// when repositoryID is empty — callers treat "" as "no prompt to apply".
func (s *Service) ResolveRepositoryStartupPrompt(ctx context.Context, repositoryID, taskTitle string, metadata map[string]interface{}) string {
	if repositoryID == "" {
		return ""
	}
	repo, err := s.repoEntities.GetRepository(ctx, repositoryID)
	if err != nil || repo == nil || repo.StartupPrompt == "" {
		return ""
	}
	return scriptengine.ResolveStartupPrompt(repo.StartupPrompt, taskTitle, metadata)
}

// FindRepositoryForInput returns the workspace repository that matches the
// given input — resolved from repository_id, github_url, or local_path in that
// order. Returns nil (with nil error) when no match exists; callers treat
// that as "no repo to apply". Used by the MCP path so create_task_kandev
// callers who identify their repo via URL or local path still get the
// repository's startup_prompt applied.
func (s *Service) FindRepositoryForInput(ctx context.Context, workspaceID string, input TaskRepositoryInput) (*models.Repository, error) {
	if input.RepositoryID != "" {
		return s.repoEntities.GetRepository(ctx, input.RepositoryID)
	}
	if input.GitHubURL != "" {
		if owner, name, err := parseGitHubRepoURL(input.GitHubURL); err == nil && workspaceID != "" {
			return s.repoEntities.GetRepositoryByProviderInfo(ctx, workspaceID, "github", owner, name)
		}
	}
	if input.LocalPath != "" && workspaceID != "" {
		repos, err := s.repoEntities.ListRepositories(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		for _, r := range repos {
			if r.LocalPath == input.LocalPath {
				return r, nil
			}
		}
	}
	return nil, nil
}

// CountActiveSessionsByRepository returns the number of agent sessions in an
// active state (CREATED / STARTING / RUNNING / WAITING_FOR_INPUT) that are
// attached to the given repository. Used by the UI to warn users before they
// attempt to delete a repository that would otherwise be blocked by
// DeleteRepository's ErrActiveTaskSessions sentinel.
func (s *Service) CountActiveSessionsByRepository(ctx context.Context, id string) (int, error) {
	if _, err := s.repoEntities.GetRepository(ctx, id); err != nil {
		return 0, err
	}
	return s.sessions.CountActiveTaskSessionsByRepository(ctx, id)
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
	if err := s.repoEntities.CreateRepositoryScript(ctx, script); err != nil {
		s.logger.Error("failed to create repository script", zap.Error(err))
		return nil, err
	}
	s.publishRepositoryScriptEvent(ctx, events.RepositoryScriptCreated, script)
	s.logger.Info("repository script created", zap.String("script_id", script.ID))
	return script, nil
}

func (s *Service) GetRepositoryScript(ctx context.Context, id string) (*models.RepositoryScript, error) {
	return s.repoEntities.GetRepositoryScript(ctx, id)
}

func (s *Service) UpdateRepositoryScript(ctx context.Context, id string, req *UpdateRepositoryScriptRequest) (*models.RepositoryScript, error) {
	script, err := s.repoEntities.GetRepositoryScript(ctx, id)
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

	if err := s.repoEntities.UpdateRepositoryScript(ctx, script); err != nil {
		s.logger.Error("failed to update repository script", zap.String("script_id", id), zap.Error(err))
		return nil, err
	}
	s.publishRepositoryScriptEvent(ctx, events.RepositoryScriptUpdated, script)
	s.logger.Info("repository script updated", zap.String("script_id", script.ID))
	return script, nil
}

func (s *Service) DeleteRepositoryScript(ctx context.Context, id string) error {
	script, err := s.repoEntities.GetRepositoryScript(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repoEntities.DeleteRepositoryScript(ctx, id); err != nil {
		s.logger.Error("failed to delete repository script", zap.String("script_id", id), zap.Error(err))
		return err
	}
	s.publishRepositoryScriptEvent(ctx, events.RepositoryScriptDeleted, script)
	s.logger.Info("repository script deleted", zap.String("script_id", id))
	return nil
}

func (s *Service) ListRepositoryScripts(ctx context.Context, repositoryID string) ([]*models.RepositoryScript, error) {
	return s.repoEntities.ListRepositoryScripts(ctx, repositoryID)
}

// ListScriptsByRepositoryIDs returns scripts for multiple repositories in a single query.
func (s *Service) ListScriptsByRepositoryIDs(ctx context.Context, repoIDs []string) (map[string][]*models.RepositoryScript, error) {
	return s.repoEntities.ListScriptsByRepositoryIDs(ctx, repoIDs)
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

	if err := s.executors.CreateExecutor(ctx, executor); err != nil {
		return nil, err
	}
	s.publishExecutorEvent(ctx, events.ExecutorCreated, executor)
	return executor, nil
}

func (s *Service) GetExecutor(ctx context.Context, id string) (*models.Executor, error) {
	return s.executors.GetExecutor(ctx, id)
}

func (s *Service) UpdateExecutor(ctx context.Context, id string, req *UpdateExecutorRequest) (*models.Executor, error) {
	executor, err := s.executors.GetExecutor(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateExecutorUpdateRequest(executor, req); err != nil {
		return nil, err
	}
	applyExecutorUpdates(executor, req)
	executor.UpdatedAt = time.Now().UTC()
	if err := s.executors.UpdateExecutor(ctx, executor); err != nil {
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
	executor, err := s.executors.GetExecutor(ctx, id)
	if err != nil {
		return err
	}
	if executor.IsSystem {
		return fmt.Errorf("system executors cannot be deleted")
	}
	active, err := s.sessions.HasActiveTaskSessionsByExecutor(ctx, id)
	if err != nil {
		s.logger.Error("failed to check active agent sessions for executor", zap.String("executor_id", id), zap.Error(err))
		return err
	}
	if active {
		return ErrActiveTaskSessions
	}
	if err := s.executors.DeleteExecutor(ctx, id); err != nil {
		return err
	}
	s.publishExecutorEvent(ctx, events.ExecutorDeleted, executor)
	return nil
}

func (s *Service) ListExecutors(ctx context.Context) ([]*models.Executor, error) {
	return s.executors.ListExecutors(ctx)
}

// Executor Profile operations

func (s *Service) CreateExecutorProfile(ctx context.Context, req *CreateExecutorProfileRequest) (*models.ExecutorProfile, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("profile name is required")
	}
	if req.ExecutorID == "" {
		return nil, fmt.Errorf("executor_id is required")
	}
	// Verify executor exists
	executor, err := s.executors.GetExecutor(ctx, req.ExecutorID)
	if err != nil {
		return nil, fmt.Errorf("executor not found: %w", err)
	}
	if executor.Type == models.ExecutorTypeSprites && !hasSpritesToken(req.EnvVars) {
		return nil, fmt.Errorf("sprites profiles require %s env var", spritesTokenEnvKey)
	}
	profile := &models.ExecutorProfile{
		ExecutorID:    req.ExecutorID,
		Name:          req.Name,
		McpPolicy:     req.McpPolicy,
		Config:        req.Config,
		PrepareScript: req.PrepareScript,
		CleanupScript: req.CleanupScript,
		EnvVars:       req.EnvVars,
	}
	if err := s.executors.CreateExecutorProfile(ctx, profile); err != nil {
		return nil, err
	}
	s.publishExecutorProfileEvent(ctx, events.ExecutorProfileCreated, profile)
	return profile, nil
}

func (s *Service) GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error) {
	return s.executors.GetExecutorProfile(ctx, id)
}

func (s *Service) UpdateExecutorProfile(ctx context.Context, id string, req *UpdateExecutorProfileRequest) (*models.ExecutorProfile, error) {
	profile, err := s.executors.GetExecutorProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	executor, err := s.executors.GetExecutor(ctx, profile.ExecutorID)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		profile.Name = *req.Name
	}
	if req.McpPolicy != nil {
		profile.McpPolicy = *req.McpPolicy
	}
	if req.Config != nil {
		profile.Config = req.Config
	}
	if req.PrepareScript != nil {
		profile.PrepareScript = *req.PrepareScript
	}
	if req.CleanupScript != nil {
		profile.CleanupScript = *req.CleanupScript
	}
	if req.EnvVars != nil {
		if executor.Type == models.ExecutorTypeSprites {
			req.EnvVars = mergeSpritesTokenEnvVars(profile.EnvVars, req.EnvVars)
		}
		profile.EnvVars = req.EnvVars
	}
	if err := s.executors.UpdateExecutorProfile(ctx, profile); err != nil {
		return nil, err
	}
	s.publishExecutorProfileEvent(ctx, events.ExecutorProfileUpdated, profile)
	return profile, nil
}

func mergeSpritesTokenEnvVars(existing, incoming []models.ProfileEnvVar) []models.ProfileEnvVar {
	merged := make([]models.ProfileEnvVar, 0, len(incoming)+1)
	var existingToken models.ProfileEnvVar
	hasExistingToken := false
	hasIncomingToken := false
	explicitRemove := false

	for _, ev := range existing {
		if strings.TrimSpace(ev.Key) != spritesTokenEnvKey {
			continue
		}
		if strings.TrimSpace(ev.SecretID) == "" && strings.TrimSpace(ev.Value) == "" {
			continue
		}
		existingToken = ev
		hasExistingToken = true
		break
	}

	for _, ev := range incoming {
		if strings.TrimSpace(ev.Key) != spritesTokenEnvKey {
			merged = append(merged, ev)
			continue
		}
		if strings.TrimSpace(ev.SecretID) == "" && strings.TrimSpace(ev.Value) == "" {
			explicitRemove = true
			continue
		}
		hasIncomingToken = true
		merged = append(merged, ev)
	}

	if !hasIncomingToken && !explicitRemove && hasExistingToken {
		merged = append(merged, existingToken)
	}
	return merged
}

func hasSpritesToken(envVars []models.ProfileEnvVar) bool {
	for _, ev := range envVars {
		if strings.TrimSpace(ev.Key) != spritesTokenEnvKey {
			continue
		}
		if strings.TrimSpace(ev.SecretID) != "" || strings.TrimSpace(ev.Value) != "" {
			return true
		}
	}
	return false
}

func (s *Service) DeleteExecutorProfile(ctx context.Context, id string) error {
	profile, err := s.executors.GetExecutorProfile(ctx, id)
	if err != nil {
		return err
	}
	if err := s.executors.DeleteExecutorProfile(ctx, id); err != nil {
		return err
	}
	s.publishExecutorProfileEvent(ctx, events.ExecutorProfileDeleted, profile)
	return nil
}

func (s *Service) ListExecutorProfiles(ctx context.Context, executorID string) ([]*models.ExecutorProfile, error) {
	return s.executors.ListExecutorProfiles(ctx, executorID)
}

func (s *Service) ListAllExecutorProfiles(ctx context.Context) ([]*models.ExecutorProfile, error) {
	return s.executors.ListAllExecutorProfiles(ctx)
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
	if err := s.environments.CreateEnvironment(ctx, environment); err != nil {
		return nil, err
	}
	s.publishEnvironmentEvent(ctx, events.EnvironmentCreated, environment)
	return environment, nil
}

func (s *Service) GetEnvironment(ctx context.Context, id string) (*models.Environment, error) {
	return s.environments.GetEnvironment(ctx, id)
}

func (s *Service) UpdateEnvironment(ctx context.Context, id string, req *UpdateEnvironmentRequest) (*models.Environment, error) {
	environment, err := s.environments.GetEnvironment(ctx, id)
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
	if err := s.environments.UpdateEnvironment(ctx, environment); err != nil {
		return nil, err
	}
	s.publishEnvironmentEvent(ctx, events.EnvironmentUpdated, environment)
	return environment, nil
}

func (s *Service) DeleteEnvironment(ctx context.Context, id string) error {
	environment, err := s.environments.GetEnvironment(ctx, id)
	if err != nil {
		return err
	}
	if environment.IsSystem {
		return fmt.Errorf("system environments cannot be deleted")
	}
	active, err := s.sessions.HasActiveTaskSessionsByEnvironment(ctx, id)
	if err != nil {
		s.logger.Error("failed to check active agent sessions for environment", zap.String("environment_id", id), zap.Error(err))
		return err
	}
	if active {
		return ErrActiveTaskSessions
	}
	if err := s.environments.DeleteEnvironment(ctx, id); err != nil {
		return err
	}
	s.publishEnvironmentEvent(ctx, events.EnvironmentDeleted, environment)
	return nil
}

func (s *Service) ListEnvironments(ctx context.Context) ([]*models.Environment, error) {
	return s.environments.ListEnvironments(ctx)
}
