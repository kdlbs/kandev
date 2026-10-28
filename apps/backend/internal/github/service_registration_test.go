package github

import (
	"context"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestRegistrationReadFailureNoAssociation(t *testing.T) {
	service, store, client := registrationServiceFixture(t, 200)
	association, err := service.RegisterExistingPRByURLForWorkspace(
		context.Background(), "workspace-registration", "user-1", "task-registration", "repo-registration",
		"https://github.com/acme/demo/pull/17", false, "operation-1",
	)
	if err == nil || association != nil {
		t.Fatalf("registration = %+v, err=%v; want immutable repository mismatch", association, err)
	}
	rows, err := store.ListTaskPRsByTask(context.Background(), "task-registration")
	if err != nil || len(rows) != 0 {
		t.Fatalf("failed identity read persisted an association: rows=%+v err=%v", rows, err)
	}
	if client.getPRCalls != 1 {
		t.Fatalf("provider PR reads = %d, want 1", client.getPRCalls)
	}
}

func TestRegistrationStoresVerifiedImmutableIdentity(t *testing.T) {
	service, store, _ := registrationServiceFixture(t, 100)
	association, err := service.RegisterExistingPRByURLForWorkspace(
		context.Background(), "workspace-registration", "user-1", "task-registration", "repo-registration",
		"https://github.com/acme/demo/pull/17", false, "operation-1",
	)
	if err != nil {
		t.Fatalf("RegisterExistingPRByURLForWorkspace: %v", err)
	}
	if association.ProviderRepositoryID != "100" || association.ConnectionScope != "builtin:workspace-registration" ||
		association.RepositoryPath != "acme/demo" || association.ComparisonPolicy != TaskPRComparisonPolicyPreserve ||
		association.Source != TaskPRSourceAgentRegistration || association.FirstOriginID != "operation-1" {
		t.Fatalf("registration identity = %+v", association)
	}
	if _, err := store.GetTaskPRByRepoAndNumber(context.Background(), "task-registration", "repo-registration", 17); err != nil {
		t.Fatalf("read registered PR: %v", err)
	}
}

func TestRegistrationKeepsAcceptedRevisionAcrossRetry(t *testing.T) {
	service, store, client := registrationServiceFixture(t, 100)
	ctx := context.Background()
	initial := registrationTaskPR("task-registration", "repo-registration", "100", 17)
	initial.WorkspaceID = "workspace-registration"
	initial.ConnectionScope = "builtin:workspace-registration"
	created, _, err := store.RegisterTaskPRObservation(ctx, initial, false, 0)
	if err != nil {
		t.Fatalf("seed initial association: %v", err)
	}
	detached, _, err := store.DetachTaskPR(ctx, created.ID)
	if err != nil {
		t.Fatalf("detach initial association: %v", err)
	}
	commit := func(ctx context.Context, registration *TaskPR, restore bool, expected int64) (*TaskPR, bool, error) {
		return store.RegisterTaskPRObservation(ctx, registration, restore, expected)
	}
	if _, err := service.RegisterExistingPRByURLForWorkspaceAtRevision(
		ctx, "workspace-registration", "user-1", "task-registration", "repo-registration",
		"https://github.com/acme/demo/pull/17", created.AssociationRevision, true, "message-op", commit,
	); err != ErrTaskPRRegistrationRevisionChanged {
		t.Fatalf("retry with pre-unlink revision = %v, want revision changed", err)
	}
	if client.getPRCalls != 0 {
		t.Fatalf("stale retry performed %d provider reads, want no mutation work", client.getPRCalls)
	}
	restored, err := service.RegisterExistingPRByURLForWorkspaceAtRevision(
		ctx, "workspace-registration", "user-1", "task-registration", "repo-registration",
		"https://github.com/acme/demo/pull/17", detached.AssociationRevision, true, "message-op", commit,
	)
	if err != nil || restored.AssociationRevision != detached.AssociationRevision+1 || restored.DetachedAt != nil {
		t.Fatalf("retry with accepted detached revision = %+v, err=%v", restored, err)
	}
}

type countedRegistrationClient struct {
	repositoryVisibilityClient
	getPRCalls int
}

func (c *countedRegistrationClient) GetPR(ctx context.Context, owner, repo string, number int) (*PR, error) {
	c.getPRCalls++
	return c.repositoryVisibilityClient.GetPR(ctx, owner, repo, number)
}

func registrationServiceFixture(t *testing.T, baseRepositoryID int64) (*Service, *Store, *countedRegistrationClient) {
	t.Helper()
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-registration")
	mock := NewMockClient()
	mock.SetPRDetail("acme", "demo", 17, &PR{
		Number: 17, HTMLURL: "https://github.com/acme/demo/pull/17", Title: "registration",
		RepoOwner: "acme", RepoName: "demo", BaseRepoID: baseRepositoryID,
		HeadBranch: "feature", HeadSHA: "head", BaseBranch: "main", State: "open",
		CreatedAt: time.Now().UTC(),
	}, nil)
	client := &countedRegistrationClient{repositoryVisibilityClient: repositoryVisibilityClient{
		Client: mock, repos: map[string]GitHubRepo{"acme/demo": testGitHubRepo("acme", "demo")},
	}}
	service := newWorkspaceAuthenticatedTestService(t, client, store, "workspace-registration")
	service.SetTaskIssueStore(&fakeTaskIssueStore{
		task:  &taskmodels.Task{ID: "task-registration", WorkspaceID: "workspace-registration"},
		repos: []*taskmodels.TaskRepository{{TaskID: "task-registration", RepositoryID: "repo-registration"}},
		entities: map[string]*taskmodels.Repository{
			"repo-registration": {
				ID: "repo-registration", WorkspaceID: "workspace-registration", Provider: "github",
				ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "demo", ProviderRepoID: "100",
			},
		},
	})
	return service, store, client
}
