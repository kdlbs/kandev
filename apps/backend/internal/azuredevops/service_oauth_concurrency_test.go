package azuredevops

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestRenewalInFlightCannotOverwriteANewSignIn(t *testing.T) {
	h := signedInHarness(t)
	ctx := context.Background()
	h.expireAccessToken(t, -time.Minute)
	h.entra.refreshEntered = make(chan struct{})
	h.entra.refreshRelease = make(chan struct{})

	renewed := make(chan error, 1)
	go func() {
		_, err := h.svc.accessToken(ctx, "ws-a")
		renewed <- err
	}()
	<-h.entra.refreshEntered

	state := h.start(t, ctx, "ws-a")
	signedIn := make(chan error, 1)
	go func() {
		_, err := h.svc.CompleteOAuth(ctx, "ws-a", &CompleteOAuthRequest{Code: "c", State: state})
		signedIn <- err
	}()
	select {
	case err := <-signedIn:
		// Without serialization the sign-in finishes while renewal is paused.
		signedIn <- err
	case <-time.After(200 * time.Millisecond):
	}
	close(h.entra.refreshRelease)
	if err := <-renewed; err != nil {
		t.Fatalf("renewal: %v", err)
	}
	if err := <-signedIn; err != nil {
		t.Fatalf("sign-in: %v", err)
	}
	if got := h.secrets.values[OAuthAccessTokenKey("ws-a")]; got != "access-2" {
		t.Fatalf("stored access token = %q, want the new sign-in's access-2", got)
	}
	if got := h.secrets.values[OAuthRefreshTokenKey("ws-a")]; got != "refresh-2" {
		t.Fatalf("stored refresh token = %q, want the new sign-in's refresh-2", got)
	}
}

func TestFailedReSignInKeepsThePreviousCredential(t *testing.T) {
	h := signedInHarness(t)
	ctx := context.Background()
	h.secrets.mu.Lock()
	h.secrets.failSetKeys = map[string]bool{OAuthAccessTokenKey("ws-a"): true}
	h.secrets.mu.Unlock()

	state := h.start(t, ctx, "ws-a")
	if _, err := h.svc.CompleteOAuth(ctx, "ws-a", &CompleteOAuthRequest{Code: "c", State: state}); err == nil {
		t.Fatal("re-sign-in must fail when the access token cannot be stored")
	}
	if got := h.secrets.values[OAuthRefreshTokenKey("ws-a")]; got != "refresh-1" {
		t.Fatalf("refresh token after failed re-sign-in = %q, want previous refresh-1", got)
	}
	if got := h.secrets.values[OAuthAccessTokenKey("ws-a")]; got != "access-1" {
		t.Fatalf("access token after failed re-sign-in = %q, want previous access-1", got)
	}
	if _, password, err := h.svc.ResolveAzureDevOpsCloneCredentials(ctx, "ws-a"); err != nil || password != "access-1" {
		t.Fatalf("clone credentials = %q, %v; want the previous connection", password, err)
	}
}

func TestConsumeOAuthFlowSurvivesConcurrentPurges(t *testing.T) {
	ctx := context.Background()
	_, store, _ := newTestService(t, nil)
	now := time.Now().UTC()
	const flows = 40
	for i := range flows {
		if err := store.InsertOAuthFlow(ctx, newTestOAuthFlow(fmt.Sprintf("s-%d", i), "ws-a", now.Add(time.Minute)), now); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	failures := make(chan string, flows)
	for i := range flows {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			flow, err := store.ConsumeOAuthFlow(ctx, fmt.Sprintf("s-%d", i), time.Now().UTC())
			if err != nil || flow == nil {
				failures <- fmt.Sprintf("s-%d: %v", i, err)
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			_ = store.InsertOAuthFlow(ctx, newTestOAuthFlow(fmt.Sprintf("other-%d", i), "ws-b", time.Now().Add(time.Minute)), time.Now().UTC())
		}(i)
	}
	wg.Wait()
	close(failures)
	for failure := range failures {
		t.Errorf("valid flow lost: %s", failure)
	}
}
