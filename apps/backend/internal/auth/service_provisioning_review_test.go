package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/store"
	userstore "github.com/kandev/kandev/internal/user/store"
)

func TestSetupPlacementFailureLeavesSetupRetryable(t *testing.T) {
	fixture := newServiceFixture(t, true)
	placementErr := errors.New("place setup administrator")
	fixture.svc.SetAdminCreatedHook(func(context.Context, string) error { return placementErr })

	user, token, err := fixture.svc.Setup(
		context.Background(), "admin@example.com", "password123", "Admin", "browser", "127.0.0.1",
	)
	if !errors.Is(err, placementErr) {
		t.Fatalf("setup error = %v, want placement error", err)
	}
	if user != nil || token != "" {
		t.Fatalf("failed setup returned user=%+v token=%q", user, token)
	}
	if fixture.svc.Mode() != ModeSetup {
		t.Fatalf("mode after failed placement = %q, want setup", fixture.svc.Mode())
	}

	fixture.svc.SetAdminCreatedHook(func(context.Context, string) error { return nil })
	if _, retryToken, retryErr := fixture.svc.Setup(
		context.Background(), "admin@example.com", "password123", "Admin", "browser", "127.0.0.1",
	); retryErr != nil || retryToken == "" {
		t.Fatalf("retry setup token=%q error=%v", retryToken, retryErr)
	}
}

func TestCreateUserInOrgRollsBackAccountWhenIdentityCreationFails(t *testing.T) {
	fixture := newServiceFixture(t, false)
	ctx := context.Background()
	const email = "first-admin@example.com"
	if err := fixture.svc.store.CreateIdentity(ctx, &store.LoginIdentity{
		UserID: userstore.DefaultUserID, Provider: store.ProviderLocal, Subject: email,
	}); err != nil {
		t.Fatalf("seed colliding identity: %v", err)
	}

	err := fixture.svc.CreateUserInOrg(ctx, "org-a", email, "password123", "First Admin")
	if err == nil {
		t.Fatal("CreateUserInOrg succeeded despite identity collision")
	}
	if user, lookupErr := fixture.svc.users.GetUserByEmail(ctx, email); lookupErr == nil || user != nil {
		t.Fatalf("failed provisioning left account user=%+v error=%v", user, lookupErr)
	}
}
