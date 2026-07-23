package mentions

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/azuredevops"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeAzureMentionService struct {
	workItems  []azuredevops.MentionWorkItem
	prs        []azuredevops.MentionPullRequest
	project    *azuredevops.MentionProject
	repository *azuredevops.MentionRepository
	err        error
}

func (s *fakeAzureMentionService) SearchMentionWorkItemsForWorkspace(
	context.Context, string, string, int,
) ([]azuredevops.MentionWorkItem, error) {
	return s.workItems, s.err
}

func (s *fakeAzureMentionService) SearchMentionPullRequestsForWorkspace(
	context.Context, string, string, int,
) ([]azuredevops.MentionPullRequest, error) {
	return s.prs, s.err
}

func (s *fakeAzureMentionService) ResolveMentionProjectForWorkspace(
	context.Context, string, string,
) (*azuredevops.MentionProject, error) {
	return s.project, s.err
}

func (s *fakeAzureMentionService) ResolveMentionRepositoryForWorkspace(
	context.Context, string, string, string,
) (*azuredevops.MentionRepository, error) {
	return s.repository, s.err
}

func TestAzureProvidersMapWorkItemsAndPullRequests(t *testing.T) {
	service := &fakeAzureMentionService{
		workItems: []azuredevops.MentionWorkItem{{
			ID: 101, Title: "Fix auth", OrganizationURL: "https://dev.azure.com/acme",
			ProjectID: "project-1", ProjectName: "Platform",
		}},
		prs: []azuredevops.MentionPullRequest{{
			ID: 42, Title: "Land auth fix", OrganizationURL: "https://dev.azure.com/acme",
			ProjectID: "project-1", ProjectName: "Platform", RepositoryID: "repo-1", RepositoryName: "widgets",
		}},
	}
	workProvider := NewAzureWorkItemProvider(service)
	prProvider := NewAzurePullRequestProvider(service)
	if descriptor := workProvider.Descriptor(); descriptor.Provider != "azure_devops" || descriptor.Kind != "work_item" || descriptor.Order != 60 {
		t.Fatalf("work descriptor = %#v", descriptor)
	}
	if descriptor := prProvider.Descriptor(); descriptor.Provider != "azure_devops" || descriptor.Kind != "pull_request" || descriptor.Order != 61 {
		t.Fatalf("PR descriptor = %#v", descriptor)
	}
	work, err := workProvider.Search(t.Context(), SearchRequest{WorkspaceID: "workspace-1", Query: "auth", Limit: 5})
	if err != nil {
		t.Fatalf("search work: %v", err)
	}
	prs, err := prProvider.Search(t.Context(), SearchRequest{WorkspaceID: "workspace-1", Query: "auth", Limit: 5})
	if err != nil {
		t.Fatalf("search PRs: %v", err)
	}
	wantWork := Candidate{
		ID: "101", Key: "Platform#101", Title: "Fix auth",
		URL:   "https://dev.azure.com/acme/Platform/_workitems/edit/101",
		Scope: "dev.azure.com/acme/project-1",
	}
	wantPR := Candidate{
		ID: "42", Key: "Platform/widgets!42", Title: "Land auth fix",
		URL:   "https://dev.azure.com/acme/Platform/_git/widgets/pullrequest/42",
		Scope: "dev.azure.com/acme/project-1/repo-1",
	}
	if len(work) != 1 || work[0] != wantWork {
		t.Fatalf("work = %#v, want %#v", work, wantWork)
	}
	if len(prs) != 1 || prs[0] != wantPR {
		t.Fatalf("PRs = %#v, want %#v", prs, wantPR)
	}
}

func TestAzureProviderAuthorizesConfiguredProjectAndCanonicalRoute(t *testing.T) {
	service := &fakeAzureMentionService{
		project: &azuredevops.MentionProject{
			OrganizationURL: "https://dev.azure.com/acme", ProjectID: "project-1", ProjectName: "Platform",
		},
		repository: &azuredevops.MentionRepository{
			OrganizationURL: "https://dev.azure.com/acme", ProjectID: "project-1", ProjectName: "Platform",
			RepositoryID: "repo-1", RepositoryName: "widgets",
		},
	}
	workProvider := NewAzureWorkItemProvider(service)
	prProvider := NewAzurePullRequestProvider(service)
	workRef := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      canonicalRef("azure_devops", "work_item", "dev.azure.com/acme/project-1", "101"),
		Provider: "azure_devops", Kind: "work_item", ID: "101", Key: "Platform#101", Title: "Fix auth",
		URL: "https://dev.azure.com/acme/Platform/_workitems/edit/101", Scope: "dev.azure.com/acme/project-1",
	}
	prRef := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      canonicalRef("azure_devops", "pull_request", "dev.azure.com/acme/project-1/repo-1", "42"),
		Provider: "azure_devops", Kind: "pull_request", ID: "42", Key: "Platform/widgets!42", Title: "Land auth",
		URL: "https://dev.azure.com/acme/Platform/_git/widgets/pullrequest/42", Scope: "dev.azure.com/acme/project-1/repo-1",
	}
	for _, test := range []struct {
		provider MentionProvider
		ref      apiv1.EntityReference
	}{
		{workProvider, workRef}, {prProvider, prRef},
	} {
		authorizer := test.provider.(ReferenceAuthorizer)
		if err := authorizer.AuthorizeReference(t.Context(), ReferenceAuthorizationRequest{
			WorkspaceID: "workspace-1", Purpose: ReferencePurposeSubmission, Reference: test.ref,
		}); err != nil {
			t.Fatalf("authorize %s: %v", test.ref.Kind, err)
		}
		invalid := test.ref
		invalid.URL += "?redirect=1"
		if err := authorizer.AuthorizeReference(t.Context(), ReferenceAuthorizationRequest{
			WorkspaceID: "workspace-1", Purpose: ReferencePurposeSubmission, Reference: invalid,
		}); err == nil {
			t.Fatalf("unsafe %s URL authorized", test.ref.Kind)
		}
	}
	foreign := workRef
	foreign.Scope = "dev.azure.com/foreign/project-1"
	foreign.Ref = canonicalRef("azure_devops", "work_item", foreign.Scope, foreign.ID)
	if err := workProvider.(ReferenceAuthorizer).AuthorizeReference(t.Context(), ReferenceAuthorizationRequest{
		WorkspaceID: "workspace-1", Purpose: ReferencePurposeSubmission, Reference: foreign,
	}); err == nil {
		t.Fatal("foreign organization scope authorized")
	}
}

func TestAzureProviderClassifiesMissingConfig(t *testing.T) {
	provider := NewAzureWorkItemProvider(&fakeAzureMentionService{err: azuredevops.ErrNotConfigured})
	_, err := provider.Search(context.Background(), SearchRequest{WorkspaceID: "workspace-1", Query: "auth", Limit: 5})
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Status != StatusNotConfigured {
		t.Fatalf("error = %v, want not configured", err)
	}
}
