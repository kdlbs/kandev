package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/mentions"
	"github.com/kandev/kandev/internal/task/models"
)

type gitLabMentionRepositoryResolverStub struct {
	workspaceID  string
	repositories []*models.Repository
	err          error
}

func (r *gitLabMentionRepositoryResolverStub) ListRepositories(
	_ context.Context,
	workspaceID string,
) ([]*models.Repository, error) {
	r.workspaceID = workspaceID
	return r.repositories, r.err
}

func TestBuiltinGitLabMentionsSyncWorkspaceRepositoryScopeBeforeSearch(t *testing.T) {
	const host = "https://gitlab.example.test"
	pool := newGitLabServiceTestPool(t)
	store, err := gitlab.NewStore(pool.Writer(), pool.Reader())
	if err != nil {
		t.Fatalf("new GitLab store: %v", err)
	}
	client := gitlab.NewMockClient(host)
	client.SeedIssue("group/api", &gitlab.Issue{
		ID: 7001, IID: 7, ProjectID: 101, Title: "Fix auth",
		WebURL: host + "/group/api/-/issues/7",
	})
	client.SeedIssue("other/private", &gitlab.Issue{
		ID: 9001, IID: 9, ProjectID: 202, Title: "Other auth",
		WebURL: host + "/other/private/-/issues/9",
	})
	service := gitlab.NewService(host, client, "mock", nil, newTestLogger())
	service.SetStore(store)
	repositories := &gitLabMentionRepositoryResolverStub{repositories: []*models.Repository{
		{
			ID: "repo-gitlab", WorkspaceID: "workspace-1", Provider: "gitlab",
			ProviderRepoID: "101", ProviderHost: "HTTPS://GITLAB.EXAMPLE.TEST/",
			ProviderOwner: "group", ProviderName: "api",
			RemoteURL: host + "/group/api.git",
		},
		{
			ID: "repo-foreign-gitlab", WorkspaceID: "workspace-1", Provider: "gitlab",
			ProviderRepoID: "202", ProviderHost: "https://gitlab.other.test",
			ProviderOwner: "other", ProviderName: "private",
			RemoteURL: "https://gitlab.other.test/other/private.git",
		},
		{
			ID: "repo-github", WorkspaceID: "workspace-1", Provider: "github",
			ProviderRepoID: "other/private", ProviderOwner: "other", ProviderName: "private",
		},
	}}
	providers := builtinMentionProviders(&Services{GitLab: service}, repositories)
	var issueProvider mentions.MentionProvider
	for _, provider := range providers {
		if provider.Descriptor().Source == "gitlab_issues" {
			issueProvider = provider
			break
		}
	}
	if issueProvider == nil {
		t.Fatal("GitLab issue provider not registered")
	}

	candidates, err := issueProvider.Search(context.Background(), mentions.SearchRequest{
		WorkspaceID: "workspace-1", Query: "auth", Limit: 5,
	})
	if err != nil {
		t.Fatalf("search GitLab mentions: %v", err)
	}
	if repositories.workspaceID != "workspace-1" {
		t.Fatalf("repository workspace = %q", repositories.workspaceID)
	}
	if len(candidates) != 1 || candidates[0].ID != "7001" ||
		candidates[0].Scope != host+"|project:101" {
		t.Fatalf("candidates = %#v, want only workspace-bound project", candidates)
	}
	scope, err := service.MentionScopeForWorkspace(context.Background(), "workspace-1")
	if err != nil {
		t.Fatalf("load synchronized scope: %v", err)
	}
	if len(scope.Projects) != 1 || scope.Projects[0].ID != 101 || scope.Projects[0].Path != "group/api" {
		t.Fatalf("scope = %#v", scope)
	}

	repositories.repositories = []*models.Repository{{
		ID: "repo-unknown-gitlab", WorkspaceID: "workspace-1", Provider: "gitlab",
		ProviderRepoID: "101", ProviderOwner: "group", ProviderName: "api",
		RemoteURL: host + "/group/api.git",
	}}
	_, err = issueProvider.Search(context.Background(), mentions.SearchRequest{
		WorkspaceID: "workspace-1", Query: "auth", Limit: 5,
	})
	var providerErr *mentions.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Status != mentions.StatusUnsupportedScope {
		t.Fatalf("search after scope removal error = %v, want unsupported scope", err)
	}
}
