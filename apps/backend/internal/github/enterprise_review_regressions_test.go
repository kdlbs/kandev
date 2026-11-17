package github

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/githubhost"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseSearchResponsesCarryHost(t *testing.T) {
	for _, host := range []string{"github.com", "git.example.com", "acme.ghe.com"} {
		t.Run(host, func(t *testing.T) {
			service, _ := newWorkspaceConnectionService(t, "alice")
			client := repositoryVisibilityClient{Client: NewMockClient(), repos: map[string]GitHubRepo{"acme/demo": {Owner: "acme", Name: "demo"}}, prs: []*PR{{RepoOwner: "acme", RepoName: "demo", Number: 7}}, issues: []*Issue{{RepoOwner: "acme", RepoName: "demo", Number: 8}}}
			service.hostTokenClientFactory = func(string, githubhost.Host) Client { return client }
			service.resolver.SetAutomationProvider(enterpriseWatchCredentials{client: client})
			_, err := service.SetWorkspaceConnection(t.Context(), "ws-1", SetWorkspaceConnectionRequest{Source: ConnectionSourcePAT, Host: host, Token: "fixture"})
			require.NoError(t, err)
			all := RepoScopeModeAll
			_, err = service.UpdateWorkspaceSettings(withExpectedGitHubHost(t.Context(), host), &UpdateWorkspaceSettingsRequest{WorkspaceID: "ws-1", RepoScopeMode: &all})
			require.NoError(t, err)
			assertEnterpriseSearchResponseHosts(t, service, host)
			// Provider/cache entries are shared; response metadata must not mutate them.
			require.Empty(t, client.prs[0].GitHubHost)
			require.Empty(t, client.issues[0].GitHubHost)
		})
	}
}

func TestEnterpriseReconnectSettingsFailureRollsBackConnection(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "alice")
	store := service.store
	ctx := t.Context()
	connection := &WorkspaceConnection{WorkspaceID: "ws-1", Source: ConnectionSourcePAT, Login: "alice", GitHubHost: "git.example.com", Status: ConnectionStatusActive, CredentialGeneration: 1}
	require.NoError(t, store.ReplaceWorkspaceHostConnection(ctx, nil, connection))
	require.NoError(t, store.DeleteWorkspaceConnection(ctx, "ws-1"))
	_, err := store.db.Exec(`CREATE TRIGGER reject_host_update BEFORE UPDATE ON github_workspace_settings BEGIN SELECT RAISE(ABORT, 'fixture settings failure'); END`)
	require.NoError(t, err)
	connection.GitHubHost = "github.com"
	require.ErrorContains(t, store.ReplaceWorkspaceHostConnection(ctx, nil, connection), "fixture settings failure")
	stored, err := store.GetWorkspaceConnection(ctx, "ws-1")
	require.NoError(t, err)
	require.Nil(t, stored)
	settings, err := store.GetWorkspaceSettings(ctx, "ws-1")
	require.NoError(t, err)
	require.Equal(t, "git.example.com", settings.GitHubHost)
}

func assertEnterpriseSearchResponseHosts(t *testing.T, service *Service, host string) {
	t.Helper()
	for range 2 {
		prs, err := service.SearchUserPRsPagedForWorkspaceUser(t.Context(), "ws-1", "", "", "", 1, 25)
		require.NoError(t, err)
		require.Len(t, prs.PRs, 1)
		issues, err := service.SearchUserIssuesPagedForWorkspaceUser(t.Context(), "ws-1", "", "", "", 1, 25)
		require.NoError(t, err)
		require.Len(t, issues.Issues, 1)
		unpagedPRs, err := service.SearchUserPRsForWorkspace(t.Context(), "ws-1", "", "", "")
		require.NoError(t, err)
		require.Len(t, unpagedPRs, 1)
		unpagedIssues, err := service.SearchUserIssuesForWorkspace(t.Context(), "ws-1", "", "", "")
		require.NoError(t, err)
		require.Len(t, unpagedIssues, 1)
		for _, resource := range []any{prs.PRs[0], issues.Issues[0], unpagedPRs[0], unpagedIssues[0]} {
			data, err := json.Marshal(resource)
			require.NoError(t, err)
			var response map[string]any
			require.NoError(t, json.Unmarshal(data, &response))
			require.Equal(t, host, response["github_host"])
		}
	}
}

func TestEnterpriseReconnectUsesRetainedSettingsHost(t *testing.T) {
	for _, host := range []string{"github.com", "other.example.com", "git.example.com"} {
		t.Run(host, func(t *testing.T) {
			service, _ := newWorkspaceConnectionService(t, "alice")
			service.hostTokenClientFactory = func(string, githubhost.Host) Client { return NewMockClient() }
			connect := func(host string) {
				_, err := service.SetWorkspaceConnection(t.Context(), "ws-1", SetWorkspaceConnectionRequest{Source: ConnectionSourcePAT, Host: host, Token: "fixture"})
				require.NoError(t, err)
			}
			connect("git.example.com")
			all := RepoScopeModeAll
			_, err := service.UpdateWorkspaceSettings(withExpectedGitHubHost(t.Context(), "git.example.com"), &UpdateWorkspaceSettingsRequest{WorkspaceID: "ws-1", RepoScopeMode: &all})
			require.NoError(t, err)
			require.NoError(t, service.DeleteWorkspaceConnection(t.Context(), "ws-1"))
			connect(host)
			settings, err := service.GetWorkspaceSettings(t.Context(), "ws-1")
			require.NoError(t, err)
			require.Equal(t, host, settings.GitHubHost)
			if host == "git.example.com" {
				require.Equal(t, RepoScopeModeAll, settings.RepoScopeMode)
			} else {
				require.Equal(t, RepoScopeModeRepos, settings.RepoScopeMode)
			}
			_, err = service.UpdateWorkspaceSettings(withExpectedGitHubHost(t.Context(), settings.GitHubHost), &UpdateWorkspaceSettingsRequest{WorkspaceID: "ws-1", RepoScopeMode: &all})
			require.NoError(t, err)
			_, err = service.UpdateActionPresets(withExpectedGitHubHost(t.Context(), settings.GitHubHost), &UpdateActionPresetsRequest{WorkspaceID: "ws-1"})
			require.NoError(t, err)
		})
	}
}
