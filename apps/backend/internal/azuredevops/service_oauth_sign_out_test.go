package azuredevops

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestSignOutRemovesMicrosoftCredentialAndKeepsSettings(t *testing.T) {
	h := signedInHarness(t)
	ctx := context.Background()
	before, _ := h.store.GetConfig(ctx, "ws-a")

	cfg, err := h.svc.SignOutOAuth(ctx, "ws-a")
	if err != nil {
		t.Fatalf("SignOutOAuth: %v", err)
	}
	if h.hasSecret(OAuthRefreshTokenKey("ws-a")) || h.hasSecret(OAuthAccessTokenKey("ws-a")) {
		t.Fatal("Microsoft tokens remain after sign-out")
	}
	if cfg.HasSecret || cfg.AuthMethod != AuthMethodOAuth || cfg.OAuthAccount != "" || cfg.TokenExpiresAt != nil {
		t.Fatalf("config after sign-out = %+v", cfg)
	}
	if cfg.OrganizationURL != before.OrganizationURL || cfg.DefaultProjectName != before.DefaultProjectName ||
		cfg.OAuthClientID != before.OAuthClientID || cfg.OAuthTenantID != before.OAuthTenantID {
		t.Fatalf("settings changed by sign-out: before %+v after %+v", before, cfg)
	}
	if cfg.LastCheckedAt != nil || cfg.LastOK {
		t.Fatalf("auth health after sign-out = checked:%v ok:%v, want reset", cfg.LastCheckedAt, cfg.LastOK)
	}
	if _, _, err := h.svc.ResolveAzureDevOpsCloneCredentials(ctx, "ws-a"); !errors.Is(err, ErrReauthRequired) {
		t.Fatalf("clone after sign-out error = %v, want ErrReauthRequired", err)
	}
	if calls := h.refreshCalls(); len(calls) != 0 {
		t.Fatalf("sign-out reached Microsoft: %v", calls)
	}

	state := h.start(t, ctx, "ws-a")
	again, err := h.svc.CompleteOAuth(ctx, "ws-a", &CompleteOAuthRequest{Code: "c", State: state})
	if err != nil || !again.HasSecret || again.ReauthRequired {
		t.Fatalf("sign-in after sign-out = %+v, %v", again, err)
	}
}

func TestSignOutRejectsNonMicrosoftConnections(t *testing.T) {
	h := newOAuthTestHarness(t)
	ctx := context.Background()
	if _, err := h.svc.SignOutOAuth(ctx, "ws-missing"); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("unconfigured sign-out error = %v, want ErrInvalidConfig", err)
	}
	if _, err := h.svc.SetConfigForWorkspace(ctx, "ws-pat", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat-a",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.SignOutOAuth(ctx, "ws-pat"); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("PAT sign-out error = %v, want ErrInvalidConfig", err)
	}
	if !h.hasSecret(SecretKeyForWorkspace("ws-pat")) {
		t.Fatal("sign-out removed a PAT")
	}
}

func TestSignOutRoute(t *testing.T) {
	router, h := newOAuthRouteFixture(t)
	state := h.start(t, context.Background(), "ws-1")
	if _, err := h.svc.CompleteOAuth(context.Background(), "ws-1", &CompleteOAuthRequest{Code: "c", State: state}); err != nil {
		t.Fatal(err)
	}
	signedOut := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/oauth/sign-out?workspace_id=ws-1", nil)
	if signedOut.Code != http.StatusOK || decodeJSON[Config](t, signedOut).HasSecret {
		t.Fatalf("sign-out = %d %s", signedOut.Code, signedOut.Body.String())
	}
	again := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/oauth/sign-out?workspace_id=ws-2", nil)
	if again.Code != http.StatusBadRequest {
		t.Fatalf("sign-out without a Microsoft connection = %d, want 400", again.Code)
	}
}
