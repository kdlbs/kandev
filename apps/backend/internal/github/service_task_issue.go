package github

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const (
	taskMetaIssueURL      = "issue_url"
	taskMetaIssueNumber   = "issue_number"
	taskMetaIssueOwner    = "issue_owner"
	taskMetaIssueRepo     = "issue_repo"
	taskMetaIssueLinked   = "github_issue_linked"
	githubProviderName    = "github"
	githubIssuePathMarker = "/issues/"
)

// TaskIssueStore is the task-facing dependency needed to link GitHub issues.
// The backendapp adapter routes metadata writes through task.Service so the
// normal task.updated event is published.
type TaskIssueStore interface {
	GetTask(ctx context.Context, taskID string) (*taskmodels.Task, error)
	ListTaskRepositories(ctx context.Context, taskID string) ([]*taskmodels.TaskRepository, error)
	GetRepository(ctx context.Context, repositoryID string) (*taskmodels.Repository, error)
	UpdateTaskMetadata(ctx context.Context, taskID string, metadata map[string]interface{}) (*taskmodels.Task, error)
}

type LinkTaskIssueRequest struct {
	Issue  string `json:"issue"`
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

type TaskIssueLinkResponse struct {
	TaskID      string `json:"task_id"`
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	IssueNumber int    `json:"issue_number"`
	IssueURL    string `json:"issue_url"`
	IssueTitle  string `json:"issue_title"`
}

func (s *Service) SetTaskIssueStore(store TaskIssueStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskIssueStore = store
}

func (s *Service) getTaskIssueStore() TaskIssueStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.taskIssueStore
}

func (s *Service) LinkTaskIssue(ctx context.Context, taskID string, req LinkTaskIssueRequest) (*TaskIssueLinkResponse, error) {
	if s.client == nil {
		return nil, ErrNoClient
	}
	store := s.getTaskIssueStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	owner, repo, number, err := resolveIssueReference(req)
	if err != nil {
		return nil, err
	}
	issue, err := s.client.GetIssue(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	if err := s.validateIssueTaskRepository(ctx, store, taskID, issue.RepoOwner, issue.RepoName); err != nil {
		return nil, err
	}
	task, err := store.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	metadata := copyMetadata(task.Metadata)
	metadata[taskMetaIssueURL] = issue.HTMLURL
	metadata[taskMetaIssueNumber] = issue.Number
	metadata[taskMetaIssueOwner] = issue.RepoOwner
	metadata[taskMetaIssueRepo] = issue.RepoName
	metadata[taskMetaIssueLinked] = true
	if _, err := store.UpdateTaskMetadata(context.WithoutCancel(ctx), taskID, metadata); err != nil {
		return nil, err
	}
	return taskIssueResponse(taskID, issue), nil
}

func (s *Service) UnlinkTaskIssue(ctx context.Context, taskID string) error {
	store := s.getTaskIssueStore()
	if store == nil {
		return errStoreUnavailable
	}
	task, err := store.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	metadata := copyMetadata(task.Metadata)
	delete(metadata, taskMetaIssueURL)
	delete(metadata, taskMetaIssueNumber)
	delete(metadata, taskMetaIssueOwner)
	delete(metadata, taskMetaIssueRepo)
	delete(metadata, taskMetaIssueLinked)
	_, err = store.UpdateTaskMetadata(context.WithoutCancel(ctx), taskID, metadata)
	return err
}

func resolveIssueReference(req LinkTaskIssueRequest) (string, string, int, error) {
	if req.Issue != "" {
		return parseIssueReference(req.Issue, req.Owner, req.Repo)
	}
	if req.Owner == "" || req.Repo == "" || req.Number <= 0 {
		return "", "", 0, ErrInvalidIssueReference
	}
	return req.Owner, req.Repo, req.Number, nil
}

func parseIssueReference(input, defaultOwner, defaultRepo string) (string, string, int, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", "", 0, ErrInvalidIssueReference
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(trimmed, "#")); err == nil && n > 0 {
		if defaultOwner == "" || defaultRepo == "" {
			return "", "", 0, fmt.Errorf("%w: owner and repo are required for issue numbers", ErrInvalidIssueReference)
		}
		return defaultOwner, defaultRepo, n, nil
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return "", "", 0, fmt.Errorf("%w: expected a GitHub issue URL", ErrInvalidIssueReference)
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	if host != "github.com" || !strings.Contains(u.Path, githubIssuePathMarker) {
		return "", "", 0, fmt.Errorf("%w: expected a GitHub issue URL", ErrInvalidIssueReference)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "issues" {
		return "", "", 0, fmt.Errorf("%w: expected /owner/repo/issues/number", ErrInvalidIssueReference)
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number <= 0 {
		return "", "", 0, fmt.Errorf("%w: invalid issue number", ErrInvalidIssueReference)
	}
	return parts[0], parts[1], number, nil
}

func (s *Service) validateIssueTaskRepository(ctx context.Context, store TaskIssueStore, taskID, owner, repo string) error {
	taskRepos, err := store.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return err
	}
	if len(taskRepos) == 0 {
		return nil
	}
	for _, taskRepo := range taskRepos {
		entity, err := store.GetRepository(ctx, taskRepo.RepositoryID)
		if err != nil {
			return err
		}
		if entity == nil {
			continue
		}
		if strings.EqualFold(entity.Provider, githubProviderName) &&
			strings.EqualFold(entity.ProviderOwner, owner) &&
			strings.EqualFold(entity.ProviderName, repo) {
			return nil
		}
	}
	return ErrIssueRepositoryMismatch
}

func copyMetadata(metadata map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(metadata)+5)
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func taskIssueResponse(taskID string, issue *Issue) *TaskIssueLinkResponse {
	return &TaskIssueLinkResponse{
		TaskID:      taskID,
		Owner:       issue.RepoOwner,
		Repo:        issue.RepoName,
		IssueNumber: issue.Number,
		IssueURL:    issue.HTMLURL,
		IssueTitle:  issue.Title,
	}
}
