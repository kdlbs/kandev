package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type fakeContributionDestinationPreparer struct {
	calls       int
	gotWorkflow *models.Workflow
	gotRepos    []*models.Repository
	destination *models.ContributionDestination
	err         error
}

func (f *fakeContributionDestinationPreparer) PrepareContributionDestination(
	_ context.Context,
	request *CreateTaskRequest,
	workflow *models.Workflow,
	repositories []*models.Repository,
) error {
	f.calls++
	f.gotWorkflow = workflow
	f.gotRepos = repositories
	if f.err != nil {
		return f.err
	}
	if f.destination != nil && len(request.Repositories) > 0 {
		request.Repositories[0].ContributionDestination = f.destination
	}
	return nil
}

func TestCreateTaskPreparesManagedDestinationBeforeTaskInsert(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	workflowID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-improve")
	workflow, err := repo.GetWorkflow(ctx, workflowID)
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	templateID := "improve-kandev"
	workflow.WorkflowTemplateID = &templateID
	if err := repo.UpdateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("update workflow template: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID:             "repo-kandev",
		WorkspaceID:    "ws-improve",
		Name:           "kandev",
		Provider:       "github",
		ProviderHost:   "https://github.com",
		ProviderOwner:  "kdlbs",
		ProviderName:   "kandev",
		ProviderRepoID: "100",
		RemoteURL:      "https://github.com/kdlbs/kandev.git",
		DefaultBranch:  "main",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}

	preparer := &fakeContributionDestinationPreparer{err: errors.New("fork preparation failed")}
	svc.SetContributionDestinationPreparer(preparer)
	_, err = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-improve",
		WorkflowID:  workflowID,
		ExternalID:  "improve-1",
		Title:       "Improve",
		Repositories: []TaskRepositoryInput{{
			RepositoryID: "repo-kandev",
			BaseBranch:   "main",
		}},
	})
	if err == nil || err.Error() != "fork preparation failed" {
		t.Fatalf("CreateTask error = %v, want fork preparation failure", err)
	}
	if preparer.calls != 1 {
		t.Fatalf("preparer calls = %d, want 1", preparer.calls)
	}
	if preparer.gotWorkflow == nil || preparer.gotWorkflow.WorkflowTemplateID == nil ||
		*preparer.gotWorkflow.WorkflowTemplateID != "improve-kandev" {
		t.Fatalf("preparer workflow = %#v, want improve-kandev template", preparer.gotWorkflow)
	}
	if len(preparer.gotRepos) != 1 || preparer.gotRepos[0].ID != "repo-kandev" {
		t.Fatalf("preparer repositories = %#v, want canonical repository", preparer.gotRepos)
	}
	if _, lookupErr := repo.GetTaskByExternalID(ctx, "ws-improve", "improve-1"); !errors.Is(lookupErr, repository.ErrTaskNotFound) &&
		!errors.Is(lookupErr, repoerrors.ErrTaskNotFound) {
		t.Fatalf("task lookup error = %v, want task not inserted", lookupErr)
	}
}

func TestCreateTaskSkipsDestinationPreparationForOrdinaryWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	workflowID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-ordinary")
	preparer := &fakeContributionDestinationPreparer{}
	svc.SetContributionDestinationPreparer(preparer)

	if _, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-ordinary",
		WorkflowID:  workflowID,
		Title:       "Ordinary",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if preparer.calls != 0 {
		t.Fatalf("preparer calls = %d, want 0", preparer.calls)
	}
}

func TestCreateTaskPersistsServerAuthoredDestinationOnCanonicalAttachment(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	workflowID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-persist-destination")
	workflow, err := repo.GetWorkflow(ctx, workflowID)
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	templateID := "improve-kandev"
	workflow.WorkflowTemplateID = &templateID
	if err := repo.UpdateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("update workflow template: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID:             "repo-persist-destination",
		WorkspaceID:    "ws-persist-destination",
		Name:           "kandev",
		Provider:       "github",
		ProviderHost:   "https://github.com",
		ProviderOwner:  "kdlbs",
		ProviderName:   "kandev",
		ProviderRepoID: "100",
		RemoteURL:      "https://github.com/kdlbs/kandev.git",
		DefaultBranch:  "main",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	destination := testContributionDestinationForService()
	preparer := &fakeContributionDestinationPreparer{destination: &destination}
	svc.SetContributionDestinationPreparer(preparer)
	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:  "ws-persist-destination",
		WorkflowID:   workflowID,
		Title:        "Persist destination",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-persist-destination", BaseBranch: "main"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	attachments, err := repo.ListTaskRepositories(ctx, result.Task.ID)
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(attachments))
	}
	got, ok, err := models.LoadContributionDestination(attachments[0].Metadata)
	if err != nil {
		t.Fatalf("LoadContributionDestination: %v", err)
	}
	if !ok || got != destination {
		t.Fatalf("stored destination = %#v, %v; want %#v, true", got, ok, destination)
	}
}

func testContributionDestinationForService() models.ContributionDestination {
	return models.ContributionDestination{
		Version:  models.ContributionDestinationVersion,
		Provider: models.ContributionDestinationProviderGitHub,
		SourceRepository: models.RemoteContributionRepository{
			Host: "github.com", Path: "kdlbs/kandev", ProviderID: "100", RemoteURL: "https://github.com/kdlbs/kandev.git",
		},
		TargetRepository: models.RemoteContributionRepository{
			Host: "github.com", Path: "alice/kandev", ProviderID: "200", RemoteURL: "https://github.com/alice/kandev.git",
		},
	}
}
