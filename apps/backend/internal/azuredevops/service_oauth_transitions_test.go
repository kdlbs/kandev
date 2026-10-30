package azuredevops

import (
	"context"
	"errors"
	"testing"
)

func (h *oauthTestHarness) hasSecret(key string) bool {
	h.secrets.mu.Lock()
	defer h.secrets.mu.Unlock()
	_, ok := h.secrets.values[key]
	return ok
}

func TestSavingDefaultProjectKeepsMicrosoftCredential(t *testing.T) {
	h := signedInHarness(t)
	ctx := context.Background()
	cfg, err := h.svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", AuthMethod: AuthMethodOAuth,
		DefaultProjectID: "p-2", DefaultProjectName: "Mobile",
	})
	if err != nil {
		t.Fatalf("save project: %v", err)
	}
	if cfg.AuthMethod != AuthMethodOAuth || cfg.DefaultProjectName != "Mobile" || !cfg.HasSecret ||
		cfg.OAuthAccount != "Ana Silva" {
		t.Fatalf("config = %+v", cfg)
	}
	if !h.hasSecret(OAuthRefreshTokenKey("ws-a")) {
		t.Fatal("saving the project removed the Microsoft credential")
	}
}

func TestOrganizationChangeOnMicrosoftConnectionRequiresSignIn(t *testing.T) {
	h := signedInHarness(t)
	ctx := context.Background()
	_, err := h.svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/other", AuthMethod: AuthMethodOAuth,
	})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
	if _, err := h.svc.SetConfigForWorkspace(ctx, "ws-new", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", AuthMethod: AuthMethodOAuth,
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("saving OAuth without sign-in error = %v, want ErrInvalidConfig", err)
	}
	cfg, _ := h.store.GetConfig(ctx, "ws-a")
	if cfg.OrganizationURL != "https://dev.azure.com/acme" {
		t.Fatalf("organization changed without sign-in: %+v", cfg)
	}
}

func TestSavingPATReplacesMicrosoftCredentialAfterSuccess(t *testing.T) {
	h := signedInHarness(t)
	ctx := context.Background()
	cfg, err := h.svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", AuthMethod: AuthMethodPAT, PAT: "pat-new",
	})
	if err != nil {
		t.Fatalf("switch to PAT: %v", err)
	}
	if cfg.AuthMethod != AuthMethodPAT || cfg.OAuthAccount != "" || cfg.TokenExpiresAt != nil || !cfg.HasSecret {
		t.Fatalf("config = %+v", cfg)
	}
	if h.hasSecret(OAuthRefreshTokenKey("ws-a")) || h.hasSecret(OAuthAccessTokenKey("ws-a")) {
		t.Fatal("Microsoft tokens remain after switching to PAT")
	}
	if _, password, err := h.svc.ResolveAzureDevOpsCloneCredentials(ctx, "ws-a"); err != nil || password != "pat-new" {
		t.Fatalf("clone password = %q, %v", password, err)
	}
}

func TestFailedPATSaveKeepsMicrosoftConnection(t *testing.T) {
	h := signedInHarness(t)
	ctx := context.Background()
	h.secrets.mu.Lock()
	h.secrets.setFailures = 1
	h.secrets.mu.Unlock()
	if _, err := h.svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", AuthMethod: AuthMethodPAT, PAT: "pat-new",
	}); err == nil {
		t.Fatal("PAT save must fail when the secret store fails")
	}
	if _, err := h.svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", AuthMethod: AuthMethodPAT,
	}); err == nil {
		t.Fatal("switching to PAT without a PAT must fail")
	}
	cfg, _ := h.svc.GetConfigForWorkspace(ctx, "ws-a")
	if cfg.AuthMethod != AuthMethodOAuth || !h.hasSecret(OAuthRefreshTokenKey("ws-a")) {
		t.Fatalf("Microsoft connection changed after failed PAT save: %+v", cfg)
	}
	if _, password, err := h.svc.ResolveAzureDevOpsCloneCredentials(ctx, "ws-a"); err != nil || password != "access-1" {
		t.Fatalf("clone password = %q, %v", password, err)
	}
}

func TestDeleteRemovesMicrosoftCredentialAndPendingSignIns(t *testing.T) {
	h := signedInHarness(t)
	ctx := context.Background()
	h.start(t, ctx, "ws-a")
	if err := h.svc.DeleteConfigForWorkspace(ctx, "ws-a"); err != nil {
		t.Fatal(err)
	}
	if h.hasSecret(OAuthRefreshTokenKey("ws-a")) || h.hasSecret(OAuthAccessTokenKey("ws-a")) {
		t.Fatal("Microsoft tokens remain after delete")
	}
	if count := countOAuthFlows(t, h.store); count != 0 {
		t.Fatalf("pending sign-ins after delete = %d", count)
	}
	if cfg, _ := h.store.GetConfig(ctx, "ws-a"); cfg != nil {
		t.Fatalf("config after delete = %+v", cfg)
	}
}

func TestCopyMicrosoftConnectionRequiresSignInInTarget(t *testing.T) {
	h := signedInHarness(t)
	ctx := context.Background()
	if _, err := h.svc.SetConfigForWorkspace(ctx, "ws-b", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat-b",
	}); err != nil {
		t.Fatal(err)
	}
	target, err := h.svc.CopyConfigToWorkspace(ctx, "ws-a", "ws-b")
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if target.AuthMethod != AuthMethodOAuth || !target.ReauthRequired || target.HasSecret ||
		target.DefaultProjectName != "Platform" || target.OAuthAccount != "" {
		t.Fatalf("target = %+v", target)
	}
	if h.hasSecret(OAuthRefreshTokenKey("ws-b")) || h.hasSecret(SecretKeyForWorkspace("ws-b")) {
		t.Fatal("copy must not carry or keep a credential in the target")
	}
	if _, _, err := h.svc.ResolveAzureDevOpsCloneCredentials(ctx, "ws-b"); !errors.Is(err, ErrReauthRequired) {
		t.Fatalf("target clone error = %v, want ErrReauthRequired", err)
	}
	source, _ := h.svc.GetConfigForWorkspace(ctx, "ws-a")
	if source.ReauthRequired || !source.HasSecret {
		t.Fatalf("source changed by copy: %+v", source)
	}
}

func TestTestConnectionForMicrosoftMethodWithoutSignIn(t *testing.T) {
	h := newOAuthTestHarness(t)
	result, err := h.svc.TestConnectionForWorkspace(context.Background(), "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", AuthMethod: AuthMethodOAuth,
	})
	if err != nil || result == nil || result.OK || result.Error == "" {
		t.Fatalf("result = %+v, %v; want ok=false with a sign-in error", result, err)
	}
}

func TestSignInWithoutPATDoesNotTryToRemoveOne(t *testing.T) {
	h := newOAuthTestHarness(t)
	h.secrets.mu.Lock()
	h.secrets.deleteFailures = 1
	h.secrets.mu.Unlock()
	state := h.start(t, context.Background(), "ws-a")
	if _, err := h.svc.CompleteOAuth(context.Background(), "ws-a", &CompleteOAuthRequest{Code: "c", State: state}); err != nil {
		t.Fatal(err)
	}
	h.secrets.mu.Lock()
	defer h.secrets.mu.Unlock()
	if h.secrets.deleteFailures != 1 {
		t.Fatal("sign-in tried to delete a PAT that did not exist")
	}
}
