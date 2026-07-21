package azuredevops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidTaskPRAssociation identifies repository/task/provider validation
// failures that controllers expose as bad requests.
var ErrInvalidTaskPRAssociation = errors.New("azure devops: invalid task PR association")

// SetRepositoryLookup wires the task-repository ownership resolver.
func (s *Service) SetRepositoryLookup(lookup RepositoryLookup) {
	s.repoLookup = lookup
}

// AssociateTaskPR validates a task repository, fetches the Azure PR, and
// persists its current summary.
func (s *Service) AssociateTaskPR(
	ctx context.Context,
	workspaceID, taskID, repositoryID string,
	pullRequestID int,
) (*TaskPR, error) {
	return s.syncTaskPR(ctx, workspaceID, taskID, repositoryID, pullRequestID)
}

// SyncTaskPR refreshes one persisted task PR association from Azure.
func (s *Service) SyncTaskPR(
	ctx context.Context,
	workspaceID, taskID, repositoryID string,
	pullRequestID int,
) (*TaskPR, error) {
	return s.syncTaskPR(ctx, workspaceID, taskID, repositoryID, pullRequestID)
}

func (s *Service) syncTaskPR(
	ctx context.Context,
	workspaceID, taskID, repositoryID string,
	pullRequestID int,
) (*TaskPR, error) {
	binding, err := s.validateTaskPRRepository(ctx, workspaceID, taskID, repositoryID, pullRequestID)
	if err != nil {
		return nil, err
	}
	feedback, err := s.GetPullRequestFeedbackForWorkspace(
		ctx, workspaceID, binding.ProviderOwner, binding.ProviderRepoID, pullRequestID,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch Azure pull request feedback: %w", err)
	}
	if feedback == nil || feedback.PullRequest == nil || feedback.PullRequest.ID != pullRequestID {
		return nil, fmt.Errorf("azure pull request %d not found", pullRequestID)
	}
	if err := validateAzurePRIdentity(feedback.PullRequest, binding); err != nil {
		return nil, err
	}
	cfg, err := s.store.GetConfig(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load Azure DevOps config: %w", err)
	}
	if cfg == nil {
		return nil, ErrNotConfigured
	}
	row := taskPRFromFeedback(taskID, repositoryID, cfg.OrganizationURL, binding, feedback)
	if err := s.store.UpsertTaskPR(ctx, row); err != nil {
		return nil, fmt.Errorf("upsert Azure task PR: %w", err)
	}
	return row, nil
}

func (s *Service) validateTaskPRRepository(
	ctx context.Context,
	workspaceID, taskID, repositoryID string,
	pullRequestID int,
) (*RepositoryBinding, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(repositoryID) == "" || pullRequestID <= 0 {
		return nil, fmt.Errorf("%w: task, repository, and positive pull request ID are required", ErrInvalidTaskPRAssociation)
	}
	if s.repoLookup == nil {
		return nil, fmt.Errorf("%w: repository lookup is not configured", ErrInvalidTaskPRAssociation)
	}
	binding, err := s.repoLookup.LookupTaskRepository(ctx, taskID, repositoryID)
	if err != nil {
		return nil, fmt.Errorf("%w: lookup task repository: %v", ErrInvalidTaskPRAssociation, err)
	}
	if binding == nil {
		return nil, fmt.Errorf("%w: repository is not linked to task", ErrInvalidTaskPRAssociation)
	}
	if binding.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("%w: repository belongs to another workspace", ErrInvalidTaskPRAssociation)
	}
	if binding.Provider != RepositoryProvider {
		return nil, fmt.Errorf("%w: repository provider must be %s", ErrInvalidTaskPRAssociation, RepositoryProvider)
	}
	if strings.TrimSpace(binding.ProviderOwner) == "" || strings.TrimSpace(binding.ProviderRepoID) == "" {
		return nil, fmt.Errorf("%w: Azure project and repository IDs are required", ErrInvalidTaskPRAssociation)
	}
	return binding, nil
}

func validateAzurePRIdentity(pr *PullRequest, binding *RepositoryBinding) error {
	if pr.ProjectID != "" && pr.ProjectID != binding.ProviderOwner {
		return fmt.Errorf("%w: pull request belongs to another Azure project", ErrInvalidTaskPRAssociation)
	}
	if pr.RepositoryID != "" && pr.RepositoryID != binding.ProviderRepoID {
		return fmt.Errorf("%w: pull request belongs to another Azure repository", ErrInvalidTaskPRAssociation)
	}
	return nil
}

func taskPRFromFeedback(
	taskID, repositoryID, organizationURL string,
	binding *RepositoryBinding,
	feedback *PullRequestFeedback,
) *TaskPR {
	pr := feedback.PullRequest
	now := time.Now().UTC()
	return &TaskPR{
		TaskID: taskID, RepositoryID: repositoryID, OrganizationURL: organizationURL,
		ProjectID: binding.ProviderOwner, AzureRepositoryID: binding.ProviderRepoID,
		PullRequestID: pr.ID, PullRequestURL: pr.WebURL, Title: pr.Title,
		SourceBranch: normalizeBranch(pr.SourceBranch), TargetBranch: normalizeBranch(pr.TargetBranch),
		AuthorID: pr.Author.ID, AuthorName: pr.Author.DisplayName, Status: pr.Status,
		ReviewState: feedback.ReviewState, PolicyState: feedback.PolicyState,
		IsDraft: pr.IsDraft, LastSyncedAt: &now,
	}
}

func normalizeBranch(branch string) string {
	return strings.TrimPrefix(branch, "refs/heads/")
}

// ListTaskPRsByWorkspace returns associations grouped by task ID.
func (s *Service) ListTaskPRsByWorkspace(ctx context.Context, workspaceID string) (map[string][]*TaskPR, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	return s.store.ListTaskPRsByWorkspace(ctx, workspaceID)
}

// ListTaskPRsByTask returns all associations linked to one task.
func (s *Service) ListTaskPRsByTask(ctx context.Context, taskID string) ([]*TaskPR, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("%w: task is required", ErrInvalidTaskPRAssociation)
	}
	return s.store.ListTaskPRsByTask(ctx, taskID)
}
