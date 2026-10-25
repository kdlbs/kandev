package jira

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type oauthRoundTripFunc func(*http.Request) (*http.Response, error)

func (f oauthRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func oauthResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func withDefaultOAuthTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = previous })
}

type contextAwareSecretStore struct {
	*fakeSecretStore
}

func (s *contextAwareSecretStore) Set(ctx context.Context, id, name, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.fakeSecretStore.Set(ctx, id, name, value)
}

func TestGeneratePKCEPairUsesS256(t *testing.T) {
	verifier, challenge, err := generatePKCEPair()
	if err != nil {
		t.Fatalf("generate pair: %v", err)
	}
	wantHash := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(wantHash[:])
	if challenge != want {
		t.Fatalf("challenge = %q, want %q", challenge, want)
	}
}

func TestMCPStateIsSingleUseAndExpires(t *testing.T) {
	states := newMCPStateManager()
	states.store(mcpPendingState{State: "fresh", CreatedAt: time.Now()})
	if _, ok := states.loadAndDelete("fresh"); !ok {
		t.Fatal("fresh state was not found")
	}
	if _, ok := states.loadAndDelete("fresh"); ok {
		t.Fatal("state was reusable")
	}

	states.store(mcpPendingState{State: "expired", CreatedAt: time.Now().Add(-mcpStateTTL - time.Second)})
	if _, ok := states.loadAndDelete("expired"); ok {
		t.Fatal("expired state remained available")
	}
}

func TestMCPScopesCoverEveryUsedTool(t *testing.T) {
	requested := make(map[string]bool)
	for _, scope := range strings.Fields(mcpScopes) {
		requested[scope] = true
	}
	for _, required := range []string{"offline_access", "read:me", "read:account", "read:jira-work", "write:jira-work", "search:jira-work"} {
		if !requested[required] {
			t.Errorf("required scope %q is not requested", required)
		}
	}
}

func TestStartOAuthFlowAuthorizesBeforeNetwork(t *testing.T) {
	f := newSvcFixture(t)
	f.svc.SetWorkspaceAuthorizer(denyOnly("ws-foreign"))
	called := false
	withDefaultOAuthTransport(t, oauthRoundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("network must not be called")
	}))

	_, err := f.svc.StartOAuthFlow(context.Background(), "ws-foreign", "https://x.atlassian.net", "http://localhost/callback")
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("expected workspace denial, got %v", err)
	}
	if called {
		t.Fatal("OAuth registration ran before authorization")
	}
}

func TestStartOAuthFlowPropagatesCancellation(t *testing.T) {
	f := newSvcFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	withDefaultOAuthTransport(t, oauthRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		return oauthResponse(http.StatusCreated, `{"client_id":"client-1"}`), nil
	}))

	_, err := f.svc.StartOAuthFlow(ctx, "ws-1", "https://x.atlassian.net", "http://localhost/callback")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestStartOAuthFlowDiscoversAuthorizationServer(t *testing.T) {
	f := newSvcFixture(t)
	var paths []string
	withDefaultOAuthTransport(t, oauthRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		switch req.URL.Path {
		case "/.well-known/oauth-protected-resource":
			return oauthResponse(http.StatusOK, `{"authorization_servers":["https://auth.example"]}`), nil
		case "/.well-known/openid-configuration":
			return oauthResponse(http.StatusOK, `{"issuer":"https://auth.example","authorization_endpoint":"https://auth.example/authorize","token_endpoint":"https://auth.example/token","registration_endpoint":"https://auth.example/register","response_types_supported":["code"]}`), nil
		case "/register":
			return oauthResponse(http.StatusCreated, `{"client_id":"client-1"}`), nil
		default:
			return oauthResponse(http.StatusNotFound, "not found"), nil
		}
	}))

	authURL, err := f.svc.StartOAuthFlow(context.Background(), "ws-1", "https://x.atlassian.net", "http://localhost/callback")
	if err != nil {
		t.Fatalf("start OAuth: %v (paths=%v)", err, paths)
	}
	if !strings.HasPrefix(authURL, "https://auth.example/authorize?") {
		t.Fatalf("authorization URL = %q", authURL)
	}
}

func TestRefreshOAuthTokenAuthorizesBeforeReadingSecrets(t *testing.T) {
	f := newSvcFixture(t)
	f.svc.SetWorkspaceAuthorizer(denyOnly("ws-foreign"))
	err := f.svc.RefreshOAuthToken(context.Background(), "ws-foreign", "stale")
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("expected workspace denial, got %v", err)
	}
}

func TestPersistRefreshedOAuthTokenOutlivesCanceledRequest(t *testing.T) {
	f := newSvcFixture(t)
	f.svc.secrets = &contextAwareSecretStore{fakeSecretStore: f.secrets}
	if err := f.store.UpsertConfigForWorkspace(context.Background(), "ws-1", &JiraConfig{
		WorkspaceID: "ws-1",
		SiteURL:     "https://x.atlassian.net",
		AuthMethod:  AuthMethodOAuth,
		ClientID:    "client-1",
		CloudID:     "cloud-1",
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := f.svc.persistRefreshedOAuthToken(ctx, "ws-1", "old-refresh", &mcpTokenResponse{
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		ExpiresIn:    300,
	})
	if err != nil {
		t.Fatalf("persist refreshed token: %v", err)
	}
	if got, _ := f.secrets.Reveal(context.Background(), OAuthRefreshTokenKeyForWorkspace("ws-1")); got != "new-refresh" {
		t.Fatalf("refresh token = %q, want new-refresh", got)
	}
	if got, _ := f.secrets.Reveal(context.Background(), OAuthAccessTokenKeyForWorkspace("ws-1")); got != "new-access" {
		t.Fatalf("access token = %q, want new-access", got)
	}
	cfg, err := f.store.GetConfigForWorkspace(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	if cfg.TokenExpiresAt == nil {
		t.Fatal("token expiry was not persisted")
	}
}

func TestPerWorkspaceMutexReleasesUnusedEntry(t *testing.T) {
	locks := newPerWorkspaceMutex()
	locks.Lock("ws-1")
	locks.Unlock("ws-1")

	locks.mu.Lock()
	defer locks.mu.Unlock()
	if len(locks.locks) != 0 {
		t.Fatalf("unused lock count = %d, want 0", len(locks.locks))
	}
}
