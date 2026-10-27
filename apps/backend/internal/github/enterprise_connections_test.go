package github

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/common/githubhost"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.1
func TestEnterpriseCLIConnectionSaveAndRead(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "alice")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "git.example.com" {
			t.Errorf("provider host = %s", r.Host)
			w.WriteHeader(403)
			return
		}
		switch r.URL.Path {
		case "/api/v3/user":
			if _, err := w.Write([]byte(`{"login":"alice"}`)); err != nil {
				t.Error(err)
			}
		case "/api/v3/repos/acme/demo":
			if _, err := w.Write([]byte(`{"id":42,"name":"demo","full_name":"acme/demo","owner":{"login":"acme"}}`)); err != nil {
				t.Error(err)
			}
		default:
			t.Errorf("unexpected API path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	// Validation must reach the named-account resolver before any provider call.
	service.resolver.ghToken = func(ctx context.Context, host, login string) (string, error) {
		return resolveGHAccountToken(ctx, &modernGHTokenRunner{}, host, login)
	}
	service.hostTokenClientFactory = func(token string, host githubhost.Host) Client {
		client := NewPATClientForHost(token, host)
		fixtureURL, _ := url.Parse(server.URL)
		base := server.Client().Transport
		client.httpClient = &http.Client{Transport: enterpriseFixtureTransport{base: base, target: fixtureURL}}
		return client
	}
	_, err := service.SetWorkspaceConnection(context.Background(), "ws-1", SetWorkspaceConnectionRequest{Source: ConnectionSourceGHCLI, Host: "git.example.com", Login: "alice"})
	if err != nil {
		t.Fatalf("save Enterprise connection: %v", err)
	}
	stored, err := service.store.GetWorkspaceConnection(context.Background(), "ws-1")
	if err != nil || stored.GitHubHost != "git.example.com" {
		t.Fatalf("stored connection = %+v, error %v", stored, err)
	}
	host, err := connectionHost(stored.GitHubHost)
	if err != nil {
		t.Fatal(err)
	}
	client := service.hostTokenClientFactory("derived-token", host).(*TokenClient)
	repository, err := client.GetRepository(context.Background(), "acme", "demo")
	if err != nil || repository == nil || repository.ID != 42 {
		t.Fatalf("read repository = %+v, error %v", repository, err)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.4
func TestEnterprisePersonalTokenWithoutExpiry(t *testing.T) {
	if err := validatePersonalTokens(GitHubOAuthTokens{AccessToken: "non-expiring"}, time.Now()); err != nil {
		t.Fatalf("valid non-expiring user token: %v", err)
	}
}

type enterpriseFixtureTransport struct {
	base   http.RoundTripper
	target *url.URL
}

func (f enterpriseFixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	destination := *copy.URL
	copy.Host = destination.Host
	destination.Host = f.target.Host
	copy.URL = &destination
	return f.base.RoundTrip(copy)
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.3
func TestEnterpriseOAuthFlowCapturesHostPolicy(t *testing.T) {
	store := &oauthFlowMemoryStore{}
	manager := NewOAuthFlowManager(store)
	var request OAuthFlowRequest
	if err := json.Unmarshal([]byte(`{"WorkspaceID":"ws-1","UserID":"alice","AppRegistrationID":"registration-test","Kind":"personal","GitHubHost":"git.example.com","RedirectURI":"https://kandev.example/callback","ExpectedAppCredentialGeneration":7}`), &request); err != nil {
		t.Fatal(err)
	}
	started, err := manager.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if started.PKCEChallenge != "" {
		t.Fatal("GHES confidential flow generated a PKCE challenge")
	}
	for _, flow := range store.flows {
		data, err := json.Marshal(flow)
		if err != nil {
			t.Fatal(err)
		}
		var row map[string]any
		if err := json.Unmarshal(data, &row); err != nil {
			t.Fatal(err)
		}
		if row["github_host"] != "git.example.com" || row["oauth_policy"] != "confidential_code" || row["redirect_uri"] != "https://kandev.example/callback" || row["app_credential_generation"] != float64(7) {
			t.Fatalf("flow identity/policy = %s", data)
		}
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.2
func TestEnterpriseInstallationHostSnapshot(t *testing.T) {
	var config AppInstallationConfig
	if err := json.Unmarshal([]byte(`{"RegistrationID":"registration-test","Slug":"kandev","CallbackURL":"https://kandev.example/callback","GitHubHost":"git.example.com"}`), &config); err != nil {
		t.Fatal(err)
	}
	service := NewAppInstallationService(config, NewOAuthFlowManager(&oauthFlowMemoryStore{}), &appInstallationMemoryStore{}, nil, nil)
	started, err := service.Start(context.Background(), "ws-1", "alice")
	if err != nil {
		t.Fatal(err)
	}
	link, err := url.Parse(started.URL)
	if err != nil || link.Host != "git.example.com" {
		t.Fatalf("installation URL = %s, error %v", started.URL, err)
	}
	connection := service.newInstallationConnection(&AuthFlow{GitHubHost: "git.example.com", AppRegistrationID: "registration-test"}, AppInstallation{ID: 42})
	if connection.GitHubHost != "git.example.com" {
		t.Fatalf("installation persisted on %s", connection.GitHubHost)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.3
func TestEnterprisePersonalConfidentialCallback(t *testing.T) {
	oauth := &personalOAuthRemote{fakeGitHubAppOAuth: fakeGitHubAppOAuth{tokens: GitHubOAuthTokens{AccessToken: "non-expiring"}, user: GitHubOAuthUser{ID: 7, Login: "alice"}, canAccess: true}}
	service := NewPersonalAuthService(PersonalAuthConfig{GitHubHost: "git.example.com", RegistrationID: "registration-test", CallbackURL: "https://kandev.example/callback"}, nil, nil, oauth)
	_, _, err := service.verifyPersonalCallback(context.Background(), PersonalAuthCallback{Code: "code"}, &AuthFlow{GitHubHost: "git.example.com", OAuthPolicy: OAuthPolicyConfidentialCode}, 42)
	if err != nil {
		t.Fatalf("confidential callback: %v", err)
	}
	if oauth.verifier != "" {
		t.Fatal("fabricated PKCE verifier")
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterpriseHostReplacementClosesRepositoryScopeAndFlows(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "alice")
	ctx := context.Background()
	if _, err := service.SetWorkspaceConnection(ctx, "ws-1", SetWorkspaceConnectionRequest{Source: ConnectionSourcePAT, Token: "public-token"}); err != nil {
		t.Fatal(err)
	}
	if err := service.store.CreateAuthFlow(ctx, &AuthFlow{StateHash: "pending", WorkspaceID: "ws-1", UserID: "alice", AppRegistrationID: "registration-test", Kind: AuthFlowKindPersonal, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	setup := &DeploymentAppRegistrationFlow{StateHash: "old-setup", RegistrationID: "new-registration", WorkspaceID: "ws-1", UserID: "alice", OwnerType: AppRegistrationOwnerUser, OwnerLogin: "alice", DisplayName: "App", Visibility: AppRegistrationVisibilityPrivate, PublicBaseURL: "https://kandev.example", ManifestRevision: 1, ExpiresAt: time.Now().Add(time.Hour)}
	if err := service.store.CreateDeploymentAppRegistrationFlow(ctx, setup); err != nil {
		t.Fatal(err)
	}
	prep := &AppRegistrationImportPreparation{RegistrationID: "old-prep", WorkspaceID: "ws-1", UserID: "alice", PublicBaseURL: "https://kandev.example", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()}
	if err := service.store.CreateAppRegistrationImportPreparation(ctx, prep); err != nil {
		t.Fatal(err)
	}
	if err := service.store.UpsertActionPresets(ctx, &ActionPresets{WorkspaceID: "ws-1", PR: []ActionPreset{{ID: "old-host", Label: "Old host", PromptTemplate: "Use old host"}}}); err != nil {
		t.Fatal(err)
	}
	service.hostTokenClientFactory = func(string, githubhost.Host) Client {
		client := NewMockClient()
		client.SetUser("alice")
		return client
	}
	if _, err := service.SetWorkspaceConnection(ctx, "ws-1", SetWorkspaceConnectionRequest{Source: ConnectionSourcePAT, Host: "git.example.com", Token: "enterprise-token"}); err != nil {
		t.Fatal(err)
	}
	settings, err := service.store.GetWorkspaceSettings(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if settings.RepoScopeMode != RepoScopeModeRepos || len(settings.RepoScopeRepos) != 0 {
		t.Fatalf("replacement scope = %+v", settings)
	}
	flow, err := service.store.GetAuthFlow(ctx, "pending")
	if err != nil || flow == nil || flow.ConsumedAt == nil {
		t.Fatalf("old flow remains live: %+v, error %v", flow, err)
	}
	storedSetup, err := service.store.GetDeploymentAppRegistrationFlow(ctx, setup.StateHash)
	if err != nil || storedSetup.ConsumedAt == nil {
		t.Fatalf("old setup flow live: %+v, error %v", storedSetup, err)
	}
	storedPrep, err := service.store.GetAppRegistrationImportPreparation(ctx, prep.RegistrationID)
	if err != nil || storedPrep.ConsumedAt == nil {
		t.Fatalf("old preparation live: %+v, error %v", storedPrep, err)
	}
	presets, err := service.store.GetActionPresets(ctx, "ws-1")
	if err != nil || presets != nil {
		t.Fatalf("old host action presets retained: %+v, error %v", presets, err)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.1
func TestEnterpriseAppImportAcceptsValidatedHost(t *testing.T) {
	request := AppRegistrationImportRequest{GitHubHost: "git.example.com", AppID: 42, WorkspaceID: "ws-1", DisplayName: "Enterprise", ClientID: "client", ClientSecret: "secret", PrivateKey: "key", WebhookSecret: "hook", Slug: "enterprise", OwnerLogin: "alice", OwnerType: AppRegistrationOwnerUser, Visibility: AppRegistrationVisibilityPrivate}
	if !validImportScalarFields(request) {
		t.Fatal("valid Enterprise App import rejected")
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.1
func TestEnterpriseAppManifestUsesHost(t *testing.T) {
	var request AppRegistrationManifestRequest
	if err := json.Unmarshal([]byte(`{"RegistrationID":"64b5cdb0-381c-4c4b-8fa5-e1127e5cf990","OwnerType":"user","OwnerLogin":"alice","PublicBaseURL":"https://kandev.example","Visibility":"private","GitHubHost":"git.example.com"}`), &request); err != nil {
		t.Fatal(err)
	}
	submission, err := BuildAppRegistrationManifest(request)
	if err != nil {
		t.Fatal(err)
	}
	link, err := url.Parse(submission.RegistrationURL)
	if err != nil || link.Host != "git.example.com" {
		t.Fatalf("manifest destination = %s, error %v", submission.RegistrationURL, err)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.2
func TestEnterpriseAppCredentialPrincipalHost(t *testing.T) {
	id := int64(42)
	provider := NewCachedInstallationCredentialProvider(NewInstallationTokenCache(fixedInstallationMinter{token: InstallationToken{Token: "token", ExpiresAt: time.Now().Add(time.Hour)}}))
	resolved, err := provider.ResolveInstallation(context.Background(), &WorkspaceConnection{GitHubHost: "git.example.com", InstallationID: &id}, ResolveCredentialRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.GitHubHost != "git.example.com" || resolved.Principal.GitHubHost != "git.example.com" {
		t.Fatalf("resolved host = %+v", resolved.Principal)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.1
func TestEnterpriseAppSetupHostPersists(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1")
	ctx := context.Background()
	var flow DeploymentAppRegistrationFlow
	err := json.Unmarshal([]byte(`{"StateHash":"flow","registration_id":"registration-test","workspace_id":"ws-1","UserID":"alice","github_host":"git.example.com","owner_type":"User","owner_login":"alice","display_name":"App","visibility":"private","public_base_url":"https://kandev.example","expires_at":"2099-01-01T00:00:00Z"}`), &flow)
	if err != nil {
		t.Fatal(err)
	}
	flow.StateHash = "flow"
	flow.UserID = "alice"
	flow.ManifestRevision = 1
	if err := store.CreateDeploymentAppRegistrationFlow(ctx, &flow); err != nil {
		t.Fatal(err)
	}
	read, err := store.GetDeploymentAppRegistrationFlow(ctx, "flow")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(read)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot["github_host"] != "git.example.com" {
		t.Fatalf("manifest flow host = %s", raw)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.4
func TestEnterprisePersonalCredentialHost(t *testing.T) {
	now := time.Now()
	repo := expiringPersonalAuthRepository(now)
	repo.workspace.GitHubHost = "git.example.com"
	repo.tokens = GitHubOAuthTokens{AccessToken: "non-expiring"}
	service := NewPersonalAuthService(PersonalAuthConfig{GitHubHost: "git.example.com", RegistrationID: "registration-test"}, nil, repo, &personalOAuthRemote{})
	resolved, err := (&personalAuthCredentialProvider{service: service}).ResolveUser(context.Background(), repo.connection, ResolveCredentialRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.GitHubHost != "git.example.com" || resolved.Principal.GitHubHost != "git.example.com" {
		t.Fatalf("personal credential host = %+v", resolved.Principal)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterpriseCredentialCacheRetainsHostIdentity(t *testing.T) {
	reader := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{"ws-1": {WorkspaceID: "ws-1", GitHubHost: "git.example.com", Source: ConnectionSourcePAT, Status: ConnectionStatusActive, CredentialGeneration: 1}}}
	resolver := NewCredentialResolver(reader, fakeAuthSecrets{WorkspacePATSecretKeyForHost("ws-1", "git.example.com"): "token", WorkspacePATSecretKeyForHost("ws-1", "other.example.com"): "other-token"})
	req := ResolveCredentialRequest{WorkspaceID: "ws-1", Purpose: CredentialPurposeAutomation}
	first, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	reader.workspaces["ws-1"].GitHubHost = "other.example.com"
	second, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.GitHubHost != "git.example.com" || second.GitHubHost != "other.example.com" {
		t.Fatalf("cache crossed host: %s -> %s", first.GitHubHost, second.GitHubHost)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.3
func TestEnterpriseOAuthCallbackRejectsRotatedApp(t *testing.T) {
	oauth := &personalOAuthRemote{fakeGitHubAppOAuth: fakeGitHubAppOAuth{tokens: GitHubOAuthTokens{AccessToken: "non-expiring"}, user: GitHubOAuthUser{ID: 7, Login: "alice"}, canAccess: true}}
	service := NewPersonalAuthService(PersonalAuthConfig{GitHubHost: "git.example.com", AppCredentialGeneration: 8, RegistrationID: "registration-test", CallbackURL: "https://kandev.example/callback"}, nil, nil, oauth)
	_, _, err := service.verifyPersonalCallback(context.Background(), PersonalAuthCallback{Code: "code"}, &AuthFlow{GitHubHost: "git.example.com", OAuthPolicy: OAuthPolicyConfidentialCode, ExpectedAppCredentialGeneration: 7, RedirectURI: "https://kandev.example/callback"}, 42)
	if !errors.Is(err, ErrOAuthFlowStale) || oauth.exchangeCode != "" {
		t.Fatalf("rotated App callback exchanged code: error %v", err)
	}
}
