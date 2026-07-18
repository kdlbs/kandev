package azuredevops

import (
	"context"
	"errors"
	"testing"
)

type taskPRServiceContract interface {
	SetRepositoryLookup(RepositoryLookup)
	AssociateTaskPR(context.Context, string, string, string, int) (*TaskPR, error)
	SyncTaskPR(context.Context, string, string, string, int) (*TaskPR, error)
}

type fakeAzureRepositoryLookup struct {
	binding *RepositoryBinding
	err     error
}

func (f fakeAzureRepositoryLookup) LookupTaskRepository(
	context.Context,
	string,
	string,
) (*RepositoryBinding, error) {
	return f.binding, f.err
}

type taskPRClient struct {
	invalidClient
	pr        *PullRequest
	reviewers []Reviewer
	policies  []PolicyEvaluation
	getCalls  int
}

func (c *taskPRClient) GetPullRequest(context.Context, string, string, int) (*PullRequest, error) {
	c.getCalls++
	if c.pr == nil {
		return nil, errors.New("pull request not found")
	}
	copy := *c.pr
	return &copy, nil
}

func (c *taskPRClient) ListReviewers(context.Context, string, string, int) ([]Reviewer, error) {
	return c.reviewers, nil
}

func (c *taskPRClient) ListThreads(context.Context, string, string, int) ([]Thread, error) {
	return []Thread{}, nil
}

func (c *taskPRClient) ListLinkedWorkItems(context.Context, string, string, int) ([]WorkItemRef, error) {
	return []WorkItemRef{}, nil
}

func (c *taskPRClient) ListPolicyEvaluations(context.Context, string, int) ([]PolicyEvaluation, error) {
	return c.policies, nil
}

func TestServiceAssociateTaskPRValidatesRepositoryOwnershipAndProvider(t *testing.T) {
	tests := []struct {
		name      string
		workspace string
		binding   *RepositoryBinding
	}{
		{name: "repository is not linked to task", workspace: "ws-a"},
		{name: "repository belongs to another workspace", workspace: "ws-a", binding: validAzureBinding("ws-b")},
		{name: "repository uses another provider", workspace: "ws-a", binding: &RepositoryBinding{
			WorkspaceID: "ws-a", Provider: "github", ProviderOwner: "project-1", ProviderRepoID: "azure-repo-1",
		}},
		{name: "repository has no project ID", workspace: "ws-a", binding: &RepositoryBinding{
			WorkspaceID: "ws-a", Provider: RepositoryProvider, ProviderRepoID: "azure-repo-1",
		}},
		{name: "repository has no Azure repository ID", workspace: "ws-a", binding: &RepositoryBinding{
			WorkspaceID: "ws-a", Provider: RepositoryProvider, ProviderOwner: "project-1",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, client := newTaskPRServiceFixture(t)
			contract, ok := any(svc).(taskPRServiceContract)
			if !ok {
				t.Fatal("Service does not implement the task PR association contract")
			}
			contract.SetRepositoryLookup(fakeAzureRepositoryLookup{binding: tt.binding})
			if _, err := contract.AssociateTaskPR(t.Context(), tt.workspace, "task-1", "repo-1", 42); err == nil {
				t.Fatal("association unexpectedly succeeded")
			}
			if client.getCalls != 0 {
				t.Fatalf("Azure API called before repository validation: %d", client.getCalls)
			}
		})
	}
}

func TestServiceAssociateTaskPRValidatesAzureIdentity(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		repoID    string
	}{
		{name: "another project", projectID: "project-2", repoID: "azure-repo-1"},
		{name: "another repository", projectID: "project-1", repoID: "azure-repo-2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, client := newTaskPRServiceFixture(t)
			svc.SetRepositoryLookup(fakeAzureRepositoryLookup{binding: validAzureBinding("ws-a")})
			client.pr.ProjectID = tt.projectID
			client.pr.RepositoryID = tt.repoID

			if _, err := svc.AssociateTaskPR(t.Context(), "ws-a", "task-1", "repo-1", 42); !errors.Is(err, ErrInvalidTaskPRAssociation) {
				t.Fatalf("error = %v, want invalid task PR association", err)
			}
			rows, err := svc.Store().ListTaskPRsByTask(t.Context(), "task-1")
			if err != nil || len(rows) != 0 {
				t.Fatalf("persisted mismatched association: rows=%+v err=%v", rows, err)
			}
		})
	}
}

func TestServiceAssociateAndSyncTaskPRRefreshesPersistedSummary(t *testing.T) {
	svc, client := newTaskPRServiceFixture(t)
	contract, ok := any(svc).(taskPRServiceContract)
	if !ok {
		t.Fatal("Service does not implement the task PR association contract")
	}
	contract.SetRepositoryLookup(fakeAzureRepositoryLookup{binding: validAzureBinding("ws-a")})

	created, err := contract.AssociateTaskPR(t.Context(), "ws-a", "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("associate task PR: %v", err)
	}
	if created.OrganizationURL != "https://dev.azure.com/acme" || created.ProjectID != "project-1" ||
		created.AzureRepositoryID != "azure-repo-1" || created.SourceBranch != "feature/azure" ||
		created.TargetBranch != "main" || created.ReviewState != "waiting" || created.PolicyState != "pending" {
		t.Fatalf("created association = %+v", created)
	}

	client.pr.Title = "Updated upstream title"
	client.reviewers = []Reviewer{{Identity: Identity{ID: "reviewer-1"}, Vote: 10}}
	client.policies = []PolicyEvaluation{{ID: "policy-1", Status: "approved", IsBlocking: true}}
	refreshed, err := contract.SyncTaskPR(t.Context(), "ws-a", "task-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("sync task PR: %v", err)
	}
	if refreshed.ID != created.ID || refreshed.Title != "Updated upstream title" ||
		refreshed.ReviewState != "approved" || refreshed.PolicyState != "success" {
		t.Fatalf("refreshed association = %+v", refreshed)
	}
	rows, err := svc.Store().ListTaskPRsByTask(t.Context(), "task-1")
	if err != nil || len(rows) != 1 || rows[0].ID != created.ID {
		t.Fatalf("persisted associations: rows=%+v err=%v", rows, err)
	}
}

func TestSummarizeReviewStateUsesDeterministicPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		votes    []int
		expected string
	}{
		{name: "rejection after waiting", votes: []int{-5, -10}, expected: "rejected"},
		{name: "rejection before waiting", votes: []int{-10, -5}, expected: "rejected"},
		{name: "waiting beats approval", votes: []int{10, -5}, expected: "waiting"},
		{name: "approval", votes: []int{5, 10}, expected: "approved"},
		{name: "no vote", votes: []int{0}, expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviewers := make([]Reviewer, 0, len(tt.votes))
			for _, vote := range tt.votes {
				reviewers = append(reviewers, Reviewer{Vote: vote})
			}
			if got := summarizeReviewState(reviewers); got != tt.expected {
				t.Fatalf("summarizeReviewState(%v) = %q, want %q", tt.votes, got, tt.expected)
			}
		})
	}
}

func newTaskPRServiceFixture(t *testing.T) (*Service, *taskPRClient) {
	t.Helper()
	db := newTestDB(t)
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	client := &taskPRClient{
		pr: &PullRequest{
			ID: 42, Title: "Initial upstream title", Status: "active", IsDraft: true,
			SourceBranch: "refs/heads/feature/azure", TargetBranch: "refs/heads/main",
			Author:    Identity{ID: "author-1", DisplayName: "Ada"},
			ProjectID: "project-1", RepositoryID: "azure-repo-1",
			WebURL: "https://dev.azure.com/acme/project/_git/repo/pullrequest/42",
		},
		reviewers: []Reviewer{{Identity: Identity{ID: "reviewer-1"}, Vote: -5}},
		policies:  []PolicyEvaluation{{ID: "policy-1", Status: "running", IsBlocking: true}},
	}
	svc := NewService(store, newFakeSecretStore(), func(*Config, string) Client { return client }, nil)
	if _, err := svc.SetConfigForWorkspace(t.Context(), "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	return svc, client
}

func validAzureBinding(workspaceID string) *RepositoryBinding {
	return &RepositoryBinding{
		WorkspaceID: workspaceID, Provider: RepositoryProvider,
		ProviderOwner: "project-1", ProviderRepoID: "azure-repo-1",
	}
}
