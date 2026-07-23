package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

// defaultPriority is the default value for the task priority column.
// Used when a caller omits priority so the DB CHECK constraint is satisfied.
const defaultPriority = "medium"

const (
	providerAzureDevOps = "azure_devops"
	providerGitHub      = "github"
	providerGitLab      = "gitlab"
	protocolHTTP        = "http"
	protocolHTTPS       = "https"
)

const defaultKandevTaskWorktreePathSegment = "/.kandev/tasks/"

// ErrSubtaskDepthExceeded is returned when a caller tries to create a
// subtask of a kanban subtask (nesting depth > 1). Office task trees are
// intentionally exempt.
var ErrSubtaskDepthExceeded = fmt.Errorf("cannot create a subtask of a subtask — maximum nesting depth is 1 for kanban tasks. Create a sibling task under the same parent or a top-level task instead")

// ErrTaskAlreadyArchived is returned by ArchiveTask when the target task
// already has archived_at set. Sentinel so cascade callers (e.g.
// DeleteWorkflow) can treat a concurrent archive as a no-op instead of
// aborting the whole operation.
var ErrTaskAlreadyArchived = errors.New("task is already archived")

type taskStopTarget struct {
	sessionID   string
	executionID string
	// terminal indicates the session is already in a terminal state (CANCELLED,
	// COMPLETED, FAILED, IDLE). Stop failures for terminal sessions are expected
	// and must not block environment cleanup — the agent is already gone.
	terminal bool
}

type taskEnvironmentCleanup struct {
	env       *models.TaskEnvironment
	deleteRow bool
}

type taskEnvironmentSessionUsageChecker interface {
	HasActiveTaskSessionsByTaskEnvironmentExcludingTask(ctx context.Context, taskEnvironmentID, taskID string) (bool, error)
}

type taskEnvironmentSessionBorrowerFinder interface {
	FindActiveTaskSessionTaskIDByTaskEnvironmentExcludingTask(ctx context.Context, taskEnvironmentID, taskID string) (string, error)
}

type taskEnvironmentOwnerTransferer interface {
	TransferTaskEnvironmentToTask(ctx context.Context, envID, taskID string) error
}

// Task operations

// isOfficeRequest returns true if the request should create an office task.
func isOfficeRequest(req *CreateTaskRequest) bool {
	return req.ProjectID != "" ||
		req.Origin == models.TaskOriginAgentCreated ||
		req.Origin == models.TaskOriginRoutine ||
		req.Origin == models.TaskOriginOnboarding
}

// CreateTask creates a new task and publishes a task.created event.
// WorkflowID is required for non-ephemeral kanban tasks.
// Office tasks (project_id set, or origin is agent_created/routine)
// auto-resolve to the workspace's office workflow.
// Ephemeral tasks (quick chat, config chat) must NOT have a workflow.
func (s *Service) CreateTask(ctx context.Context, req *CreateTaskRequest) (*models.Task, error) {
	if err := s.validateCreateTaskRequest(req); err != nil {
		return nil, err
	}
	if err := s.validateSubtaskDepth(ctx, req); err != nil {
		return nil, err
	}

	// Subtasks created without explicit repositories inherit the parent's, so
	// an inherit_parent subtask resolves a repo at launch and can reuse the
	// parent's worktree (the UI omits repositories expecting this). Mirrors the
	// MCP create_task path so UI- and agent-created subtasks behave identically.
	if err := s.inheritParentRepositories(ctx, req); err != nil {
		return nil, err
	}

	// For office tasks, resolve workflow from workspace
	if isOfficeRequest(req) && req.WorkflowID == "" {
		if err := s.resolveOfficeWorkflow(ctx, req); err != nil {
			return nil, err
		}
	}

	workflowStepID := s.resolveWorkflowStep(ctx, req)
	task := s.buildTask(req, workflowStepID)

	// Auto-assign identifier for office tasks
	if isOfficeRequest(req) {
		if err := s.assignIdentifier(ctx, task); err != nil {
			return nil, err
		}
	}

	if err := s.tasks.CreateTask(ctx, task); err != nil {
		s.logger.Error("failed to create task", zap.Error(err))
		return nil, err
	}

	// Create blocker relationships if specified.
	for _, blockerID := range req.BlockedBy {
		if err := s.AddBlocker(ctx, task.ID, blockerID); err != nil {
			return nil, fmt.Errorf("add blocker %s: %w", blockerID, err)
		}
	}

	if err := s.createTaskRepositories(ctx, task.ID, req.WorkspaceID, req.Repositories); err != nil {
		return nil, err
	}

	// Load repositories into task for response
	repos, err := s.taskRepos.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		s.logger.Error("failed to list task repositories", zap.Error(err))
	} else {
		task.Repositories = repos
	}

	s.publishTaskEvent(ctx, events.TaskCreated, task, nil)
	s.logger.Info("task created", zap.String("task_id", task.ID), zap.String("title", task.Title))

	return task, nil
}

// inheritParentRepositories fills req.Repositories from the parent task when a
// subtask is created without explicit repositories. This applies to any
// repo-less subtask (not only inherit_parent ones), matching the MCP
// create_task path (mcp/handlers.inheritedRepoInputs) so UI- and agent-created
// subtasks behave identically — the UI's new_workspace mode always sends repos,
// so in practice only inherit_parent reaches here empty. RepositoryID and
// BaseBranch carry over; CheckoutBranch is dropped on purpose because two
// worktrees can't share a working branch, so the subtask branches off the same
// base as the parent.
//
// A lookup failure is returned rather than swallowed: a subtask silently
// created with no repositories can't establish a worktree, which would
// reintroduce the exact fresh-worktree bug this inheritance is meant to fix —
// failing fast surfaces the problem at creation time instead.
func (s *Service) inheritParentRepositories(ctx context.Context, req *CreateTaskRequest) error {
	if req.ParentID == "" || len(req.Repositories) > 0 {
		return nil
	}
	parentRepos, err := s.taskRepos.ListTaskRepositories(ctx, req.ParentID)
	if err != nil {
		return fmt.Errorf("list parent repositories for subtask inheritance: %w", err)
	}
	inherited := make([]TaskRepositoryInput, 0, len(parentRepos))
	for _, r := range parentRepos {
		if r == nil || r.RepositoryID == "" {
			continue
		}
		inherited = append(inherited, TaskRepositoryInput{
			RepositoryID: r.RepositoryID,
			BaseBranch:   r.BaseBranch,
		})
	}
	if len(inherited) > 0 {
		req.Repositories = inherited
	}
	return nil
}

// validateCreateTaskRequest validates constraints for task creation.
func (s *Service) validateCreateTaskRequest(req *CreateTaskRequest) error {
	isOffice := isOfficeRequest(req)
	if !req.IsEphemeral && !isOffice && req.WorkflowID == "" {
		return fmt.Errorf("workflow_id is required for non-ephemeral tasks")
	}
	if req.IsEphemeral && req.WorkflowID != "" {
		return fmt.Errorf("workflow_id must be empty for ephemeral tasks")
	}
	return nil
}

// validateSubtaskDepth prevents nesting deeper than one level for kanban
// (non-office) tasks. Office task trees intentionally allow arbitrary depth.
func (s *Service) validateSubtaskDepth(ctx context.Context, req *CreateTaskRequest) error {
	if req.ParentID == "" {
		return nil
	}
	parent, err := s.tasks.GetTask(ctx, req.ParentID)
	if err != nil {
		return fmt.Errorf("invalid parent_id: %w", err)
	}
	if parent.ParentID != "" && !parent.IsFromOffice {
		return ErrSubtaskDepthExceeded
	}
	return nil
}

// resolveOfficeWorkflow sets WorkflowID on the request from the workspace's office workflow.
func (s *Service) resolveOfficeWorkflow(ctx context.Context, req *CreateTaskRequest) error {
	_, orchWorkflowID, err := s.tasks.GetWorkspaceTaskPrefix(ctx, req.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to get office workflow for workspace: %w", err)
	}
	if orchWorkflowID == "" {
		return fmt.Errorf("workspace %s has no office workflow configured", req.WorkspaceID)
	}
	req.WorkflowID = orchWorkflowID
	return nil
}

// resolveWorkflowStep resolves the starting workflow step for a new task.
func (s *Service) resolveWorkflowStep(ctx context.Context, req *CreateTaskRequest) string {
	workflowStepID := req.WorkflowStepID
	if workflowStepID == "" && req.WorkflowID != "" && s.startStepResolver != nil {
		var resolvedID string
		var err error
		if req.PlanMode {
			resolvedID, err = s.startStepResolver.ResolveFirstStep(ctx, req.WorkflowID)
		} else {
			resolvedID, err = s.startStepResolver.ResolveStartStep(ctx, req.WorkflowID)
		}
		if err != nil {
			s.logger.Warn("failed to resolve start step, using empty",
				zap.String("workflow_id", req.WorkflowID),
				zap.Error(err))
		} else {
			workflowStepID = resolvedID
		}
	}
	return workflowStepID
}

// buildTask constructs a Task model from the CreateTaskRequest.
func (s *Service) buildTask(req *CreateTaskRequest, workflowStepID string) *models.Task {
	state := v1.TaskStateCreated
	if req.State != nil {
		state = *req.State
	}
	origin := req.Origin
	if origin == "" {
		origin = models.TaskOriginManual
	}
	labels := req.Labels
	if labels == "" {
		labels = "[]"
	}
	priority := req.Priority
	if priority == "" {
		// Office tasks have a TEXT priority column with a CHECK constraint
		// against the canonical four-value enum; default empty to defaultPriority
		// so callers (e.g. onboarding) can omit it.
		priority = defaultPriority
	}
	metadata := req.Metadata
	if wsPath := strings.TrimSpace(req.WorkspacePath); wsPath != "" {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata[models.MetaKeyWorkspacePath] = wsPath
	}
	return &models.Task{
		ID:                     uuid.New().String(),
		WorkspaceID:            req.WorkspaceID,
		WorkflowID:             req.WorkflowID,
		WorkflowStepID:         workflowStepID,
		Title:                  req.Title,
		Description:            req.Description,
		State:                  state,
		Priority:               priority,
		Position:               req.Position,
		Metadata:               metadata,
		IsEphemeral:            req.IsEphemeral,
		ParentID:               req.ParentID,
		AssigneeAgentProfileID: req.AssigneeAgentProfileID,
		Origin:                 origin,
		ProjectID:              req.ProjectID,
		Labels:                 labels,
	}
}

// assignIdentifier generates a sequential identifier (e.g. "KAN-1") for the task.
func (s *Service) assignIdentifier(ctx context.Context, task *models.Task) error {
	prefix, _, err := s.tasks.GetWorkspaceTaskPrefix(ctx, task.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to get task prefix: %w", err)
	}
	seq, err := s.tasks.IncrementTaskSequence(ctx, task.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to increment task sequence: %w", err)
	}
	task.Identifier = fmt.Sprintf("%s-%d", prefix, seq)
	return nil
}

// createTaskRepositories creates task-repository associations, resolving local paths to repository IDs.
func (s *Service) createTaskRepositories(ctx context.Context, taskID, workspaceID string, repositories []TaskRepositoryInput) error {
	var repoByPath map[string]*models.Repository
	for _, repoInput := range repositories {
		if repoInput.RepositoryID == "" && repoInput.LocalPath != "" {
			repos, err := s.repoEntities.ListRepositories(ctx, workspaceID)
			if err != nil {
				s.logger.Error("failed to list repositories", zap.Error(err))
				return err
			}
			repoByPath = make(map[string]*models.Repository, len(repos))
			for _, repo := range repos {
				if repo.LocalPath == "" {
					continue
				}
				repoByPath[repo.LocalPath] = repo
			}
			break
		}
	}

	seen := make(map[string]bool, len(repositories))
	for i, repoInput := range repositories {
		repositoryID, baseBranch, _, err := s.resolveRepoInput(ctx, workspaceID, repoInput, repoByPath)
		if err != nil {
			return err
		}
		if repositoryID == "" {
			return fmt.Errorf("repository_id is required")
		}
		// Multi-branch validation: the same repository may appear multiple
		// times in a task on different branches. Identity is
		// (repository_id, base_branch, checkout_branch) — base_branch matters
		// because the worktree executor anchors the branch there while
		// checkout_branch stays empty, and the local-executor flow puts the
		// branch in checkout_branch with base_branch anchored to default_branch.
		// Both shapes must dedup; matching DB key is UNIQUE(task_id,
		// repository_id, base_branch, checkout_branch).
		dedupKey := repositoryID + "\x00" + baseBranch + "\x00" + repoInput.CheckoutBranch
		if seen[dedupKey] {
			label := s.repoDisplayLabel(ctx, repoInput, repositoryID)
			branchLabel := repoInput.CheckoutBranch
			if branchLabel == "" {
				branchLabel = baseBranch
			}
			if branchLabel == "" {
				return fmt.Errorf("repository %q is listed more than once for this task", label)
			}
			return fmt.Errorf("repository %q on branch %q is listed more than once for this task", label, branchLabel)
		}
		seen[dedupKey] = true
		metadata := make(map[string]interface{})
		if prNum := resolvePRNumber(repoInput); prNum > 0 {
			metadata["pr_number"] = prNum
		}
		taskRepo := &models.TaskRepository{
			TaskID:         taskID,
			RepositoryID:   repositoryID,
			BaseBranch:     baseBranch,
			CheckoutBranch: repoInput.CheckoutBranch,
			Position:       i,
			Metadata:       metadata,
		}
		if err := s.taskRepos.CreateTaskRepository(ctx, taskRepo); err != nil {
			s.logger.Error("failed to create task repository", zap.Error(err))
			return err
		}
	}
	return nil
}

// repoDisplayLabel returns a human-readable label for a repository to surface
// in the duplicate-repository error. It prefers owner/name parsed from the
// input's GitHub URL, then the resolved repo entity's owner/name (or bare
// name), and finally falls back to the repositoryID so the message is never
// empty. Best-effort: lookup failures degrade to the next fallback.
func (s *Service) repoDisplayLabel(ctx context.Context, repoInput TaskRepositoryInput, repositoryID string) string {
	if remoteURL := effectiveRemoteURL(repoInput); remoteURL != "" {
		if _, owner, name, _, err := parseRemoteRepositoryURL(remoteURL, repoInput.Provider); err == nil {
			return owner + "/" + name
		}
	}
	if repo, err := s.repoEntities.GetRepository(ctx, repositoryID); err == nil && repo != nil {
		if repo.ProviderOwner != "" && repo.ProviderName != "" {
			return repo.ProviderOwner + "/" + repo.ProviderName
		}
		if repo.Name != "" {
			return repo.Name
		}
	}
	return repositoryID
}

// ResolveRepositoryRef resolves a single TaskRepositoryInput to a
// (repositoryID, baseBranch) pair within the given workspace, creating the
// repository if necessary. Mirrors the resolution used during task creation
// (`createTaskRepositories`), but builds the local-path lookup map on demand
// so callers that only resolve one input (e.g. add_branch) don't need to
// thread the map themselves.
//
// Accepts inputs identified by RepositoryID, GitHubURL, or LocalPath. Returns
// an empty repositoryID with no error when none of those are set, letting
// callers decide whether to fall back to other defaults.
func (s *Service) ResolveRepositoryRef(ctx context.Context, workspaceID string, repoInput TaskRepositoryInput) (repositoryID, baseBranch string, created bool, err error) {
	var repoByPath map[string]*models.Repository
	if repoInput.RepositoryID == "" && repoInput.LocalPath != "" {
		repos, listErr := s.repoEntities.ListRepositories(ctx, workspaceID)
		if listErr != nil {
			return "", "", false, listErr
		}
		repoByPath = make(map[string]*models.Repository, len(repos))
		for _, repo := range repos {
			if repo.LocalPath == "" {
				continue
			}
			repoByPath[repo.LocalPath] = repo
		}
	}
	return s.resolveRepoInput(ctx, workspaceID, repoInput, repoByPath)
}

// resolveRepoInput resolves a RepositoryInput to a repositoryID and baseBranch,
// creating the repository if it doesn't exist yet. Returns created=true only
// when this call inserted a new Repository row (GitHub-URL miss → CreateRepository
// or LocalPath miss → CreateRepository); callers that want to roll back a fresh
// row on a later failure key off this flag.
func (s *Service) resolveRepoInput(ctx context.Context, workspaceID string, repoInput TaskRepositoryInput, repoByPath map[string]*models.Repository) (repositoryID, baseBranch string, created bool, err error) {
	repositoryID = repoInput.RepositoryID
	baseBranch = repoInput.BaseBranch
	if repositoryID != "" {
		return s.resolveRepoInputID(ctx, workspaceID, repositoryID, baseBranch)
	}

	if effectiveRemoteURL(repoInput) != "" {
		return s.resolveRepoInputRemote(ctx, workspaceID, repoInput, baseBranch)
	}

	if repoInput.LocalPath == "" {
		return repositoryID, baseBranch, false, nil
	}
	return s.resolveRepoInputLocal(ctx, workspaceID, repoInput, repoByPath, baseBranch)
}

func (s *Service) resolveRepoInputID(ctx context.Context, workspaceID, repositoryID, baseBranch string) (string, string, bool, error) {
	// Verify the repository belongs to the target workspace. Without this
	// check, an agent that knows a repository UUID from another workspace
	// could associate it with a task in this workspace via the MCP tool's
	// repository_id fast path (the github_url and local_path branches both
	// scope through FindOrCreateRepository, which is workspace-bound).
	repo, lookupErr := s.repoEntities.GetRepository(ctx, repositoryID)
	if lookupErr != nil {
		return "", "", false, fmt.Errorf("looking up repository %q: %w", repositoryID, lookupErr)
	}
	if repo == nil || repo.WorkspaceID != workspaceID {
		return "", "", false, fmt.Errorf("repository %q does not belong to workspace %q", repositoryID, workspaceID)
	}
	replacementID, replacementCreated, replacementErr := s.safeRepositoryIDForTaskWorktree(ctx, workspaceID, repo)
	if replacementErr != nil {
		return "", "", false, replacementErr
	}
	if replacementID != "" {
		return replacementID, baseBranch, replacementCreated, nil
	}
	return repositoryID, baseBranch, false, nil
}

func (s *Service) safeRepositoryIDForTaskWorktree(ctx context.Context, workspaceID string, repo *models.Repository) (string, bool, error) {
	if !s.isKandevTaskWorktreeRepository(repo) {
		return "", false, nil
	}
	if repo.Provider == "" || repo.ProviderOwner == "" || repo.ProviderName == "" {
		return "", false, fmt.Errorf("repository %q points at a Kandev task worktree; use the source repository or GitHub URL", repo.ID)
	}
	existing, err := s.findSafeReplacementRepository(ctx, workspaceID, repo)
	if err != nil {
		return "", false, err
	}
	if existing != nil {
		return existing.ID, false, nil
	}
	created, createErr := s.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:    workspaceID,
		Name:           repo.ProviderOwner + "/" + repo.ProviderName,
		SourceType:     sourceTypeProvider,
		Provider:       repo.Provider,
		ProviderRepoID: repo.ProviderRepoID,
		ProviderHost:   repo.ProviderHost,
		ProviderOwner:  repo.ProviderOwner,
		ProviderName:   repo.ProviderName,
		DefaultBranch:  repo.DefaultBranch,
	})
	if createErr != nil {
		return "", false, fmt.Errorf("create provider repository for task worktree %q: %w", repo.ID, createErr)
	}
	return created.ID, true, nil
}

func (s *Service) replaceTaskWorktreeRepositoryMatch(ctx context.Context, workspaceID string, repo *models.Repository) (*models.Repository, bool, error) {
	replacementID, replacementCreated, err := s.safeRepositoryIDForTaskWorktree(ctx, workspaceID, repo)
	if err != nil {
		return nil, false, err
	}
	if replacementID == "" {
		return repo, false, nil
	}
	replacement, lookupErr := s.repoEntities.GetRepository(ctx, replacementID)
	if lookupErr != nil {
		return nil, false, fmt.Errorf("looking up repository %q: %w", replacementID, lookupErr)
	}
	if replacement == nil {
		return nil, false, fmt.Errorf("replacement repository %q no longer exists", replacementID)
	}
	return replacement, replacementCreated, nil
}

// findSafeReplacementRepository prefers an existing safe local clone over a
// provider row so private/offline repositories can reuse the user's checkout.
func (s *Service) findSafeReplacementRepository(ctx context.Context, workspaceID string, repo *models.Repository) (*models.Repository, error) {
	repos, err := s.repoEntities.ListRepositories(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list repositories for task worktree replacement: %w", err)
	}
	var localClone *models.Repository
	var providerRepo *models.Repository
	for _, candidate := range repos {
		if candidate == nil || candidate.ID == repo.ID {
			continue
		}
		if !sameProviderIdentity(repo, candidate) {
			continue
		}
		if s.isKandevTaskWorktreeRepository(candidate) {
			continue
		}
		if candidate.SourceType == sourceTypeLocal && candidate.LocalPath != "" {
			if localClone == nil {
				localClone = candidate
			}
			continue
		}
		if candidate.SourceType == sourceTypeProvider && providerRepo == nil {
			providerRepo = candidate
		}
	}
	if localClone != nil {
		return localClone, nil
	}
	if providerRepo != nil {
		return providerRepo, nil
	}
	return nil, nil
}

func sameProviderIdentity(left, right *models.Repository) bool {
	return left.Provider == right.Provider &&
		normalizeProviderHost(left.Provider, left.ProviderHost) ==
			normalizeProviderHost(right.Provider, right.ProviderHost) &&
		left.ProviderOwner == right.ProviderOwner &&
		left.ProviderName == right.ProviderName
}

func (s *Service) isKandevTaskWorktreeRepository(repo *models.Repository) bool {
	return repo != nil && isKandevTaskWorktreePath(repo.LocalPath, s.discoveryConfig.TaskWorktreeRoots)
}

func isKandevTaskWorktreePath(path string, taskWorktreeRoots []string) bool {
	normalized := normalizeTaskWorktreePath(path)
	if normalized == "" {
		return false
	}
	for _, root := range taskWorktreeRoots {
		if pathAtOrInsideRoot(normalized, normalizeTaskWorktreePath(root)) {
			return true
		}
	}
	return strings.Contains(normalized, defaultKandevTaskWorktreePathSegment) ||
		strings.HasSuffix(normalized, strings.TrimSuffix(defaultKandevTaskWorktreePathSegment, "/"))
}

func normalizeTaskWorktreePath(path string) string {
	normalized := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if normalized == "." || normalized == "" {
		return ""
	}
	return normalized
}

func pathAtOrInsideRoot(path, root string) bool {
	if path == "" || root == "" {
		return false
	}
	if root != "/" {
		root = strings.TrimRight(root, "/")
	}
	return path == root || strings.HasPrefix(path, root+"/")
}

// resolveRepoInputLocal handles the LocalPath branch of resolveRepoInput.
// Looks the path up in the workspace snapshot; on miss, calls
// CreateRepository (and reports created=true). Extracted to keep
// resolveRepoInput inside the cyclomatic-complexity budget.
func (s *Service) resolveRepoInputLocal(
	ctx context.Context, workspaceID string, repoInput TaskRepositoryInput,
	repoByPath map[string]*models.Repository, baseBranch string,
) (string, string, bool, error) {
	lookupPath := repoInput.LocalPath
	canonicalPath, probedBranch, pathErr := resolveExplicitLocalRepositoryPath(repoInput.LocalPath)
	if pathErr == nil {
		lookupPath = canonicalPath
	}
	repo := repoByPath[lookupPath]
	created := false
	if repo == nil {
		if isKandevTaskWorktreePath(lookupPath, s.discoveryConfig.TaskWorktreeRoots) {
			return "", "", false, fmt.Errorf("local path %q points at a Kandev task worktree; use the source repository or GitHub URL", repoInput.LocalPath)
		}
		name := strings.TrimSpace(repoInput.Name)
		if name == "" {
			name = filepath.Base(repoInput.LocalPath)
		}
		// Resolve default_branch by probing the repo on disk so it's anchored
		// to the integration branch (origin/HEAD or main/master) rather than
		// whatever feature branch the user happens to have checked out. The
		// frontend's `default_branch` hint wins when set; otherwise we probe
		// directly. Falling back to repoInput.BaseBranch is wrong because in
		// the local-executor flow that field carries the user's working
		// branch, which would permanently pin repositories.default_branch to
		// a feature branch and break every downstream merge-base lookup.
		defaultBranch := repoInput.DefaultBranch
		if defaultBranch == "" && pathErr == nil {
			// A manually supplied path is an explicit read-only probe. Canonical
			// repository validation protects the filesystem read; discovery roots
			// only constrain automatic scans.
			if probedBranch != "" {
				defaultBranch = probedBranch
			}
		}
		createdRepo, createErr := s.CreateRepository(ctx, &CreateRepositoryRequest{
			WorkspaceID:   workspaceID,
			Name:          name,
			SourceType:    "local",
			LocalPath:     repoInput.LocalPath,
			DefaultBranch: defaultBranch,
		})
		if createErr != nil {
			return "", "", false, createErr
		}
		repo = createdRepo
		if repoByPath != nil {
			repoByPath[repoInput.LocalPath] = repo
			repoByPath[repo.LocalPath] = repo
		}
		created = true
	} else {
		replacement, replacementCreated, replaceErr := s.replaceTaskWorktreeRepositoryMatch(ctx, workspaceID, repo)
		if replaceErr != nil {
			return "", "", false, replaceErr
		}
		repo = replacement
		created = replacementCreated
	}
	if baseBranch == "" {
		baseBranch = repo.DefaultBranch
	}
	return repo.ID, baseBranch, created, nil
}

// resolveRepoInputGitHub handles the GitHub-URL branch of resolveRepoInput:
// parse owner/name, optionally probe the provider for default_branch, then
// FindOrCreateRepository. Extracted so resolveRepoInput stays under the
// cognitive-complexity budget after adding the probe-skip and probe-error
// arms.
func (s *Service) resolveRepoInputRemote(
	ctx context.Context, workspaceID string, repoInput TaskRepositoryInput, baseBranch string,
) (string, string, bool, error) {
	provider, owner, name, canonicalURL, parseErr := parseRemoteRepositoryURL(
		effectiveRemoteURL(repoInput), repoInput.Provider,
	)
	if parseErr != nil {
		return "", "", false, parseErr
	}
	if repoInput.Provider != "" && !strings.EqualFold(repoInput.Provider, provider) {
		return "", "", false, fmt.Errorf("remote_url provider %q does not match provider %q", provider, repoInput.Provider)
	}
	owner, metadataErr := validateRemoteRepositoryMetadata(repoInput, provider, owner, name)
	if metadataErr != nil {
		return "", "", false, metadataErr
	}
	defaultBranch := firstNonEmpty(repoInput.DefaultBranch, repoInput.BaseBranch)
	if defaultBranch == "" && repoInput.ResolveProviderDefaults && s.providerProber != nil && provider == providerGitHub {
		defaultBranch = s.probeProviderDefaultBranchIfMissing(ctx, workspaceID, provider, owner, name)
	}
	providerHost := remoteProviderHost(provider, canonicalURL)
	repo, repoCreated, createErr := s.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID:    workspaceID,
		Provider:       provider,
		ProviderRepoID: repoInput.ProviderRepoID,
		ProviderHost:   providerHost,
		ProviderOwner:  owner,
		ProviderName:   name,
		RemoteURL:      canonicalURL,
		DefaultBranch:  defaultBranch,
	})
	if createErr != nil {
		return "", "", false, createErr
	}
	if baseBranch == "" {
		baseBranch = repo.DefaultBranch
	}
	return repo.ID, baseBranch, repoCreated, nil
}

func validateRemoteRepositoryMetadata(
	input TaskRepositoryInput, provider, owner, name string,
) (string, error) {
	if input.ProviderOwner != "" && provider != providerAzureDevOps && input.ProviderOwner != owner {
		return "", fmt.Errorf("remote_url owner %q does not match provider_owner %q", owner, input.ProviderOwner)
	}
	if input.ProviderName != "" && input.ProviderName != name {
		return "", fmt.Errorf("remote_url repository %q does not match provider_name %q", name, input.ProviderName)
	}
	if input.ProviderOwner != "" {
		return input.ProviderOwner, nil
	}
	return owner, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func effectiveRemoteURL(input TaskRepositoryInput) string {
	if strings.TrimSpace(input.RemoteURL) != "" {
		return strings.TrimSpace(input.RemoteURL)
	}
	return strings.TrimSpace(input.GitHubURL)
}

func parseRemoteRepositoryURL(raw, providerHint string) (provider, owner, name, canonical string, err error) {
	parsed, sshStyle, parseErr := normalizeRemoteRepositoryURL(raw)
	if parseErr != nil {
		return "", "", "", "", parseErr
	}
	host := strings.ToLower(parsed.Hostname())
	originHost := strings.ToLower(parsed.Host)
	parts := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
	switch host {
	case "github.com", "www.github.com":
		return parseGitHubRemote(host, parts, sshStyle)
	case "gitlab.com", "www.gitlab.com":
		return parseGitLabRemote(parsed.Scheme, strings.TrimPrefix(originHost, "www."), parts, sshStyle)
	case "dev.azure.com":
		return parseAzureHTTPSRemote(parts)
	case "ssh.dev.azure.com":
		return parseAzureSSHRemote(parts)
	default:
		if strings.EqualFold(providerHint, providerGitLab) &&
			(parsed.Scheme == protocolHTTP || parsed.Scheme == protocolHTTPS) {
			return parseGitLabRemote(parsed.Scheme, originHost, parts, false)
		}
		return "", "", "", "", fmt.Errorf("unsupported remote repository host: %s", host)
	}
}

func normalizeRemoteRepositoryURL(raw string) (*url.URL, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false, fmt.Errorf("empty remote repository URL")
	}
	sshStyle := strings.HasPrefix(raw, "git@") && strings.Contains(raw, ":")
	if sshStyle {
		hostAndPath := strings.TrimPrefix(raw, "git@")
		separator := strings.Index(hostAndPath, ":")
		raw = "ssh://git@" + hostAndPath[:separator] + "/" + hostAndPath[separator+1:]
	} else if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, parseErr := url.Parse(raw)
	if parseErr != nil || parsed.Host == "" {
		return nil, false, fmt.Errorf("invalid remote repository URL")
	}
	return parsed, sshStyle, nil
}

func parseGitHubRemote(host string, parts []string, sshStyle bool) (string, string, string, string, error) {
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", "", fmt.Errorf("invalid GitHub repository URL: expected github.com/owner/repo")
	}
	owner, name := parts[0], parts[1]
	canonical := fmt.Sprintf("https://github.com/%s/%s.git", owner, name)
	if sshStyle {
		canonical = fmt.Sprintf("git@%s:%s/%s.git", host, owner, name)
	}
	return providerGitHub, owner, name, canonical, nil
}

func parseGitLabRemote(scheme, host string, parts []string, sshStyle bool) (string, string, string, string, error) {
	for index, part := range parts {
		if part == "-" {
			parts = parts[:index]
			break
		}
	}
	if len(parts) < 2 {
		return "", "", "", "", fmt.Errorf("invalid GitLab repository URL: expected gitlab.com/namespace/repo")
	}
	owner, name := strings.Join(parts[:len(parts)-1], "/"), parts[len(parts)-1]
	if owner == "" || name == "" {
		return "", "", "", "", fmt.Errorf("remote repository owner and name are required")
	}
	if scheme != protocolHTTP && scheme != protocolHTTPS {
		scheme = protocolHTTPS
	}
	canonical := fmt.Sprintf("%s://%s/%s/%s.git", scheme, host, owner, name)
	if sshStyle {
		canonical = fmt.Sprintf("git@%s:%s/%s.git", host, owner, name)
	}
	return providerGitLab, owner, name, canonical, nil
}

func remoteProviderHost(provider, remoteURL string) string {
	if provider == providerGitHub {
		return githubProviderHost
	}
	if !strings.EqualFold(provider, providerGitLab) {
		return ""
	}
	origin, _ := ParseGitRemoteIdentity(remoteURL)
	if strings.HasPrefix(origin, protocolHTTP+"://") || strings.HasPrefix(origin, protocolHTTPS+"://") {
		return origin
	}
	if sshHost := strings.TrimPrefix(origin, "ssh://"); sshHost != origin && sshHost != "" {
		return protocolHTTPS + "://" + sshHost
	}
	return ""
}

func parseAzureHTTPSRemote(parts []string) (string, string, string, string, error) {
	if len(parts) != 4 || parts[2] != "_git" || parts[0] == "" || parts[1] == "" || parts[3] == "" {
		return "", "", "", "", fmt.Errorf("invalid Azure DevOps repository URL: expected dev.azure.com/org/project/_git/repo")
	}
	owner, name := parts[1], parts[3]
	canonical := fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s", parts[0], owner, name)
	return providerAzureDevOps, owner, name, canonical, nil
}

func parseAzureSSHRemote(parts []string) (string, string, string, string, error) {
	if len(parts) != 4 || parts[0] != "v3" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return "", "", "", "", fmt.Errorf("invalid Azure DevOps SSH URL: expected git@ssh.dev.azure.com:v3/org/project/repo")
	}
	owner, name := parts[2], parts[3]
	canonical := fmt.Sprintf("git@ssh.dev.azure.com:v3/%s/%s/%s", parts[1], owner, name)
	return providerAzureDevOps, owner, name, canonical, nil
}

// probeProviderDefaultBranchIfMissing returns a default_branch resolved via
// the provider prober, but only when the workspace doesn't already hold the
// repo with a non-empty default_branch (the existing value wins downstream,
// so the remote round-trip would be pure waste). A DB lookup error skips
// the probe entirely — FindOrCreateRepository will hit the same DB and
// surface the real cause; we log the lookup failure for observability.
// Probe errors fall through to "" so the AddBranchToTask gate surfaces an
// actionable validation rejection rather than a silent orphan.
func (s *Service) probeProviderDefaultBranchIfMissing(
	ctx context.Context, workspaceID, provider, owner, name string,
) string {
	existing, lookupErr := s.repoEntities.GetRepositoryByProviderInfo(ctx, workspaceID, provider, githubProviderHost, owner, name)
	if lookupErr != nil {
		s.logger.Warn("resolveRepoInput: failed to look up existing repo before probe",
			zap.String("provider", provider),
			zap.String("owner", owner),
			zap.String("name", name),
			zap.Error(lookupErr))
		return ""
	}
	if existing != nil && existing.DefaultBranch != "" {
		return ""
	}
	probed, probeErr := s.providerProber.ProbeDefaultBranch(ctx, provider, owner, name)
	if probeErr != nil {
		return ""
	}
	return probed
}

// parseGitHubRepoURL parses a GitHub repository URL into owner and name.
// Supports: https://github.com/owner/repo, github.com/owner/repo,
// https://github.com/owner/repo.git, with optional trailing slashes.
func parseGitHubRepoURL(rawURL string) (owner, name string, err error) {
	provider, owner, name, _, parseErr := parseRemoteRepositoryURL(rawURL, "")
	if parseErr != nil {
		return "", "", parseErr
	}
	if provider != providerGitHub {
		return "", "", fmt.Errorf("not a GitHub URL")
	}
	return owner, name, nil
}

// resolvePRNumber returns the GitHub PR number for a repository input. Prefers
// the explicit PRNumber field; falls back to parsing a /pull/<N> path out of
// GitHubURL when present. Returns 0 when no PR is identified.
//
// The PR number is needed at worktree-creation time so fork PRs (whose head
// branch only exists on the contributor's fork) can be materialized via the
// refs/pull/<N>/head refspec on the base repo instead of a branch-name fetch
// that would 404 against origin.
func resolvePRNumber(input TaskRepositoryInput) int {
	if input.PRNumber > 0 {
		return input.PRNumber
	}
	rawURL := effectiveRemoteURL(input)
	idx := strings.Index(rawURL, "/pull/")
	if idx < 0 {
		return 0
	}
	numStr := rawURL[idx+len("/pull/"):]
	if i := strings.IndexAny(numStr, "/?#"); i >= 0 {
		numStr = numStr[:i]
	}
	n, err := strconv.Atoi(numStr)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// ReplaceTaskRepositories deletes all existing task-repository associations
// and recreates them. Exported for callers that mutate repository inputs
// (e.g. the fresh-branch flow rewriting BaseBranch) after CreateTask has
// already persisted the original set.
func (s *Service) ReplaceTaskRepositories(ctx context.Context, taskID, workspaceID string, repositories []TaskRepositoryInput) error {
	return s.replaceTaskRepositories(ctx, taskID, workspaceID, repositories)
}

// replaceTaskRepositories deletes all existing task-repository associations and recreates them.
func (s *Service) replaceTaskRepositories(ctx context.Context, taskID, workspaceID string, repositories []TaskRepositoryInput) error {
	if err := s.taskRepos.DeleteTaskRepositoriesByTask(ctx, taskID); err != nil {
		s.logger.Error("failed to delete task repositories", zap.Error(err))
		return err
	}
	return s.createTaskRepositories(ctx, taskID, workspaceID, repositories)
}

// GetTask retrieves a task by ID and populates repositories
func (s *Service) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	// Load task repositories
	repos, err := s.taskRepos.ListTaskRepositories(ctx, id)
	if err != nil {
		s.logger.Error("failed to list task repositories", zap.Error(err))
	} else {
		task.Repositories = repos
	}

	return task, nil
}

// UpdateTask updates an existing task and publishes a task.updated event
func (s *Service) UpdateTask(ctx context.Context, id string, req *UpdateTaskRequest) (*models.Task, error) {
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	var oldState *v1.TaskState
	stateChanged := false

	if req.Title != nil {
		task.Title = *req.Title
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}
	if req.State != nil && task.State != *req.State {
		current := task.State
		oldState = &current
		task.State = *req.State
		stateChanged = true
	}
	if req.WorkflowStepID != nil {
		task.WorkflowStepID = *req.WorkflowStepID
	}
	if req.Position != nil {
		task.Position = *req.Position
	}
	if req.Metadata != nil {
		task.Metadata = req.Metadata
	}
	task.UpdatedAt = time.Now().UTC()

	if err := s.tasks.UpdateTask(ctx, task); err != nil {
		s.logger.Error("failed to update task", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	// Update task repositories if provided
	if req.Repositories != nil {
		if err := s.replaceTaskRepositories(ctx, task.ID, task.WorkspaceID, req.Repositories); err != nil {
			return nil, err
		}
	}

	// Load repositories into task for response
	repos, err := s.taskRepos.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		s.logger.Error("failed to list task repositories", zap.Error(err))
	} else {
		task.Repositories = repos
	}

	if stateChanged && oldState != nil {
		s.publishTaskEvent(ctx, events.TaskStateChanged, task, oldState)
	}
	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	s.logger.Info("task updated", zap.String("task_id", task.ID))

	return task, nil
}

type taskMessageRollbackRepository interface {
	RestoreTaskMessageRollbackIfSessionState(
		ctx context.Context,
		task *models.Task,
		sessionID string,
		expectedSessionState models.TaskSessionState,
	) (bool, error)
}

// RestoreTaskMessageRollback restores message_task's task-state/workflow-step
// snapshot only while ownerSessionID still has expectedSessionState. It is a
// narrow compensation API: the repository predicate and both task-field
// writes share one SQL statement, so coordinator cancellation cannot be
// overwritten between a state check and the rollback write.
func (s *Service) RestoreTaskMessageRollback(
	ctx context.Context,
	taskID, ownerSessionID string,
	expectedSessionState models.TaskSessionState,
	state v1.TaskState,
	workflowStepID string,
) (*models.Task, bool, error) {
	repo, ok := s.tasks.(taskMessageRollbackRepository)
	if !ok {
		return nil, false, errors.New("task repository does not support guarded message rollback")
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, false, err
	}
	oldState := task.State
	restoredTask := *task
	restoredTask.State = state
	restoredTask.WorkflowStepID = workflowStepID
	updated, err := repo.RestoreTaskMessageRollbackIfSessionState(
		ctx,
		&restoredTask,
		ownerSessionID,
		expectedSessionState,
	)
	if err != nil || !updated {
		return task, updated, err
	}

	if restoredTask.State != oldState {
		s.publishTaskEvent(ctx, events.TaskStateChanged, &restoredTask, &oldState)
	}
	s.publishTaskEvent(ctx, events.TaskUpdated, &restoredTask, nil)
	return &restoredTask, true, nil
}

// ArchiveTask archives a task by setting its archived_at timestamp.
// The task remains in the DB but is excluded from active board views.
// Active agent sessions are stopped and worktrees cleaned up in background.
func (s *Service) ArchiveTask(ctx context.Context, id string) error {
	start := time.Now()

	// 1. Get task and verify it exists
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return err
	}

	if task.ArchivedAt != nil {
		return fmt.Errorf("%w: %s", ErrTaskAlreadyArchived, id)
	}

	// 2. Gather data needed for cleanup BEFORE archive
	var stopTargets []taskStopTarget
	activeSessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, id)
	if err != nil {
		return fmt.Errorf("list active task sessions for archive: %w", err)
	}
	if s.executionStopper != nil {
		stopTargets, err = s.buildStopTargets(ctx, id, activeSessions)
		if err != nil {
			return fmt.Errorf("list runtime cleanup inventory: %w", err)
		}
	}

	// 2b. Capture git archive snapshot for active sessions BEFORE stopping agents
	// Use a bounded timeout to prevent blocking the archive operation if agentctl is stuck.
	if s.gitArchiveCapture != nil && len(activeSessions) > 0 {
		for _, sess := range activeSessions {
			if sess == nil || sess.ID == "" {
				continue
			}
			snapCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := s.gitArchiveCapture.CaptureArchiveSnapshot(snapCtx, sess.ID)
			cancel()
			if err != nil {
				s.logger.Warn("failed to capture git archive snapshot",
					zap.String("task_id", id),
					zap.String("session_id", sess.ID),
					zap.Error(err))
			}
		}
	}

	sessions, err := s.sessions.ListTaskSessions(ctx, id)
	if err != nil {
		return fmt.Errorf("list task sessions for archive: %w", err)
	}

	worktrees, err := s.gatherWorktreesForDelete(ctx, id)
	if err != nil {
		return fmt.Errorf("list worktrees for archive: %w", err)
	}
	taskEnv, err := s.gatherTaskEnvironmentForCleanup(ctx, id)
	if err != nil {
		return fmt.Errorf("lookup task environment for archive: %w", err)
	}
	envCleanup := taskEnvironmentCleanup{env: taskEnv, deleteRow: true}
	cleanupJob, err := s.persistTaskResourceCleanup(
		ctx, id, models.TaskResourceCleanupTriggerArchive, "",
		sessions, worktrees, stopTargets, envCleanup, true,
	)
	if err != nil {
		return err
	}

	// 3. Set archived_at in DB
	if err := s.tasks.ArchiveTask(ctx, id); err != nil {
		s.cancelTaskResourceCleanupJob(ctx, cleanupJob)
		return err
	}

	// Register the exact inventory before CANCELLED becomes visible. A launch
	// persistence loser can then distinguish these owned executions from one
	// that raced in after the snapshot and must clean itself up.
	s.registerTaskRuntimeStopOwners(stopTargets, true)

	// 3b. Finalize active sessions in the DB. The async cleanup below tears down
	// the agent processes; this records the terminal session state, which
	// process teardown does not persist on its own. Publish a
	// session.state_changed event per session actually cancelled — the DB
	// write alone is invisible to any client cache (e.g. an Office task
	// list's "is running" indicator) that's kept fresh exclusively by that
	// event, and would otherwise show a live spinner forever after archive.
	cancelledIDs, cancelledAt, cancelErr := s.sessions.CancelActiveTaskSessionsByTaskID(ctx, id, "task archived")
	if cancelErr != nil {
		s.logger.Warn("failed to reap active sessions on archive",
			zap.String("task_id", id),
			zap.Error(cancelErr))
	} else if len(cancelledIDs) > 0 {
		s.logger.Info("reaped active sessions on archive",
			zap.String("task_id", id),
			zap.Int("count", len(cancelledIDs)))
		// Detach from ctx via WithoutCancel: the DB write above already
		// committed on a detached context, so a client disconnect here must
		// not also suppress the event lookup below (GetTaskSession per
		// cancelled session) — event-driven clients need session.state_changed
		// regardless of whether the archiving caller is still connected. Bound
		// it with a timeout to prevent blocking ArchiveTask forever if that
		// lookup or a synchronous event subscriber stalls.
		pubCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		s.publishSessionsCancelled(pubCtx, id, activeSessions, cancelledIDs, "task archived", cancelledAt)
	}

	// 4. Re-read task for updated archived_at field
	task, err = s.tasks.GetTask(ctx, id)
	if err != nil {
		return err
	}

	// 5. Publish task.updated event so frontend removes from board
	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	s.logger.Info("task archived",
		zap.String("task_id", id),
		zap.Duration("duration", time.Since(start)))

	// 6. Background: Stop agents and cleanup worktrees
	if cleanupJob != nil {
		if err := s.StartPreparedTaskResourceCleanup(ctx, cleanupJob.OperationID); err != nil {
			s.logger.Warn("start committed archive resource cleanup",
				zap.String("job_id", cleanupJob.ID), zap.String("task_id", id), zap.Error(err))
		}
	} else if len(stopTargets) > 0 || s.worktreeCleanup != nil || len(sessions) > 0 || taskEnv != nil {
		s.runAsyncTaskCleanup(id, sessions, worktrees, stopTargets, envCleanup,
			"task archived", "failed to stop session on task archive", "task archive cleanup completed")
	}

	return nil
}

func (s *Service) registerTaskRuntimeStopOwners(stopTargets []taskStopTarget, force bool) {
	if s.executionStopper == nil {
		return
	}
	for _, target := range stopTargets {
		if target.sessionID == "" || target.executionID == "" {
			continue
		}
		s.executionStopper.RegisterExecutionStopOwner(
			target.sessionID,
			target.executionID,
			force,
		)
	}
}

// DeleteTask deletes a task and publishes a task.deleted event.
// For fast UI response, the DB delete and event publish happen synchronously,
// while agent stopping and worktree cleanup happen asynchronously.
func (s *Service) DeleteTask(ctx context.Context, id string) error {
	return s.deleteTaskWithReason(ctx, id, "")
}

// DeleteTaskWithReason behaves like DeleteTask but attaches a machine-readable
// reason (e.g. "pr_approved_by_user") to the task.deleted event so the frontend
// can explain why a focused task vanished.
func (s *Service) DeleteTaskWithReason(ctx context.Context, id, reason string) error {
	return s.deleteTaskWithReason(ctx, id, reason)
}

func (s *Service) deleteTaskWithReason(ctx context.Context, id, reason string) error {
	_, err := s.deleteTaskWithReasonAndDBDelete(ctx, id, reason, models.TaskResourceCleanupTriggerDelete, func(ctx context.Context, id string) (bool, error) {
		if err := s.tasks.DeleteTask(ctx, id); err != nil {
			return false, err
		}
		return true, nil
	})
	return err
}

func (s *Service) deleteExpiredQuickChatTask(ctx context.Context, id string, cutoff time.Time) (bool, error) {
	deleted, err := s.deleteTaskWithReasonAndDBDelete(ctx, id, "", models.TaskResourceCleanupTriggerQuickChatExpire, func(ctx context.Context, id string) (bool, error) {
		return s.tasks.DeleteExpiredQuickChatTask(ctx, id, cutoff)
	})
	if errors.Is(err, taskrepo.ErrTaskNotFound) {
		return false, nil
	}
	return deleted, err
}

func (s *Service) deleteTaskWithReasonAndDBDelete(
	ctx context.Context,
	id string,
	reason string,
	trigger models.TaskResourceCleanupTrigger,
	deleteFromDB func(context.Context, string) (bool, error),
) (bool, error) {
	start := time.Now()

	// 1. Get task (sync, fast)
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return false, err
	}

	// 2. Gather data needed for cleanup BEFORE delete (sync, fast)
	sessions, err := s.sessions.ListTaskSessions(ctx, id)
	if err != nil {
		return false, fmt.Errorf("list task sessions for delete: %w", err)
	}

	worktrees, err := s.gatherWorktreesForDelete(ctx, id)
	if err != nil {
		return false, fmt.Errorf("list worktrees for delete: %w", err)
	}
	taskEnv, err := s.gatherTaskEnvironmentForCleanup(ctx, id)
	if err != nil {
		return false, fmt.Errorf("lookup task environment for delete: %w", err)
	}
	stopTargets, err := s.deleteTaskStopTargets(ctx, id)
	if err != nil {
		return false, err
	}
	if preserved, err := s.preserveTaskEnvironmentForActiveBorrower(ctx, id, taskEnv); err != nil {
		return false, err
	} else if preserved {
		s.logger.Info("transferred borrowed task environment before task delete",
			zap.String("task_id", id),
			zap.String("env_id", taskEnvironmentID(taskEnv)),
			zap.String("new_owner_task_id", taskEnv.TaskID))
	}

	envCleanup := taskEnvironmentCleanup{env: taskEnv, deleteRow: false}
	cleanupJob, err := s.persistTaskResourceCleanup(
		ctx, id, trigger, "", sessions, worktrees, stopTargets, envCleanup, true,
	)
	if err != nil {
		return false, err
	}

	// 4. Delete from DB (sync, fast)
	deleted, err := deleteFromDB(ctx, id)
	if err != nil {
		s.cancelTaskResourceCleanupJob(ctx, cleanupJob)
		s.logger.Error("failed to delete task", zap.String("task_id", id), zap.Error(err))
		return false, err
	}
	if !deleted {
		s.cancelTaskResourceCleanupJob(ctx, cleanupJob)
		return false, nil
	}

	// 5. Publish event (sync, fast) - frontend removes task immediately
	var extra map[string]interface{}
	if reason != "" {
		extra = map[string]interface{}{"reason": reason}
	}
	s.publishTaskEventWithExtra(ctx, events.TaskDeleted, task, nil, extra)
	s.logger.Info("task deleted",
		zap.String("task_id", id),
		zap.Duration("duration", time.Since(start)))

	// 6. Stop agents and cleanup worktrees in the background. Carry the
	//    envCleanup struct so the task environment row is reset alongside
	//    the worktrees (an extra task.taskEnv != nil branch keeps the
	//    cleanup running when only the env needs reclaiming).
	hasCleanup := len(stopTargets) > 0 || s.worktreeCleanup != nil || len(sessions) > 0 || task.IsEphemeral || taskEnv != nil
	if cleanupJob != nil {
		if err := s.StartPreparedTaskResourceCleanup(ctx, cleanupJob.OperationID); err != nil {
			s.logger.Warn("start committed delete resource cleanup",
				zap.String("job_id", cleanupJob.ID), zap.String("task_id", id), zap.Error(err))
		}
	} else if hasCleanup {
		s.runAsyncTaskCleanup(id, sessions, worktrees, stopTargets, envCleanup,
			"task deleted", "failed to stop session on task delete", "task cleanup completed")
	}

	return true, nil
}

func (s *Service) deleteTaskStopTargets(ctx context.Context, id string) ([]taskStopTarget, error) {
	// Must query before delete since DB records will be gone.
	if s.executionStopper == nil {
		return nil, nil
	}
	activeSessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list active sessions for delete: %w", err)
	}
	stopTargets, err := s.buildStopTargets(ctx, id, activeSessions)
	if err != nil {
		return nil, fmt.Errorf("list runtime cleanup inventory: %w", err)
	}
	return stopTargets, nil
}

// CleanupTaskResources tears down a task's runtime resources (container,
// sandbox, worktree, executor_running rows, quick-chat dir, task_environment
// row) AFTER the task row has been archived or deleted by another path.
//
// Used by HandoffService.ArchiveTaskTree / DeleteTaskTree, which bypass
// Service.ArchiveTask / Service.DeleteTask and therefore miss the runtime
// teardown those wrappers run via runAsyncTaskCleanup. Without this call the
// agent gets stopped but its container/sandbox leaks indefinitely.
//
// The caller may already have cancelled active runs separately (cascade does
// this via runCanceller before invoking us), but terminal sessions can still
// have executors_running rows. We still derive runtime stop targets from
// executors_running here so cascade cleanup does not drop durable handles.
// deleteEnvRow controls whether the task_environment row is removed (true
// for delete cascade, false for archive — archive preserves the row). Runtime
// inventory failures abort cleanup so durable stop handles remain retryable.
func (s *Service) CleanupTaskResources(ctx context.Context, taskID string, deleteEnvRow bool) {
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to list sessions for cascade cleanup",
			zap.String("task_id", taskID),
			zap.Error(err))
	}
	var stopTargets []taskStopTarget
	if s.executionStopper != nil {
		stopTargets, err = s.buildStopTargets(ctx, taskID, sessions)
		if err != nil {
			s.logger.Warn("skipping cascade cleanup because runtime inventory failed",
				zap.String("task_id", taskID),
				zap.Error(err))
			return
		}
	}
	worktrees, err := s.gatherWorktreesForDelete(ctx, taskID)
	if err != nil {
		s.logger.Warn("skipping cascade cleanup because worktree inventory failed",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	taskEnv, err := s.gatherTaskEnvironmentForCleanup(ctx, taskID)
	if err != nil {
		s.logger.Warn("skipping cascade cleanup because task environment inventory failed",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if deleteEnvRow {
		preserved, err := s.preserveTaskEnvironmentForActiveBorrower(ctx, taskID, taskEnv)
		if err != nil {
			s.logger.Warn("skipping cascade cleanup because task environment could not be preserved for borrower",
				zap.String("task_id", taskID),
				zap.String("env_id", taskEnvironmentID(taskEnv)),
				zap.Error(err))
			return
		}
		if preserved {
			deleteEnvRow = false
			s.logger.Info("transferred borrowed task environment before cascade delete",
				zap.String("task_id", taskID),
				zap.String("env_id", taskEnvironmentID(taskEnv)),
				zap.String("new_owner_task_id", taskEnv.TaskID))
		}
	}
	envCleanup := taskEnvironmentCleanup{env: taskEnv, deleteRow: deleteEnvRow}
	if len(sessions) == 0 && len(worktrees) == 0 && len(stopTargets) == 0 && taskEnv == nil {
		return
	}
	reason := "cascade archive"
	if deleteEnvRow {
		reason = "cascade delete"
	}
	s.runAsyncTaskCleanup(taskID, sessions, worktrees, stopTargets, envCleanup,
		reason, "failed to stop session on cascade cleanup", "cascade cleanup completed")
}

// gatherWorktreesForDelete collects worktrees for a task before it is deleted.
// For legacy WorktreeCleanup implementations that do not implement WorktreeProvider,
// it triggers cleanup immediately and returns nil.
func (s *Service) gatherWorktreesForDelete(ctx context.Context, taskID string) ([]*worktree.Worktree, error) {
	if s.worktreeCleanup == nil {
		return nil, nil
	}
	provider, ok := s.worktreeCleanup.(WorktreeProvider)
	if !ok {
		// Durable cleanup must persist its intent before invoking a destructive
		// legacy callback. Non-durable test/legacy wiring keeps the old behavior.
		if s.resourceCleanups == nil {
			if err := s.worktreeCleanup.OnTaskDeleted(ctx, taskID); err != nil {
				s.logger.Warn("failed to cleanup worktree on task deletion",
					zap.String("task_id", taskID), zap.Error(err))
			}
		}
		return nil, nil
	}
	worktrees, err := provider.GetAllByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return worktrees, nil
}

func (s *Service) gatherTaskEnvironmentForCleanup(ctx context.Context, taskID string) (*models.TaskEnvironment, error) {
	if s.taskEnvironments == nil {
		return nil, nil
	}
	env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return env, nil
}

func (s *Service) runAsyncTaskCleanup(
	id string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	stopTargets []taskStopTarget,
	envCleanup taskEnvironmentCleanup,
	stopReason, stopFailMsg, cleanupMsg string,
) {
	go s.runTaskCleanup(id, sessions, worktrees, stopTargets, envCleanup, stopReason, stopFailMsg, cleanupMsg)
}

func (s *Service) runTaskCleanup(
	id string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	stopTargets []taskStopTarget,
	envCleanup taskEnvironmentCleanup,
	stopReason, stopFailMsg, cleanupMsg string,
) {
	cleanupStart := time.Now()
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	refreshedTargets, err := s.refreshTaskRuntimeStopTargets(cleanupCtx, id, stopTargets)
	if err != nil {
		s.logger.Warn(cleanupMsg+" deferred because runtime inventory refresh failed",
			zap.String("task_id", id),
			zap.Error(err))
		s.signalCleanupDoneForTest()
		return
	}
	stopTargets = refreshedTargets
	s.registerTaskRuntimeStopOwners(stopTargets, true)

	failedStops := s.stopTaskRuntimeTargets(cleanupCtx, id, stopTargets, stopReason, stopFailMsg)

	cleanupErrors := s.performTaskCleanup(cleanupCtx, id, sessions, worktrees, stopTargets, envCleanup, failedStops)

	if len(cleanupErrors) > 0 {
		s.logger.Warn(cleanupMsg+" with errors",
			zap.String("task_id", id),
			zap.Int("error_count", len(cleanupErrors)),
			zap.Duration("duration", time.Since(cleanupStart)))
	} else {
		s.logger.Info(cleanupMsg,
			zap.String("task_id", id),
			zap.Duration("duration", time.Since(cleanupStart)))
	}
	s.signalCleanupDoneForTest()
}

// isCleanableSessionState reports whether a session has no running agent process
// and stop failures are therefore expected. Unlike the orchestrator's
// isTerminalSessionState (which excludes IDLE), this helper is used only during
// task cleanup to decide whether a stop failure should block environment teardown.
// IDLE is included because an idle session has already released its execution slot
// and will return ErrExecutionNotFound just like CANCELLED/COMPLETED/FAILED.
func isCleanableSessionState(state models.TaskSessionState) bool {
	switch state {
	case models.TaskSessionStateCancelled,
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateIdle:
		return true
	}
	return false
}

func (s *Service) buildStopTargets(ctx context.Context, taskID string, activeSessions []*models.TaskSession) ([]taskStopTarget, error) {
	targets := make([]taskStopTarget, 0, len(activeSessions))
	seen := make(map[string]struct{})
	// Index session states so executor_running rows can be marked terminal.
	sessionStates := make(map[string]models.TaskSessionState, len(activeSessions))
	for _, sess := range activeSessions {
		if sess != nil {
			sessionStates[sess.ID] = sess.State
		}
	}
	if s.executors != nil {
		runningRows, err := s.executors.ListExecutorsRunningByTaskID(ctx, taskID)
		if err != nil {
			return nil, err
		}
		for _, running := range runningRows {
			if running == nil || running.SessionID == "" {
				continue
			}
			target := taskStopTarget{
				sessionID:   running.SessionID,
				executionID: strings.TrimSpace(running.AgentExecutionID),
				terminal:    isCleanableSessionState(sessionStates[running.SessionID]),
			}
			targets = append(targets, target)
			seen[target.sessionID] = struct{}{}
		}
	}
	for _, sess := range activeSessions {
		if sess == nil || sess.ID == "" {
			continue
		}
		if _, ok := seen[sess.ID]; ok {
			continue
		}
		// Sessions without an executor_running row that are already in a terminal
		// state have no running process; skip creating a stop target.
		if isCleanableSessionState(sess.State) {
			continue
		}
		target := taskStopTarget{
			sessionID:   sess.ID,
			executionID: strings.TrimSpace(sess.AgentExecutionID),
		}
		if target.executionID == "" && s.executors != nil {
			running, err := s.executors.GetExecutorRunningBySessionID(ctx, sess.ID)
			if err == nil && running != nil {
				target.executionID = strings.TrimSpace(running.AgentExecutionID)
			}
		}
		targets = append(targets, target)
	}
	s.logger.Debug("prepared task cleanup stop targets",
		zap.String("task_id", taskID),
		zap.Int("count", len(targets)))
	return targets, nil
}

// refreshTaskRuntimeStopTargets merges the durable/pre-mutation snapshot with
// the live runtime inventory immediately before teardown. A launch may rotate
// an execution after the snapshot but before archive/delete becomes visible;
// keeping every exact ID makes cleanup safe across that race and worker
// restarts. A session-only fallback is retained only when no exact ID exists
// for that session; otherwise it would observe absence after the exact stop and
// keep a durable cleanup retrying forever.
func (s *Service) refreshTaskRuntimeStopTargets(
	ctx context.Context,
	taskID string,
	snapshot []taskStopTarget,
) ([]taskStopTarget, error) {
	if s.executionStopper == nil || s.executors == nil {
		return snapshot, nil
	}
	runningRows, err := s.executors.ListExecutorsRunningByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return mergeTaskStopTargets(taskStopTargetsFromRunningRows(runningRows), snapshot), nil
}

func taskStopTargetsFromRunningRows(runningRows []*models.ExecutorRunning) []taskStopTarget {
	targets := make([]taskStopTarget, 0, len(runningRows))
	for _, running := range runningRows {
		if running == nil {
			continue
		}
		targets = append(targets, taskStopTarget{
			sessionID:   strings.TrimSpace(running.SessionID),
			executionID: strings.TrimSpace(running.AgentExecutionID),
		})
	}
	return targets
}

func mergeTaskStopTargets(live, snapshot []taskStopTarget) []taskStopTarget {
	sessionsWithExactTarget := exactTaskStopTargetSessions(live, snapshot)
	targets := make([]taskStopTarget, 0, len(live)+len(snapshot))
	seen := make(map[string]int, len(live)+len(snapshot))
	appendTarget := func(target taskStopTarget) {
		target.sessionID = strings.TrimSpace(target.sessionID)
		target.executionID = strings.TrimSpace(target.executionID)
		if target.sessionID == "" {
			return
		}
		if target.executionID == "" {
			if _, hasExact := sessionsWithExactTarget[target.sessionID]; hasExact {
				return
			}
		}
		key := target.sessionID + "\x00" + target.executionID
		if index, exists := seen[key]; exists {
			targets[index].terminal = targets[index].terminal || target.terminal
			return
		}
		seen[key] = len(targets)
		targets = append(targets, target)
	}
	for _, target := range live {
		appendTarget(target)
	}
	for _, target := range snapshot {
		appendTarget(target)
	}
	return targets
}

func exactTaskStopTargetSessions(targetSets ...[]taskStopTarget) map[string]struct{} {
	exact := make(map[string]struct{})
	for _, targets := range targetSets {
		for _, target := range targets {
			sessionID := strings.TrimSpace(target.sessionID)
			if sessionID != "" && strings.TrimSpace(target.executionID) != "" {
				exact[sessionID] = struct{}{}
			}
		}
	}
	return exact
}

func (s *Service) stopTaskRuntimeTargets(ctx context.Context, taskID string, stopTargets []taskStopTarget, stopReason, stopFailMsg string) map[string]struct{} {
	failedStops := make(map[string]struct{})
	if s.executionStopper == nil || len(stopTargets) == 0 {
		return failedStops
	}
	for _, target := range stopTargets {
		if context.Cause(ctx) != nil {
			return failedStops
		}
		if target.executionID != "" {
			if err := s.executionStopper.StopExecution(ctx, target.executionID, stopReason, true); err != nil {
				if runtimeStopAlreadyComplete(err) {
					continue
				}
				if target.terminal {
					s.logger.Debug("stop failed for terminal session execution (expected), proceeding with cleanup",
						zap.String("task_id", taskID),
						zap.String("session_id", target.sessionID),
						zap.Error(err))
					continue
				}
				failedStops[target.sessionID] = struct{}{}
				s.logger.Warn(stopFailMsg,
					zap.String("task_id", taskID),
					zap.String("session_id", target.sessionID),
					zap.String("execution_id", target.executionID),
					zap.Error(err))
			}
			continue
		}
		if err := s.executionStopper.StopSession(ctx, target.sessionID, stopReason, true); err != nil {
			if target.terminal {
				s.logger.Debug("stop failed for terminal session (expected), proceeding with cleanup",
					zap.String("task_id", taskID),
					zap.String("session_id", target.sessionID),
					zap.Error(err))
				continue
			}
			failedStops[target.sessionID] = struct{}{}
			s.logger.Warn(stopFailMsg,
				zap.String("task_id", taskID),
				zap.String("session_id", target.sessionID),
				zap.Error(err))
		}
	}
	return failedStops
}

func runtimeStopAlreadyComplete(err error) bool {
	return errors.Is(err, runtimeapi.ErrNotFound)
}

// performTaskCleanup handles post-deletion cleanup operations.
// Handles worktree cleanup, executor_running records, and quick-chat workspace directories.
// Agent stopping is handled separately in the DeleteTask background goroutine.
// Returns a slice of errors encountered (empty if all succeeded).
func (s *Service) performTaskCleanup(
	ctx context.Context,
	taskID string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	stopTargets []taskStopTarget,
	envCleanup taskEnvironmentCleanup,
	preserveExecutorRows map[string]struct{},
) []error {
	var errs []error
	if cause := context.Cause(ctx); cause != nil {
		return []error{cause}
	}
	hasPreservedRuntimes := len(preserveExecutorRows) > 0

	if hasPreservedRuntimes {
		s.logger.Warn("skipping shared environment cleanup after failed runtime stop",
			zap.String("task_id", taskID),
			zap.Int("preserved_runtime_count", len(preserveExecutorRows)))
	}
	errs = append(errs, s.cleanupDestructiveTaskResources(ctx, taskID, sessions, worktrees, envCleanup, preserveExecutorRows)...)
	if cause := context.Cause(ctx); cause != nil {
		return append(errs, cause)
	}

	sessionIDs := cleanupSessionIDs(sessions, stopTargets)
	for _, sessionID := range sessionIDs {
		if _, preserve := preserveExecutorRows[sessionID]; preserve {
			s.logger.Warn("preserving executor runtime row after failed stop",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID))
			continue
		}
		if cause := context.Cause(ctx); cause != nil {
			return append(errs, cause)
		}
		if err := s.executors.DeleteExecutorRunningBySessionID(ctx, sessionID); err != nil {
			s.logger.Debug("failed to delete executor runtime for session",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
			// Don't add to errs - this is a debug-level issue
		}
	}

	// Cleanup quick-chat workspace directories for all tasks (not just ephemeral).
	// Non-ephemeral office tasks also get quick-chat dirs allocated by manager_launch.go;
	// both cases must be cleaned up to avoid a disk leak.
	if cause := context.Cause(ctx); cause != nil {
		return append(errs, cause)
	}
	errs = append(errs, s.cleanupQuickChatDirs(ctx, taskID, sessionIDs, preserveExecutorRows)...)

	return errs
}

func (s *Service) signalCleanupDoneForTest() {
	if s.cleanupDoneForTest == nil {
		return
	}
	select {
	case s.cleanupDoneForTest <- struct{}{}:
	default:
	}
}

func cleanupSessionIDs(sessions []*models.TaskSession, stopTargets []taskStopTarget) []string {
	sessionIDs := make([]string, 0, len(sessions)+len(stopTargets))
	seen := make(map[string]struct{})
	for _, session := range sessions {
		if session == nil {
			continue
		}
		sessionIDs = appendUniqueSessionID(sessionIDs, seen, session.ID)
	}
	for _, target := range stopTargets {
		sessionIDs = appendUniqueSessionID(sessionIDs, seen, target.sessionID)
	}
	return sessionIDs
}

func appendUniqueSessionID(sessionIDs []string, seen map[string]struct{}, sessionID string) []string {
	if sessionID == "" {
		return sessionIDs
	}
	if _, ok := seen[sessionID]; ok {
		return sessionIDs
	}
	seen[sessionID] = struct{}{}
	return append(sessionIDs, sessionID)
}

func (s *Service) cleanupQuickChatDirs(
	ctx context.Context,
	taskID string,
	sessionIDs []string,
	preserveExecutorRows map[string]struct{},
) []error {
	if s.quickChatDir == "" {
		return nil
	}
	var errs []error
	for _, sessionID := range sessionIDs {
		if _, preserve := preserveExecutorRows[sessionID]; preserve {
			continue
		}
		sessionDir := filepath.Join(s.quickChatDir, sessionID)
		if _, statErr := os.Stat(sessionDir); statErr != nil {
			// Directory does not exist — nothing to remove.
			continue
		}
		if cause := context.Cause(ctx); cause != nil {
			return append(errs, cause)
		}
		if err := os.RemoveAll(sessionDir); err != nil {
			s.logger.Warn("failed to cleanup quick-chat workspace directory",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("path", sessionDir),
				zap.Error(err))
			errs = append(errs, fmt.Errorf("cleanup quick-chat dir %s: %w", sessionID, err))
			continue
		}
		s.logger.Debug("cleaned up quick-chat workspace directory",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("path", sessionDir))
	}
	return errs
}

func (s *Service) cleanupDestructiveTaskResources(
	ctx context.Context,
	taskID string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	envCleanup taskEnvironmentCleanup,
	preserveExecutorRows map[string]struct{},
) []error {
	var errs []error
	if cause := context.Cause(ctx); cause != nil {
		return []error{cause}
	}
	skipOwnedEnvironment, err := s.hasActiveOtherTaskSessionsForEnvironment(ctx, taskID, envCleanup.env)
	if err != nil {
		s.logger.Warn("skipping task environment cleanup after shared-environment ownership check failed",
			zap.String("task_id", taskID),
			zap.String("env_id", taskEnvironmentID(envCleanup.env)),
			zap.Error(err))
		errs = append(errs, fmt.Errorf("check task environment ownership %s: %w", taskEnvironmentID(envCleanup.env), err))
		skipOwnedEnvironment = true
	}
	if skipOwnedEnvironment {
		s.logger.Info("skipping task environment cleanup while another task still uses it",
			zap.String("task_id", taskID),
			zap.String("env_id", taskEnvironmentID(envCleanup.env)))
	}
	if cause := context.Cause(ctx); cause != nil {
		return append(errs, cause)
	}
	if len(preserveExecutorRows) == 0 && !skipOwnedEnvironment {
		errs = append(errs, s.cleanupTaskEnvironment(ctx, taskID, envCleanup)...)
		if cause := context.Cause(ctx); cause != nil {
			return append(errs, cause)
		}
	}
	originalWorktreeCount := len(worktrees)
	worktrees = s.filterOwnedWorktreesForTaskCleanup(ctx, taskID, sessions, worktrees, envCleanup.env, skipOwnedEnvironment)
	worktrees = cleanupEligibleWorktrees(worktrees, envCleanup.env, preserveExecutorRows)
	var referenceErrs []error
	worktrees, referenceErrs = s.filterSharedWorktreesForTaskCleanup(ctx, taskID, sessions, worktrees)
	errs = append(errs, referenceErrs...)
	if len(worktrees) == 0 {
		if originalWorktreeCount > 0 {
			s.logger.Debug("no task worktrees eligible for cleanup",
				zap.String("task_id", taskID),
				zap.Int("input_count", originalWorktreeCount),
				zap.Int("preserved_runtime_count", len(preserveExecutorRows)))
		}
		return errs
	}
	cleaner, ok := s.worktreeCleanup.(WorktreeBatchCleaner)
	if !ok {
		return errs
	}
	if cause := context.Cause(ctx); cause != nil {
		return append(errs, cause)
	}
	if err := cleaner.CleanupWorktrees(ctx, worktrees); err != nil {
		s.logger.Warn("failed to cleanup worktrees after delete",
			zap.String("task_id", taskID),
			zap.Error(err))
		errs = append(errs, fmt.Errorf("cleanup worktrees: %w", err))
	}
	return errs
}

func (s *Service) filterSharedWorktreesForTaskCleanup(
	ctx context.Context,
	taskID string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
) ([]*worktree.Worktree, []error) {
	guard, ok := s.worktreeCleanup.(worktreeReferenceGuard)
	if !ok || len(worktrees) == 0 {
		return worktrees, nil
	}
	excludedSessionIDs := taskSessionIDs(sessions)
	filtered := worktrees[:0]
	var errs []error
	for _, wt := range worktrees {
		count, err := guard.CountActiveWorktreeReferences(ctx, wt.ID, excludedSessionIDs)
		if err != nil {
			errs = append(errs, fmt.Errorf("count active references for worktree %s: %w", wt.ID, err))
			continue
		}
		if count == 0 {
			filtered = append(filtered, wt)
			continue
		}
		if err := guard.ReleaseWorktreeReference(ctx, wt); err != nil {
			errs = append(errs, fmt.Errorf("release shared worktree reference %s: %w", wt.ID, err))
			continue
		}
		s.logger.Info("preserving worktree still referenced by another active task",
			zap.String("task_id", taskID),
			zap.String("worktree_id", wt.ID),
			zap.Int("active_references", count))
	}
	return filtered, errs
}

func taskSessionIDs(sessions []*models.TaskSession) []string {
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		if session != nil && session.ID != "" {
			ids = append(ids, session.ID)
		}
	}
	return ids
}

func taskEnvironmentID(env *models.TaskEnvironment) string {
	if env == nil {
		return ""
	}
	return env.ID
}

func (s *Service) hasActiveOtherTaskSessionsForEnvironment(ctx context.Context, taskID string, env *models.TaskEnvironment) (bool, error) {
	if env == nil || env.ID == "" || s.sessions == nil {
		return false, nil
	}
	checker, ok := s.sessions.(taskEnvironmentSessionUsageChecker)
	if !ok {
		return false, nil
	}
	return checker.HasActiveTaskSessionsByTaskEnvironmentExcludingTask(ctx, env.ID, taskID)
}

func (s *Service) preserveTaskEnvironmentForActiveBorrower(ctx context.Context, taskID string, env *models.TaskEnvironment) (bool, error) {
	if env == nil || env.ID == "" || s.sessions == nil {
		return false, nil
	}
	finder, ok := s.sessions.(taskEnvironmentSessionBorrowerFinder)
	if !ok {
		return false, nil
	}
	borrowerTaskID, err := finder.FindActiveTaskSessionTaskIDByTaskEnvironmentExcludingTask(ctx, env.ID, taskID)
	if err != nil {
		return false, fmt.Errorf("find task environment borrower %s: %w", env.ID, err)
	}
	if borrowerTaskID == "" {
		return false, nil
	}
	ownerTransfer, ok := s.taskEnvironments.(taskEnvironmentOwnerTransferer)
	if !ok {
		return false, fmt.Errorf("task environment repository cannot transfer borrowed environment %s", env.ID)
	}
	if err := ownerTransfer.TransferTaskEnvironmentToTask(ctx, env.ID, borrowerTaskID); err != nil {
		return false, fmt.Errorf("transfer task environment %s to %s: %w", env.ID, borrowerTaskID, err)
	}
	env.TaskID = borrowerTaskID
	return true, nil
}

func (s *Service) filterOwnedWorktreesForTaskCleanup(
	ctx context.Context,
	taskID string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	ownedEnv *models.TaskEnvironment,
	skipOwnedEnvironment bool,
) []*worktree.Worktree {
	if len(worktrees) == 0 {
		return worktrees
	}
	bySession := make(map[string]*models.TaskSession, len(sessions))
	for _, sess := range sessions {
		if sess != nil && sess.ID != "" {
			bySession[sess.ID] = sess
		}
	}
	envCache := map[string]*models.TaskEnvironment{}
	filtered := worktrees[:0]
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		sess, sessionLoaded := bySession[wt.SessionID]
		if s.taskOwnsSessionWorktree(ctx, taskID, wt.SessionID, sess, sessionLoaded, ownedEnv, skipOwnedEnvironment, envCache) {
			filtered = append(filtered, wt)
		}
	}
	return filtered
}

func (s *Service) taskOwnsSessionWorktree(
	ctx context.Context,
	taskID string,
	sessionID string,
	session *models.TaskSession,
	sessionLoaded bool,
	ownedEnv *models.TaskEnvironment,
	skipOwnedEnvironment bool,
	envCache map[string]*models.TaskEnvironment,
) bool {
	if session == nil {
		if sessionID == "" {
			return true
		}
		if !sessionLoaded {
			s.logger.Warn("skipping task worktree cleanup because session ownership cannot be checked",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID))
		}
		return false
	}
	if session.TaskEnvironmentID == "" {
		return true
	}
	if ownedEnv != nil && session.TaskEnvironmentID == ownedEnv.ID {
		return !skipOwnedEnvironment
	}
	if s.taskEnvironments == nil {
		s.logger.Warn("skipping task worktree cleanup because task environment ownership cannot be checked",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.String("task_environment_id", session.TaskEnvironmentID))
		return false
	}
	env, ok := envCache[session.TaskEnvironmentID]
	if !ok {
		var err error
		env, err = s.taskEnvironments.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
		if err != nil {
			s.logger.Warn("skipping task worktree cleanup because task environment lookup failed",
				zap.String("task_id", taskID),
				zap.String("session_id", session.ID),
				zap.String("task_environment_id", session.TaskEnvironmentID),
				zap.Error(err))
			return false
		}
		envCache[session.TaskEnvironmentID] = env
	}
	if env == nil || env.TaskID != taskID {
		return false
	}
	return true
}

func cleanupEligibleWorktrees(worktrees []*worktree.Worktree, env *models.TaskEnvironment, preserveExecutorRows map[string]struct{}) []*worktree.Worktree {
	worktrees = excludeEnvironmentWorktree(worktrees, env)
	if len(preserveExecutorRows) == 0 || len(worktrees) == 0 {
		return worktrees
	}
	filtered := worktrees[:0]
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		if _, preserve := preserveExecutorRows[wt.SessionID]; preserve {
			continue
		}
		filtered = append(filtered, wt)
	}
	return filtered
}

func (s *Service) cleanupTaskEnvironment(
	ctx context.Context,
	taskID string,
	cleanup taskEnvironmentCleanup,
) []error {
	if cleanup.env == nil {
		return nil
	}
	if cause := context.Cause(ctx); cause != nil {
		return []error{cause}
	}
	if err := s.teardownEnvironmentResources(ctx, cleanup.env); err != nil {
		s.logger.Warn("failed to teardown task environment during task cleanup",
			zap.String("task_id", taskID),
			zap.String("env_id", cleanup.env.ID),
			zap.Error(err))
		return []error{fmt.Errorf("teardown task environment %s: %w", cleanup.env.ID, err)}
	}
	if cleanup.deleteRow {
		if cause := context.Cause(ctx); cause != nil {
			return []error{cause}
		}
		if err := s.taskEnvironments.DeleteTaskEnvironment(ctx, cleanup.env.ID); err != nil {
			s.logger.Warn("failed to delete task environment row during task cleanup",
				zap.String("task_id", taskID),
				zap.String("env_id", cleanup.env.ID),
				zap.Error(err))
			return []error{fmt.Errorf("delete task environment row %s: %w", cleanup.env.ID, err)}
		}
	}
	return nil
}

func excludeEnvironmentWorktree(worktrees []*worktree.Worktree, env *models.TaskEnvironment) []*worktree.Worktree {
	if env == nil || env.WorktreeID == "" || len(worktrees) == 0 {
		return worktrees
	}
	filtered := worktrees[:0]
	for _, wt := range worktrees {
		if wt == nil || wt.ID == env.WorktreeID {
			continue
		}
		filtered = append(filtered, wt)
	}
	return filtered
}

// ListTasks returns all tasks for a workflow
func (s *Service) ListTasks(ctx context.Context, workflowID string) ([]*models.Task, error) {
	tasks, err := s.tasks.ListTasks(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	if err := s.loadTaskRepositoriesBatch(ctx, tasks); err != nil {
		s.logger.Error("failed to batch-load task repositories", zap.Error(err))
	}

	return tasks, nil
}

// ListTasksByWorkspace returns paginated tasks for a workspace with task repositories loaded.
// If query is non-empty, filters by task title, description, repository name, or repository path.
// workflowID and repositoryID, when non-empty, further restrict results to that workflow/repository.
func (s *Service) ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, sort string, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*models.Task, int, error) {
	tasks, total, err := s.tasks.ListTasksByWorkspace(ctx, workspaceID, workflowID, repositoryID, query, page, pageSize, sort, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig)
	if err != nil {
		return nil, 0, err
	}

	tasks, total = s.augmentWithPRMatches(ctx, tasks, total, prSearchOptions{
		workspaceID:      workspaceID,
		workflowID:       workflowID,
		repositoryID:     repositoryID,
		query:            query,
		page:             page,
		pageSize:         pageSize,
		includeArchived:  includeArchived,
		includeEphemeral: includeEphemeral,
		onlyEphemeral:    onlyEphemeral,
		excludeConfig:    excludeConfig,
	})

	if err := s.loadTaskRepositoriesBatch(ctx, tasks); err != nil {
		s.logger.Error("failed to batch-load task repositories", zap.Error(err))
	}

	return tasks, total, nil
}

type prSearchOptions struct {
	workspaceID      string
	workflowID       string
	repositoryID     string
	query            string
	page             int
	pageSize         int
	includeArchived  bool
	includeEphemeral bool
	onlyEphemeral    bool
	excludeConfig    bool
}

// parsePRQuery extracts a positive PR number from a search query, accepting an
// optional leading '#'. Returns (0, false) when the query is not a PR number.
func parsePRQuery(query string) (int, bool) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(query), "#")
	if trimmed == "" {
		return 0, false
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// isConfigTask reports whether a task is a config-mode task (mirrors the SQL
// `json_extract(metadata, '$.config_mode') IS NOT 1` filter). JSON-decoded
// numbers arrive as float64, so accept both numeric 1 and bool true.
func isConfigTask(task *models.Task) bool {
	switch v := task.Metadata["config_mode"].(type) {
	case float64:
		return v == 1
	case int:
		return v == 1
	case bool:
		return v
	default:
		return false
	}
}

// augmentWithPRMatches surfaces tasks associated with a PR number when the
// search query looks like one. PR matches are prepended (most relevant) and
// deduped against the existing results; `total` grows by the net-new count.
// Best-effort: a missing resolver or a lookup error leaves results unchanged.
//
// Augmentation only applies to the first page of an unscoped search. It is
// skipped for page > 1 (the prepend+truncate only makes sense against page 1,
// otherwise a PR match would re-appear on every page and push out a real
// result) and when a workflow or repository filter is set (a PR-matched task
// isn't guaranteed to satisfy those filters, and the only caller that searches
// by PR number — the Cmd+K command panel — sets neither).
func (s *Service) augmentWithPRMatches(ctx context.Context, tasks []*models.Task, total int, opts prSearchOptions) ([]*models.Task, int) {
	if opts.page > 1 || opts.workflowID != "" || opts.repositoryID != "" {
		return tasks, total
	}
	prNum, ok := parsePRQuery(opts.query)
	if !ok || s.prTaskResolver == nil {
		return tasks, total
	}
	ids, err := s.prTaskResolver.FindTaskIDsByPRNumber(ctx, opts.workspaceID, prNum)
	if err != nil {
		s.logger.Warn("PR-number task lookup failed", zap.Int("pr_number", prNum), zap.Error(err))
		return tasks, total
	}

	matched := s.fetchPRMatchedTasks(ctx, ids, tasks, opts)
	if len(matched) == 0 {
		return tasks, total
	}

	merged := make([]*models.Task, 0, len(matched)+len(tasks))
	merged = append(merged, matched...)
	merged = append(merged, tasks...)
	total += len(matched)
	if len(merged) > opts.pageSize {
		merged = merged[:opts.pageSize]
	}
	return merged, total
}

// fetchPRMatchedTasks batch-loads the resolver's task IDs that aren't already in
// `existing`, applies the same visibility filters as the repository search, and
// returns the survivors in resolver order. The resolver returns distinct IDs,
// so excluding the already-present ones is enough to keep the result deduped.
func (s *Service) fetchPRMatchedTasks(ctx context.Context, ids []string, existing []*models.Task, opts prSearchOptions) []*models.Task {
	seen := make(map[string]struct{}, len(existing))
	for _, t := range existing {
		seen[t.ID] = struct{}{}
	}
	var fetchIDs []string
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			fetchIDs = append(fetchIDs, id)
		}
	}
	if len(fetchIDs) == 0 {
		return nil
	}
	fetched, err := s.tasks.GetTasksByIDs(ctx, fetchIDs)
	if err != nil {
		s.logger.Warn("PR-match task fetch failed", zap.Error(err))
		return nil
	}
	byID := make(map[string]*models.Task, len(fetched))
	for _, t := range fetched {
		byID[t.ID] = t
	}
	var matched []*models.Task
	for _, id := range fetchIDs {
		task := byID[id]
		if task == nil || s.prMatchFilteredOut(task, opts) {
			continue
		}
		matched = append(matched, task)
	}
	return matched
}

// prMatchFilteredOut applies the same visibility filters the repository search
// uses, so a PR-matched task respects includeArchived / ephemeral / config flags.
func (s *Service) prMatchFilteredOut(task *models.Task, opts prSearchOptions) bool {
	if !opts.includeArchived && task.ArchivedAt != nil {
		return true
	}
	if opts.onlyEphemeral && !task.IsEphemeral {
		return true
	}
	if !opts.includeEphemeral && !opts.onlyEphemeral && task.IsEphemeral {
		return true
	}
	if opts.excludeConfig && isConfigTask(task) {
		return true
	}
	return false
}

// loadTaskRepositoriesBatch loads repositories for multiple tasks in a single query.
func (s *Service) loadTaskRepositoriesBatch(ctx context.Context, tasks []*models.Task) error {
	if len(tasks) == 0 {
		return nil
	}
	taskIDs := make([]string, len(tasks))
	for i, t := range tasks {
		taskIDs[i] = t.ID
	}
	repoMap, err := s.taskRepos.ListTaskRepositoriesByTaskIDs(ctx, taskIDs)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		task.Repositories = repoMap[task.ID]
	}
	return nil
}
