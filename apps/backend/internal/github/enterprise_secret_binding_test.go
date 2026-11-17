package github

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/common/githubhost"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type pausedPATSecretReader struct {
	secrets  *fakeConnectionSecrets
	admitted chan struct{}
	release  chan struct{}
}

func (p pausedPATSecretReader) Reveal(ctx context.Context, key string) (string, error) {
	close(p.admitted)
	select {
	case <-p.release:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return p.secrets.Reveal(ctx, key)
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterpriseReplacementNeverRebindsOldPATRead(t *testing.T) {
	service, secrets := newWorkspaceConnectionService(t, "alice")
	ctx := context.Background()
	old, err := service.SetWorkspaceConnection(ctx, "ws-1", SetWorkspaceConnectionRequest{Source: ConnectionSourcePAT, Token: "public-token"})
	if err != nil {
		t.Fatal(err)
	}
	barrier := pausedPATSecretReader{secrets: secrets, admitted: make(chan struct{}), release: make(chan struct{})}
	reader := NewCredentialResolver(service.store, barrier)
	type result struct {
		credential *ResolvedCredential
		err        error
	}
	done := make(chan result, 1)
	go func() { resolved, err := reader.resolvePAT(ctx, old); done <- result{resolved, err} }()
	<-barrier.admitted
	service.hostTokenClientFactory = func(string, githubhost.Host) Client {
		client := NewMockClient()
		client.SetUser("alice")
		return client
	}
	_, err = service.SetWorkspaceConnection(ctx, "ws-1", SetWorkspaceConnectionRequest{Source: ConnectionSourcePAT, Host: "git.example.com", Token: "enterprise-token"})
	close(barrier.release)
	if err != nil {
		t.Fatal(err)
	}
	got := <-done
	if got.err == nil && got.credential.credential != "public-token" {
		t.Fatalf("old-host snapshot received replacement token %q", got.credential.credential)
	}
}

func enterprisePersonalRepository(t *testing.T) (*StorePersonalConnectionRepository, *fakeConnectionSecrets, *UserConnection) {
	t.Helper()
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1")
	seedPersonalAppConnection(t, store, "ws-1")
	if _, err := store.db.Exec(`UPDATE github_app_registrations SET github_host='git.example.com' WHERE id='registration-personal-test'; UPDATE github_workspace_connections SET github_host='git.example.com' WHERE workspace_id='ws-1'`); err != nil {
		t.Fatal(err)
	}
	secrets := newFakeConnectionSecrets()
	connection := &UserConnection{WorkspaceID: "ws-1", UserID: "alice", AppRegistrationID: "registration-personal-test", GitHubUserID: 7, Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 1}
	return NewStorePersonalConnectionRepository(store, secrets), secrets, connection
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.4
func TestEnterprisePersonalSecretsAreHostBound(t *testing.T) {
	repo, secrets, connection := enterprisePersonalRepository(t)
	publicKey := UserAccessTokenSecretKey(connection.WorkspaceID, connection.UserID)
	secrets.values[publicKey] = "public-token"
	if err := repo.ReplacePersonalConnection(context.Background(), connection, GitHubOAuthTokens{AccessToken: "enterprise-token", RefreshToken: "refresh"}, 0); err != nil {
		t.Fatal(err)
	}
	if secrets.values[publicKey] != "public-token" {
		t.Fatal("Enterprise personal token overwrote public secret binding")
	}
	tokens, err := repo.GetPersonalTokens(context.Background(), connection.WorkspaceID, connection.UserID)
	if err != nil || tokens.AccessToken != "enterprise-token" {
		t.Fatalf("host-bound personal token = %+v, error %v", tokens, err)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.4
func TestEnterprisePersonalNonExpiringTokenPersistsWithoutRefresh(t *testing.T) {
	repo, _, connection := enterprisePersonalRepository(t)
	if err := repo.ReplacePersonalConnection(context.Background(), connection, GitHubOAuthTokens{AccessToken: "non-expiring"}, 0); err != nil {
		t.Fatalf("persist valid non-expiring token: %v", err)
	}
	tokens, err := repo.GetPersonalTokens(context.Background(), connection.WorkspaceID, connection.UserID)
	if err != nil || tokens.AccessToken != "non-expiring" || tokens.RefreshToken != "" {
		t.Fatalf("non-expiring token roundtrip = %+v, error %v", tokens, err)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.4
func TestEnterprisePersonalWorkspaceHostMismatchFailsBeforeRefresh(t *testing.T) {
	repo := expiringPersonalAuthRepository(time.Now())
	repo.workspace.GitHubHost = "other.example.com"
	remote := &personalOAuthRemote{}
	service := NewPersonalAuthService(PersonalAuthConfig{GitHubHost: "git.example.com", RegistrationID: "registration-test"}, nil, repo, remote)
	_, _, err := service.ResolvePersonalToken(context.Background(), "workspace-1", "user-1")
	if !errors.Is(err, ErrGitHubPersonalRequired) || remote.refreshes.Load() != 0 {
		t.Fatalf("mismatched host refreshed token: %v, refresh calls %d", err, remote.refreshes.Load())
	}
}

type personalReadBarrierKey struct{}
type pausedPersonalSecretStore struct {
	*fakeConnectionSecrets
	admitted chan struct{}
	release  chan struct{}
}

func (p pausedPersonalSecretStore) Reveal(ctx context.Context, key string) (string, error) {
	if ctx.Value(personalReadBarrierKey{}) != nil {
		close(p.admitted)
		select {
		case <-p.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return p.fakeConnectionSecrets.Reveal(ctx, key)
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterpriseReplacementNeverRebindsOldPersonalRead(t *testing.T) {
	service, secrets := newWorkspaceConnectionService(t, "alice")
	ctx := context.Background()
	oldApp := activeAppWorkspace("ws-1", 42)
	if err := service.store.UpsertWorkspaceConnection(ctx, oldApp); err != nil {
		t.Fatal(err)
	}
	barrier := pausedPersonalSecretStore{fakeConnectionSecrets: secrets, admitted: make(chan struct{}), release: make(chan struct{})}
	repo := NewStorePersonalConnectionRepository(service.store, barrier)
	service.personalConnections = repo
	old := &UserConnection{WorkspaceID: "ws-1", UserID: "alice", AppRegistrationID: oldApp.AppRegistrationID, GitHubUserID: 7, Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 1}
	if err := repo.ReplacePersonalConnection(ctx, old, GitHubOAuthTokens{AccessToken: "public-token", RefreshToken: "public-refresh"}, 0); err != nil {
		t.Fatal(err)
	}
	oldSnapshot, err := repo.GetUserConnection(ctx, "ws-1", "alice")
	if err != nil {
		t.Fatal(err)
	}
	positive, err := repo.GetPersonalTokensForConnection(ctx, oldSnapshot)
	if err != nil || positive.AccessToken != "public-token" {
		t.Fatalf("old registration control failed: %+v, error %v", positive, err)
	}
	type result struct {
		tokens GitHubOAuthTokens
		err    error
	}
	done := make(chan result, 1)
	go func() {
		tokens, err := repo.GetPersonalTokensForConnection(context.WithValue(ctx, personalReadBarrierKey{}, true), oldSnapshot)
		done <- result{tokens, err}
	}()
	<-barrier.admitted
	registration := newAppRegistration("enterprise-registration", 202, "Enterprise", time.Now())
	registration.GitHubHost = "git.example.com"
	if err := service.store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
		close(barrier.release)
		t.Fatal(err)
	}
	replacement := *oldApp
	replacement.GitHubHost = registration.GitHubHost
	replacement.AppRegistrationID = registration.ID
	replacement.CredentialGeneration++
	if err := (&serviceAppConnectionStore{service: service}).ReplaceWorkspaceConnection(ctx, &replacement, workspaceConnectionExpectation(oldApp)); err != nil {
		close(barrier.release)
		t.Fatal(err)
	}
	generation, err := repo.GetPersonalConnectionGeneration(ctx, "ws-1", "alice")
	if err != nil {
		close(barrier.release)
		t.Fatal(err)
	}
	replacementUser := *old
	replacementUser.AppRegistrationID = registration.ID
	replacementUser.CredentialGeneration = generation + 1
	if err := repo.ReplacePersonalConnection(ctx, &replacementUser, GitHubOAuthTokens{AccessToken: "enterprise-token"}, generation); err != nil {
		close(barrier.release)
		t.Fatal(err)
	}
	close(barrier.release)
	got := <-done
	if got.err == nil && got.tokens.AccessToken != "public-token" {
		t.Fatalf("old registration read received replacement token %q", got.tokens.AccessToken)
	}
	current, err := repo.GetPersonalTokens(ctx, "ws-1", "alice")
	if err != nil || current.AccessToken != "enterprise-token" {
		t.Fatalf("replacement token = %+v, error %v", current, err)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.2
func TestEnterpriseAppConnectionCannotRebindPublicInstallationToken(t *testing.T) {
	_, privateKey := testAppPrivateKey(t)
	app, err := NewAppClientForHost(42, privateKey, githubhost.Public())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	app.httpClient = &http.Client{Transport: appRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "api.github.com" {
			t.Errorf("mint destination %s", req.URL.Host)
		}
		return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`{"token":"public-installation-token","expires_at":"2099-01-01T00:00:00Z","permissions":{"contents":"read"}}`)), Header: make(http.Header)}, nil
	})}
	provider := NewCachedInstallationCredentialProvider(NewAppInstallationTokenCache("registration", 1, app))
	id := int64(7)
	connection := &WorkspaceConnection{WorkspaceID: "ws-1", AppRegistrationID: "registration", InstallationID: &id, GitHubHost: "github.com"}
	control, err := provider.ResolveInstallation(context.Background(), connection, ResolveCredentialRequest{})
	if err != nil || control.GitHubHost != "github.com" || calls != 1 {
		t.Fatalf("valid public mint control: %+v, error %v, calls %d", control, err, calls)
	}
	connection.GitHubHost = "git.example.com"
	rebound, err := provider.ResolveInstallation(context.Background(), connection, ResolveCredentialRequest{})
	if !errors.Is(err, ErrGitHubHostMismatch) || rebound != nil || calls != 1 {
		t.Fatalf("public installation token rebound to Enterprise: %+v, error %v, calls %d", rebound, err, calls)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterpriseCrossHostConnectionCleanupFailureKeepsCommittedAuthority(t *testing.T) {
	for _, source := range []ConnectionSource{ConnectionSourceGHCLI, ConnectionSourceGitHubAppInstallation} {
		t.Run(string(source), func(t *testing.T) {
			service, secrets := newWorkspaceConnectionService(t, "alice")
			ctx := context.Background()
			old, err := service.SetWorkspaceConnection(ctx, "ws-1", SetWorkspaceConnectionRequest{Source: ConnectionSourcePAT, Token: "public-token"})
			if err != nil {
				t.Fatal(err)
			}
			mode := TaskGitCredentialsModeExecutor
			if _, err := service.UpdateWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{WorkspaceID: "ws-1", TaskGitCredentialsMode: &mode}); err != nil {
				t.Fatal(err)
			}
			secrets.deleteErrKey = WorkspacePATSecretKey("ws-1")
			secrets.deleteErr = errors.New("obsolete secret unavailable")
			if source == ConnectionSourceGHCLI {
				service.resolver.ghToken = func(context.Context, string, string) (string, error) { return "enterprise-cli-token", nil }
				service.hostTokenClientFactory = func(string, githubhost.Host) Client {
					client := NewMockClient()
					client.SetUser("alice")
					return client
				}
				_, err = service.SetWorkspaceConnection(ctx, "ws-1", SetWorkspaceConnectionRequest{Source: source, Host: "git.example.com", Login: "alice"})
			} else {
				registration := newAppRegistration("enterprise-registration", 202, "Enterprise", time.Now())
				registration.GitHubHost = "git.example.com"
				if err := service.store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
					t.Fatal(err)
				}
				id := int64(42)
				replacement := &WorkspaceConnection{WorkspaceID: "ws-1", GitHubHost: registration.GitHubHost, Source: source, AppRegistrationID: registration.ID, InstallationID: &id, InstallationAccountLogin: "acme", InstallationAccountType: "Organization", Status: ConnectionStatusActive, CredentialGeneration: old.CredentialGeneration + 1}
				err = (&serviceAppConnectionStore{service: service}).ReplaceWorkspaceConnection(ctx, replacement, workspaceConnectionExpectation(old))
			}
			if err != nil {
				t.Fatalf("committed host replacement returned obsolete cleanup failure: %v", err)
			}
			current, err := service.store.GetWorkspaceConnection(ctx, "ws-1")
			if err != nil || current.GitHubHost != "git.example.com" || current.Source != source || current.CredentialGeneration != old.CredentialGeneration+1 {
				t.Fatalf("committed authority rolled back: %+v, error %v", current, err)
			}
			settings, err := service.GetWorkspaceSettings(ctx, "ws-1")
			if err != nil || settings.GitHubHost != current.GitHubHost || settings.RepoScopeMode != RepoScopeModeRepos || settings.TaskGitCredentialsMode != TaskGitCredentialsModeExecutor {
				t.Fatalf("connection/settings mixed authorities: %+v, error %v", settings, err)
			}
		})
	}
}
