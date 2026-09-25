package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/kandev/kandev/internal/worktree/copyfiles"
)

const (
	spritesTokenEnvKey                = "SPRITES_API_TOKEN"
	workspaceDeletePageSize           = 500
	workspaceDeleteCleanupConcurrency = 8
)

type workflowDeleteTaskLister interface {
	ListTasksForDeletion(context.Context, string, string, int, int) ([]*models.Task, int, error)
}

var ErrWorkspaceConfirmNameMismatch = errors.New("confirm_name does not match workspace name")

const maxRepositorySecretBindings = 100

const maxProviderScopeBytes = 512

var repositorySecretKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func normalizeProviderHost(provider, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if strings.EqualFold(strings.TrimSpace(provider), githubProviderName) {
			return githubProviderHost
		}
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return scheme + "://" + strings.ToLower(parsed.Host)
}

func validateProviderScope(raw string) (string, error) {
	scope := strings.TrimSpace(raw)
	if scope == "" {
		return "", nil
	}
	if len(scope) > maxProviderScopeBytes || strings.ContainsRune(scope, '\x00') {
		return "", fmt.Errorf("%w: provider_scope is invalid", ErrInvalidRepositorySettings)
	}
	return scope, nil
}

// validateProviderScopeAndRepoIDPair enforces repoclone.Cloner's
// WorkspaceProviderRepositoryPath invariant: a non-empty provider_scope
// requires a paired provider_repo_id, because the scope-isolated clone
// layout needs both to build a unique path. A bare provider_repo_id with no
// scope is fine — that's the normal shape for every built-in provider
// (GitHub, GitLab, Azure DevOps), none of which resolve a provider
// connection scope — and falls through to the legacy owner/name layout.
func validateProviderScopeAndRepoIDPair(scope, repoID string) error {
	if strings.TrimSpace(scope) != "" && strings.TrimSpace(repoID) == "" {
		return fmt.Errorf("%w: provider_scope requires a paired provider_repo_id", ErrInvalidRepositorySettings)
	}
	return nil
}

type workspaceDeleteTaskCleanup struct {
	task        *models.Task
	sessions    []*models.TaskSession
	worktrees   []*worktree.Worktree
	stopTargets []taskStopTarget
	taskEnv     *models.TaskEnvironment
	attachments []*models.TaskMessageAttachment
	cleanupJob  *models.TaskResourceCleanupJob
}

type workspaceAttachmentLister interface {
	ListMessageAttachmentsByWorkspace(ctx context.Context, workspaceID string) ([]*models.TaskMessageAttachment, error)
}
type repositorySessionPruner interface {
	DeleteRepositoryIfNoActiveTaskSessions(ctx context.Context, id string) (bool, error)
}

// Workspace operations

// CreateWorkspace creates a new workspace
func (s *Service) CreateWorkspace(ctx context.Context, req *CreateWorkspaceRequest) (*models.Workspace, error) {
	// Authenticated callers own what they create; the request-body owner is
	// only honored for internal/synthetic callers (pre-auth compatibility).
	ownerID := req.OwnerID
	if userID, scoped := callerScope(ctx); scoped {
		ownerID = userID
	}
	workspace := &models.Workspace{
		ID:                          uuid.New().String(),
		Name:                        req.Name,
		Description:                 req.Description,
		OwnerID:                     ownerID,
		DefaultExecutorID:           normalizeOptionalID(req.DefaultExecutorID),
		DefaultEnvironmentID:        normalizeOptionalID(req.DefaultEnvironmentID),
		DefaultAgentProfileID:       normalizeOptionalID(req.DefaultAgentProfileID),
		DefaultConfigAgentProfileID: normalizeOptionalID(req.DefaultConfigAgentProfileID),
		// The tenant comes from the creating identity and from nowhere else.
		// There is no org field on the request on purpose: a caller must not
		// be able to place a workspace in another tenant.
		OrgID: callerOrgID(ctx),
	}
	placement, placementErr := s.placementFor(ctx, ownerID, workspace.OrgID)
	if placementErr != nil {
		return nil, placementErr
	}
	workspace.UnitID = placement

	var kanbanWorkflow *models.Workflow
	if req.BootstrapKanbanWorkflow {
		if s.workspaceBootstrapper == nil {
			err := errors.New("workspace bootstrapper is not configured")
			s.logger.Error("failed to create workspace with Kanban bootstrap", zap.Error(err))
			return nil, err
		}
		workflow, err := s.workspaceBootstrapper.CreateWorkspaceWithKanban(ctx, workspace)
		if err != nil {
			s.logger.Error("failed to create workspace with Kanban bootstrap", zap.Error(err))
			return nil, err
		}
		kanbanWorkflow = workflow
	} else if err := s.workspaces.CreateWorkspace(ctx, workspace); err != nil {
		s.logger.Error("failed to create workspace", zap.Error(err))
		return nil, err
	}

	if s.workspaceDefaultsInitializer != nil {
		if err := s.workspaceDefaultsInitializer.InitializeWorkspaceDefaults(ctx, workspace.ID); err != nil {
			s.logger.Warn("failed to initialize workspace defaults", zap.String("workspace_id", workspace.ID), zap.Error(err))
		}
	}

	if err := s.seedWorkspaceOwnerMember(ctx, workspace); err != nil {
		return nil, err
	}

	s.publishWorkspaceEvent(ctx, events.WorkspaceCreated, workspace)
	s.logger.Info("workspace created", zap.String("workspace_id", workspace.ID), zap.String("name", workspace.Name))
	if kanbanWorkflow != nil {
		s.publishWorkflowEvent(ctx, events.WorkflowCreated, kanbanWorkflow)
		s.logger.Info("workflow created", zap.String("workflow_id", kanbanWorkflow.ID), zap.String("name", kanbanWorkflow.Name))
	}
	return workspace, nil
}

// GetWorkspace retrieves a workspace by ID
func (s *Service) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
	workspace, err := s.workspaces.GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}
	if scoped := s.workspaceDecision(ctx, workspace); !scoped.CanRead() {
		return nil, repoerrors.ErrWorkspaceNotFound
	}
	return workspace, nil
}

// UpdateWorkspace updates an existing workspace
func (s *Service) UpdateWorkspace(ctx context.Context, id string, req *UpdateWorkspaceRequest) (*models.Workspace, error) {
	workspace, err := s.workspaces.GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireWorkspaceManage(ctx, workspace); err != nil {
		return nil, err
	}

	if req.UnitID != nil {
		if err := s.moveWorkspaceToUnit(ctx, workspace, *req.UnitID); err != nil {
			return nil, err
		}
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
	if err := s.requireWorkspaceManage(ctx, workspace); err != nil {
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
	if err := s.requireWorkspaceManage(ctx, workspace); err != nil {
		return err
	}
	if confirmName != workspace.Name {
		return ErrWorkspaceConfirmNameMismatch
	}
	return s.deleteWorkspace(ctx, workspace, &confirmName)
}
func (s *Service) prepareWorkspaceAttachmentCleanup(ctx context.Context, workspaceID string) (*models.TaskResourceCleanupJob, error) {
	attachmentRepo := s.attachments
	if attachmentRepo == nil && s.attachmentSvc != nil {
		attachmentRepo = s.attachmentSvc.repo
	}
	lister, ok := attachmentRepo.(workspaceAttachmentLister)
	if !ok {
		if attachmentRepo != nil {
			return nil, fmt.Errorf("workspace attachment repository cannot list attachments")
		}
		if s.resourceCleanups == nil && s.attachmentSvc != nil {
			return nil, fmt.Errorf("workspace attachment cleanup persistence is unavailable")
		}
		return nil, nil
	}
	attachments, err := lister.ListMessageAttachmentsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list workspace attachments for cleanup: %w", err)
	}
	if len(attachments) == 0 {
		if s.resourceCleanups == nil && s.attachmentSvc == nil {
			return nil, fmt.Errorf("workspace attachment cleanup executor is unavailable")
		}
		return nil, nil
	}
	if s.resourceCleanups == nil {
		if s.attachmentSvc != nil {
			return nil, fmt.Errorf("workspace attachment cleanup persistence is unavailable")
		}
		return nil, fmt.Errorf("workspace attachment cleanup executor is unavailable")
	}
	if s.attachmentSvc == nil {
		return nil, fmt.Errorf("workspace attachment cleanup executor is unavailable")
	}
	return s.persistTaskResourceCleanup(
		ctx, "", models.TaskResourceCleanupTriggerWorkspaceDelete,
		newTaskResourceCleanupOperationID(models.TaskResourceCleanupTriggerWorkspaceDelete, "workspace-attachments:"+workspaceID),
		nil, nil, nil, attachments, taskEnvironmentCleanup{}, true, false, workspaceID,
	)
}

// DeleteOrganizationWorkspaces removes every workspace in one organization
// through the same lifecycle as an ordinary workspace deletion. Authorization
// is performed by the operator-only organization controller before this
// narrow internal seam is called.
func (s *Service) DeleteOrganizationWorkspaces(ctx context.Context, orgID string) error {
	if orgID == "" {
		return nil
	}
	workspaces, err := s.workspaces.ListWorkspaces(ctx)
	if err != nil {
		return fmt.Errorf("list organization workspaces: %w", err)
	}
	for _, workspace := range workspaces {
		if workspace == nil || workspace.OrgID != orgID {
			continue
		}
		if err := s.deleteWorkspace(ctx, workspace, nil); err != nil {
			return fmt.Errorf("delete workspace %s: %w", workspace.ID, err)
		}
	}
	return nil
}

func (s *Service) deleteWorkspace(ctx context.Context, workspace *models.Workspace, confirmedName *string) error {
	var releaseAttachmentAdmission func()
	if s.attachmentSvc != nil {
		releaseAttachmentAdmission = s.attachmentSvc.beginWorkspaceDeletion(workspace.ID)
		defer releaseAttachmentAdmission()
	}
	tasks, err := s.listAllTasksForWorkspaceDelete(ctx, workspace.ID)
	if err != nil {
		return err
	}
	// Runtime cleanup needs task rows before the cascade removes them.
	workspaceAttachmentCleanup, err := s.prepareWorkspaceAttachmentCleanup(ctx, workspace.ID)
	if err != nil {
		return err
	}
	// Record canvas artifact cleanup before the workspace cascade removes its
	// task and workspace rows. This keeps the release ownership boundary
	// durable across a process stop between the two operations.
	if s.canvasCleanup != nil {
		if err := s.canvasCleanup.CleanupWorkspaceCanvases(ctx, workspace.ID); err != nil {
			return fmt.Errorf("cleanup workspace canvases before workspace delete: %w", err)
		}
	}
	cleanups := make([]workspaceDeleteTaskCleanup, 0, len(tasks)+1)
	if workspaceAttachmentCleanup != nil {
		cleanups = append(cleanups, workspaceDeleteTaskCleanup{cleanupJob: workspaceAttachmentCleanup})
	}
	taskCleanups, err := s.prepareWorkspaceDeleteTaskCleanups(ctx, tasks)
	if err != nil {
		cancelErr := s.cancelWorkspaceDeleteTaskCleanupJobs(ctx, cleanups)
		return errors.Join(err, cancelErr)
	}
	cleanups = append(cleanups, taskCleanups...)

	var deletedWorkspaceAttachments []*models.TaskMessageAttachment
	var deletedTasks []*models.Task
	var deletedWorkflows []*models.Workflow
	transactionalCleanup, hasTransactionalCleanup := s.workspaceSecretDeleter.(secrets.WorkspaceSecretTransactionalDeleter)
	transactionalCascade, hasTransactionalCascade := s.workspaces.(transactionalWorkspaceCascade)
	useTransactionalCleanup := hasTransactionalCascade &&
		(hasTransactionalCleanup || s.attachmentSvc != nil)
	switch {
	case useTransactionalCleanup:
		cleanup := func(cleanupCtx context.Context, tx *sqlx.Tx) error {
			if hasTransactionalCleanup {
				if err := transactionalCleanup.DeleteWorkspaceSecretsTx(cleanupCtx, tx, workspace.ID); err != nil {
					return err
				}
			}
			if s.attachmentSvc != nil {
				var err error
				deletedWorkspaceAttachments, err = s.attachmentSvc.DeleteWorkspaceAttachmentsTx(
					cleanupCtx, tx, workspace.ID,
				)
				if err != nil {
					return err
				}
			}
			return nil
		}
		if confirmedName == nil {
			deletedTasks, deletedWorkflows, err = transactionalCascade.DeleteWorkspaceCascadeWithSecretCleanup(ctx, workspace.ID, cleanup)
		} else {
			deletedTasks, deletedWorkflows, err = transactionalCascade.DeleteWorkspaceCascadeWithNameAndSecretCleanup(ctx, workspace.ID, *confirmedName, cleanup)
		}
	case confirmedName == nil:
		deletedTasks, deletedWorkflows, err = s.workspaces.DeleteWorkspaceCascade(ctx, workspace.ID)
	default:
		deletedTasks, deletedWorkflows, err = s.workspaces.DeleteWorkspaceCascadeWithName(ctx, workspace.ID, *confirmedName)
	}
	if err != nil {
		cancelErr := s.cancelWorkspaceDeleteTaskCleanupJobs(ctx, cleanups)
		return s.mapWorkspaceDeleteError(workspace.ID, errors.Join(err, cancelErr))
	}
	var postCommitErr error
	if s.attachmentSvc != nil {
		s.attachmentSvc.RemoveBytes(deletedWorkspaceAttachments)
	}
	if s.workspaceSecretDeleter != nil && (!hasTransactionalCleanup || !hasTransactionalCascade) {
		if err := s.workspaceSecretDeleter.DeleteWorkspaceSecrets(ctx, workspace.ID); err != nil {
			s.logger.Error("failed to delete workspace secrets", zap.String("workspace_id", workspace.ID), zap.Error(err))
			postCommitErr = errors.Join(postCommitErr, err)
		}
	}
	var lateCleanupErr error
	cleanups, lateCleanupErr = s.appendWorkspaceDeleteMissingTaskCleanups(ctx, cleanups, deletedTasks)
	postCommitErr = errors.Join(postCommitErr, lateCleanupErr)
	s.publishWorkspaceDeleteChildEvents(ctx, deletedTasks, deletedWorkflows)
	s.runWorkspaceDeleteTaskCleanups(cleanups, deletedTasks)
	s.publishWorkspaceEvent(ctx, events.WorkspaceDeleted, workspace)
	s.logger.Info("workspace deleted", zap.String("workspace_id", workspace.ID))
	if postCommitErr != nil {
		return &CascadePostCommitError{Err: postCommitErr}
	}
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
			cancelErr := s.cancelWorkspaceDeleteTaskCleanupJobs(ctx, cleanups)
			return nil, errors.Join(err, cancelErr)
		}
		cleanups = append(cleanups, cleanup)
	}
	return cleanups, nil
}

func (s *Service) appendWorkspaceDeleteMissingTaskCleanups(
	ctx context.Context,
	cleanups []workspaceDeleteTaskCleanup,
	deletedTasks []*models.Task,
) ([]workspaceDeleteTaskCleanup, error) {
	prepared := make(map[string]struct{}, len(cleanups))
	for _, cleanup := range cleanups {
		if cleanup.task != nil && cleanup.task.ID != "" {
			prepared[cleanup.task.ID] = struct{}{}
		}
	}
	var errs []error
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
			errs = append(errs, fmt.Errorf("prepare late workspace task cleanup %q: %w", task.ID, err))
			if job, persistErr := s.persistTaskResourceCleanup(
				ctx, task.ID, models.TaskResourceCleanupTriggerWorkspaceDelete,
				newTaskResourceCleanupOperationID(models.TaskResourceCleanupTriggerWorkspaceDelete, task.ID),
				nil, nil, nil, nil, taskEnvironmentCleanup{}, false, false, "",
			); persistErr != nil {
				errs = append(errs, fmt.Errorf("persist late workspace task cleanup %q: %w", task.ID, persistErr))
			} else if job != nil {
				cleanups = append(cleanups, workspaceDeleteTaskCleanup{task: task, cleanupJob: job})
			}
			continue
		}
		cleanups = append(cleanups, cleanup)
		prepared[task.ID] = struct{}{}
	}
	return cleanups, errors.Join(errs...)
}

func (s *Service) prepareWorkspaceDeleteTaskCleanup(ctx context.Context, task *models.Task) (workspaceDeleteTaskCleanup, error) {
	worktrees, err := s.gatherWorktreesForDelete(ctx, task.ID)
	if err != nil {
		return workspaceDeleteTaskCleanup{}, fmt.Errorf("list worktrees for workspace delete task %q: %w", task.ID, err)
	}
	taskEnv, err := s.gatherTaskEnvironmentForCleanup(ctx, task.ID)
	if err != nil {
		return workspaceDeleteTaskCleanup{}, fmt.Errorf("lookup environment for workspace delete task %q: %w", task.ID, err)
	}
	var attachments []*models.TaskMessageAttachment
	attachmentRepo := s.attachments
	if attachmentRepo == nil && s.attachmentSvc != nil {
		attachmentRepo = s.attachmentSvc.repo
	}
	if attachmentRepo != nil {
		attachments, err = attachmentRepo.ListMessageAttachmentsByTask(ctx, task.ID)
		if err != nil {
			return workspaceDeleteTaskCleanup{}, fmt.Errorf("list attachments for workspace delete task %q: %w", task.ID, err)
		}
	}
	cleanup := workspaceDeleteTaskCleanup{task: task, worktrees: worktrees, taskEnv: taskEnv, attachments: attachments}
	cleanup.sessions, err = s.sessions.ListTaskSessions(ctx, task.ID)
	if err != nil {
		return workspaceDeleteTaskCleanup{}, fmt.Errorf("list task sessions for workspace delete task %q: %w", task.ID, err)
	}
	if s.executionStopper != nil {
		activeSessions, listErr := s.sessions.ListActiveTaskSessionsByTaskID(ctx, task.ID)
		if listErr != nil {
			return workspaceDeleteTaskCleanup{}, fmt.Errorf("list active sessions for workspace delete task %q: %w", task.ID, listErr)
		}
		cleanup.stopTargets, err = s.buildStopTargets(ctx, task.ID, activeSessions)
		if err != nil {
			return workspaceDeleteTaskCleanup{}, fmt.Errorf("list runtime cleanup inventory: %w", err)
		}
	}
	cleanup.cleanupJob, err = s.persistTaskResourceCleanup(
		ctx, task.ID, models.TaskResourceCleanupTriggerWorkspaceDelete,
		newTaskResourceCleanupOperationID(models.TaskResourceCleanupTriggerWorkspaceDelete, task.ID),
		cleanup.sessions, cleanup.worktrees, cleanup.stopTargets, cleanup.attachments,
		taskEnvironmentCleanup{env: cleanup.taskEnv, deleteRow: false}, true, true, task.WorkspaceID,
	)
	return cleanup, err
}
func (s *Service) cancelWorkspaceDeleteTaskCleanupJobs(ctx context.Context, cleanups []workspaceDeleteTaskCleanup) error {
	jobIDs := make([]string, 0, len(cleanups))
	for _, cleanup := range cleanups {
		if cleanup.cleanupJob != nil {
			jobIDs = append(jobIDs, cleanup.cleanupJob.ID)
		}
	}
	s.cancelTaskResourceCleanupRuns(jobIDs)
	transitionCtx, cancel := detachedCleanupTransitionContext(ctx)
	defer cancel()
	var cancellationErrs []error
	for _, cleanup := range cleanups {
		if cleanup.cleanupJob == nil || s.resourceCleanups == nil {
			continue
		}
		cas, ok := s.resourceCleanups.(taskResourceCleanupCancellationCAS)
		if !ok {
			cancellationErrs = append(cancellationErrs,
				fmt.Errorf("%w: cleanup repository cannot fence cancellation for %s",
					ErrCleanupCancellationRace, cleanup.cleanupJob.ID))
			continue
		}
		cancelled, err := cas.CancelTaskResourceCleanupJobIfPending(transitionCtx, cleanup.cleanupJob.ID)
		if err != nil {
			cancellationErrs = append(cancellationErrs, err)
		} else if !cancelled {
			cancellationErrs = append(cancellationErrs,
				fmt.Errorf("%w: workspace-delete cleanup %s was claimed concurrently",
					ErrCleanupCancellationRace, cleanup.cleanupJob.ID))
		}
	}
	if len(cancellationErrs) > 0 {
		return errors.Join(cancellationErrs...)
	}
	return nil
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
			if cleanup.cleanupJob != nil {
				jobs = append(jobs, cleanup)
			}
			continue
		}
		if _, ok := deletedTaskIDs[cleanup.task.ID]; !ok {
			continue
		}
		hasCleanup := len(cleanup.stopTargets) > 0 || s.worktreeCleanup != nil ||
			len(cleanup.sessions) > 0 || cleanup.task.IsEphemeral || cleanup.taskEnv != nil ||
			len(cleanup.attachments) > 0
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
	if cleanup.cleanupJob != nil {
		transitionCtx, cancel := detachedCleanupTransitionContext(context.Background())
		defer cancel()
		if err := s.StartPreparedTaskResourceCleanup(
			transitionCtx, cleanup.cleanupJob.OperationID,
		); err != nil {
			s.logger.Warn("start workspace delete task cleanup job",
				zap.String("job_id", cleanup.cleanupJob.ID),
				zap.String("task_id", cleanup.cleanupJob.TaskID), zap.Error(err))
		}
		return
	}
	envCleanup := taskEnvironmentCleanup{env: cleanup.taskEnv, deleteRow: false}
	s.runTaskCleanup(cleanup.task.ID, cleanup.sessions, cleanup.worktrees, cleanup.stopTargets, envCleanup, true,
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
			ctx, workspaceID, "", "", "", page, workspaceDeletePageSize, "", true, true, false, false,
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

// ListWorkspaces returns the workspaces visible to the ctx identity: all of
// them for internal/synthetic callers, only owned (plus pre-auth unowned)
// rows for authenticated users.
func (s *Service) ListWorkspaces(ctx context.Context) ([]*models.Workspace, error) {
	workspaces, err := s.workspaces.ListWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	return s.filterWorkspacesForCaller(ctx, workspaces), nil
}

// Workflow operations

// CreateWorkflow creates a new workflow
func (s *Service) CreateWorkflow(ctx context.Context, req *CreateWorkflowRequest) (*models.Workflow, error) {
	if err := s.AuthorizeWorkspaceScope(ctx, req.WorkspaceID, authz.ScopeRepositoryManage); err != nil {
		return nil, err
	}
	workflow := &models.Workflow{
		ID:                 uuid.New().String(),
		WorkspaceID:        req.WorkspaceID,
		Name:               req.Name,
		Description:        req.Description,
		Prompt:             req.Prompt,
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
	if err := s.authorizeWorkflowID(ctx, id); err != nil {
		return nil, err
	}
	return s.workflows.GetWorkflow(ctx, id)
}

// UpdateWorkflow updates an existing workflow
func (s *Service) UpdateWorkflow(ctx context.Context, id string, req *UpdateWorkflowRequest) (*models.Workflow, error) {
	if err := s.authorizeWorkflowID(ctx, id); err != nil {
		return nil, err
	}
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
	if req.Prompt != nil {
		workflow.Prompt = *req.Prompt
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

// SetWorkflowSource stamps a workflow's provenance ("manual" | "github") and
// the repo-relative file path it was synced from. Used by workflow-sync to
// mark workflows it owns.
func (s *Service) SetWorkflowSource(ctx context.Context, id, source, sourcePath string) error {
	workflow, err := s.workflows.GetWorkflow(ctx, id)
	if err != nil {
		return err
	}
	if workflow.Source == source && workflow.SourcePath == sourcePath {
		return nil
	}
	workflow.Source = source
	workflow.SourcePath = sourcePath
	workflow.UpdatedAt = time.Now().UTC()
	if err := s.workflows.UpdateWorkflow(ctx, workflow); err != nil {
		s.logger.Error("failed to update workflow source", zap.String("workflow_id", id), zap.Error(err))
		return err
	}
	s.publishWorkflowEvent(ctx, events.WorkflowUpdated, workflow)
	return nil
}

// DeleteWorkflow deletes a workflow, archiving its remaining tasks first so
// they do not linger as orphan rows pointing at a workflow_id that no longer
// exists (the tasks.workflow_id FK was dropped to support empty workflow_id
// on ephemeral tasks, so SQLite cannot cascade for us).
func (s *Service) listWorkflowTasksForDelete(
	ctx context.Context,
	workflow *models.Workflow,
) ([]*models.Task, error) {
	var all []*models.Task
	for page := 1; ; page++ {
		lister, ok := s.tasks.(workflowDeleteTaskLister)
		if !ok {
			return nil, errors.New("task repository cannot enumerate all task origins for workflow deletion")
		}
		tasks, total, err := lister.ListTasksForDeletion(
			ctx, workflow.WorkspaceID, workflow.ID, page, workspaceDeletePageSize,
		)
		if err != nil {
			return nil, fmt.Errorf("list tasks for workflow delete cascade: %w", err)
		}
		all = append(all, tasks...)
		if len(tasks) == 0 || len(all) >= total {
			return all, nil
		}
	}
}

func (s *Service) DeleteWorkflow(ctx context.Context, id string) error {
	if err := s.authorizeWorkflowID(ctx, id); err != nil {
		return err
	}
	workflow, err := s.workflows.GetWorkflow(ctx, id)
	if err != nil {
		return err
	}
	if workflow == nil {
		return fmt.Errorf("workflow not found: %s", id)
	}

	tasks, err := s.listWorkflowTasksForDelete(ctx, workflow)
	if err != nil {
		s.logger.Error("failed to list tasks for workflow delete cascade",
			zap.String("workflow_id", id), zap.Error(err))
		return err
	}
	archived := 0
	var postCommitErr error
	for _, task := range tasks {
		if task == nil || task.WorkspaceID != workflow.WorkspaceID {
			taskID, taskWorkspaceID := "", ""
			if task != nil {
				taskID = task.ID
				taskWorkspaceID = task.WorkspaceID
			}
			s.logger.Warn("skipping task outside workflow workspace during workflow delete cascade",
				zap.String("workflow_id", id),
				zap.String("workflow_workspace_id", workflow.WorkspaceID),
				zap.String("task_id", taskID),
				zap.String("task_workspace_id", taskWorkspaceID))
			continue
		}

		var archiveErr error
		if s.workflowTaskArchiveCoordinator != nil {
			outcome, err := s.workflowTaskArchiveCoordinator.ArchiveTaskTree(ctx, task.ID, false)
			if outcome != nil && len(outcome.ArchivedTaskIDs) > 0 {
				archived += len(outcome.ArchivedTaskIDs)
			}
			var cascadePostCommitErr *CascadePostCommitError
			if errors.As(err, &cascadePostCommitErr) {
				s.logger.Warn("workflow task archived with post-commit lifecycle errors",
					zap.String("workflow_id", id), zap.String("task_id", task.ID), zap.Error(err))
				postCommitErr = errors.Join(postCommitErr, err)
				continue
			}
		} else {
			archiveErr = s.ArchiveTask(ctx, task.ID)
			if errors.Is(archiveErr, ErrTaskAlreadyArchived) {
				continue
			}
		}
		if archiveErr != nil {
			err = archiveErr
			if errors.Is(err, ErrTaskAlreadyArchived) {
				continue
			}
			s.logger.Error("failed to archive task during workflow delete cascade",
				zap.String("workflow_id", id),
				zap.String("task_id", task.ID),
				zap.Error(err))
			return err
		}
		if s.workflowTaskArchiveCoordinator == nil {
			archived++
		}
	}

	if err := s.workflows.DeleteWorkflow(ctx, id); err != nil {
		s.logger.Error("failed to delete workflow", zap.String("workflow_id", id), zap.Error(err))
		return err
	}

	s.publishWorkflowEvent(ctx, events.WorkflowDeleted, workflow)
	s.logger.Info("workflow deleted",
		zap.String("workflow_id", id),
		zap.Int("archived_tasks", archived))
	if postCommitErr != nil {
		return &CascadePostCommitError{Err: postCommitErr}
	}
	return nil
}

// ListWorkflows returns workflows for a workspace, excluding hidden ones by default.
// Pass includeHidden=true to include system-only flows like Improve Kandev.
func (s *Service) ListWorkflows(ctx context.Context, workspaceID string, includeHidden bool) ([]*models.Workflow, error) {
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return nil, err
	}
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
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return err
	}
	if err := s.workflows.ReorderWorkflows(ctx, workspaceID, workflowIDs); err != nil {
		s.logger.Error("failed to reorder workflows", zap.String("workspace_id", workspaceID), zap.Error(err))
		return err
	}
	s.logger.Info("reordered workflows", zap.String("workspace_id", workspaceID), zap.Int("count", len(workflowIDs)))
	return nil
}

// Repository operations

func (s *Service) CreateRepository(ctx context.Context, req *CreateRepositoryRequest) (*models.Repository, error) {
	localPath, err := canonicalRepositoryLocalPath(req.LocalPath)
	if err != nil {
		return nil, err
	}
	return s.createRepository(ctx, req, localPath, true)
}

func (s *Service) createRepositoryWithCanonicalPath(
	ctx context.Context,
	req *CreateRepositoryRequest,
) (*models.Repository, error) {
	return s.createRepository(ctx, req, req.LocalPath, false)
}

func (s *Service) createRepository(
	ctx context.Context,
	req *CreateRepositoryRequest,
	localPath string,
	resolveProvider bool,
) (*models.Repository, error) {
	if err := s.AuthorizeWorkspaceScope(ctx, req.WorkspaceID, authz.ScopeRepositoryManage); err != nil {
		return nil, err
	}
	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = sourceTypeLocal
	}
	if req.DefaultBranch != "" && !securityutil.IsValidDefaultBranchName(req.DefaultBranch) {
		return nil, fmt.Errorf("%w: invalid default branch", ErrInvalidRepositorySettings)
	}
	prefix := strings.TrimSpace(req.WorktreeBranchPrefix)
	if err := worktree.ValidateBranchPrefix(prefix); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidRepositorySettings, err)
	}
	if prefix == "" {
		prefix = worktree.DefaultBranchPrefix
	}
	template := worktree.NormalizeBranchNameTemplate(req.WorktreeBranchTemplate)
	if err := worktree.ValidateBranchNameTemplate(template); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidRepositorySettings, err)
	}
	pullBeforeWorktree := true
	if req.PullBeforeWorktree != nil {
		pullBeforeWorktree = *req.PullBeforeWorktree
	}
	if err := copyfiles.ValidateSpec(req.CopyFiles); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidRepositorySettings, err)
	}
	bindings, err := s.validateRepositorySecretBindings(ctx, req.WorkspaceID, req.SecretBindings)
	if err != nil {
		return nil, err
	}
	providerScope, err := validateProviderScope(req.ProviderScope)
	if err != nil {
		return nil, err
	}
	repository := &models.Repository{
		ID:                     uuid.New().String(),
		WorkspaceID:            req.WorkspaceID,
		Name:                   req.Name,
		SourceType:             sourceType,
		LocalPath:              localPath,
		Provider:               req.Provider,
		ProviderRepoID:         req.ProviderRepoID,
		ProviderHost:           normalizeProviderHost(req.Provider, req.ProviderHost),
		ProviderScope:          providerScope,
		ProviderOwner:          req.ProviderOwner,
		ProviderName:           req.ProviderName,
		RemoteURL:              req.RemoteURL,
		DefaultBranch:          req.DefaultBranch,
		WorktreeBranchPrefix:   prefix,
		WorktreeBranchTemplate: template,
		PullBeforeWorktree:     pullBeforeWorktree,
		SetupScript:            req.SetupScript,
		CleanupScript:          req.CleanupScript,
		DevScript:              req.DevScript,
		CopyFiles:              req.CopyFiles,
		SecretBindings:         bindings,
	}

	if resolveProvider {
		resolveRepositoryProviderIdentity(repository)
	}

	if err := validateProviderScopeAndRepoIDPair(repository.ProviderScope, repository.ProviderRepoID); err != nil {
		return nil, err
	}

	if mutator, ok := s.repoEntities.(taskrepo.RepositorySecretBindingMutator); ok {
		if err := mutator.CreateRepositoryWithSecretBindings(ctx, repository, bindings); err != nil {
			s.logger.Error("failed to create repository", zap.Error(err))
			return nil, err
		}
	} else if len(bindings) > 0 {
		return nil, fmt.Errorf("%w: repository secret bindings are unavailable", ErrInvalidRepositorySettings)
	} else if err := s.repoEntities.CreateRepository(ctx, repository); err != nil {
		s.logger.Error("failed to create repository", zap.Error(err))
		return nil, err
	}

	s.publishRepositoryEvent(ctx, events.RepositoryCreated, repository)
	s.logger.Info("repository created", zap.String("repository_id", repository.ID))
	return repository, nil
}

// resolveRepositoryProviderIdentity fills missing provider metadata from a
// local repository origin. RemoteURL is resolved independently of the
// provider/host/owner/name fields: those are only tagged for the well-known
// github.com/gitlab.com hosts (see ResolveGitRemoteProviderIdentity), but
// self-hosted GitLab/GitHub Enterprise instances still need a populated
// RemoteURL so downstream identity matching (e.g. GitLab MR-task linking)
// has something to compare against instead of failing closed.
//
// canonicalCloneOrigin is tried first because it produces the exact
// provider-canonical clone URL (e.g. the ".git"-suffixed GitHub/GitLab.com
// form) that other code, including test fixtures rewriting Git's clone
// transport via "insteadOf" config, matches against verbatim. The broader
// ResolveGitRemoteIdentity-based fallback only runs when canonicalCloneOrigin
// doesn't recognize the host (e.g. a self-hosted GitLab/GitHub Enterprise
// instance), since it doesn't guarantee a byte-identical canonical form.
func resolveRepositoryProviderIdentity(repository *models.Repository) {
	if repository.LocalPath == "" {
		return
	}
	if repository.Provider == "" || repository.ProviderHost == "" {
		p, h, o, n := ResolveGitRemoteProviderIdentity(repository.LocalPath)
		if repository.Provider == "" {
			repository.Provider = p
		}
		if repository.Provider != "" && (strings.HasPrefix(h, "http://") || strings.HasPrefix(h, "https://")) {
			repository.ProviderHost = h
			repository.ProviderOwner = o
			repository.ProviderName = n
		}
	}
	if repository.RemoteURL == "" {
		if origin := canonicalCloneOrigin(repository.LocalPath); origin != "" {
			repository.RemoteURL = origin
		} else if origin, owner, name := ResolveGitRemoteIdentity(repository.LocalPath); origin != "" && owner != "" && name != "" {
			repository.RemoteURL = origin + "/" + owner + "/" + name
		}
	}
}

// canonicalCloneOrigin returns a credential-free clone URL for a local
// checkout's origin. Invalid and unsupported origins deliberately remain
// empty so host-local-only repositories cannot leak credentials into storage.
func canonicalCloneOrigin(localPath string) string {
	raw, err := readGitRemoteOriginURL(localPath)
	if err != nil || raw == "" {
		return ""
	}
	_, _, _, canonical, err := parseRemoteRepositoryURL(raw, "")
	if err != nil {
		return ""
	}
	return canonical
}

func (s *Service) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	repo, err := s.repoEntities.GetRepository(ctx, id)
	if err != nil {
		return nil, err
	}
	if repo != nil {
		if err := s.authorizeWorkspaceID(ctx, repo.WorkspaceID); err != nil {
			return nil, repoerrors.ErrRepositoryNotFound
		}
	}
	return repo, nil
}

// GetRepositoryByProviderInfo looks up a repository by workspace and provider identity.
// Returns nil (with nil error) when no matching repository exists.
func (s *Service) GetRepositoryByProviderInfo(ctx context.Context, workspaceID, provider, host, owner, name string) (*models.Repository, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	provider = strings.TrimSpace(provider)
	return s.repoEntities.GetRepositoryByProviderIdentity(ctx, models.ProviderRepositoryIdentity{
		WorkspaceID: workspaceID, Provider: provider,
		Host: normalizeProviderHost(provider, host), Owner: strings.TrimSpace(owner), Name: strings.TrimSpace(name),
	})
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
	s.repoResolveMu.Lock()
	defer s.repoResolveMu.Unlock()
	req.WorkspaceID = strings.TrimSpace(req.WorkspaceID)
	req.Provider = strings.TrimSpace(req.Provider)
	req.ProviderRepoID = strings.TrimSpace(req.ProviderRepoID)
	providerScope, err := validateProviderScope(req.ProviderScope)
	if err != nil {
		return nil, false, err
	}
	req.ProviderScope = providerScope
	req.ProviderOwner = strings.TrimSpace(req.ProviderOwner)
	req.ProviderName = strings.TrimSpace(req.ProviderName)
	req.RemoteURL = strings.TrimSpace(req.RemoteURL)
	req.ProviderHost = normalizeProviderHost(req.Provider, req.ProviderHost)
	existing, err := s.repoEntities.GetRepositoryByProviderIdentity(ctx, models.ProviderRepositoryIdentity{
		WorkspaceID: req.WorkspaceID, Provider: req.Provider, Scope: req.ProviderScope,
		RepositoryID: req.ProviderRepoID, Host: req.ProviderHost, Owner: req.ProviderOwner, Name: req.ProviderName,
	})
	if err != nil {
		return nil, false, fmt.Errorf("lookup repository: %w", err)
	}
	if existing != nil {
		replacement, replacementCreated, replacementErr := s.replaceTaskWorktreeRepositoryMatch(ctx, req.WorkspaceID, existing)
		if replacementErr != nil {
			return nil, false, replacementErr
		}
		existing = replacement
		dirty := false
		if existing.LocalPath == "" && req.LocalPath != "" {
			localPath, pathErr := canonicalRepositoryLocalPath(req.LocalPath)
			if pathErr != nil {
				return nil, false, pathErr
			}
			existing.LocalPath = localPath
			dirty = true
		}
		if existing.ProviderHost == "" && req.ProviderHost != "" {
			existing.ProviderHost = normalizeProviderHost(req.Provider, req.ProviderHost)
			dirty = true
		}
		if existing.ProviderScope == "" && req.ProviderScope != "" {
			existing.ProviderScope = req.ProviderScope
			dirty = true
		}
		// Backfill default_branch when the caller carries one and the existing
		// row is still empty. Lets the synchronous add_branch probe persist its
		// answer onto a previously-empty Repository row (e.g. one created by
		// an earlier create_task that left default_branch unset). Validated
		// here directly: this value reaches git argv (worktree fetch/pull via
		// FallbackBaseBranch) on paths that never go through ancestry.Check,
		// so that defense-in-depth guard does not cover it. Invalid input
		// skips only this field's backfill rather than failing the whole
		// find-or-create call, since this is a best-effort backfill.
		if existing.DefaultBranch == "" && req.DefaultBranch != "" {
			if securityutil.IsValidDefaultBranchName(req.DefaultBranch) {
				existing.DefaultBranch = req.DefaultBranch
				dirty = true
			} else {
				s.logger.Warn("skipping default_branch backfill: invalid branch name",
					zap.String("repository_id", existing.ID))
			}
		}
		if existing.RemoteURL == "" && req.RemoteURL != "" {
			existing.RemoteURL = req.RemoteURL
			dirty = true
		}
		if existing.ProviderRepoID == "" && req.ProviderRepoID != "" {
			existing.ProviderRepoID = req.ProviderRepoID
			dirty = true
		}
		if dirty {
			if updateErr := s.repoEntities.UpdateRepository(ctx, existing); updateErr != nil {
				s.logger.Warn("failed to backfill repository fields",
					zap.String("repository_id", existing.ID), zap.Error(updateErr))
			}
		}
		return existing, replacementCreated, nil
	}

	name := fmt.Sprintf("%s/%s", req.ProviderOwner, req.ProviderName)
	created, createErr := s.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:    req.WorkspaceID,
		Name:           name,
		SourceType:     sourceTypeProvider,
		LocalPath:      req.LocalPath,
		Provider:       req.Provider,
		ProviderRepoID: req.ProviderRepoID,
		ProviderHost:   req.ProviderHost,
		ProviderScope:  req.ProviderScope,
		ProviderOwner:  req.ProviderOwner,
		ProviderName:   req.ProviderName,
		RemoteURL:      req.RemoteURL,
		DefaultBranch:  req.DefaultBranch,
	})
	if createErr != nil {
		return nil, false, createErr
	}
	return created, true, nil
}

// FindOrCreateRepositoryByLocalPath looks up a repository by its canonical
// local_path within workspaceID, creating one from req if none exists.
// Mirrors FindOrCreateRepository's provider-identity flow: the lookup and the
// insert both happen while holding repoResolveMu, so two resolvers racing to
// register the same not-yet-known on-disk repo (e.g. two task-creation
// requests naming the same local_path) converge on one row instead of each
// inserting its own. canonicalPath must already be resolved (see
// resolveExplicitLocalRepositoryPath); pass "" when canonicalization failed
// or was skipped, which disables the lookup and always creates — matching
// prior behavior for that edge case, since there is no reliable identity to
// dedupe against.
//
// Returns created=true only when this call inserted the new row.
func (s *Service) FindOrCreateRepositoryByLocalPath(
	ctx context.Context, workspaceID, canonicalPath string, req *CreateRepositoryRequest,
) (*models.Repository, bool, error) {
	s.repoResolveMu.Lock()
	defer s.repoResolveMu.Unlock()

	if canonicalPath != "" {
		existing, err := s.repoEntities.GetRepositoryByLocalPath(ctx, workspaceID, canonicalPath)
		if err != nil {
			return nil, false, fmt.Errorf("lookup repository by local path: %w", err)
		}
		if existing != nil {
			replacement, replacementCreated, replaceErr := s.replaceTaskWorktreeRepositoryMatch(ctx, workspaceID, existing)
			if replaceErr != nil {
				return nil, false, replaceErr
			}
			return replacement, replacementCreated, nil
		}
	}
	created, createErr := s.CreateRepository(ctx, req)
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
	if repository != nil {
		if err := s.AuthorizeWorkspaceScope(ctx, repository.WorkspaceID, authz.ScopeRepositoryManage); err != nil {
			return nil, repoerrors.ErrRepositoryNotFound
		}
	}
	updates := *req
	if req.LocalPath != nil {
		localPath, pathErr := canonicalRepositoryLocalPath(*req.LocalPath)
		if pathErr != nil {
			return nil, pathErr
		}
		updates.LocalPath = &localPath
	}
	if err := applyRepositoryUpdates(repository, &updates); err != nil {
		return nil, err
	}
	var replacement []models.RepositorySecretBinding
	if req.SecretBindings != nil {
		replacement, err = s.validateRepositorySecretBindings(ctx, repository.WorkspaceID, *req.SecretBindings)
		if err != nil {
			return nil, err
		}
		repository.SecretBindings = replacement
	}
	repository.UpdatedAt = time.Now().UTC()

	if req.SecretBindings != nil {
		mutator, ok := s.repoEntities.(taskrepo.RepositorySecretBindingMutator)
		if !ok {
			return nil, fmt.Errorf("%w: repository secret bindings are unavailable", ErrInvalidRepositorySettings)
		}
		if err := mutator.UpdateRepositoryWithSecretBindings(ctx, repository, replacement); err != nil {
			s.logger.Error("failed to update repository", zap.String("repository_id", id), zap.Error(err))
			return nil, err
		}
	} else if err := s.repoEntities.UpdateRepository(ctx, repository); err != nil {
		s.logger.Error("failed to update repository", zap.String("repository_id", id), zap.Error(err))
		return nil, err
	}

	s.publishRepositoryEvent(ctx, events.RepositoryUpdated, repository)
	s.logger.Info("repository updated", zap.String("repository_id", repository.ID))
	return repository, nil
}

func (s *Service) validateRepositorySecretBindings(
	ctx context.Context, workspaceID string, inputs []RepositorySecretBindingInput,
) ([]models.RepositorySecretBinding, error) {
	if len(inputs) > maxRepositorySecretBindings {
		return nil, fmt.Errorf("%w: at most %d secret bindings are allowed", ErrInvalidRepositorySettings, maxRepositorySecretBindings)
	}
	if len(inputs) == 0 {
		return nil, nil
	}
	catalog, ok := s.secretStore.(secrets.ScopedSecretStore)
	if !ok {
		return nil, fmt.Errorf("%w: secret binding validation is unavailable", ErrInvalidRepositorySettings)
	}
	seen := make(map[string]struct{}, len(inputs))
	bindings := make([]models.RepositorySecretBinding, 0, len(inputs))
	for _, input := range inputs {
		key := strings.TrimSpace(input.Key)
		if key != input.Key || key == "" || len(key) > 256 || !repositorySecretKeyPattern.MatchString(key) ||
			key == "TASK_DESCRIPTION" || strings.HasPrefix(key, "KANDEV_") {
			return nil, fmt.Errorf("%w: invalid secret binding key", ErrInvalidRepositorySettings)
		}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%w: duplicate secret binding key", ErrInvalidRepositorySettings)
		}
		seen[key] = struct{}{}
		secretID := strings.TrimSpace(input.SecretID)
		if secretID == "" {
			return nil, fmt.Errorf("%w: secret binding reference is required", ErrInvalidRepositorySettings)
		}
		if _, err := catalog.GetForWorkspace(ctx, secretID, workspaceID); err != nil {
			return nil, fmt.Errorf("%w: secret binding reference is not available", ErrInvalidRepositorySettings)
		}
		bindings = append(bindings, models.RepositorySecretBinding{Key: key, SecretID: secretID})
	}
	return bindings, nil
}

func canonicalRepositoryLocalPath(localPath string) (string, error) {
	if localPath == "" {
		return "", nil
	}
	canonicalPath, _, err := resolveExplicitLocalRepositoryPath(localPath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidRepositorySettings, err)
	}
	return canonicalPath, nil
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
	if req.ProviderHost != nil {
		repository.ProviderHost = normalizeProviderHost(repository.Provider, *req.ProviderHost)
	}
	if req.ProviderScope != nil {
		providerScope, err := validateProviderScope(*req.ProviderScope)
		if err != nil {
			return err
		}
		repository.ProviderScope = providerScope
	}
	if req.ProviderOwner != nil {
		repository.ProviderOwner = *req.ProviderOwner
	}
	if req.ProviderName != nil {
		repository.ProviderName = *req.ProviderName
	}
	if req.RemoteURL != nil {
		repository.RemoteURL = strings.TrimSpace(*req.RemoteURL)
	}
	if req.DefaultBranch != nil {
		if *req.DefaultBranch != "" && !securityutil.IsValidDefaultBranchName(*req.DefaultBranch) {
			return fmt.Errorf("%w: invalid default branch", ErrInvalidRepositorySettings)
		}
		repository.DefaultBranch = *req.DefaultBranch
	}
	if req.WorktreeBranchPrefix != nil {
		prefix := strings.TrimSpace(*req.WorktreeBranchPrefix)
		if err := worktree.ValidateBranchPrefix(prefix); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidRepositorySettings, err)
		}
		repository.WorktreeBranchPrefix = prefix
	}
	if req.WorktreeBranchTemplate != nil {
		template := worktree.NormalizeBranchNameTemplate(*req.WorktreeBranchTemplate)
		if err := worktree.ValidateBranchNameTemplate(template); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidRepositorySettings, err)
		}
		repository.WorktreeBranchTemplate = template
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
		if err := copyfiles.ValidateSpec(*req.CopyFiles); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidRepositorySettings, err)
		}
		repository.CopyFiles = *req.CopyFiles
	}
	if err := validateProviderScopeAndRepoIDPair(repository.ProviderScope, repository.ProviderRepoID); err != nil {
		return err
	}
	return nil
}

func (s *Service) DeleteRepository(ctx context.Context, id string) error {
	repository, err := s.repoEntities.GetRepository(ctx, id)
	if err != nil {
		return err
	}
	if repository != nil {
		if err := s.AuthorizeWorkspaceScope(ctx, repository.WorkspaceID, authz.ScopeRepositoryManage); err != nil {
			if IsForbidden(err) {
				return err
			}
			return repoerrors.ErrRepositoryNotFound
		}
	}
	active, err := s.sessions.HasActiveTaskSessionsByRepository(ctx, id)
	if err != nil {
		s.logger.Error("failed to check active agent sessions for repository", zap.String("repository_id", id), zap.Error(err))
		return err
	}
	if active {
		return ErrActiveTaskSessions
	}
	// Captured before the delete, because deleting prunes the membership rows
	// that identify the affected sets.
	affectedSetIDs := s.repositorySetIDsHolding(ctx, id)
	if err := s.repoEntities.DeleteRepository(ctx, id); err != nil {
		s.logger.Error("failed to delete repository", zap.String("repository_id", id), zap.Error(err))
		return err
	}
	s.publishRepositoryEvent(ctx, events.RepositoryDeleted, repository)
	// A client that already loaded the workspace only reacts to repository_set.*
	// events, so without this it keeps offering the deleted member until reload.
	s.publishRepositorySetsAfterMembershipChange(ctx, affectedSetIDs)
	s.logger.Info("repository deleted", zap.String("repository_id", id))
	return nil
}

func (s *Service) ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error) {
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return nil, err
	}
	repositories, err := s.repoEntities.ListRepositories(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	live := make([]*models.Repository, 0, len(repositories))
	pruner, canPrune := s.repoEntities.(repositorySessionPruner)
	for _, repository := range repositories {
		if repository == nil || repository.SourceType != sourceTypeLocal || !s.isKandevTaskWorktreeRepository(repository) {
			live = append(live, repository)
			continue
		}
		if _, statErr := os.Stat(repository.LocalPath); !errors.Is(statErr, os.ErrNotExist) {
			live = append(live, repository)
			continue
		}
		if !canPrune {
			live = append(live, repository)
			continue
		}
		deleted, err := pruner.DeleteRepositoryIfNoActiveTaskSessions(ctx, repository.ID)
		if err != nil {
			s.logger.Warn("failed to prune missing task worktree repository",
				zap.String("repository_id", repository.ID),
				zap.String("local_path", repository.LocalPath),
				zap.Error(err))
			live = append(live, repository)
			continue
		}
		if deleted {
			s.publishRepositoryEvent(ctx, events.RepositoryDeleted, repository)
			continue
		}
		current, getErr := s.repoEntities.GetRepository(ctx, repository.ID)
		if getErr == nil {
			live = append(live, current)
			continue
		}
		if !errors.Is(getErr, taskrepo.ErrRepositoryNotFound) {
			s.logger.Warn("failed to re-read retained task worktree repository, using cached value",
				zap.String("repository_id", repository.ID),
				zap.Error(getErr))
			live = append(live, repository)
		}
	}
	return dedupeRepositoriesByIdentity(live), nil
}

// repositoryIdentityKey returns a stable dedup key for repo: local_path when
// set (a local checkout is identified by where it lives on disk), otherwise
// the provider identity tuple when host/owner/name are all present (a
// provider-backed repo is identified by where it lives upstream), otherwise
// the row's own ID. A missing provider_host (legacy rows, or self-managed
// providers we've never normalized a host for) means we cannot tell two
// same-namespace repos on different unknown hosts apart, so those rows fail
// closed to their own ID instead of risking a false-positive collapse.
// Placeholder rows with neither local_path nor provider also fall back to ID
// so they never collide with one another.
func repositoryIdentityKey(repo *models.Repository) string {
	if repo.LocalPath != "" {
		return "local\x00" + repo.LocalPath
	}
	if repo.Provider != "" && repo.ProviderScope != "" && repo.ProviderRepoID != "" {
		return "provider-scope\x00" + repo.Provider + "\x00" + repo.ProviderScope + "\x00" + repo.ProviderRepoID
	}
	if repo.Provider != "" && repo.ProviderHost != "" && repo.ProviderOwner != "" && repo.ProviderName != "" {
		return "provider\x00" + repo.Provider + "\x00" + repo.ProviderHost + "\x00" + repo.ProviderOwner + "\x00" + repo.ProviderName
	}
	return "id\x00" + repo.ID
}

// dedupeRepositoriesByIdentity collapses rows that share a
// repositoryIdentityKey — e.g. two rows for the same local_path left behind
// by a resolver race, or the same provider repo registered twice — down to
// one. Keeps the earliest-created row per key (ties broken by the smaller
// ID), matching the winner FindOrCreateRepository /
// FindOrCreateRepositoryByLocalPath resolve future references to via
// GetRepositoryByProviderInfo / GetRepositoryByLocalPath's
// `ORDER BY created_at ASC, id ASC` — so callers do not add a
// task_repositories link the UI would then de-list. This is a read-time
// safety net: it hides pre-existing duplicate rows from callers without
// touching the underlying table, so a caller that still deletes by ID (e.g.
// DeleteRepository) must use the ID this function returned, not one filtered
// out. Preserves the relative order of first occurrence.
func dedupeRepositoriesByIdentity(repos []*models.Repository) []*models.Repository {
	winners := make(map[string]*models.Repository, len(repos))
	for _, repo := range repos {
		if repo == nil {
			continue
		}
		key := repositoryIdentityKey(repo)
		current, ok := winners[key]
		if !ok || repo.CreatedAt.Before(current.CreatedAt) ||
			(repo.CreatedAt.Equal(current.CreatedAt) && repo.ID < current.ID) {
			winners[key] = repo
		}
	}
	deduped := make([]*models.Repository, 0, len(winners))
	seen := make(map[string]struct{}, len(winners))
	for _, repo := range repos {
		if repo == nil {
			continue
		}
		key := repositoryIdentityKey(repo)
		if _, done := seen[key]; done {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, winners[key])
	}
	return deduped
}

// CountActiveSessionsByRepository returns the number of agent sessions in an
// active state (CREATED / STARTING / RUNNING / WAITING_FOR_INPUT) that are
// attached to the given repository. Used by the UI to warn users before they
// attempt to delete a repository that would otherwise be blocked by
// DeleteRepository's ErrActiveTaskSessions sentinel.
func (s *Service) CountActiveSessionsByRepository(ctx context.Context, id string) (int, error) {
	if err := s.authorizeRepositoryID(ctx, id); err != nil {
		return 0, err
	}
	if _, err := s.repoEntities.GetRepository(ctx, id); err != nil {
		return 0, err
	}
	return s.sessions.CountActiveTaskSessionsByRepository(ctx, id)
}

// Repository script operations

func (s *Service) CreateRepositoryScript(ctx context.Context, req *CreateRepositoryScriptRequest) (*models.RepositoryScript, error) {
	if err := s.authorizeRepositoryID(ctx, req.RepositoryID); err != nil {
		return nil, err
	}
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
	script, err := s.repoEntities.GetRepositoryScript(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRepositoryID(ctx, script.RepositoryID); err != nil {
		return nil, err
	}
	return script, nil
}

func (s *Service) UpdateRepositoryScript(ctx context.Context, id string, req *UpdateRepositoryScriptRequest) (*models.RepositoryScript, error) {
	script, err := s.repoEntities.GetRepositoryScript(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRepositoryID(ctx, script.RepositoryID); err != nil {
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
	if err := s.authorizeRepositoryID(ctx, script.RepositoryID); err != nil {
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
	if err := s.authorizeRepositoryID(ctx, repositoryID); err != nil {
		return nil, err
	}
	return s.repoEntities.ListRepositoryScripts(ctx, repositoryID)
}

// ListScriptsByRepositoryIDs returns scripts for multiple repositories in a single query.
func (s *Service) ListScriptsByRepositoryIDs(ctx context.Context, repoIDs []string) (map[string][]*models.RepositoryScript, error) {
	return s.repoEntities.ListScriptsByRepositoryIDs(ctx, repoIDs)
}

// Executor operations

var ErrKubernetesAdminRequired = errors.New("administrator identity required for Kubernetes settings")

// ErrRemoteDockerAdminRequired gates remote Docker executor mutation. A saved
// profile grants effective root on the remote host, so it is not an ordinary
// member operation.
var ErrRemoteDockerAdminRequired = errors.New("administrator identity required for remote Docker settings")

func requireKubernetesAdmin(ctx context.Context) error {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || !identity.IsAdmin() {
		return ErrKubernetesAdminRequired
	}
	return nil
}

func requireRemoteDockerAdmin(ctx context.Context) error {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || !identity.IsAdmin() {
		return ErrRemoteDockerAdminRequired
	}
	return nil
}

// requireExecutorTypeAdmin applies the admin gate for executor types whose
// configuration is an administrative grant over a machine.
func requireExecutorTypeAdmin(ctx context.Context, types ...models.ExecutorType) error {
	for _, t := range types {
		switch t {
		case models.ExecutorTypeKubernetes:
			if err := requireKubernetesAdmin(ctx); err != nil {
				return err
			}
		case models.ExecutorTypeRemoteDocker:
			if err := requireRemoteDockerAdmin(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateExecutorForType(executorType models.ExecutorType, config map[string]string) error {
	if err := validateExecutorConfig(config); err != nil {
		return err
	}
	if executorType != models.ExecutorTypeKubernetes {
		return nil
	}
	if _, err := agentkubernetes.ParseExecutorConfig(config); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutorConfig, err)
	}
	return nil
}

func validateKubernetesProfileConfig(config map[string]string) error {
	if _, err := agentkubernetes.ParseProfileConfig(config); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutorConfig, err)
	}
	return nil
}

func (s *Service) CreateExecutor(ctx context.Context, req *CreateExecutorRequest) (*models.Executor, error) {
	if err := requireExecutorTypeAdmin(ctx, req.Type); err != nil {
		return nil, err
	}
	if err := validateExecutorForType(req.Type, req.Config); err != nil {
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
	s.notifyExecutorSaved(ctx, nil, executor)
	return executor, nil
}

// notifyExecutorSaved forwards a committed executor create/update to the
// wired ExecutorSaveObserver, if any. before is nil on create.
func (s *Service) notifyExecutorSaved(ctx context.Context, before, after *models.Executor) {
	if s.executorSaveObserver == nil {
		return
	}
	s.executorSaveObserver.OnExecutorSaved(ctx, before, after)
}

func (s *Service) GetExecutor(ctx context.Context, id string) (*models.Executor, error) {
	return s.executors.GetExecutor(ctx, id)
}

func (s *Service) UpdateExecutor(ctx context.Context, id string, req *UpdateExecutorRequest) (*models.Executor, error) {
	executor, err := s.executors.GetExecutor(ctx, id)
	if err != nil {
		return nil, err
	}
	targetType := executor.Type
	if req.Type != nil {
		targetType = *req.Type
	}
	if err := requireExecutorTypeAdmin(ctx, executor.Type, targetType); err != nil {
		return nil, err
	}
	if err := validateExecutorUpdateRequest(executor, req); err != nil {
		return nil, err
	}
	if req.Type != nil && *req.Type != executor.Type &&
		(executor.Type == models.ExecutorTypeKubernetes || *req.Type == models.ExecutorTypeKubernetes) {
		retained, inventoryErr := s.hasExecutorRunningInventory(ctx, id)
		if inventoryErr != nil {
			s.logger.Error("failed to check retained Kubernetes inventory before executor type change",
				zap.String("executor_id", id), zap.Error(inventoryErr))
			return nil, inventoryErr
		}
		if retained {
			return nil, ErrActiveTaskSessions
		}
	}
	if err := s.guardRetainedRemoteDockerConnection(ctx, executor, req); err != nil {
		return nil, err
	}
	before := *executor
	applyExecutorUpdates(executor, req)
	executor.UpdatedAt = time.Now().UTC()
	if err := s.executors.UpdateExecutor(ctx, executor); err != nil {
		return nil, err
	}
	s.publishExecutorEvent(ctx, events.ExecutorUpdated, executor)
	s.notifyExecutorSaved(ctx, &before, executor)
	return executor, nil
}

// validateExecutorUpdateRequest validates config and system executor constraints.
func validateExecutorUpdateRequest(executor *models.Executor, req *UpdateExecutorRequest) error {
	targetType := executor.Type
	if req.Type != nil {
		targetType = *req.Type
	}
	if req.Config != nil {
		if err := validateExecutorForType(targetType, req.Config); err != nil {
			return err
		}
	} else if targetType == models.ExecutorTypeKubernetes {
		if err := validateExecutorForType(targetType, executor.Config); err != nil {
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
	if err := requireExecutorTypeAdmin(ctx, executor.Type); err != nil {
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
	// Both types retain compute past an ordinary stop, and both are reached
	// again through this row: deleting it soft-deletes the only record that
	// says where the Pod or container lives.
	if executor.Type == models.ExecutorTypeKubernetes || executor.Type == models.ExecutorTypeRemoteDocker {
		retained, inventoryErr := s.hasExecutorRunningInventory(ctx, id)
		if inventoryErr != nil {
			s.logger.Error("failed to check retained inventory for executor",
				zap.String("executor_id", id),
				zap.String("executor_type", string(executor.Type)),
				zap.Error(inventoryErr))
			return inventoryErr
		}
		if retained {
			return ErrActiveTaskSessions
		}
	}
	if err := s.executors.DeleteExecutor(ctx, id); err != nil {
		return err
	}
	s.publishExecutorEvent(ctx, events.ExecutorDeleted, executor)
	return nil
}

// remoteDockerConnectionKeys are the config fields that decide which daemon a
// remote Docker profile reaches.
// The keys are spelled locally, as this package already does for the SSH
// executor, rather than importing the runtime tier.
var remoteDockerConnectionKeys = []string{
	sshMetaHost,
	sshMetaHostAlias,
	sshMetaPort,
	sshMetaUser,
	sshMetaIdentitySource,
	sshMetaIdentityFile,
	sshMetaProxyJump,
	sshMetaHostFingerprint,
}

// guardRetainedRemoteDockerConnection refuses to repoint a remote Docker
// executor while a container it created is still retained.
//
// An ordinary stop preserves the container, and every later inspect, resume,
// and teardown reaches it through this row's current connection. Changing the
// connection leaves that container on the original host with nothing pointing
// at it. Fields that do not select a daemon, a rename for instance, stay
// editable: the guard protects reachability, not the row.
func (s *Service) guardRetainedRemoteDockerConnection(
	ctx context.Context, executor *models.Executor, req *UpdateExecutorRequest,
) error {
	if executor.Type != models.ExecutorTypeRemoteDocker || req.Config == nil {
		return nil
	}
	if !remoteDockerConnectionChanged(executor.Config, req.Config) {
		return nil
	}
	retained, err := s.hasExecutorRunningInventory(ctx, executor.ID)
	if err != nil {
		s.logger.Error("failed to check retained remote Docker inventory before a connection change",
			zap.String("executor_id", executor.ID), zap.Error(err))
		return err
	}
	if retained {
		return ErrActiveTaskSessions
	}
	return nil
}

func remoteDockerConnectionChanged(current, next map[string]string) bool {
	for _, key := range remoteDockerConnectionKeys {
		if strings.TrimSpace(current[key]) != strings.TrimSpace(next[key]) {
			return true
		}
	}
	return false
}

func (s *Service) hasExecutorRunningInventory(ctx context.Context, executorID string) (bool, error) {
	if retained, err := s.hasKubernetesEnvironmentInventory(ctx, executorID); err != nil || retained {
		return retained, err
	}
	running, err := s.executors.ListExecutorsRunning(ctx)
	if err != nil {
		return false, err
	}
	for _, row := range running {
		if row == nil {
			continue
		}
		currentID := strings.TrimSpace(row.ExecutorID)
		recordedID, recordedOK := row.Metadata[agentkubernetes.MetadataKeyResourceExecutorID].(string)
		recordedID = strings.TrimSpace(recordedID)
		if currentID == executorID {
			return true, nil
		}
		if recordedID == executorID {
			return true, nil
		}
		// Only two matching, nonempty associations can prove that a Kubernetes
		// row belongs to some other executor. Anything less must block cleanup.
		if row.Runtime == agentruntime.RuntimeKubernetes &&
			(currentID == "" || !recordedOK || recordedID == "" || currentID != recordedID) {
			return true, nil
		}
	}
	return false, nil
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
	if err := requireExecutorTypeAdmin(ctx, executor.Type); err != nil {
		return nil, err
	}
	if executor.Type == models.ExecutorTypeKubernetes {
		if err := validateKubernetesProfileConfig(req.Config); err != nil {
			return nil, err
		}
	}
	if err := s.validateGlobalProfileEnvRefs(ctx, req.EnvVars); err != nil {
		return nil, err
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
	if err := requireExecutorTypeAdmin(ctx, executor.Type); err != nil {
		return nil, err
	}
	if executor.Type == models.ExecutorTypeKubernetes {
		config := profile.Config
		if req.Config != nil {
			config = req.Config
		}
		if err := validateKubernetesProfileConfig(config); err != nil {
			return nil, err
		}
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
		if err := s.validateGlobalProfileEnvRefs(ctx, req.EnvVars); err != nil {
			return nil, err
		}
		if executor.Type == models.ExecutorTypeSprites {
			req.EnvVars = mergeSpritesTokenEnvVars(profile.EnvVars, req.EnvVars)
		}
		profile.EnvVars = req.EnvVars
	}
	var updateErr error
	if req.ExpectedUpdatedAt != nil {
		updateErr = s.executors.UpdateExecutorProfileIfUnmodified(ctx, profile, *req.ExpectedUpdatedAt)
	} else {
		updateErr = s.executors.UpdateExecutorProfile(ctx, profile)
	}
	if updateErr != nil {
		return nil, updateErr
	}
	s.publishExecutorProfileEvent(ctx, events.ExecutorProfileUpdated, profile)
	return profile, nil
}

func (s *Service) validateGlobalProfileEnvRefs(ctx context.Context, envVars []models.ProfileEnvVar) error {
	if s.secretStore == nil {
		return nil
	}
	for i, envVar := range envVars {
		if strings.TrimSpace(envVar.SecretID) == "" {
			continue
		}
		if err := secrets.ValidateGlobalReference(ctx, s.secretStore, envVar.SecretID); err != nil {
			return fmt.Errorf("profile env_vars[%d] secret must be global", i)
		}
	}
	return nil
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
	executor, err := s.executors.GetExecutor(ctx, profile.ExecutorID)
	if err != nil && !errors.Is(err, models.ErrExecutorNotFound) {
		return err
	}
	if executor != nil {
		if err := requireExecutorTypeAdmin(ctx, executor.Type); err != nil {
			return err
		}
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
