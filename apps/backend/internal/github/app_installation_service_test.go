package github

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
)

type appInstallationMemoryStore struct {
	connection *WorkspaceConnection
}

func (s *appInstallationMemoryStore) GetWorkspaceConnection(_ context.Context, workspaceID string) (*WorkspaceConnection, error) {
	if s.connection == nil || s.connection.WorkspaceID != workspaceID {
		return nil, nil
	}
	copy := *s.connection
	return &copy, nil
}

func (s *appInstallationMemoryStore) ReplaceWorkspaceConnection(
	_ context.Context,
	connection *WorkspaceConnection,
	expected WorkspaceConnectionExpectation,
) error {
	if !matchesWorkspaceConnectionExpectation(s.connection, expected) {
		return ErrOAuthFlowStale
	}
	copy := *connection
	if s.connection != nil {
		copy.CreatedAt = s.connection.CreatedAt
	}
	s.connection = &copy
	return nil
}

type fakeAppInstallationVerifier struct {
	installation AppInstallation
	err          error
}

func (f *fakeAppInstallationVerifier) GetInstallation(_ context.Context, _ int64) (AppInstallation, error) {
	return f.installation, f.err
}

type fakeGitHubAppOAuth struct {
	tokens          GitHubOAuthTokens
	user            GitHubOAuthUser
	userErr         error
	canAccess       bool
	exchangeCode    string
	verifier        string
	exchangeStarted chan struct{}
	releaseExchange chan struct{}
}

func (f *fakeGitHubAppOAuth) ExchangeUserCode(
	ctx context.Context, code, verifier, _ string,
) (GitHubOAuthTokens, error) {
	f.exchangeCode = code
	f.verifier = verifier
	if f.exchangeStarted != nil {
		select {
		case f.exchangeStarted <- struct{}{}:
		default:
		}
	}
	if f.releaseExchange != nil {
		select {
		case <-f.releaseExchange:
		case <-ctx.Done():
			return GitHubOAuthTokens{}, ctx.Err()
		}
	}
	return f.tokens, nil
}

func (f *fakeGitHubAppOAuth) GetOAuthUser(_ context.Context, _ string) (GitHubOAuthUser, error) {
	return f.user, f.userErr
}

func (f *fakeGitHubAppOAuth) UserCanAccessInstallation(_ context.Context, _ string, _ int64) (bool, error) {
	return f.canAccess, nil
}

func TestAppInstallationStartBuildsStateBoundInstallURL(t *testing.T) {
	flowStore := &oauthFlowMemoryStore{}
	flows := NewOAuthFlowManager(flowStore)
	flows.random = strings.NewReader(strings.Repeat("d", oauthRandomBytes))
	service := NewAppInstallationService(
		AppInstallationConfig{RegistrationID: "registration-test", Slug: "kandev-app", CallbackURL: "https://kandev.example/api/v1/github/app/install/callback"},
		flows, &appInstallationMemoryStore{}, &fakeAppInstallationVerifier{}, &fakeGitHubAppOAuth{},
	)

	started, err := service.Start(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	parsed, err := url.Parse(started.URL)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	if parsed.Host != "github.com" || parsed.Path != "/apps/kandev-app/installations/new" {
		t.Fatalf("install URL = %q", started.URL)
	}
	if parsed.Query().Get("state") == "" || parsed.Query().Has("redirect_uri") {
		t.Fatalf("install URL query = %v, want only supported state binding", parsed.Query())
	}
}

func TestAppInstallationCompleteAcceptsOAuthDuringInstallCallbackShape(t *testing.T) {
	flowStore := &oauthFlowMemoryStore{}
	flows := NewOAuthFlowManager(flowStore)
	flows.random = strings.NewReader(strings.Repeat("e", oauthRandomBytes))
	store := &appInstallationMemoryStore{connection: &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourcePAT, Status: ConnectionStatusActive,
		CredentialGeneration: 3,
	}}
	verifier := &fakeAppInstallationVerifier{installation: AppInstallation{
		ID: 42, AccountID: 7, AccountLogin: "acme", AccountType: "Organization",
		Permissions: InstallationPermissions{"contents": PermissionWrite},
	}}
	oauth := &fakeGitHubAppOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "temporary-user-token"},
		user:   GitHubOAuthUser{ID: 11, Login: "octocat"}, canAccess: true,
	}
	service := NewAppInstallationService(
		AppInstallationConfig{RegistrationID: "registration-test", Slug: "kandev-app", CallbackURL: "https://kandev.example/callback"},
		flows, store, verifier, oauth,
	)
	started, err := service.Start(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	result, err := service.Complete(context.Background(), AppInstallationCallback{
		State: started.State, Code: "oauth-code", SetupAction: "install", InstallationID: 42,
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if result.AuthorizingUser.Login != "octocat" || result.Installation.AccountLogin != "acme" {
		t.Fatalf("Complete() = %+v", result)
	}
	if store.connection.Source != ConnectionSourceGitHubAppInstallation ||
		store.connection.CredentialGeneration != 4 || store.connection.InstallationID == nil ||
		*store.connection.InstallationID != 42 {
		t.Fatalf("persisted connection = %+v", store.connection)
	}
}

func TestAppInstallationCompleteRejectsSpoofedAssociation(t *testing.T) {
	flowStore := &oauthFlowMemoryStore{}
	flows := NewOAuthFlowManager(flowStore)
	flows.random = strings.NewReader(strings.Repeat("f", oauthRandomBytes))
	store := &appInstallationMemoryStore{}
	service := NewAppInstallationService(
		AppInstallationConfig{RegistrationID: "registration-test", Slug: "kandev-app", CallbackURL: "https://kandev.example/callback"},
		flows,
		store,
		&fakeAppInstallationVerifier{installation: AppInstallation{ID: 42}},
		&fakeGitHubAppOAuth{tokens: GitHubOAuthTokens{AccessToken: "token"}, user: GitHubOAuthUser{ID: 11}, canAccess: false},
	)
	started, err := service.Start(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	_, err = service.Complete(context.Background(), AppInstallationCallback{
		WorkspaceID: "workspace-1", UserID: "user-1", State: started.State,
		Code: "oauth-code", SetupAction: "install", InstallationID: 42,
	})
	if !errors.Is(err, ErrInstallationAssociationUnverified) {
		t.Fatalf("Complete() error = %v, want %v", err, ErrInstallationAssociationUnverified)
	}
	if store.connection != nil {
		t.Fatalf("spoofed callback persisted %+v", store.connection)
	}
}

func TestAppInstallationCompleteRejectsSetupURLShapeWithoutOAuthCode(t *testing.T) {
	flowStore := &oauthFlowMemoryStore{}
	flows := NewOAuthFlowManager(flowStore)
	flows.random = strings.NewReader(strings.Repeat("g", oauthRandomBytes))
	service := NewAppInstallationService(
		AppInstallationConfig{RegistrationID: "registration-test", Slug: "kandev-app", CallbackURL: "https://kandev.example/callback"},
		flows,
		&appInstallationMemoryStore{},
		&fakeAppInstallationVerifier{installation: AppInstallation{ID: 42}},
		&fakeGitHubAppOAuth{},
	)
	started, err := service.Start(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	_, err = service.Complete(context.Background(), AppInstallationCallback{
		State: started.State, SetupAction: "install", InstallationID: 42,
	})
	if !errors.Is(err, ErrInstallationAssociationUnverified) {
		t.Fatalf("Complete() error = %v, want setup-only callback rejection", err)
	}
}

func TestAppInstallationCompleteCannotOverwriteNewerPAT(t *testing.T) {
	flowStore := &oauthFlowMemoryStore{}
	flows := NewOAuthFlowManager(flowStore)
	flows.random = strings.NewReader(strings.Repeat("h", oauthRandomBytes))
	store := &appInstallationMemoryStore{connection: &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourcePAT, Login: "old",
		Status: ConnectionStatusActive, CredentialGeneration: 1,
	}}
	service := NewAppInstallationService(
		AppInstallationConfig{RegistrationID: "registration-test", Slug: "kandev-app", CallbackURL: "https://kandev.example/callback"},
		flows, store,
		&fakeAppInstallationVerifier{installation: AppInstallation{
			ID: 42, AccountLogin: "acme", AccountType: "Organization",
		}},
		&fakeGitHubAppOAuth{
			tokens: GitHubOAuthTokens{AccessToken: "temporary"},
			user:   GitHubOAuthUser{ID: 11, Login: "octocat"}, canAccess: true,
		},
	)
	started, err := service.Start(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	store.connection = &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourcePAT, Login: "new",
		Status: ConnectionStatusActive, CredentialGeneration: 2,
	}

	_, err = service.Complete(context.Background(), AppInstallationCallback{
		State: started.State, Code: "oauth-code", SetupAction: "install", InstallationID: 42,
	})
	if !errors.Is(err, ErrOAuthFlowStale) || store.connection.Source != ConnectionSourcePAT ||
		store.connection.Login != "new" || store.connection.CredentialGeneration != 2 {
		t.Fatalf("Complete() error = %v, connection = %+v", err, store.connection)
	}
}
