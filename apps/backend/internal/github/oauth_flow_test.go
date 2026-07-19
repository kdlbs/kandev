package github

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type oauthFlowMemoryStore struct {
	flows map[string]*AuthFlow
}

func (s *oauthFlowMemoryStore) CreateAuthFlow(_ context.Context, flow *AuthFlow) error {
	if s.flows == nil {
		s.flows = make(map[string]*AuthFlow)
	}
	copy := *flow
	s.flows[flow.StateHash] = &copy
	return nil
}

func (s *oauthFlowMemoryStore) ConsumeAuthFlow(_ context.Context, stateHash string, now time.Time) (*AuthFlow, error) {
	flow := s.flows[stateHash]
	if flow == nil || flow.ConsumedAt != nil || !flow.ExpiresAt.After(now) {
		return nil, ErrAuthFlowUnavailable
	}
	copy := *flow
	copy.ConsumedAt = &now
	flow.ConsumedAt = &now
	return &copy, nil
}

func TestOAuthFlowStartPersistsOnlyStateDigestAndPKCEVerifier(t *testing.T) {
	store := &oauthFlowMemoryStore{}
	manager := NewOAuthFlowManager(store)
	manager.now = func() time.Time { return time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC) }
	manager.random = strings.NewReader(strings.Repeat("a", oauthRandomBytes*2))

	installationID := int64(42)
	started, err := manager.Start(context.Background(), OAuthFlowRequest{
		WorkspaceID:                 "workspace-1",
		UserID:                      "user-1",
		Kind:                        AuthFlowKindPersonal,
		ExpectedWorkspaceSource:     ConnectionSourceGitHubAppInstallation,
		ExpectedWorkspaceGeneration: 3,
		ExpectedInstallationID:      &installationID,
		ExpectedPersonalGeneration:  7,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if started.State == "" || started.PKCEChallenge == "" {
		t.Fatalf("Start() = %+v, want state and PKCE challenge", started)
	}
	digest := sha256.Sum256([]byte(started.State))
	persisted := store.flows[stateDigestString(digest)]
	if persisted == nil {
		t.Fatal("state digest was not persisted")
	}
	if strings.Contains(persisted.StateHash, started.State) || persisted.PKCEVerifier == "" {
		t.Fatalf("persisted flow leaked state or omitted verifier: %+v", persisted)
	}
	if got, want := persisted.ExpiresAt.Sub(manager.now()), personalOAuthStateTTL; got != want {
		t.Fatalf("flow TTL = %v, want %v", got, want)
	}
	if persisted.ExpectedWorkspaceSource != ConnectionSourceGitHubAppInstallation ||
		persisted.ExpectedWorkspaceGeneration != 3 || persisted.ExpectedInstallationID == nil ||
		*persisted.ExpectedInstallationID != 42 || persisted.ExpectedPersonalGeneration != 7 {
		t.Fatalf("persisted flow expectation = %+v", persisted)
	}
}

func TestOAuthFlowConsumeRejectsMismatchAndReplay(t *testing.T) {
	store := &oauthFlowMemoryStore{}
	manager := NewOAuthFlowManager(store)
	manager.now = func() time.Time { return time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC) }
	manager.random = io.LimitReader(strings.NewReader(strings.Repeat("b", oauthRandomBytes)), oauthRandomBytes)

	started, err := manager.Start(context.Background(), OAuthFlowRequest{
		WorkspaceID: "workspace-1", UserID: "user-1", Kind: AuthFlowKindAppInstallation,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	_, err = manager.Consume(context.Background(), started.State, OAuthFlowExpectation{
		WorkspaceID: "other-workspace", UserID: "user-1", Kind: AuthFlowKindAppInstallation,
	})
	if !errors.Is(err, ErrOAuthStateMismatch) {
		t.Fatalf("Consume() mismatch error = %v, want %v", err, ErrOAuthStateMismatch)
	}
	_, err = manager.Consume(context.Background(), started.State, OAuthFlowExpectation{
		WorkspaceID: "workspace-1", UserID: "user-1", Kind: AuthFlowKindAppInstallation,
	})
	if !errors.Is(err, ErrOAuthStateInvalid) {
		t.Fatalf("Consume() replay error = %v, want %v", err, ErrOAuthStateInvalid)
	}
}

func TestOAuthFlowConsumeRejectsExpiredState(t *testing.T) {
	store := &oauthFlowMemoryStore{}
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	manager := NewOAuthFlowManager(store)
	manager.now = func() time.Time { return now }
	manager.random = strings.NewReader(strings.Repeat("c", oauthRandomBytes))
	started, err := manager.Start(context.Background(), OAuthFlowRequest{
		WorkspaceID: "workspace-1", UserID: "user-1", Kind: AuthFlowKindAppInstallation,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	manager.now = func() time.Time { return now.Add(appInstallationStateTTL + time.Second) }
	_, err = manager.Consume(context.Background(), started.State, OAuthFlowExpectation{
		WorkspaceID: "workspace-1", UserID: "user-1", Kind: AuthFlowKindAppInstallation,
	})
	if !errors.Is(err, ErrOAuthStateInvalid) {
		t.Fatalf("Consume() expired error = %v, want %v", err, ErrOAuthStateInvalid)
	}
}

func TestOAuthFlowGitHubClientExchangesRefreshesAndVerifiesUser(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if r.Form.Get("client_id") != "client-id" || r.Form.Get("client_secret") != "client-secret" {
				t.Fatalf("OAuth client credentials = %v", r.Form)
			}
			if r.Form.Get("grant_type") == "refresh_token" {
				if r.Form.Get("refresh_token") != "old-refresh" {
					t.Fatalf("refresh form = %v", r.Form)
				}
			} else if r.Form.Get("code") != "code" || r.Form.Get("code_verifier") != "verifier" {
				t.Fatalf("exchange form = %v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access", "refresh_token": "refresh",
				"expires_in": 28800, "refresh_token_expires_in": 15552000,
			})
		case "/user":
			if r.Header.Get("Authorization") != "Bearer access" {
				t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 11, "login": "octocat"})
		case "/user/installations/42":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewGitHubOAuthClient("client-id", "client-secret")
	client.webBaseURL = server.URL
	client.apiBaseURL = server.URL
	client.httpClient = server.Client()
	client.now = func() time.Time { return now }
	tokens, err := client.ExchangeUserCode(context.Background(), "code", "verifier", "https://kandev.example/callback")
	if err != nil {
		t.Fatalf("ExchangeUserCode() error = %v", err)
	}
	if tokens.AccessExpiresAt != now.Add(8*time.Hour) || tokens.RefreshExpiresAt == nil {
		t.Fatalf("ExchangeUserCode() = %+v", tokens)
	}
	if _, err := client.RefreshUserToken(context.Background(), "old-refresh"); err != nil {
		t.Fatalf("RefreshUserToken() error = %v", err)
	}
	user, err := client.GetOAuthUser(context.Background(), tokens.AccessToken)
	if err != nil || user.ID != 11 || user.Login != "octocat" {
		t.Fatalf("GetOAuthUser() = %+v, %v", user, err)
	}
	accessible, err := client.UserCanAccessInstallation(context.Background(), tokens.AccessToken, 42)
	if err != nil || !accessible {
		t.Fatalf("UserCanAccessInstallation() = %v, %v", accessible, err)
	}
	accessible, err = client.UserCanAccessInstallation(context.Background(), tokens.AccessToken, 99)
	if err != nil || accessible {
		t.Fatalf("missing UserCanAccessInstallation() = %v, %v", accessible, err)
	}
}
