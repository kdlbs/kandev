package backendapp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	mcp "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// taskChangeLinkCoordinator is the provider-neutral MCP seam. It keeps task
// reach checks and repository identity resolution at the host boundary, then
// delegates persistence and provider reads to the established services.
type taskChangeLinkCoordinator struct {
	tasks              *taskservice.Service
	github             githubChangeLinkProvider
	gitlab             gitlabChangeLinkProvider
	singleUserIdentity func() (authn.Identity, bool)
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
	if req.Old == nil {
		return nil, fmt.Errorf("current task change identity is required")
	}
	// Replacing an association with the exact same identity is already the
	// requested final state. Do not detach it after the idempotent link.
	if req.Link == *req.Old {
		return c.list(ctx, req.TaskID)
	}
	if req.Link.Provider != req.Old.Provider {
		return nil, fmt.Errorf("cross-provider task change replacement is not supported")
	}
	before, err := c.list(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	newAlreadyLinked := taskChangeLinksContain(before, req.Link)
	// Resolve and persist the incoming association first. A failed provider
	// fetch therefore leaves the current association untouched.
	if err := c.link(ctx, req.TaskID, req.Link); err != nil {
		return nil, err
	}
	if err := c.unlink(ctx, req.TaskID, *req.Old); err != nil {
		if newAlreadyLinked {
			return nil, err
		}
		return c.rollbackReplacement(ctx, req.TaskID, req.Link, err)
	}
	return c.list(ctx, req.TaskID)
}

func (c taskChangeLinkCoordinator) rollbackReplacement(
	ctx context.Context, taskID string, newLink mcp.TaskChangeLink, originalErr error,
) ([]mcp.TaskChangeLink, error) {
	rollbackErr := c.unlink(ctx, taskID, newLink)
	if rollbackErr == nil {
		return nil, originalErr
	}
	activeLinks, stateErr := c.list(ctx, taskID)
	state := fmt.Errorf("active task change links after failed replacement: %v", activeLinks)
	if stateErr != nil {
		state = fmt.Errorf("active task change links unavailable after failed replacement: %w", stateErr)
	}
	return activeLinks, errors.Join(originalErr, fmt.Errorf("rollback new task change link: %w", rollbackErr), state)
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
		if !ok && c.singleUserIdentity != nil {
			identity, ok = c.singleUserIdentity()
		}
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
		for _, mr := range mrs {
			if mr.RepositoryID == link.RepositoryID && mr.MRIID == link.Number {
				return c.gitlab.UnlinkTaskMR(ctx, task.WorkspaceID, mr.ID)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported provider %q", link.Provider)
	}
}

func (c taskChangeLinkCoordinator) list(ctx context.Context, taskID string) ([]mcp.TaskChangeLink, error) {
	if _, _, err := c.taskRepository(ctx, taskID, ""); err != nil {
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
		for _, mr := range mrs {
			links = append(links, mcp.TaskChangeLink{Provider: taskChangeProviderGitLab, RepositoryID: mr.RepositoryID, Number: mr.MRIID})
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
