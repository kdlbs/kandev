package backendapp

import (
	"context"
	"fmt"
	"strings"

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
	tasks  *taskservice.Service
	github *github.Service
	gitlab *gitlab.Service
}

const (
	taskChangeProviderGitHub = "github"
	taskChangeProviderGitLab = "gitlab"
)

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
	// Resolve and persist the incoming association first. A failed provider
	// fetch therefore leaves the current association untouched.
	if err := c.link(ctx, req.TaskID, req.Link); err != nil {
		return nil, err
	}
	if err := c.unlink(ctx, req.TaskID, *req.Old); err != nil {
		return nil, err
	}
	return c.list(ctx, req.TaskID)
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
		_, err = c.github.AssociateExistingPRByURL(ctx, task.ID, repo.ID, url)
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
	task, _, err := c.taskRepository(ctx, taskID, link.RepositoryID)
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
	if strings.TrimSpace(host) != "" && !strings.Contains(strings.ToLower(host), "github.com") {
		return "", fmt.Errorf("repository is not a GitHub repository")
	}
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("repository has no canonical GitHub identity")
	}
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, name, number), nil
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
