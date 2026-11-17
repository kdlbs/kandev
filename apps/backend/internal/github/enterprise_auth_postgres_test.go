package github

import (
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.3
// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterprisePostgresAuthHostRoundTrip(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := database.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY); CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, archived_at TIMESTAMPTZ); INSERT INTO workspaces(id) VALUES ('ws-1')`); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(database, database); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	now := time.Now().UTC()
	registration := newAppRegistration("registration-test", 42, "Enterprise", now)
	registration.GitHubHost = "git.example.com"
	if err := store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
		t.Fatal(err)
	}
	flow := &AuthFlow{StateHash: "auth", WorkspaceID: "ws-1", UserID: "alice", AppRegistrationID: registration.ID, Kind: AuthFlowKindPersonal, GitHubHost: registration.GitHubHost, OAuthPolicy: OAuthPolicyConfidentialCode, RedirectURI: "https://kandev.example/personal/callback", ExpectedAppCredentialGeneration: registration.CredentialGeneration, ExpiresAt: now.Add(time.Hour)}
	if err := store.CreateAuthFlow(ctx, flow); err != nil {
		t.Fatal(err)
	}
	read, err := store.GetAuthFlow(ctx, flow.StateHash)
	if err != nil || read.GitHubHost != flow.GitHubHost || read.OAuthPolicy != flow.OAuthPolicy || read.RedirectURI != flow.RedirectURI || read.ExpectedAppCredentialGeneration != flow.ExpectedAppCredentialGeneration {
		t.Fatalf("auth flow roundtrip = %+v, error %v", read, err)
	}
	setup := &DeploymentAppRegistrationFlow{StateHash: "setup", RegistrationID: "new-registration", WorkspaceID: "ws-1", UserID: "alice", GitHubHost: registration.GitHubHost, OwnerType: AppRegistrationOwnerUser, OwnerLogin: "alice", DisplayName: "App", Visibility: AppRegistrationVisibilityPrivate, PublicBaseURL: "https://kandev.example", ManifestRevision: 1, ExpiresAt: now.Add(time.Hour)}
	if err := store.CreateDeploymentAppRegistrationFlow(ctx, setup); err != nil {
		t.Fatal(err)
	}
	storedSetup, err := store.GetDeploymentAppRegistrationFlow(ctx, setup.StateHash)
	if err != nil || storedSetup.GitHubHost != setup.GitHubHost {
		t.Fatalf("manifest flow roundtrip = %+v, error %v", storedSetup, err)
	}
	prep := &AppRegistrationImportPreparation{RegistrationID: "preparation", WorkspaceID: "ws-1", UserID: "alice", GitHubHost: registration.GitHubHost, PublicBaseURL: "https://kandev.example", ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	if err := store.CreateAppRegistrationImportPreparation(ctx, prep); err != nil {
		t.Fatal(err)
	}
	storedPrep, err := store.GetAppRegistrationImportPreparation(ctx, prep.RegistrationID)
	if err != nil || storedPrep.GitHubHost != prep.GitHubHost {
		t.Fatalf("import preparation roundtrip = %+v, error %v", storedPrep, err)
	}
	old := &WorkspaceConnection{WorkspaceID: "ws-1", Source: ConnectionSourcePAT, Login: "alice", GitHubHost: "github.com", Status: ConnectionStatusActive, CredentialGeneration: 1}
	if err := store.UpsertWorkspaceConnection(ctx, old); err != nil {
		t.Fatal(err)
	}
	replacement := *old
	replacement.GitHubHost = registration.GitHubHost
	replacement.CredentialGeneration++
	if err := store.ReplaceWorkspaceHostConnection(ctx, old, &replacement); err != nil {
		t.Fatal(err)
	}
	settings, err := store.GetWorkspaceSettings(ctx, "ws-1")
	if err != nil || settings.GitHubHost != replacement.GitHubHost || settings.RepoScopeMode != RepoScopeModeRepos {
		t.Fatalf("host scope roundtrip = %+v, error %v", settings, err)
	}
	read, err = store.GetAuthFlow(ctx, flow.StateHash)
	if err != nil || read.ConsumedAt == nil {
		t.Fatalf("old flow remains live = %+v, error %v", read, err)
	}
}

func TestEnterprisePostgresReconnectRetainedHost(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	_, err := database.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY); CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, archived_at TIMESTAMPTZ); INSERT INTO workspaces(id) VALUES ('ws-1')`)
	require.NoError(t, err)
	store, err := NewStore(database, database)
	require.NoError(t, err)
	ctx := t.Context()
	connection := &WorkspaceConnection{WorkspaceID: "ws-1", Source: ConnectionSourcePAT, Login: "alice", GitHubHost: "git.example.com", Status: ConnectionStatusActive, CredentialGeneration: 1}
	require.NoError(t, store.ReplaceWorkspaceHostConnection(ctx, nil, connection))
	require.NoError(t, store.DeleteWorkspaceConnection(ctx, "ws-1"))
	connection.GitHubHost = "github.com"
	connection.CredentialGeneration++
	require.NoError(t, store.ReplaceWorkspaceHostConnection(ctx, nil, connection))
	settings, err := store.GetWorkspaceSettings(ctx, "ws-1")
	require.NoError(t, err)
	require.Equal(t, "github.com", settings.GitHubHost)
	require.Equal(t, RepoScopeModeRepos, settings.RepoScopeMode)
}
