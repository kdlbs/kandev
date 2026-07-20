package azuredevops

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	reviewStateApproved = "approved"
	policyStateSuccess  = "success"
)

// PullRequestFeedback keeps Azure-native detail while providing normalized
// summary states for shared task UI.
type PullRequestFeedback struct {
	PullRequest     *PullRequest       `json:"pullRequest"`
	Reviewers       []Reviewer         `json:"reviewers"`
	Threads         []Thread           `json:"threads"`
	LinkedWorkItems []WorkItemRef      `json:"linkedWorkItems"`
	Policies        []PolicyEvaluation `json:"policies"`
	ReviewState     string             `json:"reviewState"`
	PolicyState     string             `json:"policyState"`
}

func (s *Service) clientForWorkspace(ctx context.Context, workspaceID string) (Client, error) {
	cfg, pat, err := s.resolveCredentials(ctx, workspaceID, &SetConfigRequest{})
	if err != nil {
		return nil, err
	}
	return s.clientFn(cfg, pat), nil
}

func (s *Service) ListProjectsForWorkspace(ctx context.Context, workspaceID string) ([]Project, error) {
	client, err := s.clientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.ListProjects(ctx)
}

func (s *Service) ListRepositoriesForWorkspace(ctx context.Context, workspaceID, projectID string) ([]Repository, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("%w: project required", ErrInvalidConfig)
	}
	client, err := s.clientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.ListRepositories(ctx, projectID)
}

type branchClient interface {
	ListBranches(ctx context.Context, projectID, repositoryID string) ([]Branch, error)
}

func (s *Service) ListBranchesForWorkspace(
	ctx context.Context, workspaceID, organization, projectID, repositoryID string,
) ([]Branch, error) {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(repositoryID) == "" {
		return nil, fmt.Errorf("%w: project and repository required", ErrInvalidConfig)
	}
	cfg, pat, err := s.resolveCredentials(ctx, workspaceID, &SetConfigRequest{})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(organization) != "" {
		requestedURL, validationErr := ValidateOrganizationURL("https://dev.azure.com/" + organization)
		if validationErr != nil || !strings.EqualFold(requestedURL, cfg.OrganizationURL) {
			return nil, fmt.Errorf("%w: requested organization does not match workspace configuration", ErrInvalidConfig)
		}
	}
	client := s.clientFn(cfg, pat)
	reader, ok := client.(branchClient)
	if !ok {
		return nil, errors.New("azure devops: branch listing is unavailable")
	}
	return reader.ListBranches(ctx, projectID, repositoryID)
}

func (s *Service) SearchWorkItemsForWorkspace(
	ctx context.Context,
	workspaceID, projectID, wiql string,
	top int,
) (*WorkItemSearchResult, error) {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(wiql) == "" {
		return nil, fmt.Errorf("%w: project and wiql required", ErrInvalidConfig)
	}
	client, err := s.clientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.QueryWIQL(ctx, projectID, wiql, top)
}

func (s *Service) GetWorkItemForWorkspace(ctx context.Context, workspaceID, projectID string, id int) (*WorkItem, error) {
	if strings.TrimSpace(projectID) == "" || id <= 0 {
		return nil, fmt.Errorf("%w: project and positive work item id required", ErrInvalidConfig)
	}
	client, err := s.clientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.GetWorkItem(ctx, projectID, id)
}

func (s *Service) ListPullRequestsForWorkspace(
	ctx context.Context,
	workspaceID string,
	filter PullRequestFilter,
) (*PullRequestPage, error) {
	if strings.TrimSpace(filter.ProjectID) == "" || strings.TrimSpace(filter.RepositoryID) == "" {
		return nil, fmt.Errorf("%w: project and repository required", ErrInvalidConfig)
	}
	client, err := s.clientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	filter, err = resolveCurrentUserPullRequestFilters(ctx, client, filter)
	if err != nil {
		return nil, err
	}
	return client.ListPullRequests(ctx, filter)
}

func resolveCurrentUserPullRequestFilters(
	ctx context.Context,
	client Client,
	filter PullRequestFilter,
) (PullRequestFilter, error) {
	creatorIsMe := strings.EqualFold(strings.TrimSpace(filter.CreatorID), "@me")
	reviewerIsMe := strings.EqualFold(strings.TrimSpace(filter.ReviewerID), "@me")
	if !creatorIsMe && !reviewerIsMe {
		return filter, nil
	}
	identity, err := client.TestAuth(ctx)
	if err != nil {
		return filter, err
	}
	if identity == nil || !identity.OK || strings.TrimSpace(identity.ID) == "" {
		return filter, fmt.Errorf("%w: authenticated Azure DevOps identity is unavailable", ErrInvalidConfig)
	}
	if creatorIsMe {
		filter.CreatorID = identity.ID
	}
	if reviewerIsMe {
		filter.ReviewerID = identity.ID
	}
	return filter, nil
}

func (s *Service) GetPullRequestForWorkspace(
	ctx context.Context,
	workspaceID, projectID, repositoryID string,
	id int,
) (*PullRequest, error) {
	client, err := s.clientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.GetPullRequest(ctx, projectID, repositoryID, id)
}

func (s *Service) GetPullRequestFeedbackForWorkspace(
	ctx context.Context,
	workspaceID, projectID, repositoryID string,
	id int,
) (*PullRequestFeedback, error) {
	client, err := s.clientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	pr, err := client.GetPullRequest(ctx, projectID, repositoryID, id)
	if err != nil {
		return nil, err
	}
	reviewers, err := client.ListReviewers(ctx, projectID, repositoryID, id)
	if err != nil {
		return nil, err
	}
	threads, err := client.ListThreads(ctx, projectID, repositoryID, id)
	if err != nil {
		return nil, err
	}
	refs, err := client.ListLinkedWorkItems(ctx, projectID, repositoryID, id)
	if err != nil {
		return nil, err
	}
	policies, err := client.ListPolicyEvaluations(ctx, projectID, id)
	if err != nil {
		return nil, err
	}
	return &PullRequestFeedback{
		PullRequest: pr, Reviewers: nonNilReviewers(reviewers), Threads: nonNilThreads(threads),
		LinkedWorkItems: nonNilWorkItemRefs(refs), Policies: nonNilPolicies(policies),
		ReviewState: summarizeReviewState(reviewers), PolicyState: summarizePolicyState(policies),
	}, nil
}

func summarizeReviewState(reviewers []Reviewer) string {
	hasApproval := false
	hasWaiting := false
	for _, reviewer := range reviewers {
		switch reviewer.Vote {
		case -10:
			return "rejected"
		case -5:
			hasWaiting = true
		case 5, 10:
			hasApproval = true
		}
	}
	if hasWaiting {
		return "waiting"
	}
	if hasApproval {
		return reviewStateApproved
	}
	return ""
}

func summarizePolicyState(policies []PolicyEvaluation) string {
	if len(policies) == 0 {
		return ""
	}
	pending := false
	for _, policy := range policies {
		switch strings.ToLower(policy.Status) {
		case "rejected", "broken":
			if policy.IsBlocking {
				return "failure"
			}
		case "queued", "running":
			pending = true
		}
	}
	if pending {
		return "pending"
	}
	return policyStateSuccess
}

func nonNilReviewers(items []Reviewer) []Reviewer {
	if items == nil {
		return []Reviewer{}
	}
	return items
}
func nonNilThreads(items []Thread) []Thread {
	if items == nil {
		return []Thread{}
	}
	return items
}
func nonNilWorkItemRefs(items []WorkItemRef) []WorkItemRef {
	if items == nil {
		return []WorkItemRef{}
	}
	return items
}
func nonNilPolicies(items []PolicyEvaluation) []PolicyEvaluation {
	if items == nil {
		return []PolicyEvaluation{}
	}
	return items
}
