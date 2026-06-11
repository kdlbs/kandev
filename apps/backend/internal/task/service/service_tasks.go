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
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/gitref"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// defaultPriority is the default value for the task priority column.
// Used when a caller omits priority so the DB CHECK constraint is satisfied.
const defaultPriority = "medium"

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
}

type taskEnvironmentCleanup struct {
	env       *models.TaskEnvironment
	deleteRow bool
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
	if repoInput.GitHubURL != "" {
		if owner, name, err := parseGitHubRepoURL(repoInput.GitHubURL); err == nil {
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
		return repositoryID, baseBranch, false, nil
	}

	// Handle GitHub URL: parse owner/name and use FindOrCreateRepository
	if repoInput.GitHubURL != "" {
		return s.resolveRepoInputGitHub(ctx, workspaceID, repoInput, baseBranch)
	}

	if repoInput.LocalPath == "" {
		return repositoryID, baseBranch, false, nil
	}
	return s.resolveRepoInputLocal(ctx, workspaceID, repoInput, repoByPath, baseBranch)
}

// resolveRepoInputLocal handles the LocalPath branch of resolveRepoInput.
// Looks the path up in the workspace snapshot; on miss, calls
// CreateRepository (and reports created=true). Extracted to keep
// resolveRepoInput inside the cyclomatic-complexity budget.
func (s *Service) resolveRepoInputLocal(
	ctx context.Context, workspaceID string, repoInput TaskRepositoryInput,
	repoByPath map[string]*models.Repository, baseBranch string,
) (string, string, bool, error) {
	repo := repoByPath[repoInput.LocalPath]
	created := false
	if repo == nil {
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
		if defaultBranch == "" {
			// Probe must operate on a path validated against the discovery
			// allowlist — repoInput.LocalPath comes straight from the HTTP body
			// and feeds into os.Stat/ReadFile inside gitref.DefaultBranch, so
			// without this guard a caller could traverse the filesystem.
			if safePath, pathErr := s.resolveAllowedLocalPath(repoInput.LocalPath); pathErr == nil {
				if probed, err := gitref.DefaultBranch(safePath); err == nil && probed != "" && probed != "HEAD" {
					defaultBranch = probed
				}
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
		}
		created = true
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
func (s *Service) resolveRepoInputGitHub(
	ctx context.Context, workspaceID string, repoInput TaskRepositoryInput, baseBranch string,
) (string, string, bool, error) {
	owner, name, parseErr := parseGitHubRepoURL(repoInput.GitHubURL)
	if parseErr != nil {
		return "", "", false, parseErr
	}
	defaultBranch := repoInput.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = repoInput.BaseBranch
	}
	if defaultBranch == "" && repoInput.ResolveProviderDefaults && s.providerProber != nil {
		defaultBranch = s.probeProviderDefaultBranchIfMissing(ctx, workspaceID, "github", owner, name)
	}
	repo, repoCreated, createErr := s.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID:   workspaceID,
		Provider:      "github",
		ProviderOwner: owner,
		ProviderName:  name,
		DefaultBranch: defaultBranch,
	})
	if createErr != nil {
		return "", "", false, createErr
	}
	if baseBranch == "" {
		baseBranch = repo.DefaultBranch
	}
	return repo.ID, baseBranch, repoCreated, nil
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
	existing, lookupErr := s.repoEntities.GetRepositoryByProviderInfo(ctx, workspaceID, provider, owner, name)
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
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", "", fmt.Errorf("empty GitHub URL")
	}

	// Add scheme if missing so url.Parse works correctly
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}

	parsed, parseErr := url.Parse(rawURL)
	if parseErr != nil {
		return "", "", fmt.Errorf("invalid GitHub URL: %w", parseErr)
	}

	if parsed.Host != "github.com" && parsed.Host != "www.github.com" {
		return "", "", fmt.Errorf("not a GitHub URL: %s", parsed.Host)
	}

	// Path should be /owner/name (possibly with .git suffix and trailing slash)
	path := strings.Trim(parsed.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid GitHub repository URL: expected github.com/owner/repo")
	}

	return parts[0], parts[1], nil
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
	rawURL := strings.TrimSpace(input.GitHubURL)
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
		s.logger.Warn("failed to list active sessions for archive",
			zap.String("task_id", id),
			zap.Error(err))
	}
	if s.executionStopper != nil {
		stopTargets = s.buildStopTargets(ctx, id, activeSessions)
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
		s.logger.Warn("failed to list task sessions for archive",
			zap.String("task_id", id),
			zap.Error(err))
	}

	var worktrees []*worktree.Worktree
	if s.worktreeCleanup != nil {
		if provider, ok := s.worktreeCleanup.(WorktreeProvider); ok {
			worktrees, err = provider.GetAllByTaskID(ctx, id)
			if err != nil {
				s.logger.Warn("failed to list worktrees for archive",
					zap.String("task_id", id),
					zap.Error(err))
			}
		}
	}
	taskEnv := s.gatherTaskEnvironmentForCleanup(ctx, id)

	// 3. Set archived_at in DB
	if err := s.tasks.ArchiveTask(ctx, id); err != nil {
		return err
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
	// Note: isEphemeral=false for archive to preserve quick-chat directories
	envCleanup := taskEnvironmentCleanup{env: taskEnv, deleteRow: true}
	if len(stopTargets) > 0 || s.worktreeCleanup != nil || len(sessions) > 0 || taskEnv != nil {
		s.runAsyncTaskCleanup(id, sessions, worktrees, stopTargets, envCleanup, false,
			"task archived", "failed to stop session on task archive", "task archive cleanup completed")
	}

	return nil
}

// DeleteTask deletes a task and publishes a task.deleted event.
// For fast UI response, the DB delete and event publish happen synchronously,
// while agent stopping and worktree cleanup happen asynchronously.
func (s *Service) DeleteTask(ctx context.Context, id string) error {
	start := time.Now()

	// 1. Get task (sync, fast)
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return err
	}

	// 2. Gather data needed for cleanup BEFORE delete (sync, fast)
	sessions, err := s.sessions.ListTaskSessions(ctx, id)
	if err != nil {
		s.logger.Warn("failed to list task sessions for delete",
			zap.String("task_id", id),
			zap.Error(err))
	}

	worktrees := s.gatherWorktreesForDelete(ctx, id)
	taskEnv := s.gatherTaskEnvironmentForCleanup(ctx, id)

	// 3. Get active sessions for stopping agents (sync, fast)
	// Must query before delete since DB records will be gone
	var stopTargets []taskStopTarget
	if s.executionStopper != nil {
		activeSessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, id)
		if err != nil {
			s.logger.Warn("failed to list active sessions for delete",
				zap.String("task_id", id),
				zap.Error(err))
		}
		stopTargets = s.buildStopTargets(ctx, id, activeSessions)
	}

	// 4. Delete from DB (sync, fast)
	if err := s.tasks.DeleteTask(ctx, id); err != nil {
		s.logger.Error("failed to delete task", zap.String("task_id", id), zap.Error(err))
		return err
	}

	// 5. Publish event (sync, fast) - frontend removes task immediately
	s.publishTaskEvent(ctx, events.TaskDeleted, task, nil)
	s.logger.Info("task deleted",
		zap.String("task_id", id),
		zap.Duration("duration", time.Since(start)))

	// 6. Stop agents and cleanup worktrees in the background. Carry the
	//    envCleanup struct so the task environment row is reset alongside
	//    the worktrees (an extra task.taskEnv != nil branch keeps the
	//    cleanup running when only the env needs reclaiming).
	envCleanup := taskEnvironmentCleanup{env: taskEnv, deleteRow: false}
	hasCleanup := len(stopTargets) > 0 || s.worktreeCleanup != nil || len(sessions) > 0 || task.IsEphemeral || taskEnv != nil
	if hasCleanup {
		s.runAsyncTaskCleanup(id, sessions, worktrees, stopTargets, envCleanup, task.IsEphemeral,
			"task deleted", "failed to stop session on task delete", "task cleanup completed")
	}

	return nil
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
// The caller is expected to have cancelled active runs separately (cascade
// does this via runCanceller before invoking us), so stopTargets is empty.
// deleteEnvRow controls whether the task_environment row is removed (true
// for delete cascade, false for archive — archive preserves the row).
// Best-effort and idempotent; failures are logged.
func (s *Service) CleanupTaskResources(ctx context.Context, taskID string, deleteEnvRow bool) {
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to list sessions for cascade cleanup",
			zap.String("task_id", taskID),
			zap.Error(err))
	}
	worktrees := s.gatherWorktreesForDelete(ctx, taskID)
	taskEnv := s.gatherTaskEnvironmentForCleanup(ctx, taskID)
	envCleanup := taskEnvironmentCleanup{env: taskEnv, deleteRow: deleteEnvRow}
	if len(sessions) == 0 && len(worktrees) == 0 && taskEnv == nil {
		return
	}
	reason := "cascade archive"
	if deleteEnvRow {
		reason = "cascade delete"
	}
	s.runAsyncTaskCleanup(taskID, sessions, worktrees, nil, envCleanup, false,
		reason, "failed to stop session on cascade cleanup", "cascade cleanup completed")
}

// gatherWorktreesForDelete collects worktrees for a task before it is deleted.
// For legacy WorktreeCleanup implementations that do not implement WorktreeProvider,
// it triggers cleanup immediately and returns nil.
func (s *Service) gatherWorktreesForDelete(ctx context.Context, taskID string) []*worktree.Worktree {
	if s.worktreeCleanup == nil {
		return nil
	}
	provider, ok := s.worktreeCleanup.(WorktreeProvider)
	if !ok {
		// Fallback for legacy implementations: cleanup before delete.
		if err := s.worktreeCleanup.OnTaskDeleted(ctx, taskID); err != nil {
			s.logger.Warn("failed to cleanup worktree on task deletion",
				zap.String("task_id", taskID),
				zap.Error(err))
		}
		return nil
	}
	worktrees, err := provider.GetAllByTaskID(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to list worktrees for delete",
			zap.String("task_id", taskID),
			zap.Error(err))
	}
	return worktrees
}

func (s *Service) gatherTaskEnvironmentForCleanup(ctx context.Context, taskID string) *models.TaskEnvironment {
	if s.taskEnvironments == nil {
		return nil
	}
	env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to lookup task environment for cleanup",
			zap.String("task_id", taskID),
			zap.Error(err))
		return nil
	}
	return env
}

func (s *Service) runAsyncTaskCleanup(
	id string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	stopTargets []taskStopTarget,
	envCleanup taskEnvironmentCleanup,
	isEphemeral bool,
	stopReason, stopFailMsg, cleanupMsg string,
) {
	go func() {
		cleanupStart := time.Now()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if s.executionStopper != nil && len(stopTargets) > 0 {
			for _, target := range stopTargets {
				if target.executionID != "" {
					if err := s.executionStopper.StopExecution(cleanupCtx, target.executionID, stopReason, true); err != nil {
						s.logger.Warn(stopFailMsg,
							zap.String("task_id", id),
							zap.String("session_id", target.sessionID),
							zap.String("execution_id", target.executionID),
							zap.Error(err))
					}
					continue
				}
				if err := s.executionStopper.StopSession(cleanupCtx, target.sessionID, stopReason, true); err != nil {
					s.logger.Warn(stopFailMsg,
						zap.String("task_id", id),
						zap.String("session_id", target.sessionID),
						zap.Error(err))
				}
			}
		}

		cleanupErrors := s.performTaskCleanup(cleanupCtx, id, sessions, worktrees, envCleanup, isEphemeral)

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
	}()
}

func (s *Service) buildStopTargets(ctx context.Context, taskID string, activeSessions []*models.TaskSession) []taskStopTarget {
	targets := make([]taskStopTarget, 0, len(activeSessions))
	for _, sess := range activeSessions {
		if sess == nil || sess.ID == "" {
			continue
		}
		target := taskStopTarget{
			sessionID:   sess.ID,
			executionID: strings.TrimSpace(sess.AgentExecutionID),
		}
		if target.executionID == "" {
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
	return targets
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
	envCleanup taskEnvironmentCleanup,
	isEphemeral bool,
) []error {
	var errs []error

	errs = append(errs, s.cleanupTaskEnvironment(ctx, taskID, envCleanup)...)
	worktrees = excludeEnvironmentWorktree(worktrees, envCleanup.env)

	// Cleanup worktrees
	if len(worktrees) > 0 {
		if cleaner, ok := s.worktreeCleanup.(WorktreeBatchCleaner); ok {
			if err := cleaner.CleanupWorktrees(ctx, worktrees); err != nil {
				s.logger.Warn("failed to cleanup worktrees after delete",
					zap.String("task_id", taskID),
					zap.Error(err))
				errs = append(errs, fmt.Errorf("cleanup worktrees: %w", err))
			}
		}
	}

	// Delete executor running records for sessions
	for _, session := range sessions {
		if session == nil || session.ID == "" {
			continue
		}
		if err := s.executors.DeleteExecutorRunningBySessionID(ctx, session.ID); err != nil {
			s.logger.Debug("failed to delete executor runtime for session",
				zap.String("task_id", taskID),
				zap.String("session_id", session.ID),
				zap.Error(err))
			// Don't add to errs - this is a debug-level issue
		}
	}

	// Cleanup quick-chat workspace directories for all tasks (not just ephemeral).
	// Non-ephemeral office tasks also get quick-chat dirs allocated by manager_launch.go;
	// both cases must be cleaned up to avoid a disk leak.
	if s.quickChatDir != "" {
		for _, session := range sessions {
			if session == nil || session.ID == "" {
				continue
			}
			sessionDir := filepath.Join(s.quickChatDir, session.ID)
			if _, statErr := os.Stat(sessionDir); statErr != nil {
				// Directory does not exist — nothing to remove.
				continue
			}
			if err := os.RemoveAll(sessionDir); err != nil {
				s.logger.Warn("failed to cleanup quick-chat workspace directory",
					zap.String("task_id", taskID),
					zap.String("session_id", session.ID),
					zap.String("path", sessionDir),
					zap.Error(err))
				errs = append(errs, fmt.Errorf("cleanup quick-chat dir %s: %w", session.ID, err))
			} else {
				s.logger.Debug("cleaned up quick-chat workspace directory",
					zap.String("task_id", taskID),
					zap.String("session_id", session.ID),
					zap.String("path", sessionDir))
			}
		}
	}

	return errs
}

func (s *Service) cleanupTaskEnvironment(
	ctx context.Context,
	taskID string,
	cleanup taskEnvironmentCleanup,
) []error {
	if cleanup.env == nil {
		return nil
	}
	if err := s.teardownEnvironmentResources(ctx, cleanup.env); err != nil {
		s.logger.Warn("failed to teardown task environment during task cleanup",
			zap.String("task_id", taskID),
			zap.String("env_id", cleanup.env.ID),
			zap.Error(err))
		return []error{fmt.Errorf("teardown task environment %s: %w", cleanup.env.ID, err)}
	}
	if cleanup.deleteRow {
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
func (s *Service) ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*models.Task, int, error) {
	tasks, total, err := s.tasks.ListTasksByWorkspace(ctx, workspaceID, workflowID, repositoryID, query, page, pageSize, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig)
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
