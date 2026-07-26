package slack

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// denyOnly returns a workspace authorizer that allows every workspace except
// the named ones, which it denies with the same ErrWorkspaceNotFound the real
// task-service authorizer returns.
func denyOnly(denied ...string) func(context.Context, string) error {
	blocked := make(map[string]bool, len(denied))
	for _, ws := range denied {
		blocked[ws] = true
	}
	return func(_ context.Context, ws string) error {
		if blocked[ws] {
			return repoerrors.ErrWorkspaceNotFound
		}
		return nil
	}
}

// TestCopyConfigToWorkspace_DeniesForeignTarget is the regression for the HIGH
// finding: the copy target arrives in the request body, which the query-only
// integration middleware never authorizes, so without a service-layer check a
// caller could copy their config + credentials into another user's workspace.
func TestCopyConfigToWorkspace_DeniesForeignTarget(t *testing.T) {
	ctx := context.Background()
	secrets := newFakeSecrets()
	svc := newCopyTestService(t, secrets)
	done := make(chan struct{}, 4)
	svc.SetProbeHook(func() { done <- struct{}{} })

	const src, victim = "ws-src", "ws-victim"
	if _, err := svc.SetConfigForWorkspace(ctx, src, &SetConfigRequest{
		AuthMethod: AuthMethodCookie,
		Token:      "xoxc-attacker",
		Cookie:     "d-attacker",
	}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	<-done

	// The victim already has a healthy Slack connection of their own. A denied
	// copy must leave it byte-for-byte untouched — including its health status,
	// which the up-front guard protects from the pre-write UpdateAuthHealth flip.
	if err := svc.store.UpsertConfigForWorkspace(ctx, victim, &SlackConfig{
		AuthMethod: AuthMethodCookie, CommandPrefix: "!vic", UtilityAgentID: "ua-vic",
	}); err != nil {
		t.Fatalf("seed victim config: %v", err)
	}
	if err := secrets.Set(ctx, SecretKeyForToken(victim), "Slack token", "xoxc-victim"); err != nil {
		t.Fatalf("seed victim token: %v", err)
	}
	if err := secrets.Set(ctx, SecretKeyForCookie(victim), "Slack cookie", "d-victim"); err != nil {
		t.Fatalf("seed victim cookie: %v", err)
	}
	if err := svc.store.UpdateAuthHealthForWorkspace(ctx, victim, true, "", "", "", time.Now().UTC()); err != nil {
		t.Fatalf("seed victim health: %v", err)
	}

	svc.SetWorkspaceAuthorizer(denyOnly(victim))

	if _, err := svc.CopyConfigToWorkspace(ctx, src, victim); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
	got, cfgErr := svc.store.GetConfigForWorkspace(ctx, victim)
	if cfgErr != nil || got == nil {
		t.Fatalf("victim config vanished: cfg=%+v err=%v", got, cfgErr)
	}
	if got.CommandPrefix != "!vic" {
		t.Errorf("victim config overwritten by copy: %q", got.CommandPrefix)
	}
	if !got.LastOk {
		t.Errorf("victim health flipped to unhealthy by unauthorized copy")
	}
	if v := secrets.get(SecretKeyForToken(victim)); v != "xoxc-victim" {
		t.Errorf("victim token overwritten by copy: %q", v)
	}
	if v := secrets.get(SecretKeyForCookie(victim)); v != "d-victim" {
		t.Errorf("victim cookie overwritten by copy: %q", v)
	}
}

// TestConfigForWorkspace_DeniesForeignWorkspace is the regression for the LOW
// finding: a scoped caller must not read, write, or delete a workspace's config
// it does not own.
func TestConfigForWorkspace_DeniesForeignWorkspace(t *testing.T) {
	ctx := context.Background()
	svc := newCopyTestService(t, newFakeSecrets())
	const foreign = "ws-foreign"
	svc.SetWorkspaceAuthorizer(denyOnly(foreign))

	if _, err := svc.GetConfigForWorkspace(ctx, foreign); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Errorf("Get: expected ErrWorkspaceNotFound, got %v", err)
	}
	if _, err := svc.SetConfigForWorkspace(ctx, foreign, &SetConfigRequest{
		AuthMethod: AuthMethodCookie, Token: "t", Cookie: "c",
	}); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Errorf("Set: expected ErrWorkspaceNotFound, got %v", err)
	}
	if err := svc.DeleteConfigForWorkspace(ctx, foreign); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Errorf("Delete: expected ErrWorkspaceNotFound, got %v", err)
	}
}
