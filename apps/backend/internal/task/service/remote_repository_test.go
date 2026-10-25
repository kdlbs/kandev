package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestParseRemoteRepositoryURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		raw       string
		provider  string
		owner     string
		repo      string
		canonical string
	}{
		{"github", "https://github.com/acme/api/pull/12", "github", "acme", "api", "https://github.com/acme/api.git"},
		{"gitlab subgroup", "https://gitlab.com/acme/platform/api.git", "gitlab", "acme/platform", "api", "https://gitlab.com/acme/platform/api.git"},
		{"gitlab merge request", "https://gitlab.com/acme/platform/api/-/merge_requests/12", "gitlab", "acme/platform", "api", "https://gitlab.com/acme/platform/api.git"},
		{"azure devops", "https://dev.azure.com/acme/Platform/_git/api", "azure_devops", "Platform", "api", "https://dev.azure.com/acme/Platform/_git/api"},
		{"github ssh", "git@github.com:acme/api.git", "github", "acme", "api", "git@github.com:acme/api.git"},
		{"azure ssh", "git@ssh.dev.azure.com:v3/acme/Platform/api", "azure_devops", "Platform", "api", "git@ssh.dev.azure.com:v3/acme/Platform/api"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider, owner, repo, canonical, err := parseRemoteRepositoryURL(tc.raw, "")
			if err != nil {
				t.Fatal(err)
			}
			if provider != tc.provider || owner != tc.owner || repo != tc.repo || canonical != tc.canonical {
				t.Fatalf("got (%q, %q, %q, %q)", provider, owner, repo, canonical)
			}
		})
	}
}

func TestParseRemoteRepositoryURLRejectsUnsupportedHost(t *testing.T) {
	t.Parallel()
	if _, _, _, _, err := parseRemoteRepositoryURL("https://example.com/acme/api", ""); err == nil {
		t.Fatal("expected unsupported host error")
	}
}

func TestParseRemoteRepositoryURLAcceptsProviderHintForSelfManagedGitLab(t *testing.T) {
	t.Parallel()

	provider, owner, repo, canonical, err := parseRemoteRepositoryURL(
		"https://gitlab.internal:8443/acme/platform/api",
		"gitlab",
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider != "gitlab" || owner != "acme/platform" || repo != "api" ||
		canonical != "https://gitlab.internal:8443/acme/platform/api.git" {
		t.Fatalf("got (%q, %q, %q, %q)", provider, owner, repo, canonical)
	}
	if host := remoteProviderHost(provider, canonical); host != "https://gitlab.internal:8443" {
		t.Fatalf("provider host = %q", host)
	}
}

func TestResolveRepositoryRef_TrustedDescriptorPreservesCustomProviderCloneURL(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	repositoryID, baseBranch, created, err := svc.ResolveRepositoryRef(ctx, "ws-1", TaskRepositoryInput{
		RemoteURL:                 "https://forge.example.test/context/scm/TEAM/widgets.git",
		Provider:                  "custom-provider",
		ProviderHost:              "https://forge.example.test/context",
		ProviderScope:             "forge-instance-a",
		ProviderRepoID:            "repo-99",
		ProviderOwner:             "TEAM",
		ProviderName:              "widgets",
		DefaultBranch:             "main",
		BaseBranch:                "release/1.0",
		TrustedProviderDescriptor: true,
	})
	if err != nil {
		t.Fatalf("ResolveRepositoryRef() unexpected error: %v", err)
	}
	if !created || baseBranch != "release/1.0" {
		t.Fatalf("ResolveRepositoryRef() created=%v base=%q, want true/release/1.0", created, baseBranch)
	}
	stored, err := repo.GetRepository(ctx, repositoryID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.Provider != "custom-provider" || stored.ProviderHost != "https://forge.example.test" || stored.ProviderScope != "forge-instance-a" ||
		stored.RemoteURL != "https://forge.example.test/context/scm/TEAM/widgets.git" {
		t.Fatalf("stored repository = %+v, want custom descriptor with exact clone URL", stored)
	}
}

func TestResolveRepositoryRef_TrustedDescriptorSeparatesProviderScopesAndRejectsLegacyAdoption(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "legacy", WorkspaceID: "ws-1", Name: "TEAM/widgets", SourceType: sourceTypeProvider,
		Provider: "bitbucket", ProviderHost: "https://forge.example.test", ProviderRepoID: "42",
		ProviderOwner: "TEAM", ProviderName: "widgets",
	}); err != nil {
		t.Fatal(err)
	}

	resolve := func(scope, cloneURL string) string {
		t.Helper()
		id, _, _, err := svc.ResolveRepositoryRef(ctx, "ws-1", TaskRepositoryInput{
			RemoteURL: cloneURL, Provider: "bitbucket", ProviderHost: "https://forge.example.test",
			ProviderScope: scope, ProviderRepoID: "42", ProviderOwner: "TEAM", ProviderName: "widgets",
			DefaultBranch: "main", TrustedProviderDescriptor: true,
		})
		if err != nil {
			t.Fatalf("ResolveRepositoryRef(%q): %v", scope, err)
		}
		return id
	}

	first := resolve("https://forge.example.test/dc-a", "https://forge.example.test/dc-a/scm/TEAM/widgets.git")
	second := resolve("https://forge.example.test/dc-b", "https://forge.example.test/dc-b/scm/TEAM/widgets.git")
	if first == "legacy" || second == "legacy" || first == second {
		t.Fatalf("provider scopes reused unsafe rows: legacy=%q first=%q second=%q", "legacy", first, second)
	}
}

func TestResolveRepositoryRef_TrustedDescriptorRejectsCredentialsAndIncompleteIdentity(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	_, _, _, err := svc.ResolveRepositoryRef(ctx, "ws-1", TaskRepositoryInput{
		RemoteURL: "https://token@forge.example.test/context/scm/TEAM/widgets.git",
		Provider:  "custom-provider", ProviderHost: "https://forge.example.test", ProviderScope: "forge-instance-a", ProviderOwner: "TEAM", ProviderName: "widgets",
		TrustedProviderDescriptor: true,
	})
	if err == nil {
		t.Fatal("ResolveRepositoryRef() accepted credential-bearing/incomplete descriptor")
	}
}

func TestResolveRepositoryRef_BrowserPluginDescriptorKeepsBuiltinHostRejection(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	_, _, _, err := svc.ResolveRepositoryRef(ctx, "ws-1", TaskRepositoryInput{
		RemoteURL:      "https://forge.example.test/acme/widgets.git",
		Provider:       "forge",
		ProviderHost:   "https://forge.example.test",
		ProviderRepoID: "42",
		ProviderOwner:  "acme",
		ProviderName:   "widgets",
	})
	if err == nil {
		t.Fatal("ResolveRepositoryRef() accepted a browser-supplied plugin descriptor")
	}
}

func TestResolveRepositoryRef_TrustedDescriptorRejectsForeignCloneOrigin(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	// An owned provider must not carry a clone URL off its own origin: the
	// clone would present that provider's managed credentials to another host.
	_, _, _, err := svc.ResolveRepositoryRef(ctx, "ws-1", TaskRepositoryInput{
		RemoteURL:                 "https://attacker.example.test/acme/widgets.git",
		Provider:                  "bitbucket",
		ProviderHost:              "https://bitbucket.org",
		ProviderRepoID:            "{7c445c85}",
		ProviderOwner:             "acme",
		ProviderName:              "widgets",
		TrustedProviderDescriptor: true,
	})
	if err == nil {
		t.Fatal("ResolveRepositoryRef() accepted a trusted descriptor outside the provider origin")
	}
}

func TestResolveRepositoryRef_BuiltinProviderKeepsCoreParsePath(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	repositoryID, _, _, err := svc.ResolveRepositoryRef(ctx, "ws-1", TaskRepositoryInput{
		RemoteURL: "https://github.com/acme/api", Provider: "github",
	})
	if err != nil {
		t.Fatalf("ResolveRepositoryRef() unexpected error: %v", err)
	}
	stored, err := repo.GetRepository(ctx, repositoryID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.RemoteURL != "https://github.com/acme/api.git" {
		t.Fatalf("stored remote_url = %q, want the parser-canonicalized URL", stored.RemoteURL)
	}
}
