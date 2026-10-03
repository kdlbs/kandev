package service

import (
	"context"
	"errors"
	"testing"
)

func TestRegisterRemoteRepositoryPersistsPluginProviderIdentity(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createRepositorySelectionWorkspace(t, repo)
	resolver := &repositorySelectionResolverStub{resolve: authoritativeRepositoryInput}
	svc.SetRepositorySelectionResolver(resolver)

	repository, created, err := svc.RegisterRemoteRepository(ctx, &RegisterRemoteRepositoryRequest{
		WorkspaceID: "ws-1",
		RemoteURL:   "https://bitbucket.example.test/projects/TEAM/fixture",
		Provider:    "fixture-source-control", ProviderHost: "https://attacker.example.test",
		ProviderRepoID: "repo-42", ProviderOwner: "attacker", ProviderName: "wrong",
		DefaultBranch: "browser-default",
	})
	if err != nil {
		t.Fatalf("RegisterRemoteRepository: %v", err)
	}
	if !created || repository == nil {
		t.Fatalf("created = %v, repository = %+v, want a newly created repository", created, repository)
	}
	if len(resolver.calls) != 1 || resolver.calls[0].ProviderOwner != "attacker" {
		t.Fatalf("resolver calls = %+v, want one call with untrusted browser input", resolver.calls)
	}
	stored, err := repo.GetRepository(ctx, repository.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.Provider != "fixture-source-control" || stored.ProviderHost != "https://bitbucket.example.test" ||
		stored.ProviderRepoID != "repo-42" || stored.ProviderOwner != "TEAM" || stored.ProviderName != "fixture" ||
		stored.RemoteURL != "https://bitbucket.example.test/scm/TEAM/fixture.git" || stored.DefaultBranch != "main" {
		t.Fatalf("stored repository = %+v, want authoritative plugin identity", stored)
	}
}

func TestRegisterRemoteRepositoryReturnsExistingRepository(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createRepositorySelectionWorkspace(t, repo)
	svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{resolve: authoritativeRepositoryInput})
	request := &RegisterRemoteRepositoryRequest{
		WorkspaceID: "ws-1",
		RemoteURL:   "https://bitbucket.example.test/projects/TEAM/fixture",
		Provider:    "fixture-source-control",
	}

	first, created, err := svc.RegisterRemoteRepository(ctx, request)
	if err != nil || !created {
		t.Fatalf("first RegisterRemoteRepository: created=%v err=%v", created, err)
	}
	second, created, err := svc.RegisterRemoteRepository(ctx, request)
	if err != nil {
		t.Fatalf("second RegisterRemoteRepository: %v", err)
	}
	if created || second.ID != first.ID {
		t.Fatalf("second call created=%v id=%q, want existing repository %q", created, second.ID, first.ID)
	}
	repositories, err := repo.ListRepositories(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 1 {
		t.Fatalf("repositories = %d, want exactly one", len(repositories))
	}
}

func TestRegisterRemoteRepositoryResolutionFailureLeavesNoWrites(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createRepositorySelectionWorkspace(t, repo)
	svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{resolve: func(TaskRepositoryInput) (TaskRepositoryInput, error) {
		return TaskRepositoryInput{}, errors.New("upstream plugin response contains secret details")
	}})

	_, _, err := svc.RegisterRemoteRepository(ctx, &RegisterRemoteRepositoryRequest{
		WorkspaceID: "ws-1",
		RemoteURL:   "https://bitbucket.example.test/projects/TEAM/fixture",
		Provider:    "fixture-source-control",
	})
	assertRepositorySelectionError(t, err, RepositorySelectionErrorUnavailable, "secret")
	repositories, listErr := repo.ListRepositories(ctx, "ws-1")
	if listErr != nil {
		t.Fatalf("ListRepositories: %v", listErr)
	}
	if len(repositories) != 0 {
		t.Fatalf("repositories after failed resolution = %d, want zero", len(repositories))
	}
}

func TestRegisterRemoteRepositoryRejectsMissingLocator(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createRepositorySelectionWorkspace(t, repo)

	_, _, err := svc.RegisterRemoteRepository(ctx, &RegisterRemoteRepositoryRequest{WorkspaceID: "ws-1"})
	if !errors.Is(err, ErrInvalidRepositorySettings) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidRepositorySettings)
	}
}

func TestRegisterRemoteRepositoryRejectsUnsupportedHostWithoutProvider(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createRepositorySelectionWorkspace(t, repo)

	_, _, err := svc.RegisterRemoteRepository(ctx, &RegisterRemoteRepositoryRequest{
		WorkspaceID: "ws-1", RemoteURL: "https://git.example.test/acme/api.git",
	})
	assertRepositorySelectionError(t, err, RepositorySelectionErrorInvalid, "")
	repositories, listErr := repo.ListRepositories(ctx, "ws-1")
	if listErr != nil {
		t.Fatalf("ListRepositories: %v", listErr)
	}
	if len(repositories) != 0 {
		t.Fatalf("repositories after invalid locator = %d, want zero", len(repositories))
	}
}

func TestRegisterRemoteRepositoryRejectsMismatchedBuiltInHints(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createRepositorySelectionWorkspace(t, repo)
	for name, request := range map[string]*RegisterRemoteRepositoryRequest{
		"owner":    {WorkspaceID: "ws-1", RemoteURL: "https://github.com/acme/api", ProviderOwner: "someone-else"},
		"name":     {WorkspaceID: "ws-1", RemoteURL: "https://github.com/acme/api", ProviderName: "other"},
		"provider": {WorkspaceID: "ws-1", RemoteURL: "https://github.com/acme/api", Provider: "gitlab"},
		"host":     {WorkspaceID: "ws-1", RemoteURL: "https://github.com/acme/api", ProviderHost: "https://ghe.example.test"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := svc.RegisterRemoteRepository(ctx, request)
			assertRepositorySelectionError(t, err, RepositorySelectionErrorInvalid, "")
		})
	}
	repositories, err := repo.ListRepositories(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 0 {
		t.Fatalf("repositories after mismatched hints = %d, want zero", len(repositories))
	}
}

func TestRegisterRemoteRepositoryCreatesBuiltInGitHubRepository(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createRepositorySelectionWorkspace(t, repo)

	repository, created, err := svc.RegisterRemoteRepository(ctx, &RegisterRemoteRepositoryRequest{
		WorkspaceID: "ws-1", RemoteURL: "https://github.com/acme/api", DefaultBranch: "develop",
	})
	if err != nil {
		t.Fatalf("RegisterRemoteRepository: %v", err)
	}
	if !created {
		t.Fatal("created = false, want a new repository")
	}
	stored, err := repo.GetRepository(ctx, repository.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.Provider != providerGitHub || stored.ProviderOwner != "acme" || stored.ProviderName != "api" ||
		stored.RemoteURL != "https://github.com/acme/api.git" || stored.DefaultBranch != "develop" {
		t.Fatalf("stored repository = %+v, want GitHub identity for acme/api on develop", stored)
	}
}
