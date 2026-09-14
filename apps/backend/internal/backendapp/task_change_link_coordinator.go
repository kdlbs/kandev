package backendapp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/auth/authn"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	mcp "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

// taskChangeLinkCoordinator is the provider-neutral MCP seam. It keeps task
// reach checks and repository identity resolution at the host boundary, then
// delegates persistence and provider reads to the established services.
type taskChangeLinkCoordinator struct {
	tasks  *taskservice.Service
	github githubChangeLinkProvider
	gitlab gitlabChangeLinkProvider
	logger *commonlogger.Logger
}

func (c taskChangeLinkCoordinator) ManageTaskChangeRequest(
	ctx context.Context, req mcp.TaskChangeLinkRequest,
) (mcp.TaskChangeLinkMutationResult, error) {
	switch req.Operation {
	case "", "link":
		return c.simpleMutation(ctx, req, c.LinkTaskChange)
	case "unlink":
		return c.simpleMutation(ctx, req, c.UnlinkTaskChange)
	case "replace":
		return c.replaceTaskChangeResult(ctx, req)
	default:
		return c.taskChangeMutationFailure(req.TaskID, fmt.Errorf("unsupported task change operation %q", req.Operation))
	}
}

func (c taskChangeLinkCoordinator) simpleMutation(
	ctx context.Context,
	req mcp.TaskChangeLinkRequest,
	apply func(context.Context, mcp.TaskChangeLinkRequest) ([]mcp.TaskChangeLink, error),
) (mcp.TaskChangeLinkMutationResult, error) {
	links, err := apply(ctx, req)
	if err != nil {
		return c.taskChangeMutationFailure(req.TaskID, err)
	}
	return mcp.TaskChangeLinkMutationResult{TaskID: req.TaskID, Links: links, StateKnown: true}, nil
}

const (
	taskChangeMutationOperationError = "task change request operation failed"
	taskChangeMutationRollbackError  = "task change request rollback could not be completed"
	taskChangeMutationReadbackError  = "task change request state readback failed"
)

func (c taskChangeLinkCoordinator) taskChangeMutationFailure(taskID string, err error) (mcp.TaskChangeLinkMutationResult, error) {
	c.logMutationError(taskID, "operation", err)
	return mcp.TaskChangeLinkMutationResult{
		TaskID: taskID, OperationError: taskChangeMutationOperationError, StateKnown: false,
	}, err
}

func (c taskChangeLinkCoordinator) logMutationError(taskID, stage string, err error) {
	if c.logger == nil || err == nil {
		return
	}
	c.logger.Error("task change request mutation failed",
		zap.String("task_id", taskID), zap.String("stage", stage), zap.Error(err))
}

const (
	taskChangeProviderGitHub = "github"
	taskChangeProviderGitLab = "gitlab"
)

type githubChangeLinkProvider interface {
	AssociateExistingPRByURLForWorkspace(context.Context, string, string, string, string, string) (*github.TaskPR, error)
	DetachTaskPR(context.Context, string, string) (*github.TaskPR, error)
	ListTaskPRs(context.Context, []string) (map[string][]*github.TaskPR, error)
}

type gitlabChangeLinkProvider interface {
	AssociateExistingMRByURL(context.Context, string, string, string, string) (*gitlab.TaskMR, error)
	ListTaskMRsByTask(context.Context, string) ([]*gitlab.TaskMR, error)
	UnlinkTaskMR(context.Context, string, string) error
}

func (c taskChangeLinkCoordinator) LinkTaskChange(ctx context.Context, req mcp.TaskChangeLinkRequest) ([]mcp.TaskChangeLink, error) {
	if err := c.link(ctx, req.TaskID, req.Link); err != nil {
		return nil, err
	}
	return c.list(ctx, req.TaskID)
}

func (c taskChangeLinkCoordinator) UnlinkTaskChange(ctx context.Context, req mcp.TaskChangeLinkRequest) ([]mcp.TaskChangeLink, error) {
	if err := c.unlink(ctx, req.TaskID, req.Link); err != nil {
		return nil, err
	}
	return c.list(ctx, req.TaskID)
}

func (c taskChangeLinkCoordinator) ReplaceTaskChange(ctx context.Context, req mcp.TaskChangeLinkRequest) ([]mcp.TaskChangeLink, error) {
	result, err := c.replaceTaskChangeResult(ctx, req)
	return result.Links, err
}

func (c taskChangeLinkCoordinator) replaceTaskChangeResult(
	ctx context.Context, req mcp.TaskChangeLinkRequest,
) (mcp.TaskChangeLinkMutationResult, error) {
	result := mcp.TaskChangeLinkMutationResult{TaskID: req.TaskID}
	if req.Old == nil {
		return c.taskChangeMutationFailure(req.TaskID, fmt.Errorf("current task change identity is required"))
	}
	// Replacing an association with the exact same identity is already the
	// requested final state. Do not detach it after the idempotent link.
	if req.Link == *req.Old {
		links, err := c.list(ctx, req.TaskID)
		if err != nil {
			return c.taskChangeMutationFailure(req.TaskID, err)
		}
		result.Links, result.StateKnown = links, true
		return result, nil
	}
	if req.Link.Provider != req.Old.Provider {
		return c.taskChangeMutationFailure(req.TaskID, fmt.Errorf("cross-provider task change replacement is not supported"))
	}
	before, err := c.list(ctx, req.TaskID)
	if err != nil {
		return c.taskChangeMutationFailure(req.TaskID, err)
	}
	result.Links, result.StateKnown = before, true
	newAlreadyLinked := taskChangeLinksContain(before, req.Link)
	// Resolve and persist the incoming association first. A failed provider
	// fetch therefore leaves the current association untouched.
	if err := c.link(ctx, req.TaskID, req.Link); err != nil {
		c.logMutationError(req.TaskID, "link replacement", err)
		result.OperationError = taskChangeMutationOperationError
		return result, err
	}
	if err := c.unlink(ctx, req.TaskID, *req.Old); err != nil {
		if newAlreadyLinked {
			return c.readReplacementFailureState(ctx, result, err)
		}
		return c.rollbackReplacementResult(ctx, req, result, err)
	}
	links, listErr := c.list(ctx, req.TaskID)
	if listErr != nil {
		c.logMutationError(req.TaskID, "replacement readback", listErr)
		result.OperationError = taskChangeMutationReadbackError
		result.StateKnown = false
		result.Links = nil
		return result, listErr
	}
	result.Links, result.StateKnown = links, true
	return result, nil
}

func (c taskChangeLinkCoordinator) readReplacementFailureState(
	ctx context.Context, result mcp.TaskChangeLinkMutationResult, operationErr error,
) (mcp.TaskChangeLinkMutationResult, error) {
	links, stateErr := c.list(ctx, result.TaskID)
	c.logMutationError(result.TaskID, "replacement operation", operationErr)
	result.OperationError = taskChangeMutationOperationError
	if stateErr != nil {
		c.logMutationError(result.TaskID, "replacement readback", stateErr)
		result.StateKnown = false
		result.Links = nil
		return result, errors.Join(operationErr, fmt.Errorf("active task change links unavailable after failed replacement: %w", stateErr))
	}
	result.Links, result.StateKnown = links, true
	return result, operationErr
}

func (c taskChangeLinkCoordinator) rollbackReplacementResult(
	ctx context.Context,
	req mcp.TaskChangeLinkRequest,
	result mcp.TaskChangeLinkMutationResult,
	operationErr error,
) (mcp.TaskChangeLinkMutationResult, error) {
	rollbackErr := c.unlink(ctx, req.TaskID, req.Link)
	c.logMutationError(req.TaskID, "replacement operation", operationErr)
	result.OperationError = taskChangeMutationOperationError
	if rollbackErr == nil {
		links, stateErr := c.list(ctx, req.TaskID)
		if stateErr != nil {
			c.logMutationError(req.TaskID, "rollback readback", stateErr)
			result.OperationError = taskChangeMutationReadbackError
			result.StateKnown = false
			result.Links = nil
			return result, errors.Join(operationErr, fmt.Errorf("active task change links unavailable after rollback: %w", stateErr))
		}
		result.Links, result.StateKnown = links, true
		return result, operationErr
	}
	c.logMutationError(req.TaskID, "replacement rollback", rollbackErr)
	result.RollbackError = taskChangeMutationRollbackError
	activeLinks, stateErr := c.list(ctx, req.TaskID)
	if stateErr != nil {
		c.logMutationError(req.TaskID, "rollback readback", stateErr)
		result.StateKnown = false
		result.Links = nil
		return result, errors.Join(
			operationErr,
			fmt.Errorf("rollback new task change link: %w", rollbackErr),
			fmt.Errorf("active task change links unavailable after failed replacement: %w", stateErr),
		)
	}
	result.Links, result.StateKnown = activeLinks, true
	return result, errors.Join(
		operationErr,
		fmt.Errorf("rollback new task change link: %w", rollbackErr),
		fmt.Errorf("active task change links after failed replacement: %v", activeLinks),
	)
}

func taskChangeLinksContain(links []mcp.TaskChangeLink, target mcp.TaskChangeLink) bool {
	for _, link := range links {
		if link.Provider == target.Provider && link.RepositoryID == target.RepositoryID && link.Number == target.Number {
			return true
		}
	}
	return false
}

func (c taskChangeLinkCoordinator) link(ctx context.Context, taskID string, link mcp.TaskChangeLink) error {
	task, repo, err := c.taskRepository(ctx, taskID, link.RepositoryID)
	if err != nil {
		return err
	}
	switch link.Provider {
	case taskChangeProviderGitHub:
		if c.github == nil {
			return fmt.Errorf("GitHub PR links are not available")
		}
		url, err := githubChangeURL(repo.ProviderHost, repo.ProviderOwner, repo.ProviderName, link.Number)
		if err != nil {
			return err
		}
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok || strings.TrimSpace(identity.UserID) == "" {
			return fmt.Errorf("authenticated user identity is required for GitHub PR links")
		}
		_, err = c.github.AssociateExistingPRByURLForWorkspace(ctx, task.WorkspaceID, identity.UserID, task.ID, repo.ID, url)
		return err
	case taskChangeProviderGitLab:
		if c.gitlab == nil {
			return fmt.Errorf("GitLab MR links are not available")
		}
		url, err := gitlabChangeURL(repo.ProviderHost, repo.ProviderOwner, repo.ProviderName, link.Number)
		if err != nil {
			return err
		}
		_, err = c.gitlab.AssociateExistingMRByURL(ctx, task.WorkspaceID, task.ID, repo.ID, url)
		return err
	default:
		return fmt.Errorf("unsupported provider %q", link.Provider)
	}
}

func (c taskChangeLinkCoordinator) unlink(ctx context.Context, taskID string, link mcp.TaskChangeLink) error {
	task, _, err := c.taskRepository(ctx, taskID, "")
	if err != nil {
		return err
	}
	switch link.Provider {
	case taskChangeProviderGitHub:
		if c.github == nil {
			return fmt.Errorf("GitHub PR links are not available")
		}
		prs, err := c.githubLinks(ctx, task.ID)
		if err != nil {
			return err
		}
		for _, pr := range prs {
			if pr.RepositoryID == link.RepositoryID && pr.PRNumber == link.Number {
				_, err = c.github.DetachTaskPR(ctx, task.WorkspaceID, pr.ID)
				return err
			}
		}
		return nil
	case taskChangeProviderGitLab:
		if c.gitlab == nil {
			return fmt.Errorf("GitLab MR links are not available")
		}
		mrs, err := c.gitlab.ListTaskMRsByTask(ctx, task.ID)
		if err != nil {
			return err
		}
		identityResolver := gitlabTaskChangeRequestAdapter{tasks: c.tasks}
		targetRepositoryID := strings.TrimSpace(link.RepositoryID)
		// A persisted canonical identity is sufficient to identify the exact
		// association, even when its repository was later removed from the task.
		// Scan these rows before resolving legacy rows so an unrelated legacy MR
		// with the same IID cannot prevent cleanup of the requested association.
		for _, mr := range mrs {
			if mr == nil || mr.MRIID != link.Number {
				continue
			}
			if strings.TrimSpace(mr.RepositoryID) == targetRepositoryID {
				return c.gitlab.UnlinkTaskMR(ctx, task.WorkspaceID, mr.ID)
			}
		}
		for _, mr := range mrs {
			if mr == nil || mr.MRIID != link.Number || strings.TrimSpace(mr.RepositoryID) != "" {
				continue
			}
			repositoryID, identityStatus, resolveErr := identityResolver.resolveGitLabRepositoryIdentityWithError(ctx, task, mr)
			if resolveErr != nil {
				return fmt.Errorf("GitLab MR identity lookup failed: %w", resolveErr)
			}
			if identityStatus == mcp.TaskChangeRequestIdentityAmbiguous {
				candidates := identityResolver.gitLabRepositoryCandidates(ctx, task, mr)
				if _, matchesTarget := candidates[targetRepositoryID]; matchesTarget {
					return fmt.Errorf("GitLab MR identity is ambiguous")
				}
			}
			if identityStatus == mcp.TaskChangeRequestIdentityResolved && repositoryID == targetRepositoryID {
				return c.gitlab.UnlinkTaskMR(ctx, task.WorkspaceID, mr.ID)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported provider %q", link.Provider)
	}
}

func (c taskChangeLinkCoordinator) list(ctx context.Context, taskID string) ([]mcp.TaskChangeLink, error) {
	task, _, err := c.taskRepository(ctx, taskID, "")
	if err != nil {
		return nil, err
	}
	links := []mcp.TaskChangeLink{}
	if c.github != nil {
		prs, err := c.githubLinks(ctx, taskID)
		if err != nil {
			return nil, err
		}
		for _, pr := range prs {
			links = append(links, mcp.TaskChangeLink{Provider: taskChangeProviderGitHub, RepositoryID: pr.RepositoryID, Number: pr.PRNumber})
		}
	}
	if c.gitlab != nil {
		mrs, err := c.gitlab.ListTaskMRsByTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		identityResolver := gitlabTaskChangeRequestAdapter{tasks: c.tasks}
		for _, mr := range mrs {
			if mr == nil {
				continue
			}
			repositoryID := strings.TrimSpace(mr.RepositoryID)
			if repositoryID == "" {
				resolvedID, identityStatus := identityResolver.resolveGitLabRepositoryIdentity(ctx, task, mr)
				if identityStatus == mcp.TaskChangeRequestIdentityResolved {
					repositoryID = resolvedID
				}
			}
			links = append(links, mcp.TaskChangeLink{Provider: taskChangeProviderGitLab, RepositoryID: repositoryID, Number: mr.MRIID})
		}
	}
	return links, nil
}

func (c taskChangeLinkCoordinator) githubLinks(ctx context.Context, taskID string) ([]*github.TaskPR, error) {
	byTask, err := c.github.ListTaskPRs(ctx, []string{taskID})
	if err != nil {
		return nil, err
	}
	return byTask[taskID], nil
}

func (c taskChangeLinkCoordinator) taskRepository(ctx context.Context, taskID, repositoryID string) (*models.Task, *models.Repository, error) {
	if c.tasks == nil {
		return nil, nil, fmt.Errorf("task service is not available")
	}
	task, err := c.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return nil, nil, fmt.Errorf("task not found")
	}
	if repositoryID == "" {
		return task, nil, nil
	}
	for _, taskRepo := range task.Repositories {
		if taskRepo.RepositoryID != repositoryID {
			continue
		}
		repo, getErr := c.tasks.GetRepository(ctx, repositoryID)
		if getErr != nil || repo == nil || repo.WorkspaceID != task.WorkspaceID {
			return nil, nil, fmt.Errorf("repository not found for task")
		}
		return task, repo, nil
	}
	return nil, nil, fmt.Errorf("repository not found for task")
}

func githubChangeURL(host, owner, name string, number int) (string, error) {
	normalizedHost, err := normalizedGitHubHost(host)
	if err != nil {
		return "", err
	}
	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	if owner == "" || name == "" {
		return "", fmt.Errorf("repository has no canonical GitHub identity")
	}
	return fmt.Sprintf("https://%s/%s/%s/pull/%d", normalizedHost, owner, name, number), nil
}

func normalizedGitHubHost(host string) (string, error) {
	rawHost := strings.TrimSpace(host)
	if rawHost == "" {
		return "", fmt.Errorf("repository has no canonical GitHub host")
	}
	if !strings.Contains(rawHost, "://") {
		rawHost = "https://" + rawHost
	}
	parsed, err := url.Parse(rawHost)
	if err != nil || parsed.User != nil || parsed.Host == "" || parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("repository has invalid GitHub host")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Port() != "" {
		return "", fmt.Errorf("repository has invalid GitHub host")
	}
	normalizedHost := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if normalizedHost != "github.com" && !strings.HasSuffix(normalizedHost, ".github.com") {
		return "", fmt.Errorf("repository is not a GitHub repository")
	}
	return normalizedHost, nil
}

func gitlabChangeURL(host, owner, name string, number int) (string, error) {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	if host == "" || strings.TrimSpace(owner) == "" || strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("repository has no canonical GitLab identity")
	}
	if !strings.HasPrefix(host, "https://") && !strings.HasPrefix(host, "http://") {
		return "", fmt.Errorf("repository has invalid GitLab host")
	}
	return fmt.Sprintf("%s/%s/%s/-/merge_requests/%d", host, owner, name, number), nil
}
