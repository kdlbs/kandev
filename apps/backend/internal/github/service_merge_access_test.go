package github

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func seedMergeAccessRepository(client *MockClient, owner string, known, push bool) {
	client.AddRepos(owner, []GitHubRepo{{Owner: owner, Name: "site", FullName: owner + "/site"}})
	client.SetRepositoryDetails(GitHubRepository{
		Owner: owner, Name: "site", FullName: owner + "/site", PushAccessKnown: known, PushAccess: push,
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.15
func TestService_MergeAccessUsesTargetAndDoesNotCachePermissions(t *testing.T) {
	client := NewMockClient()
	seedMergeAccessRepository(client, "upstream", true, false)
	seedMergeAccessRepository(client, "test-user", true, true)
	service := newTestService(client)
	configureTestWorkspaceAuth(t, service, client, testWorkspaceID)

	access, err := service.GetRepoMergeAccessForWorkspace(context.Background(), testWorkspaceID, DefaultUserID, "upstream", "site")
	require.NoError(t, err)
	require.Equal(t, MergeAccessDenied, access.State)
	seedMergeAccessRepository(client, "upstream", true, true)
	access, err = service.GetRepoMergeAccessForWorkspace(context.Background(), testWorkspaceID, DefaultUserID, "upstream", "site")
	require.NoError(t, err)
	require.Equal(t, MergeAccessAllowed, access.State)
}

func TestService_MergeAccessInstallationCapabilities(t *testing.T) {
	for _, tt := range []struct {
		name              string
		gitWrite, prWrite bool
		want              MergeAccessState
	}{
		{"both writes", true, true, MergeAccessAllowed},
		{"no contents write", false, true, MergeAccessDenied},
		{"no PR write", true, false, MergeAccessDenied},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockClient()
			seedMergeAccessRepository(client, "upstream", false, false)
			service := newAppReviewRequestService(t, client, map[GitHubAppCapability]bool{
				CapabilityGitWrite: tt.gitWrite, CapabilityPullRequestWrite: tt.prWrite,
			})
			access, err := service.GetRepoMergeAccessForWorkspace(context.Background(), "workspace-a", DefaultUserID, "upstream", "site")
			require.NoError(t, err)
			require.Equal(t, tt.want, access.State)
			require.Equal(t, AuthPrincipalApp, access.Actor.Kind)
		})
	}
}

func TestService_MergeAccessDoesNotReplaceDeniedPersonalActorWithApp(t *testing.T) {
	automation := NewMockClient()
	seedMergeAccessRepository(automation, "upstream", true, true)
	personal := NewMockClient()
	seedMergeAccessRepository(personal, "upstream", true, false)
	service := newAppReviewRequestService(t, automation, allTokenCapabilities())
	connections := service.resolver.connections.(*fakeConnectionReader)
	connections.users = map[string]*UserConnection{
		"workspace-a:default-user": {
			WorkspaceID: "workspace-a", UserID: DefaultUserID, Login: "contributor",
			AppRegistrationID: "registration-a", Status: ConnectionStatusActive,
		},
	}
	service.resolver.SetUserProvider(fixedUserCredentialProvider{client: personal})
	access, err := service.GetRepoMergeAccessForWorkspace(context.Background(), "workspace-a", DefaultUserID, "upstream", "site")
	require.NoError(t, err)
	require.Equal(t, MergeAccessDenied, access.State)
	require.Equal(t, "contributor", access.Actor.Login)
	require.Equal(t, AuthPrincipalHuman, access.Actor.Kind)
}

type mergeAccessReadClient struct {
	*MockClient
	repository *GitHubRepository
	err        error
}

func (c mergeAccessReadClient) GetRepository(context.Context, string, string) (*GitHubRepository, error) {
	return c.repository, c.err
}

func TestService_MergeAccessCannotConfirmIncompleteReads(t *testing.T) {
	for _, tt := range []struct {
		name    string
		client  Client
		wantErr bool
	}{
		{"nil repository", mergeAccessReadClient{MockClient: NewMockClient()}, false},
		{"provider failure", mergeAccessReadClient{MockClient: NewMockClient(), err: errors.New("unavailable")}, true},
		{"unsupported reader", repositoryVisibilityClient{Client: NewMockClient(), repos: map[string]GitHubRepo{"upstream/site": {}}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if client, ok := tt.client.(mergeAccessReadClient); ok {
				client.AddRepos("upstream", []GitHubRepo{{Owner: "upstream", Name: "site"}})
			}
			service := newTestService(tt.client)
			configureTestWorkspaceAuth(t, service, tt.client, testWorkspaceID)
			access, err := service.GetRepoMergeAccessForWorkspace(context.Background(), testWorkspaceID, DefaultUserID, "upstream", "site")
			require.Equal(t, tt.wantErr, err != nil)
			require.Equal(t, MergeAccessUnknown, access.State)
		})
	}
}

func TestService_MergeAccessRequiresWorkspaceAuthorization(t *testing.T) {
	client := NewMockClient()
	seedMergeAccessRepository(client, "upstream", true, true)
	service := newTestService(client)
	configureTestWorkspaceAuth(t, service, client, testWorkspaceID)
	denied := errors.New("workspace access denied")
	service.workspaceAuthorizer = func(context.Context, string) error { return denied }
	access, err := service.GetRepoMergeAccessForWorkspace(context.Background(), testWorkspaceID, DefaultUserID, "upstream", "site")
	require.ErrorIs(t, err, denied)
	require.Equal(t, MergeAccessUnknown, access.State)
	require.Nil(t, access.Actor)
}

func TestService_MergeAccessRejectsRepositoryOutsideWorkspaceScope(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, testWorkspaceID)
	require.NoError(t, store.UpsertWorkspaceSettings(context.Background(), &WorkspaceSettings{
		WorkspaceID: testWorkspaceID, RepoScopeMode: RepoScopeModeRepos,
		RepoScopeRepos: []RepoFilter{{Owner: "upstream", Name: "allowed"}},
	}))
	client := NewMockClient()
	seedMergeAccessRepository(client, "upstream", true, true)
	service := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	configureTestWorkspaceAuth(t, service, client, testWorkspaceID)
	access, err := service.GetRepoMergeAccessForWorkspace(context.Background(), testWorkspaceID, DefaultUserID, "upstream", "site")
	require.ErrorIs(t, err, ErrRepoNotResolvable)
	require.Equal(t, MergeAccessUnknown, access.State)
	require.Nil(t, access.Actor)
	require.Zero(t, client.GetRepositoryCallCount())
}
