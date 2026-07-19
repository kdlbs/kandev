package github

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type fakeConnectionSecrets struct {
	values       map[string]string
	setErr       error
	setErrKey    string
	deleteErr    error
	deleteErrKey string
}

func newFakeConnectionSecrets() *fakeConnectionSecrets {
	return &fakeConnectionSecrets{values: make(map[string]string)}
}

func (f *fakeConnectionSecrets) Reveal(_ context.Context, id string) (string, error) {
	value, ok := f.values[id]
	if !ok {
		return "", errors.New("secret not found")
	}
	return value, nil
}

func (f *fakeConnectionSecrets) Set(_ context.Context, id, _ string, value string) error {
	if f.setErr != nil && (f.setErrKey == "" || f.setErrKey == id) {
		return f.setErr
	}
	f.values[id] = value
	return nil
}

func (f *fakeConnectionSecrets) Delete(_ context.Context, id string) error {
	if f.deleteErr != nil && (f.deleteErrKey == "" || f.deleteErrKey == id) {
		return f.deleteErr
	}
	delete(f.values, id)
	return nil
}

func (f *fakeConnectionSecrets) Exists(_ context.Context, id string) (bool, error) {
	_, ok := f.values[id]
	return ok, nil
}

func (f *fakeConnectionSecrets) ListIDs(context.Context) ([]string, error) {
	ids := make([]string, 0, len(f.values))
	for id := range f.values {
		ids = append(ids, id)
	}
	return ids, nil
}

func newWorkspaceConnectionService(t *testing.T, login string) (*Service, *fakeConnectionSecrets) {
	t.Helper()
	store := newTestStore(t)
	if _, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS workspaces (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO workspaces (id) VALUES ('ws-1')`); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	secrets := newFakeConnectionSecrets()
	service.SetConnectionSecretStore(secrets)
	service.tokenClientFactory = func(string) Client {
		client := NewMockClient()
		client.SetUser(login)
		return client
	}
	return service, secrets
}

func TestSetWorkspaceConnectionPATIsScopedAndValidatedBeforeWrite(t *testing.T) {
	service, secrets := newWorkspaceConnectionService(t, "octocat")
	connection, err := service.SetWorkspaceConnection(context.Background(), "ws-1", SetWorkspaceConnectionRequest{
		Source: ConnectionSourcePAT,
		Token:  "ghp_workspace",
	})
	if err != nil {
		t.Fatalf("SetWorkspaceConnection: %v", err)
	}
	if connection.Login != "octocat" || connection.CredentialGeneration != 1 {
		t.Fatalf("connection = %+v", connection)
	}
	if got := secrets.values[WorkspacePATSecretKey("ws-1")]; got != "ghp_workspace" {
		t.Fatalf("stored PAT = %q", got)
	}
}

func TestWorkspaceAuthStatusIncludesEffectiveActorsAndCapabilities(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "octocat")
	if _, err := service.SetWorkspaceConnection(context.Background(), "ws-1", SetWorkspaceConnectionRequest{
		Source: ConnectionSourcePAT,
		Token:  "ghp_workspace",
	}); err != nil {
		t.Fatalf("SetWorkspaceConnection: %v", err)
	}

	status, err := service.GetWorkspaceAuthStatus(context.Background(), "ws-1", DefaultUserID)
	if err != nil {
		t.Fatalf("GetWorkspaceAuthStatus: %v", err)
	}
	if !status.Authenticated || status.AuthMethod != string(ConnectionSourcePAT) ||
		!status.TokenConfigured || status.Username != "octocat" {
		t.Fatalf("unexpected compatibility status: %+v", status)
	}
	if status.Automation == nil || status.Automation.Actor == nil ||
		status.Automation.Actor.Login != "octocat" || status.Automation.Actor.Kind != AuthPrincipalHuman {
		t.Fatalf("unexpected automation actor: %+v", status.Automation)
	}
	if len(status.Automation.MissingCapabilities) != 0 ||
		!status.Automation.Capabilities[CapabilityPullRequestWrite] {
		t.Fatalf("unexpected capabilities: %+v", status.Automation)
	}
	if status.EffectivePersonalActor == nil || status.EffectivePersonalActor.Login != "octocat" ||
		status.EffectiveManualMutationActor == nil || status.EffectiveManualMutationActor.Login != "octocat" {
		t.Fatalf("unexpected effective actors: personal=%+v mutation=%+v",
			status.EffectivePersonalActor, status.EffectiveManualMutationActor)
	}
}

func TestSetWorkspaceConnectionCLIStoresSelectionNotToken(t *testing.T) {
	service, secrets := newWorkspaceConnectionService(t, "work-user")
	service.resolver.ghToken = func(_ context.Context, host, login string) (string, error) {
		if host != "github.com" || login != "work-user" {
			t.Fatalf("selected account = %s@%s", login, host)
		}
		return "cli-derived-token", nil
	}
	connection, err := service.SetWorkspaceConnection(context.Background(), "ws-1", SetWorkspaceConnectionRequest{
		Source: ConnectionSourceGHCLI,
		Host:   "github.com",
		Login:  "work-user",
	})
	if err != nil {
		t.Fatalf("SetWorkspaceConnection: %v", err)
	}
	if connection.Source != ConnectionSourceGHCLI || connection.Login != "work-user" {
		t.Fatalf("connection = %+v", connection)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("CLI token was persisted: %#v", secrets.values)
	}
}

func TestSetWorkspaceConnectionValidationFailurePreservesPreviousPAT(t *testing.T) {
	service, secrets := newWorkspaceConnectionService(t, "octocat")
	if _, err := service.SetWorkspaceConnection(context.Background(), "ws-1", SetWorkspaceConnectionRequest{
		Source: ConnectionSourcePAT, Token: "first",
	}); err != nil {
		t.Fatal(err)
	}
	service.tokenClientFactory = func(string) Client { return &NoopClient{} }
	if _, err := service.SetWorkspaceConnection(context.Background(), "ws-1", SetWorkspaceConnectionRequest{
		Source: ConnectionSourcePAT, Token: "invalid",
	}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("error = %v, want ErrInvalidToken", err)
	}
	if got := secrets.values[WorkspacePATSecretKey("ws-1")]; got != "first" {
		t.Fatalf("previous PAT changed to %q", got)
	}
	connection, err := service.store.GetWorkspaceConnection(context.Background(), "ws-1")
	if err != nil || connection.CredentialGeneration != 1 {
		t.Fatalf("connection after failure = %+v, err %v", connection, err)
	}
}

func TestDeleteWorkspaceConnectionRemovesOwnedPAT(t *testing.T) {
	service, secrets := newWorkspaceConnectionService(t, "octocat")
	if _, err := service.SetWorkspaceConnection(context.Background(), "ws-1", SetWorkspaceConnectionRequest{
		Source: ConnectionSourcePAT, Token: "token",
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteWorkspaceConnection(context.Background(), "ws-1"); err != nil {
		t.Fatalf("DeleteWorkspaceConnection: %v", err)
	}
	if _, ok := secrets.values[WorkspacePATSecretKey("ws-1")]; ok {
		t.Fatal("workspace PAT remains after disconnect")
	}
}

func TestDeleteWorkspaceConnectionPreservesPATConnectionWhenSecretDeleteFails(t *testing.T) {
	service, secrets := newWorkspaceConnectionService(t, "octocat")
	if _, err := service.SetWorkspaceConnection(context.Background(), "ws-1", SetWorkspaceConnectionRequest{
		Source: ConnectionSourcePAT, Token: "token",
	}); err != nil {
		t.Fatal(err)
	}
	secrets.deleteErr = errors.New("secret store unavailable")
	secrets.deleteErrKey = WorkspacePATSecretKey("ws-1")

	if err := service.DeleteWorkspaceConnection(context.Background(), "ws-1"); err == nil {
		t.Fatal("expected disconnect failure")
	}
	connection, err := service.store.GetWorkspaceConnection(context.Background(), "ws-1")
	if err != nil || connection == nil || connection.Source != ConnectionSourcePAT {
		t.Fatalf("connection after failed disconnect = %+v, err %v", connection, err)
	}
	if got := secrets.values[WorkspacePATSecretKey("ws-1")]; got != "token" {
		t.Fatalf("PAT after failed disconnect = %q", got)
	}
}

func TestSetWorkspaceConnectionPATRevokesPersonalAppConnections(t *testing.T) {
	service, secrets := newWorkspaceConnectionService(t, "octocat")
	installationID := int64(42)
	if err := service.store.UpsertWorkspaceConnection(context.Background(), activeAppWorkspace("ws-1", installationID)); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	personal := &UserConnection{
		WorkspaceID: "ws-1", UserID: "user-1", GitHubUserID: 7, Login: "human",
		Status: ConnectionStatusActive, AccessExpiresAt: now.Add(time.Hour), CredentialGeneration: 1,
	}
	if err := service.personalConnections.ReplacePersonalConnection(context.Background(), personal, GitHubOAuthTokens{
		AccessToken: "personal-access", RefreshToken: "personal-refresh", AccessExpiresAt: personal.AccessExpiresAt,
	}, 0); err != nil {
		t.Fatal(err)
	}

	if _, err := service.SetWorkspaceConnection(context.Background(), "ws-1", SetWorkspaceConnectionRequest{
		Source: ConnectionSourcePAT, Token: "workspace-token",
	}); err != nil {
		t.Fatal(err)
	}
	stored, err := service.store.GetUserConnection(context.Background(), "ws-1", "user-1")
	if err != nil || stored != nil {
		t.Fatalf("personal connection after App replacement = %+v, err %v", stored, err)
	}
	if _, ok := secrets.values[UserAccessTokenSecretKey("ws-1", "user-1")]; ok {
		t.Fatal("personal access secret remains after App replacement")
	}
	if _, ok := secrets.values[UserRefreshTokenSecretKey("ws-1", "user-1")]; ok {
		t.Fatal("personal refresh secret remains after App replacement")
	}
}

func TestDeleteWorkspaceAppConnectionRevokesPersonalConnections(t *testing.T) {
	service, secrets := newWorkspaceConnectionService(t, "octocat")
	installationID := int64(42)
	if err := service.store.UpsertWorkspaceConnection(context.Background(), activeAppWorkspace("ws-1", installationID)); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	personal := &UserConnection{
		WorkspaceID: "ws-1", UserID: "user-1", GitHubUserID: 7, Login: "human",
		Status: ConnectionStatusActive, AccessExpiresAt: now.Add(time.Hour), CredentialGeneration: 1,
	}
	if err := service.personalConnections.ReplacePersonalConnection(context.Background(), personal, GitHubOAuthTokens{
		AccessToken: "personal-access", RefreshToken: "personal-refresh", AccessExpiresAt: personal.AccessExpiresAt,
	}, 0); err != nil {
		t.Fatal(err)
	}

	if err := service.DeleteWorkspaceConnection(context.Background(), "ws-1"); err != nil {
		t.Fatal(err)
	}
	stored, err := service.store.GetUserConnection(context.Background(), "ws-1", "user-1")
	if err != nil || stored != nil || len(secrets.values) != 0 {
		t.Fatalf("personal state after App disconnect = %+v, secrets=%#v, err %v", stored, secrets.values, err)
	}
}

func TestDeleteWorkspaceAppConnectionCompensatesPersonalRevokeFailure(t *testing.T) {
	service, secrets := newWorkspaceConnectionService(t, "octocat")
	installationID := int64(42)
	if err := service.store.UpsertWorkspaceConnection(context.Background(), activeAppWorkspace("ws-1", installationID)); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	personal := &UserConnection{
		WorkspaceID: "ws-1", UserID: "user-1", GitHubUserID: 7, Login: "human",
		Status: ConnectionStatusActive, AccessExpiresAt: now.Add(time.Hour), CredentialGeneration: 1,
	}
	if err := service.personalConnections.ReplacePersonalConnection(context.Background(), personal, GitHubOAuthTokens{
		AccessToken: "personal-access", RefreshToken: "personal-refresh", AccessExpiresAt: personal.AccessExpiresAt,
	}, 0); err != nil {
		t.Fatal(err)
	}
	secrets.deleteErr = errors.New("personal secret delete failed")
	secrets.deleteErrKey = UserRefreshTokenSecretKey("ws-1", "user-1")

	if err := service.DeleteWorkspaceConnection(context.Background(), "ws-1"); err == nil {
		t.Fatal("expected disconnect failure")
	}
	workspace, err := service.store.GetWorkspaceConnection(context.Background(), "ws-1")
	if err != nil || workspace == nil || workspace.InstallationID == nil || *workspace.InstallationID != installationID {
		t.Fatalf("workspace connection after compensation = %+v, err %v", workspace, err)
	}
	stored, err := service.store.GetUserConnection(context.Background(), "ws-1", "user-1")
	if err != nil || stored == nil {
		t.Fatalf("personal connection after compensation = %+v, err %v", stored, err)
	}
}

func TestDifferentAppInstallationRequiresPersonalConnectionRevocation(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "octocat")
	oldInstallationID, newInstallationID := int64(42), int64(84)
	existing := activeAppWorkspace("ws-1", oldInstallationID)
	if err := service.store.UpsertWorkspaceConnection(context.Background(), existing); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	personal := &UserConnection{
		WorkspaceID: "ws-1", UserID: "user-1", GitHubUserID: 7, Login: "human",
		Status: ConnectionStatusActive, AccessExpiresAt: now.Add(time.Hour), CredentialGeneration: 1,
	}
	if err := service.personalConnections.ReplacePersonalConnection(context.Background(), personal, GitHubOAuthTokens{
		AccessToken: "personal-access", RefreshToken: "personal-refresh", AccessExpiresAt: personal.AccessExpiresAt,
	}, 0); err != nil {
		t.Fatal(err)
	}
	replacement := activeAppWorkspace("ws-1", newInstallationID)
	if err := service.revokePersonalForAutomationTransition(context.Background(), existing, replacement); err != nil {
		t.Fatal(err)
	}
	stored, err := service.store.GetUserConnection(context.Background(), "ws-1", "user-1")
	if err != nil || stored != nil {
		t.Fatalf("personal connection after installation replacement = %+v, err %v", stored, err)
	}
}

func TestSetWorkspaceConnectionRejectsStaleValidatedMutation(t *testing.T) {
	service, secrets := newWorkspaceConnectionService(t, "octocat")
	started := make(chan struct{})
	release := make(chan struct{})
	service.tokenClientFactory = func(token string) Client {
		if token == "slow" {
			close(started)
			<-release
		}
		client := NewMockClient()
		client.SetUser("octocat")
		return client
	}
	slowResult := make(chan error, 1)
	go func() {
		_, err := service.SetWorkspaceConnection(context.Background(), "ws-1", SetWorkspaceConnectionRequest{
			Source: ConnectionSourcePAT, Token: "slow",
		})
		slowResult <- err
	}()
	<-started
	if _, err := service.SetWorkspaceConnection(context.Background(), "ws-1", SetWorkspaceConnectionRequest{
		Source: ConnectionSourcePAT, Token: "newer",
	}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-slowResult; !errors.Is(err, ErrWorkspaceConnectionStale) {
		t.Fatalf("slow mutation error = %v, want stale", err)
	}
	if got := secrets.values[WorkspacePATSecretKey("ws-1")]; got != "newer" {
		t.Fatalf("PAT after stale mutation = %q", got)
	}
}

func TestWorkspaceConnectionStaleUsesConflictResponse(t *testing.T) {
	status, code := githubAuthErrorResponse(ErrWorkspaceConnectionStale)
	if status != http.StatusConflict || code != "github_connection_changed" {
		t.Fatalf("stale response = %d, %q", status, code)
	}
}
