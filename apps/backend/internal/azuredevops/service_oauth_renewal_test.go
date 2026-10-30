package azuredevops

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// signedInHarness returns a harness whose ws-a is connected with access-1 /
// refresh-1, expiring in an hour.
func signedInHarness(t *testing.T) *oauthTestHarness {
	t.Helper()
	h := newOAuthTestHarness(t)
	ctx := context.Background()
	state := h.start(t, ctx, "ws-a")
	if _, err := h.svc.CompleteOAuth(ctx, "ws-a", &CompleteOAuthRequest{Code: "c", State: state}); err != nil {
		t.Fatalf("CompleteOAuth: %v", err)
	}
	return h
}

func (h *oauthTestHarness) expireAccessToken(t *testing.T, in time.Duration) {
	t.Helper()
	if err := h.store.UpdateOAuthTokenExpiry(context.Background(), "ws-a", time.Now().UTC().Add(in)); err != nil {
		t.Fatal(err)
	}
}

func (h *oauthTestHarness) refreshCalls() []string {
	h.entra.mu.Lock()
	defer h.entra.mu.Unlock()
	return append([]string(nil), h.entra.refreshTokens...)
}

func TestAccessTokenUsesStoredTokenWhileFresh(t *testing.T) {
	h := signedInHarness(t)
	_, password, err := h.svc.ResolveAzureDevOpsCloneCredentials(context.Background(), "ws-a")
	if err != nil || password != "access-1" {
		t.Fatalf("clone password = %q, %v", password, err)
	}
	if calls := h.refreshCalls(); len(calls) != 0 {
		t.Fatalf("fresh token was renewed: %v", calls)
	}
}

func TestAccessTokenRenewsNearExpiryAndPersistsRotation(t *testing.T) {
	h := signedInHarness(t)
	h.expireAccessToken(t, time.Minute)
	_, password, err := h.svc.ResolveAzureDevOpsCloneCredentials(context.Background(), "ws-a")
	if err != nil || password != "access-2" {
		t.Fatalf("clone password = %q, %v; want renewed access-2", password, err)
	}
	if calls := h.refreshCalls(); len(calls) != 1 || calls[0] != "refresh-1" {
		t.Fatalf("refresh calls = %v", calls)
	}
	if got := h.secrets.values[OAuthRefreshTokenKey("ws-a")]; got != "refresh-2" {
		t.Fatalf("stored refresh token = %q, want rotated refresh-2", got)
	}
	cfg, _ := h.store.GetConfig(context.Background(), "ws-a")
	if cfg.TokenExpiresAt == nil || time.Until(*cfg.TokenExpiresAt) < 50*time.Minute {
		t.Fatalf("token expiry after renewal = %v", cfg.TokenExpiresAt)
	}
}

func TestConcurrentRenewalIssuesOneTokenRequest(t *testing.T) {
	h := signedInHarness(t)
	h.expireAccessToken(t, -time.Minute)
	var wg sync.WaitGroup
	results := make([]string, 8)
	errs := make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = h.svc.accessToken(context.Background(), "ws-a")
		}(i)
	}
	wg.Wait()
	for i := range results {
		if errs[i] != nil || results[i] != "access-2" {
			t.Fatalf("caller %d = %q, %v", i, results[i], errs[i])
		}
	}
	if calls := h.refreshCalls(); len(calls) != 1 {
		t.Fatalf("refresh calls = %v, want exactly one", calls)
	}
}

func TestRenewSkipsWhenRejectedTokenWasAlreadyRotated(t *testing.T) {
	h := signedInHarness(t)
	ctx := context.Background()
	for range 2 {
		renewed, err := h.svc.renewAccessToken(ctx, "ws-a", "access-1")
		if err != nil || !renewed {
			t.Fatalf("renew = %v, %v", renewed, err)
		}
	}
	if calls := h.refreshCalls(); len(calls) != 1 {
		t.Fatalf("refresh calls = %v, want one for the same rejected token", calls)
	}
}

func TestRenewalRequiringSignInFlagsConnectionAndStopsCallingMicrosoft(t *testing.T) {
	h := signedInHarness(t)
	ctx := context.Background()
	h.entra.refreshErr = &EntraError{Code: "invalid_grant"}
	h.expireAccessToken(t, -time.Minute)
	for range 2 {
		if _, _, err := h.svc.ResolveAzureDevOpsCloneCredentials(ctx, "ws-a"); !errors.Is(err, ErrReauthRequired) {
			t.Fatalf("error = %v, want ErrReauthRequired", err)
		}
	}
	if calls := h.refreshCalls(); len(calls) != 1 {
		t.Fatalf("refresh calls = %v, want no retries after a terminal failure", calls)
	}
	cfg, _ := h.svc.GetConfigForWorkspace(ctx, "ws-a")
	if !cfg.ReauthRequired || cfg.DefaultProjectName != "Platform" {
		t.Fatalf("config after terminal failure = %+v", cfg)
	}
	h.svc.RecordAuthHealthForWorkspace(ctx, "ws-a")
	stored, _ := h.store.GetConfig(ctx, "ws-a")
	if stored.LastOK || stored.LastError == "" {
		t.Fatalf("health after terminal failure = ok:%v error:%q", stored.LastOK, stored.LastError)
	}

	h.entra.refreshErr = nil
	state := h.start(t, ctx, "ws-a")
	cfg, err := h.svc.CompleteOAuth(ctx, "ws-a", &CompleteOAuthRequest{Code: "c", State: state})
	if err != nil || cfg.ReauthRequired || !cfg.LastOK {
		t.Fatalf("re-sign-in = %+v, %v; want flag cleared and healthy", cfg, err)
	}
}

func TestTransientRenewalFailureDoesNotRequireSignIn(t *testing.T) {
	h := signedInHarness(t)
	ctx := context.Background()
	h.entra.refreshErr = &EntraError{Code: "temporarily_unavailable"}
	h.expireAccessToken(t, -time.Minute)
	if _, err := h.svc.accessToken(ctx, "ws-a"); err == nil || errors.Is(err, ErrReauthRequired) {
		t.Fatalf("error = %v, want a transient failure", err)
	}
	cfg, _ := h.store.GetConfig(ctx, "ws-a")
	if cfg.ReauthRequired {
		t.Fatal("transient failure must not require sign-in")
	}
	h.entra.refreshErr = nil
	if token, err := h.svc.accessToken(ctx, "ws-a"); err != nil || token != "access-2" {
		t.Fatalf("retry after transient failure = %q, %v", token, err)
	}
}
