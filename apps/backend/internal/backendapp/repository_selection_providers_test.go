package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/azuredevops"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type repositorySelectionGitHubStub struct {
	workspaceID string
	userID      string
	owner       string
	name        string
	repository  *github.GitHubRepository
	err         error
}

func (s *repositorySelectionGitHubStub) InspectRepositoryForWorkspace(
	_ context.Context, workspaceID, userID, owner, name string,
) (*github.GitHubRepository, error) {
	s.workspaceID, s.userID, s.owner, s.name = workspaceID, userID, owner, name
	return s.repository, s.err
}

type repositorySelectionGitLabStub struct {
	workspaceID string
	host        string
	client      gitlab.Client
}

func (s *repositorySelectionGitLabStub) ClientForWorkspaceHost(
	_ context.Context, workspaceID, host string,
) (gitlab.Client, error) {
	s.workspaceID, s.host = workspaceID, host
	return s.client, nil
}

type repositorySelectionAzureStub struct {
	workspaceID  string
	projectID    string
	projects     []azuredevops.Project
	repositories []azuredevops.Repository
}

func (s *repositorySelectionAzureStub) ListProjectsForWorkspace(
	_ context.Context, workspaceID string,
) ([]azuredevops.Project, error) {
	s.workspaceID = workspaceID
	return s.projects, nil
}

func (s *repositorySelectionAzureStub) ListRepositoriesForWorkspace(
	_ context.Context, workspaceID, projectID string,
) ([]azuredevops.Repository, error) {
	s.workspaceID, s.projectID = workspaceID, projectID
	return s.repositories, nil
}

func TestRepositorySelectionResolverInspectsExactGitHubRepository(t *testing.T) {
	provider := &repositorySelectionGitHubStub{repository: &github.GitHubRepository{
		FullName: "octo/outside-catalog", Owner: "octo", Name: "outside-catalog",
		CloneURL: "https://github.com/octo/outside-catalog.git", DefaultBranch: "stable",
	}}
	resolver := repositorySelectionResolver{github: provider}
	input := taskservice.TaskRepositoryInput{
		Provider: "github", RemoteURL: "https://github.com/octo/outside-catalog",
	}

	verified, err := resolver.ResolveRepositorySelection(context.Background(), "workspace-a", input)
	if err != nil {
		t.Fatalf("ResolveRepositorySelection: %v", err)
	}
	if provider.workspaceID != "workspace-a" || provider.userID != github.DefaultUserID ||
		provider.owner != "octo" || provider.name != "outside-catalog" {
		t.Fatalf("exact GitHub lookup = workspace %q user %q repo %q/%q", provider.workspaceID, provider.userID, provider.owner, provider.name)
	}
	if verified.ProviderRepoID != "octo/outside-catalog" || verified.DefaultBranch != "stable" ||
		verified.RemoteURL != "https://github.com/octo/outside-catalog.git" || !verified.TrustedProviderDescriptor {
		t.Fatalf("verified GitHub descriptor = %+v", verified)
	}
}

func TestRepositorySelectionResolverUsesConfiguredGitLabOrigin(t *testing.T) {
	const host = "https://gitlab.corp.example"
	provider := &repositorySelectionGitLabStub{client: gitlab.NewMockClient(host)}
	resolver := repositorySelectionResolver{gitlab: provider}
	verified, err := resolver.ResolveRepositorySelection(context.Background(), "workspace-a", taskservice.TaskRepositoryInput{
		Provider: "gitlab", RemoteURL: host + "/kandev/sample.git",
	})
	if err != nil {
		t.Fatalf("ResolveRepositorySelection: %v", err)
	}
	if provider.workspaceID != "workspace-a" || provider.host != host {
		t.Fatalf("GitLab client lookup = workspace %q host %q", provider.workspaceID, provider.host)
	}
	if verified.ProviderHost != host || verified.ProviderScope != "" || verified.ProviderRepoID != "1" ||
		verified.ProviderOwner != "kandev" || verified.ProviderName != "sample" ||
		verified.DefaultBranch != "main" || verified.RemoteURL != host+"/kandev/sample.git" {
		t.Fatalf("verified GitLab descriptor = %+v", verified)
	}
}

func TestRepositorySelectionResolverRejectsGitLabOriginMismatch(t *testing.T) {
	provider := &repositorySelectionGitLabStub{client: gitlab.NewMockClient("https://gitlab.corp.example")}
	resolver := repositorySelectionResolver{gitlab: provider}
	_, err := resolver.ResolveRepositorySelection(context.Background(), "workspace-a", taskservice.TaskRepositoryInput{
		Provider: "gitlab", ProviderHost: "https://gitlab.corp.example",
		RemoteURL: "https://gitlab.attacker.example/kandev/sample.git",
	})
	assertRepositorySelectionAdapterError(t, err, taskservice.RepositorySelectionErrorInvalid)
}

func TestRepositorySelectionResolverCanonicalizesAzureHTTPSAndSSH(t *testing.T) {
	provider := &repositorySelectionAzureStub{
		projects: []azuredevops.Project{{ID: "project-17", Name: "Platform"}},
		repositories: []azuredevops.Repository{{
			ID: "repo-22", Name: "runtime", ProjectID: "project-17", ProjectName: "Platform",
			WebURL: "https://dev.azure.com/acme/Platform/_git/runtime", DefaultBranch: "refs/heads/release",
		}},
	}
	resolver := repositorySelectionResolver{azureDevOps: provider}
	urls := []string{
		"https://dev.azure.com/acme/Platform/_git/runtime",
		"git@ssh.dev.azure.com:v3/acme/Platform/runtime",
	}
	for _, remoteURL := range urls {
		t.Run(remoteURL, func(t *testing.T) {
			verified, err := resolver.ResolveRepositorySelection(context.Background(), "workspace-a", taskservice.TaskRepositoryInput{
				Provider: "azure_devops", RemoteURL: remoteURL,
			})
			if err != nil {
				t.Fatalf("ResolveRepositorySelection: %v", err)
			}
			if verified.RemoteURL != "https://dev.azure.com/acme/Platform/_git/runtime" ||
				verified.ProviderHost != "https://dev.azure.com" || verified.ProviderScope != "project-17" ||
				verified.ProviderRepoID != "repo-22" || verified.DefaultBranch != "release" {
				t.Fatalf("verified Azure descriptor = %+v", verified)
			}
		})
	}
	if provider.workspaceID != "workspace-a" || provider.projectID != "project-17" {
		t.Fatalf("Azure metadata lookups = workspace %q project %q", provider.workspaceID, provider.projectID)
	}
}

func TestRepositorySelectionResolverRejectsAzureScopeMismatch(t *testing.T) {
	provider := &repositorySelectionAzureStub{
		projects: []azuredevops.Project{{ID: "project-17", Name: "Platform"}},
		repositories: []azuredevops.Repository{{
			ID: "repo-22", Name: "runtime", ProjectID: "project-17",
			WebURL: "https://dev.azure.com/acme/Platform/_git/runtime", DefaultBranch: "refs/heads/release",
		}},
	}
	resolver := repositorySelectionResolver{azureDevOps: provider}
	_, err := resolver.ResolveRepositorySelection(context.Background(), "workspace-a", taskservice.TaskRepositoryInput{
		Provider: "azure_devops", ProviderScope: "different-project",
		RemoteURL: "https://dev.azure.com/acme/Platform/_git/runtime",
	})
	assertRepositorySelectionAdapterError(t, err, taskservice.RepositorySelectionErrorNotFound)
}

func assertRepositorySelectionAdapterError(
	t *testing.T, err error, want taskservice.RepositorySelectionErrorCode,
) {
	t.Helper()
	var selectionError *taskservice.RepositorySelectionError
	if !errors.As(err, &selectionError) || selectionError.Code != want {
		t.Fatalf("selection error = %v, want code %q", err, want)
	}
}
