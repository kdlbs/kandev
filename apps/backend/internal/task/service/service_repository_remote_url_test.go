package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"fmt"
	"os"
	"os/exec"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func seedRemoteURLRepository(t *testing.T) (*Service, *MockEventBus, *sqliterepo.Repository, *models.Repository) {
	t.Helper()
	svc, bus, repo := createTestService(t)
	return svc, bus, repo, seedRemoteURLRepositoryIn(t, repo)
}

func seedRemoteURLRepositoryIn(t *testing.T, repo *sqliterepo.Repository) *models.Repository {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "remote-url-ws", Name: "Workspace"}))
	row := &models.Repository{
		ID: "remote-url-repo", WorkspaceID: "remote-url-ws", Name: "acme/widget",
		SourceType: "provider", Provider: providerGitHub, ProviderHost: "https://github.com",
		ProviderOwner: "acme", ProviderName: "widget",
		RemoteURL: "git@github.com:acme/widget.git", DefaultBranch: "main", PullBeforeWorktree: true,
	}
	require.NoError(t, repo.CreateRepository(ctx, row))
	return row
}

func TestRepositoryRemoteURLValidation(t *testing.T) {
	for _, test := range []struct {
		name      string
		proposed  string
		effective string
	}{
		{name: "https canonicalizes", proposed: "https://github.com/acme/widget", effective: "https://github.com/acme/widget.git"},
		{name: "https with git suffix", proposed: "https://github.com/acme/widget.git", effective: "https://github.com/acme/widget.git"},
		{name: "conventional scp is retained", proposed: "git@github.com:acme/widget.git", effective: "git@github.com:acme/widget.git"},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, bus, _, row := seedRemoteURLRepository(t)
			preview, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), row.ID, test.proposed)
			require.NoError(t, err)
			require.Equal(t, test.effective, preview.EffectiveRemoteURL)
			require.Equal(t, "locator_identity", preview.ValidationScope)
			require.Equal(t, row.RemoteURL, preview.CurrentRemoteURL)
			require.Empty(t, bus.GetPublishedEvents())
		})
	}

	for _, test := range []struct {
		name     string
		proposed string
	}{
		{name: "empty", proposed: ""},
		{name: "blank", proposed: "   "},
		{name: "different owner", proposed: "https://github.com/other/widget.git"},
		{name: "different repository", proposed: "https://github.com/acme/other.git"},
		{name: "different host", proposed: "https://github.example.com/acme/widget.git"},
		{name: "different port", proposed: "https://github.com:8443/acme/widget.git"},
		{name: "encoded different port", proposed: "https://github.com:8443/acme/w%69dget.git"},
		{name: "browser resource path", proposed: "https://github.com/acme/widget/pull/1"},
		{name: "gitlab style resource suffix", proposed: "https://github.com/acme/widget/-/issues/1"},
		{name: "embedded credentials", proposed: "https://token@github.com/acme/widget.git"},
		{name: "embedded password", proposed: "https://user:secret@github.com/acme/widget.git"},
		{name: "query", proposed: "https://github.com/acme/widget.git?ref=main"},
		{name: "empty query", proposed: "https://github.com/acme/widget.git?"},
		{name: "fragment", proposed: "https://github.com/acme/widget.git#main"},
		{name: "empty fragment", proposed: "https://github.com/acme/widget.git#"},
		{name: "encoded separator", proposed: "https://github.com/acme%2fwidget.git"},
		{name: "encoded backslash", proposed: "https://github.com/acme/widget%5cextra.git"},
		{name: "encoded control", proposed: "https://github.com/acme/widget%00extra.git"},
		{name: "encoded traversal", proposed: "https://github.com/acme/%2e%2e/widget.git"},
		{name: "backslash", proposed: `https://github.com/acme\widget.git`},
		{name: "local absolute path", proposed: "/srv/repositories/widget.git"},
		{name: "local file url", proposed: "file:///srv/repositories/widget.git"},
		{name: "relative path", proposed: "../widget.git"},
		{name: "ssh scp different host", proposed: "git@git.example.com:acme/widget.git"},
		{name: "ssh scp traversal", proposed: "git@github.com:acme/../widget.git"},
		{name: "ssh scp port", proposed: "git@github.com:2222:acme/widget.git"},
		{name: "ssh scp ipv6", proposed: "git@[2001:db8::1]:acme/widget.git"},
		{name: "unsupported scheme", proposed: "ftp://github.com/acme/widget.git"},
		{name: "ssh url user info", proposed: "ssh://git@github.com/acme/widget.git"},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, _, _, row := seedRemoteURLRepository(t)
			preview, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), row.ID, test.proposed)
			require.Error(t, err)
			require.Nil(t, preview)
			require.NotContains(t, err.Error(), "secret")
			require.NotContains(t, err.Error(), "token@")
		})
	}
}

func TestRepositoryRemoteURLEncodedAzurePath(t *testing.T) {
	svc, bus, repo, _ := seedRemoteURLRepository(t)
	azure := &models.Repository{
		ID: "azure-encoded", WorkspaceID: "remote-url-ws", Name: "project name/widget", SourceType: "provider",
		Provider: providerAzureDevOps, ProviderHost: "https://dev.azure.com",
		ProviderScope: "acme", ProviderRepoID: "1111", ProviderOwner: "project name", ProviderName: "widget",
		RemoteURL: "https://dev.azure.com/acme/project%20name/_git/widget",
	}
	require.NoError(t, repo.CreateRepository(t.Context(), azure))

	preview, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), azure.ID,
		"https://dev.azure.com/acme/project%20name/_git/widget")
	require.NoError(t, err)
	require.Equal(t, "https://dev.azure.com/acme/project%20name/_git/widget", preview.EffectiveRemoteURL)
	require.Equal(t, "locator_identity", preview.ValidationScope)

	updated, err := svc.UpdateRepository(t.Context(), azure.ID, &UpdateRepositoryRequest{
		RemoteURL: stringPointer("https://dev.azure.com/acme/project%20name/_git/widget"),
	})
	require.NoError(t, err)
	require.Equal(t, "https://dev.azure.com/acme/project%20name/_git/widget", updated.RemoteURL)
	events := bus.GetPublishedEvents()
	require.Len(t, events, 1)
	require.Equal(t, "https://dev.azure.com/acme/project%20name/_git/widget", events[0].Data.(map[string]interface{})["remote_url"])

	preview, err = svc.ValidateRepositoryRemoteURLUpdate(t.Context(), azure.ID,
		"https://dev.azure.com/acme/project%20name/_git/widget")
	require.NoError(t, err)
	require.Equal(t, "https://dev.azure.com/acme/project%20name/_git/widget", preview.CurrentRemoteURL)
	require.False(t, preview.CurrentRemoteURLRedacted)
}

func TestRepositoryRemoteURLPluginEncodedAndUnicodePath(t *testing.T) {
	svc, _, repo, _ := seedRemoteURLRepository(t)
	plugin := &models.Repository{
		ID: "plugin-encoded", WorkspaceID: "remote-url-ws", Name: "TEAM/fixture", SourceType: "provider",
		Provider: "fixture-source-control", ProviderHost: "https://bitbucket.example.test",
		ProviderScope: "workspace-a", ProviderRepoID: "repo-42", ProviderOwner: "TEAM", ProviderName: "fixture",
		RemoteURL: "https://bitbucket.example.test/scm/TEAM/fixture.git",
	}
	require.NoError(t, repo.CreateRepository(t.Context(), plugin))
	exactCloneURL := "https://bitbucket.example.test/scm/TEAM/fixture%20%C3%A9.git"
	svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{resolve: func(input TaskRepositoryInput) (TaskRepositoryInput, error) {
		return TaskRepositoryInput{
			RemoteURL: exactCloneURL, Provider: "fixture-source-control", ProviderHost: "https://bitbucket.example.test",
			ProviderScope: "workspace-a", ProviderRepoID: "repo-42", ProviderOwner: "TEAM", ProviderName: "fixture",
			DefaultBranch: "main", TrustedProviderDescriptor: true,
		}, nil
	}})

	preview, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), plugin.ID,
		"https://bitbucket.example.test/projects/TEAM/fixture%20%C3%A9")
	require.NoError(t, err)
	require.Equal(t, exactCloneURL, preview.EffectiveRemoteURL)
	require.Equal(t, "provider_inspection", preview.ValidationScope)
}

func TestRepositoryRemoteURLRejectsMissingOrConflictingSavedOrigin(t *testing.T) {
	t.Run("github missing saved host", func(t *testing.T) {
		svc, bus, repo, row := seedRemoteURLRepository(t)
		row.ProviderHost = ""
		row.RemoteURL = "https://ghe.example.test/acme/widget.git"
		require.NoError(t, repo.UpdateRepository(t.Context(), row))
		_, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), row.ID, "https://github.com/acme/widget.git")
		require.ErrorIs(t, err, ErrInvalidRepositorySettings)
		stored, err := repo.GetRepository(t.Context(), row.ID)
		require.NoError(t, err)
		require.Equal(t, "https://ghe.example.test/acme/widget.git", stored.RemoteURL)
		require.Empty(t, bus.GetPublishedEvents())
	})

	t.Run("azure missing saved host", func(t *testing.T) {
		svc, _, repo, _ := seedRemoteURLRepository(t)
		azure := &models.Repository{
			ID: "azure-missing-host", WorkspaceID: "remote-url-ws", Name: "project/widget", SourceType: "provider",
			Provider: providerAzureDevOps, ProviderScope: "acme", ProviderRepoID: "1111",
			ProviderOwner: "project", ProviderName: "widget",
			RemoteURL: "https://dev.azure.com/acme/project/_git/widget",
		}
		require.NoError(t, repo.CreateRepository(t.Context(), azure))
		_, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), azure.ID,
			"https://dev.azure.com/acme/project/_git/widget")
		require.ErrorIs(t, err, ErrInvalidRepositorySettings)
	})

	t.Run("github conflicting legacy origin", func(t *testing.T) {
		svc, bus, repo, row := seedRemoteURLRepository(t)
		row.ProviderHost = "https://ghe.example.test"
		row.RemoteURL = "https://github.com/acme/widget.git"
		require.NoError(t, repo.UpdateRepository(t.Context(), row))
		_, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), row.ID, "https://github.com/acme/widget.git")
		require.ErrorIs(t, err, ErrInvalidRepositorySettings)
		stored, err := repo.GetRepository(t.Context(), row.ID)
		require.NoError(t, err)
		require.Equal(t, "https://github.com/acme/widget.git", stored.RemoteURL)
		require.Empty(t, bus.GetPublishedEvents())
	})

	t.Run("azure conflicting legacy origin", func(t *testing.T) {
		svc, _, repo, _ := seedRemoteURLRepository(t)
		azure := &models.Repository{
			ID: "azure-conflict", WorkspaceID: "remote-url-ws", Name: "project/widget", SourceType: "provider",
			Provider: providerAzureDevOps, ProviderHost: "https://dev.azure.com",
			ProviderOwner: "project", ProviderName: "widget",
			RemoteURL: "https://other.example.test/acme/project/_git/widget",
		}
		require.NoError(t, repo.CreateRepository(t.Context(), azure))
		_, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), azure.ID,
			"https://dev.azure.com/acme/project/_git/widget")
		require.ErrorIs(t, err, ErrInvalidRepositorySettings)
	})
}

func TestRepositoryRemoteURLUpdatePreservesExactSavedIdentity(t *testing.T) {
	svc, bus, repo, row := seedRemoteURLRepository(t)
	proposed := "https://github.com/acme/widget.git"
	sourceType, provider, owner, name := " provider ", " github ", " acme ", " widget "
	updated, err := svc.UpdateRepository(t.Context(), row.ID, &UpdateRepositoryRequest{
		RemoteURL: &proposed, SourceType: &sourceType, Provider: &provider,
		ProviderOwner: &owner, ProviderName: &name,
	})
	require.NoError(t, err)
	require.Equal(t, proposed, updated.RemoteURL)
	stored, err := repo.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, "provider", stored.SourceType)
	require.Equal(t, "github", stored.Provider)
	require.Equal(t, "acme", stored.ProviderOwner)
	require.Equal(t, "widget", stored.ProviderName)
	require.Equal(t, "https://github.com", stored.ProviderHost)
	require.Len(t, bus.GetPublishedEvents(), 1)
}

func TestParseRepositoryRepairCandidateUnicodeAndEscapedPaths(t *testing.T) {
	for _, raw := range []string{
		"https://git.example.test/team/caf%C3%A9.git",
		"https://git.example.test/team/café.git",
		"https://git.example.test/team/project%20name/_git/widget",
	} {
		candidate, err := parseRepositoryRepairCandidate(raw)
		require.NoError(t, err, raw)
		require.Equal(t, raw, candidate.raw)
	}
	for _, raw := range []string{
		"https://git.example.test/team/a%2fb.git",
		"https://git.example.test/team/a%5cb.git",
		"https://git.example.test/team/a%00b.git",
		"https://git.example.test/team/%2e%2e/b.git",
		"https://git.example.test/team/a%09b.git",
	} {
		_, err := parseRepositoryRepairCandidate(raw)
		require.Error(t, err, raw)
	}
}

func localGitDirectory(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	command := exec.Command("git", "init", "--initial-branch=main", directory)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	return directory
}

func TestRepositoryRemoteURLAllowsDeliberateFullIdentityUpdates(t *testing.T) {
	t.Run("local source conversion", func(t *testing.T) {
		svc, _, repo, _ := seedRemoteURLRepository(t)
		local := &models.Repository{
			ID: "local-repo", WorkspaceID: "remote-url-ws", Name: "local", SourceType: sourceTypeLocal,
			LocalPath: "/tmp/old-source", DefaultBranch: "main",
		}
		require.NoError(t, repo.CreateRepository(t.Context(), local))
		sourceType, localPath := sourceTypeLocal, localGitDirectory(t)
		remoteURL := "https://github.com/e2e/widget.git"
		updated, err := svc.UpdateRepository(t.Context(), local.ID, &UpdateRepositoryRequest{
			SourceType: &sourceType, LocalPath: &localPath, RemoteURL: &remoteURL,
		})
		require.NoError(t, err)
		require.Equal(t, sourceTypeLocal, updated.SourceType)
		require.Equal(t, remoteURL, updated.RemoteURL)
	})

	t.Run("local source full DTO round trip", func(t *testing.T) {
		svc, _, repo, _ := seedRemoteURLRepository(t)
		localPath := localGitDirectory(t)
		local := &models.Repository{
			ID: "local-roundtrip", WorkspaceID: "remote-url-ws", Name: "local", SourceType: sourceTypeLocal,
			LocalPath: localPath, RemoteURL: "https://github.com/e2e/widget.git", DefaultBranch: "main",
		}
		require.NoError(t, repo.CreateRepository(t.Context(), local))
		sourceType, remoteURL := sourceTypeLocal, local.RemoteURL
		updated, err := svc.UpdateRepository(t.Context(), local.ID, &UpdateRepositoryRequest{
			SourceType: &sourceType, LocalPath: &localPath, RemoteURL: &remoteURL,
		})
		require.NoError(t, err)
		require.Equal(t, remoteURL, updated.RemoteURL)
		require.Equal(t, sourceTypeLocal, updated.SourceType)
	})

	t.Run("provider identity switch", func(t *testing.T) {
		svc, bus, repo, row := seedRemoteURLRepository(t)
		sourceType, localPath := "provider", localGitDirectory(t)
		provider, host, owner, name := "github", "https://github.com", "e2e", "relocated"
		remoteURL := "https://github.com/e2e/relocated.git"
		updated, err := svc.UpdateRepository(t.Context(), row.ID, &UpdateRepositoryRequest{
			SourceType: &sourceType, LocalPath: &localPath, Provider: &provider,
			ProviderHost: &host, ProviderOwner: &owner, ProviderName: &name, RemoteURL: &remoteURL,
		})
		require.NoError(t, err)
		require.Equal(t, remoteURL, updated.RemoteURL)
		stored, err := repo.GetRepository(t.Context(), row.ID)
		require.NoError(t, err)
		require.Equal(t, "github", stored.Provider)
		require.Equal(t, "e2e", stored.ProviderOwner)
		require.Equal(t, "relocated", stored.ProviderName)
		require.Equal(t, remoteURL, stored.RemoteURL)
		require.Len(t, bus.GetPublishedEvents(), 1)
	})
}

func TestRepositoryRemoteURLPreviewIsWriteFree(t *testing.T) {
	svc, bus, repo, row := seedRemoteURLRepository(t)
	before, err := repo.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	eventsBefore := len(bus.GetPublishedEvents())

	preview, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), row.ID, "https://github.com/acme/widget.git")
	require.NoError(t, err)
	require.True(t, preview.Changed)
	require.Equal(t, row.ID, preview.RepositoryID)
	require.Equal(t, row.WorkspaceID, preview.WorkspaceID)
	require.Equal(t, row.Provider, preview.Provider)
	require.Equal(t, row.ProviderHost, preview.ProviderHost)
	require.Equal(t, row.ProviderOwner, preview.ProviderOwner)
	require.Equal(t, row.ProviderName, preview.ProviderName)

	stored, err := repo.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, before.RemoteURL, stored.RemoteURL)
	require.Equal(t, before.UpdatedAt, stored.UpdatedAt)
	require.Len(t, bus.GetPublishedEvents(), eventsBefore)
}

func TestRepositoryRemoteURLPreviewRedactsUnsafeSavedLocator(t *testing.T) {
	svc, _, repo, row := seedRemoteURLRepository(t)
	row.RemoteURL = "https://secret-token@github.com/acme/widget.git"
	require.NoError(t, repo.UpdateRepository(t.Context(), row))

	preview, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), row.ID, "https://github.com/acme/widget.git")
	require.NoError(t, err)
	require.Empty(t, preview.CurrentRemoteURL)
	require.True(t, preview.CurrentRemoteURLRedacted)
	require.NotContains(t, preview.CurrentRemoteURL, "secret-token")
}

func TestRepositoryRemoteURLUpdatePersistsValidatedReplacement(t *testing.T) {
	svc, bus, repo, row := seedRemoteURLRepository(t)
	proposed := "https://github.com/acme/widget.git"
	updated, err := svc.UpdateRepository(t.Context(), row.ID, &UpdateRepositoryRequest{RemoteURL: &proposed})
	require.NoError(t, err)
	require.Equal(t, proposed, updated.RemoteURL)
	require.Equal(t, row.ID, updated.ID)
	require.Equal(t, row.WorkspaceID, updated.WorkspaceID)
	require.Equal(t, row.Provider, updated.Provider)
	require.Equal(t, row.ProviderHost, updated.ProviderHost)
	require.Equal(t, row.ProviderOwner, updated.ProviderOwner)
	require.Equal(t, row.ProviderName, updated.ProviderName)

	stored, err := repo.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, proposed, stored.RemoteURL)
	require.Equal(t, row.ProviderOwner, stored.ProviderOwner)
	require.Equal(t, row.ProviderName, stored.ProviderName)

	events := bus.GetPublishedEvents()
	require.Len(t, events, 1)
	require.Equal(t, "repository.updated", events[0].Type)
	data := events[0].Data.(map[string]interface{})
	require.Equal(t, proposed, data["remote_url"])
}

func TestRepositoryRemoteURLUpdateRejectsChangedIdentityFields(t *testing.T) {
	svc, _, repo, row := seedRemoteURLRepository(t)
	before, err := repo.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	proposed := "https://github.com/acme/widget.git"
	otherOwner := "other"
	_, err = svc.UpdateRepository(t.Context(), row.ID, &UpdateRepositoryRequest{RemoteURL: &proposed, ProviderOwner: &otherOwner})
	require.Error(t, err)
	stored, err := repo.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, before, stored)
}

func TestRepositoryRemoteURLUpdatePreservesOmittedURL(t *testing.T) {
	svc, bus, repo, row := seedRemoteURLRepository(t)
	repair := "https://github.com/acme/widget.git"
	_, err := svc.UpdateRepository(t.Context(), row.ID, &UpdateRepositoryRequest{RemoteURL: &repair})
	require.NoError(t, err)
	eventsBefore := len(bus.GetPublishedEvents())

	name := "renamed"
	updated, err := svc.UpdateRepository(t.Context(), row.ID, &UpdateRepositoryRequest{Name: &name})
	require.NoError(t, err)
	require.Equal(t, repair, updated.RemoteURL)
	stored, err := repo.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, repair, stored.RemoteURL)
	require.Equal(t, name, stored.Name)
	require.Len(t, bus.GetPublishedEvents(), eventsBefore+1)
}

func TestRepositoryRemoteURLUpdatePreservesURLForOrdinarySave(t *testing.T) {
	svc, _, repo, row := seedRemoteURLRepository(t)
	repair := "https://github.com/acme/widget.git"
	_, err := svc.UpdateRepository(t.Context(), row.ID, &UpdateRepositoryRequest{RemoteURL: &repair})
	require.NoError(t, err)

	stale, err := repo.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	defaultBranch, pull := stale.DefaultBranch, stale.PullBeforeWorktree
	stale.Name = "stale"
	stale.RemoteURL = row.RemoteURL
	require.NoError(t, repo.UpdateRepositoryWithCheckoutIntent(t.Context(), stale, models.RepositoryCheckoutIntent{
		DefaultBranch: &defaultBranch, PullBeforeWorktree: &pull,
	}))
	stored, err := repo.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, repair, stored.RemoteURL)
	require.Equal(t, "stale", stored.Name)
}

func TestRepositoryRemoteURLAccess(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		svc, _, _ := createTestService(t)
		_, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), "missing", "https://github.com/acme/widget.git")
		require.ErrorIs(t, err, repoerrors.ErrRepositoryNotFound)
	})

	t.Run("foreign workspace is not revealed", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		require.NoError(t, repo.CreateWorkspace(t.Context(), &models.Workspace{ID: "foreign-ws", Name: "B", OwnerID: "user-b"}))
		require.NoError(t, repo.CreateRepository(t.Context(), &models.Repository{
			ID: "foreign-repo", WorkspaceID: "foreign-ws", Name: "B/widget", Provider: providerGitHub,
			ProviderHost: "https://github.com", ProviderOwner: "B", ProviderName: "widget",
			RemoteURL: "git@github.com:B/widget.git",
		}))
		_, err := svc.ValidateRepositoryRemoteURLUpdate(ctxAs("user-a"), "foreign-repo", "https://github.com/B/widget.git")
		require.ErrorIs(t, err, repoerrors.ErrRepositoryNotFound)
	})

	t.Run("read only workspace is refused", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		require.NoError(t, repo.CreateWorkspace(t.Context(), &models.Workspace{ID: "read-only-ws", Name: "Improve Kandev"}))
		require.NoError(t, repo.CreateRepository(t.Context(), &models.Repository{
			ID: "read-only-repo", WorkspaceID: "read-only-ws", Name: "kandev/widget", Provider: providerGitHub,
			ProviderHost: "https://github.com", ProviderOwner: "kandev", ProviderName: "widget",
			RemoteURL: "git@github.com:kandev/widget.git",
		}))
		_, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), "read-only-repo", "https://github.com/kandev/widget.git")
		require.ErrorIs(t, err, ErrRepositoryReadOnly)
		proposed := "https://github.com/kandev/widget.git"
		_, err = svc.UpdateRepository(t.Context(), "read-only-repo", &UpdateRepositoryRequest{RemoteURL: &proposed})
		require.ErrorIs(t, err, ErrRepositoryReadOnly)
	})
}

func TestRepositoryRemoteURLPluginInspection(t *testing.T) {
	svc, _, repo, _ := seedRemoteURLRepository(t)
	plugin := &models.Repository{
		ID: "plugin-repo", WorkspaceID: "remote-url-ws", Name: "TEAM/fixture", SourceType: "provider",
		Provider: "fixture-source-control", ProviderHost: "https://bitbucket.example.test",
		ProviderScope: "workspace-a", ProviderRepoID: "repo-42", ProviderOwner: "TEAM", ProviderName: "fixture",
		RemoteURL: "https://bitbucket.example.test/scm/TEAM/fixture.git",
	}
	require.NoError(t, repo.CreateRepository(t.Context(), plugin))
	resolver := &repositorySelectionResolverStub{resolve: authoritativeRepositoryInput}
	svc.SetRepositorySelectionResolver(resolver)

	preview, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), plugin.ID, "https://bitbucket.example.test/projects/TEAM/fixture")
	require.NoError(t, err)
	require.Equal(t, "provider_inspection", preview.ValidationScope)
	require.Equal(t, "https://bitbucket.example.test/scm/TEAM/fixture.git", preview.EffectiveRemoteURL)
	require.Equal(t, "workspace-a", preview.ProviderScope)
	require.Equal(t, "repo-42", preview.ProviderRepoID)
	require.NotEmpty(t, resolver.calls)

	updated, err := svc.UpdateRepository(t.Context(), plugin.ID, &UpdateRepositoryRequest{
		RemoteURL: stringPointer("https://bitbucket.example.test/projects/TEAM/fixture"),
	})
	require.NoError(t, err)
	require.Equal(t, "https://bitbucket.example.test/scm/TEAM/fixture.git", updated.RemoteURL)
	require.Equal(t, "workspace-a", updated.ProviderScope)
	require.Equal(t, "repo-42", updated.ProviderRepoID)
}

func TestRepositoryRemoteURLPluginIdentityMismatchFailsClosed(t *testing.T) {
	svc, _, repo, _ := seedRemoteURLRepository(t)
	plugin := &models.Repository{
		ID: "plugin-repo", WorkspaceID: "remote-url-ws", Name: "TEAM/fixture", SourceType: "provider",
		Provider: "fixture-source-control", ProviderHost: "https://bitbucket.example.test",
		ProviderScope: "workspace-a", ProviderRepoID: "repo-42", ProviderOwner: "TEAM", ProviderName: "fixture",
		RemoteURL: "https://bitbucket.example.test/scm/TEAM/fixture.git",
	}
	require.NoError(t, repo.CreateRepository(t.Context(), plugin))
	svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{resolve: func(input TaskRepositoryInput) (TaskRepositoryInput, error) {
		resolved, _ := authoritativeRepositoryInput(input)
		resolved.ProviderOwner = "OTHER"
		return resolved, nil
	}})

	proposed := "https://bitbucket.example.test/projects/TEAM/fixture"
	_, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), plugin.ID, proposed)
	require.ErrorIs(t, err, ErrInvalidRepositorySettings)

	svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{resolve: func(TaskRepositoryInput) (TaskRepositoryInput, error) {
		return TaskRepositoryInput{}, NewRepositorySelectionError(RepositorySelectionErrorUnavailable, errors.New("provider down"))
	}})
	_, err = svc.ValidateRepositoryRemoteURLUpdate(t.Context(), plugin.ID, proposed)
	var selectionErr *RepositorySelectionError
	require.ErrorAs(t, err, &selectionErr)
	require.Equal(t, RepositorySelectionErrorUnavailable, selectionErr.Code)
	require.NotContains(t, err.Error(), "provider down")
}

func TestRepositoryRemoteURLPluginRequiresSavedImmutableIdentity(t *testing.T) {
	svc, _, repo, _ := seedRemoteURLRepository(t)
	plugin := &models.Repository{
		ID: "plugin-repo", WorkspaceID: "remote-url-ws", Name: "TEAM/fixture", SourceType: "provider",
		Provider: "fixture-source-control", ProviderHost: "https://bitbucket.example.test",
		ProviderScope: "workspace-a", ProviderOwner: "TEAM", ProviderName: "fixture",
		RemoteURL: "https://bitbucket.example.test/scm/TEAM/fixture.git",
	}
	require.NoError(t, repo.CreateRepository(t.Context(), plugin))
	resolver := &repositorySelectionResolverStub{resolve: authoritativeRepositoryInput}
	svc.SetRepositorySelectionResolver(resolver)

	_, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), plugin.ID, "https://bitbucket.example.test/projects/TEAM/fixture")
	require.ErrorIs(t, err, ErrInvalidRepositorySettings)
	require.Empty(t, resolver.calls)
}

func TestRepositoryRemoteURLGitLabSelfManagedOrigin(t *testing.T) {
	svc, _, repo, _ := seedRemoteURLRepository(t)
	gitlab := &models.Repository{
		ID: "gitlab-repo", WorkspaceID: "remote-url-ws", Name: "group/project", SourceType: "provider",
		Provider: providerGitLab, ProviderHost: "https://gitlab.internal",
		ProviderOwner: "group/subgroup", ProviderName: "project",
		RemoteURL: "https://gitlab.internal/group/subgroup/project.git",
	}
	require.NoError(t, repo.CreateRepository(t.Context(), gitlab))

	_, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), gitlab.ID, "git@gitlab.internal:group/subgroup/project.git")
	require.Error(t, err)

	svc.SetGitLabRepositoryOriginVerifier(&exactGitLabOriginVerifier{origin: "https://gitlab.internal"})
	preview, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), gitlab.ID, "git@gitlab.internal:group/subgroup/project.git")
	require.NoError(t, err)
	require.Equal(t, "git@gitlab.internal:group/subgroup/project.git", preview.EffectiveRemoteURL)

	_, err = svc.ValidateRepositoryRemoteURLUpdate(t.Context(), gitlab.ID, "https://gitlab.internal/group/other/project.git")
	require.ErrorIs(t, err, ErrInvalidRepositorySettings)
}

func TestRepositoryRemoteURLAzureRequiresOrganizationEvidence(t *testing.T) {
	svc, _, repo, _ := seedRemoteURLRepository(t)
	azure := &models.Repository{
		ID: "azure-repo", WorkspaceID: "remote-url-ws", Name: "project/widget", SourceType: "provider",
		Provider: providerAzureDevOps, ProviderHost: "https://dev.azure.com",
		ProviderScope: "acme", ProviderRepoID: "1111",
		ProviderOwner: "project", ProviderName: "widget",
		RemoteURL: "https://dev.azure.com/acme/project/_git/widget",
	}
	require.NoError(t, repo.CreateRepository(t.Context(), azure))

	preview, err := svc.ValidateRepositoryRemoteURLUpdate(t.Context(), azure.ID, "https://dev.azure.com/acme/project/_git/widget")
	require.NoError(t, err)
	require.Equal(t, "https://dev.azure.com/acme/project/_git/widget", preview.EffectiveRemoteURL)

	_, err = svc.ValidateRepositoryRemoteURLUpdate(t.Context(), azure.ID, "https://dev.azure.com/other/project/_git/widget")
	require.ErrorIs(t, err, ErrInvalidRepositorySettings)

	azure.ProviderScope = ""
	azure.RemoteURL = ""
	require.NoError(t, repo.UpdateRepository(t.Context(), azure))
	_, err = svc.ValidateRepositoryRemoteURLUpdate(t.Context(), azure.ID, "https://dev.azure.com/acme/project/_git/widget")
	require.ErrorIs(t, err, ErrInvalidRepositorySettings)
}

func TestRepositoryRemoteURLConcurrentSQLite(t *testing.T) {
	for _, held := range []int{0, 1} {
		t.Run(fmt.Sprintf("ordinary_save_and_repair/held_%d", held), func(t *testing.T) {
			fixtures := remoteURLConcurrentFixture(t)
			repair := "https://github.com/acme/widget.git"
			requests := [2]*UpdateRepositoryRequest{
				{Name: checkoutPtr("Saved")},
				{RemoteURL: &repair},
			}
			result := checkoutOverlap(t, fixtures, held, requests)
			require.NoError(t, result[0].err)
			require.NoError(t, result[1].err)
			stored, err := fixtures[0].gate.Repository.GetRepository(t.Context(), "checkout-repo")
			require.NoError(t, err)
			require.Equal(t, repair, stored.RemoteURL)
			if held == 0 {
				require.Equal(t, "Saved", stored.Name)
			}
			require.Equal(t, repair, result[held].row.RemoteURL)
			remoteURLEvent(t, fixtures[0], result[0].row.RemoteURL)
			remoteURLEvent(t, fixtures[1], result[1].row.RemoteURL)
		})
	}

	for _, held := range []int{0, 1} {
		t.Run(fmt.Sprintf("two_valid_repairs_follow_commit_order/held_%d", held), func(t *testing.T) {
			fixtures := remoteURLConcurrentFixture(t)
			first := "https://github.com/acme/widget.git"
			second := "git@github.com:acme/widget.git"
			requests := [2]*UpdateRepositoryRequest{{RemoteURL: &first}, {RemoteURL: &second}}
			result := checkoutOverlap(t, fixtures, held, requests)
			require.NoError(t, result[0].err)
			require.NoError(t, result[1].err)
			want := *requests[held].RemoteURL
			stored, err := fixtures[0].gate.Repository.GetRepository(t.Context(), "checkout-repo")
			require.NoError(t, err)
			require.Equal(t, want, stored.RemoteURL)
			require.Equal(t, want, result[held].row.RemoteURL)
			remoteURLEvent(t, fixtures[0], result[0].row.RemoteURL)
			remoteURLEvent(t, fixtures[1], result[1].row.RemoteURL)
		})
	}

	t.Run("identity_change_during_inspection_conflicts", func(t *testing.T) {
		fixtures := remoteURLConcurrentFixture(t)
		gate := fixtures[0].gate
		gate.armed.Store(true)
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			repair := "https://github.com/acme/widget.git"
			_, err := fixtures[0].api.UpdateRepository(ctx, "checkout-repo", &UpdateRepositoryRequest{RemoteURL: &repair})
			done <- err
		}()
		select {
		case <-gate.observed:
		case <-ctx.Done():
			t.Fatal("repair snapshot read not observed")
		}
		stored, err := fixtures[0].gate.Repository.GetRepository(ctx, "checkout-repo")
		require.NoError(t, err)
		stored.ProviderOwner = "other"
		require.NoError(t, fixtures[0].gate.UpdateRepository(ctx, stored))
		close(gate.release)
		select {
		case err := <-done:
			require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
		case <-ctx.Done():
			t.Fatal("repair did not finish after identity change")
		}
		stored, err = fixtures[0].gate.Repository.GetRepository(t.Context(), "checkout-repo")
		require.NoError(t, err)
		require.Equal(t, "other", stored.ProviderOwner)
		require.Equal(t, "git@github.com:acme/widget.git", stored.RemoteURL)
	})

	t.Run("binding_failure_rolls_back_url", func(t *testing.T) {
		fixtures := remoteURLConcurrentFixture(t)
		_, err := fixtures[0].db.Exec(`CREATE TRIGGER reject_remote_url_binding BEFORE INSERT ON repository_secret_bindings BEGIN SELECT RAISE(ABORT, 'binding rejected'); END`)
		require.NoError(t, err)
		repair := "https://github.com/acme/widget.git"
		bindings := []RepositorySecretBindingInput{{Key: "TOKEN", SecretID: "ref"}}
		_, err = fixtures[0].api.UpdateRepository(t.Context(), "checkout-repo", &UpdateRepositoryRequest{
			RemoteURL: &repair, SecretBindings: &bindings,
		})
		require.Error(t, err)
		stored, err := fixtures[0].gate.Repository.GetRepository(t.Context(), "checkout-repo")
		require.NoError(t, err)
		require.Equal(t, "git@github.com:acme/widget.git", stored.RemoteURL)
		require.Empty(t, fixtures[0].bus.GetPublishedEvents())
	})
}

func remoteURLConcurrentFixture(t *testing.T) [2]checkoutServiceFixture {
	t.Helper()
	fixtures := checkoutServices(t, true)
	row, err := fixtures[0].gate.Repository.GetRepository(t.Context(), "checkout-repo")
	require.NoError(t, err)
	row.Name = "acme/widget"
	row.SourceType = "provider"
	row.Provider = providerGitHub
	row.ProviderHost = "https://github.com"
	row.ProviderOwner = "acme"
	row.ProviderName = "widget"
	row.RemoteURL = "git@github.com:acme/widget.git"
	require.NoError(t, fixtures[0].gate.UpdateRepository(t.Context(), row))
	return fixtures
}

func remoteURLEvent(t *testing.T, fixture checkoutServiceFixture, want string) {
	t.Helper()
	events := fixture.bus.GetPublishedEvents()
	require.NotEmpty(t, events)
	found := false
	for _, event := range events {
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			continue
		}
		if data["remote_url"] == want {
			found = true
		}
	}
	require.True(t, found, "missing committed remote_url %q in %+v", want, events[0].Data)
}
