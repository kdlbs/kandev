package azuredevops

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
)

const testOAuthRedirectURL = "http://localhost:38429/api/v1/azure-devops/oauth/callback"

// fakeEntra records token-endpoint traffic and issues numbered tokens.
type fakeEntra struct {
	mu            sync.Mutex
	tenant        string
	discoverErr   error
	exchangeErr   error
	refreshErr    error
	exchanges     []string
	refreshTokens []string
	clientIDs     []string
	redirects     []string
	// refreshEntered and refreshRelease, when set, pause Refresh after it is
	// called and before it issues tokens.
	refreshEntered chan struct{}
	refreshRelease chan struct{}
	issued         int
}

func (f *fakeEntra) DiscoverTenant(context.Context, string) (string, error) {
	if f.discoverErr != nil {
		return "", f.discoverErr
	}
	if f.tenant == "" {
		return "tenant-1", nil
	}
	return f.tenant, nil
}

func (f *fakeEntra) AuthorizeURL(req entraAuthorizeRequest) string {
	return newEntraHTTPClient().AuthorizeURL(req)
}

func (f *fakeEntra) ExchangeCode(_ context.Context, _, clientID, code, redirect, verifier string) (*entraToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exchanges = append(f.exchanges, code+":"+verifier)
	f.clientIDs = append(f.clientIDs, clientID)
	f.redirects = append(f.redirects, redirect)
	if f.exchangeErr != nil {
		return nil, f.exchangeErr
	}
	return f.issueLocked(), nil
}

func (f *fakeEntra) Refresh(_ context.Context, _, clientID, refreshToken string) (*entraToken, error) {
	if f.refreshRelease != nil {
		f.refreshEntered <- struct{}{}
		<-f.refreshRelease
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshTokens = append(f.refreshTokens, refreshToken)
	f.clientIDs = append(f.clientIDs, clientID)
	if f.refreshErr != nil {
		return nil, f.refreshErr
	}
	return f.issueLocked(), nil
}

func (f *fakeEntra) issueLocked() *entraToken {
	f.issued++
	suffix := string(rune('0' + f.issued))
	return &entraToken{AccessToken: "access-" + suffix, RefreshToken: "refresh-" + suffix, ExpiresIn: 3600}
}

// recordingAuthClient captures the Authorization header each client would send.
type recordingAuthClient struct {
	fakeClient
	auth Authorizer
}

type oauthTestHarness struct {
	svc     *Service
	store   *Store
	secrets *fakeSecretStore
	entra   *fakeEntra
	mu      sync.Mutex
	headers []string
}

func newOAuthTestHarness(t *testing.T) *oauthTestHarness {
	t.Helper()
	h := &oauthTestHarness{entra: &fakeEntra{}}
	factory := func(_ *Config, auth Authorizer) Client {
		header, err := auth.Header(context.Background())
		if err != nil {
			return &fakeClient{err: err}
		}
		h.mu.Lock()
		h.headers = append(h.headers, header)
		h.mu.Unlock()
		return &recordingAuthClient{
			fakeClient: fakeClient{result: &TestConnectionResult{OK: true, ID: "id-1", DisplayName: "Ana Silva"}},
			auth:       auth,
		}
	}
	h.svc, h.store, h.secrets = newTestService(t, factory)
	h.svc.ConfigureOAuth(OAuthSettings{ClientID: "client-1", RedirectURL: testOAuthRedirectURL})
	h.svc.entra = h.entra
	return h
}

func userContext(userID string) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID, Role: authn.RoleMember})
}

func (h *oauthTestHarness) start(t *testing.T, ctx context.Context, workspaceID string) string {
	t.Helper()
	result, err := h.svc.StartOAuth(ctx, workspaceID, &StartOAuthRequest{
		OrganizationURL: "https://dev.azure.com/acme", DefaultProjectID: "p-1", DefaultProjectName: "Platform",
	})
	if err != nil {
		t.Fatalf("StartOAuth: %v", err)
	}
	return result.State
}

func TestOAuthUnavailableWithoutClientID(t *testing.T) {
	svc, _, _ := newTestService(t, nil)
	svc.ConfigureOAuth(OAuthSettings{RedirectURL: testOAuthRedirectURL})
	if svc.OAuthAvailable() {
		t.Fatal("OAuth must be unavailable without a client ID")
	}
	_, err := svc.StartOAuth(context.Background(), "ws-a", &StartOAuthRequest{OrganizationURL: "https://dev.azure.com/acme"})
	if !errors.Is(err, ErrOAuthUnavailable) {
		t.Fatalf("StartOAuth error = %v, want ErrOAuthUnavailable", err)
	}
}

func TestStartOAuthStoresHashedFlowAndReturnsAuthorizeURL(t *testing.T) {
	h := newOAuthTestHarness(t)
	result, err := h.svc.StartOAuth(context.Background(), "ws-a", &StartOAuthRequest{
		OrganizationURL: "https://dev.azure.com/acme/",
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(result.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Path != "/tenant-1/oauth2/v2.0/authorize" || query.Get("client_id") != "client-1" ||
		query.Get("redirect_uri") != testOAuthRedirectURL || query.Get("state") != result.State ||
		query.Get("code_challenge") == "" || len(result.State) < 40 {
		t.Fatalf("authorize URL = %s, state = %q", result.AuthURL, result.State)
	}
	var stored []string
	if err := h.store.ro.Select(&stored, `SELECT state_hash FROM azure_devops_oauth_flows`); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0] == result.State || stored[0] != hashOAuthState(result.State) {
		t.Fatalf("stored state hashes = %v, want only the hash of the state", stored)
	}
}

func TestStartOAuthRejectsInvalidOrganizationBeforeDiscovery(t *testing.T) {
	h := newOAuthTestHarness(t)
	h.entra.discoverErr = errors.New("discovery must not run")
	_, err := h.svc.StartOAuth(context.Background(), "ws-a", &StartOAuthRequest{OrganizationURL: "https://example.com/acme"})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}

func TestCompleteOAuthCreatesMicrosoftAccountConnection(t *testing.T) {
	h := newOAuthTestHarness(t)
	ctx := userContext("user-1")
	state := h.start(t, ctx, "ws-a")
	cfg, err := h.svc.CompleteOAuth(ctx, "ws-a", &CompleteOAuthRequest{Code: "code-1", State: state})
	if err != nil {
		t.Fatalf("CompleteOAuth: %v", err)
	}
	if cfg.AuthMethod != AuthMethodOAuth || cfg.OAuthAccount != "Ana Silva" || !cfg.HasSecret ||
		cfg.OrganizationURL != "https://dev.azure.com/acme" || cfg.DefaultProjectName != "Platform" ||
		cfg.OAuthTenantID != "tenant-1" || cfg.TokenExpiresAt == nil || cfg.ReauthRequired {
		t.Fatalf("config = %+v", cfg)
	}
	if got := h.secrets.values[OAuthRefreshTokenKey("ws-a")]; got != "refresh-1" {
		t.Fatalf("refresh token secret = %q", got)
	}
	if got := h.secrets.values[OAuthAccessTokenKey("ws-a")]; got != "access-1" {
		t.Fatalf("access token secret = %q", got)
	}
	if len(h.entra.exchanges) != 1 || !strings.HasPrefix(h.entra.exchanges[0], "code-1:") {
		t.Fatalf("exchanges = %v", h.entra.exchanges)
	}
	if len(h.headers) == 0 || h.headers[0] != "Bearer access-1" {
		t.Fatalf("identity probe headers = %v, want bearer access token", h.headers)
	}
	encoded, _ := json.Marshal(cfg)
	if strings.Contains(string(encoded), "access-1") || strings.Contains(string(encoded), "refresh-1") {
		t.Fatalf("config response leaks a token: %s", encoded)
	}
	stored, _ := h.store.GetConfig(context.Background(), "ws-a")
	if stored == nil || !stored.LastOK {
		t.Fatalf("health after sign-in = %+v, want probed OK", stored)
	}
}

func TestCompleteOAuthRejectsInvalidAttemptsWithoutWriting(t *testing.T) {
	h := newOAuthTestHarness(t)
	owner := userContext("user-1")
	consumed := h.start(t, owner, "ws-a")
	if _, err := h.svc.CompleteOAuth(owner, "ws-a", &CompleteOAuthRequest{Code: "c", State: consumed}); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.DeleteConfigForWorkspace(owner, "ws-a"); err != nil {
		t.Fatal(err)
	}
	foreignUser := h.start(t, owner, "ws-a")
	foreignWorkspace := h.start(t, owner, "ws-a")
	expired := h.start(t, owner, "ws-a")
	if _, err := h.store.db.Exec(h.store.db.Rebind(
		`UPDATE azure_devops_oauth_flows SET expires_at = ? WHERE state_hash = ?`),
		time.Now().UTC().Add(-time.Second), hashOAuthState(expired)); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		ctx       context.Context
		workspace string
		state     string
	}{
		"reused":            {owner, "ws-a", consumed},
		"unknown":           {owner, "ws-a", "unknown-state"},
		"empty":             {owner, "ws-a", ""},
		"expired":           {owner, "ws-a", expired},
		"foreign user":      {userContext("user-2"), "ws-a", foreignUser},
		"foreign workspace": {owner, "ws-b", foreignWorkspace},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := h.svc.CompleteOAuth(tc.ctx, tc.workspace, &CompleteOAuthRequest{Code: "c", State: tc.state})
			if !errors.Is(err, ErrOAuthFlowInvalid) {
				t.Fatalf("error = %v, want ErrOAuthFlowInvalid", err)
			}
		})
	}
	for _, workspace := range []string{"ws-a", "ws-b"} {
		if cfg, _ := h.store.GetConfig(context.Background(), workspace); cfg != nil {
			t.Fatalf("%s config written by an invalid attempt: %+v", workspace, cfg)
		}
	}
	if len(h.entra.exchanges) != 1 {
		t.Fatalf("invalid attempts reached the token endpoint: %v", h.entra.exchanges)
	}
}

func TestCompleteOAuthExchangeFailureKeepsExistingPATConnection(t *testing.T) {
	h := newOAuthTestHarness(t)
	ctx := context.Background()
	if _, err := h.svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat-a",
	}); err != nil {
		t.Fatal(err)
	}
	h.entra.exchangeErr = &EntraError{Code: "access_denied"}
	state := h.start(t, ctx, "ws-a")
	if _, err := h.svc.CompleteOAuth(ctx, "ws-a", &CompleteOAuthRequest{Code: "c", State: state}); err == nil {
		t.Fatal("exchange failure must fail completion")
	}
	cfg, _ := h.svc.GetConfigForWorkspace(ctx, "ws-a")
	if cfg == nil || cfg.AuthMethod != AuthMethodPAT || h.secrets.values[SecretKeyForWorkspace("ws-a")] != "pat-a" {
		t.Fatalf("PAT connection changed after failed sign-in: %+v", cfg)
	}
	if _, ok := h.secrets.values[OAuthRefreshTokenKey("ws-a")]; ok {
		t.Fatal("failed sign-in stored a refresh token")
	}
}

func TestCompleteOAuthReplacesPATAfterSuccess(t *testing.T) {
	h := newOAuthTestHarness(t)
	ctx := context.Background()
	if _, err := h.svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat-a",
	}); err != nil {
		t.Fatal(err)
	}
	state := h.start(t, ctx, "ws-a")
	if _, err := h.svc.CompleteOAuth(ctx, "ws-a", &CompleteOAuthRequest{Code: "c", State: state}); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.secrets.values[SecretKeyForWorkspace("ws-a")]; ok {
		t.Fatal("PAT must be removed after a successful Microsoft sign-in")
	}
	username, password, err := h.svc.ResolveAzureDevOpsCloneCredentials(ctx, "ws-a")
	if err != nil || username != "kandev" || password != "access-1" {
		t.Fatalf("clone credentials = %q/%q, %v; want the Microsoft access token", username, password, err)
	}
}
