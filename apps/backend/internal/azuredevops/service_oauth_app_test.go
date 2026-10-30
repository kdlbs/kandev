package azuredevops

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"
)

const (
	workspaceClientID    = "04b07795-8ddb-461a-bbee-02f9e1bf7b46"
	workspaceRedirectURL = "http://localhost:48429"
)

func TestStartOAuthUsesWorkspaceApplicationWithoutInstallationDefault(t *testing.T) {
	h := newOAuthTestHarness(t)
	h.svc.ConfigureOAuth(OAuthSettings{RedirectURL: testOAuthRedirectURL})
	ctx := context.Background()
	started, err := h.svc.StartOAuth(ctx, "ws-a", &StartOAuthRequest{
		OrganizationURL: "https://dev.azure.com/acme",
		ClientID:        workspaceClientID,
		RedirectURL:     workspaceRedirectURL,
	})
	if err != nil {
		t.Fatalf("StartOAuth: %v", err)
	}
	query := mustParseQuery(t, started.AuthURL)
	if query.Get("client_id") != workspaceClientID || query.Get("redirect_uri") != workspaceRedirectURL {
		t.Fatalf("authorize URL = %s", started.AuthURL)
	}
	cfg, err := h.svc.CompleteOAuth(ctx, "ws-a", &CompleteOAuthRequest{Code: "c", State: started.State})
	if err != nil {
		t.Fatalf("CompleteOAuth: %v", err)
	}
	if h.entra.clientIDs[0] != workspaceClientID || h.entra.redirects[0] != workspaceRedirectURL {
		t.Fatalf("exchange client/redirect = %v / %v", h.entra.clientIDs, h.entra.redirects)
	}
	if cfg.OAuthClientID != workspaceClientID || cfg.OAuthRedirectURL != workspaceRedirectURL {
		t.Fatalf("config application = %q / %q", cfg.OAuthClientID, cfg.OAuthRedirectURL)
	}
}

func TestRenewalUsesTheClientThatIssuedTheRefreshToken(t *testing.T) {
	h := newOAuthTestHarness(t)
	ctx := context.Background()
	started, err := h.svc.StartOAuth(ctx, "ws-a", &StartOAuthRequest{
		OrganizationURL: "https://dev.azure.com/acme", ClientID: workspaceClientID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.CompleteOAuth(ctx, "ws-a", &CompleteOAuthRequest{Code: "c", State: started.State}); err != nil {
		t.Fatal(err)
	}
	h.svc.ConfigureOAuth(OAuthSettings{ClientID: "11111111-1111-1111-1111-111111111111", RedirectURL: testOAuthRedirectURL})
	h.expireAccessToken(t, -time.Minute)
	if _, err := h.svc.accessToken(ctx, "ws-a"); err != nil {
		t.Fatal(err)
	}
	if got := h.entra.clientIDs[len(h.entra.clientIDs)-1]; got != workspaceClientID {
		t.Fatalf("refresh client = %q, want the client that signed in", got)
	}
}

func TestStartOAuthRejectsInvalidApplicationSettings(t *testing.T) {
	h := newOAuthTestHarness(t)
	for name, req := range map[string]StartOAuthRequest{
		"client not a GUID":     {ClientID: "not-a-guid"},
		"redirect not loopback": {RedirectURL: "http://kandev.example.com/cb"},
		"redirect with query":   {RedirectURL: "http://localhost:48429/?next=x"},
		"redirect not absolute": {RedirectURL: "/api/v1/azure-devops/oauth/callback"},
	} {
		t.Run(name, func(t *testing.T) {
			req.OrganizationURL = "https://dev.azure.com/acme"
			if _, err := h.svc.StartOAuth(context.Background(), "ws-a", &req); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestStartOAuthWithoutAnyApplicationIsUnavailable(t *testing.T) {
	h := newOAuthTestHarness(t)
	h.svc.ConfigureOAuth(OAuthSettings{RedirectURL: testOAuthRedirectURL})
	_, err := h.svc.StartOAuth(context.Background(), "ws-a", &StartOAuthRequest{OrganizationURL: "https://dev.azure.com/acme"})
	if !errors.Is(err, ErrOAuthUnavailable) {
		t.Fatalf("error = %v, want ErrOAuthUnavailable", err)
	}
}

func mustParseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Query()
}
