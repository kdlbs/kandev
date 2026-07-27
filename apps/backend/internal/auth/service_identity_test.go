package auth

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

// IdentityForUser backs in-session agent MCP scoping (internal/mcp/scope),
// where the owning user is known from the agent's stream but no credential is
// presented.

func TestIdentityForUserReturnsRealStoredIdentity(t *testing.T) {
	f := newServiceFixture(t, false)
	setupEnabled(t, f)
	member, err := f.svc.AdminCreateUser(context.Background(), "member@x.dev", "memberpass123", "Member", usermodels.RoleMember)
	if err != nil {
		t.Fatalf("create member: %v", err)
	}

	identity, ok := f.svc.IdentityForUser(context.Background(), member.ID)
	if !ok {
		t.Fatal("expected the member's identity to resolve")
	}
	if identity.UserID != member.ID {
		t.Errorf("UserID = %q, want %q", identity.UserID, member.ID)
	}
	if identity.Role != authn.RoleMember {
		t.Errorf("Role = %q, want member", identity.Role)
	}
	if identity.Synthetic {
		t.Error("identity must not be synthetic — synthetic identities are treated as unscoped")
	}
	// No credential was presented, so neither credential handle is set.
	if identity.SessionID != "" || identity.TokenID != "" {
		t.Errorf("unexpected credential handles: session=%q token=%q", identity.SessionID, identity.TokenID)
	}
}

func TestIdentityForUserCarriesAdminRole(t *testing.T) {
	f := newServiceFixture(t, false)
	setupEnabled(t, f)
	admin, err := f.svc.AdminCreateUser(context.Background(), "admin2@x.dev", "adminpass123", "Admin Two", usermodels.RoleAdmin)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	identity, ok := f.svc.IdentityForUser(context.Background(), admin.ID)
	if !ok {
		t.Fatal("expected the admin's identity to resolve")
	}
	if identity.Role != authn.RoleAdmin {
		t.Errorf("Role = %q, want admin", identity.Role)
	}
}

// TestIdentityForUserFalseWhenAuthDisabled keeps the opt-out path unscoped.
func TestIdentityForUserFalseWhenAuthDisabled(t *testing.T) {
	f := newServiceFixture(t, false)

	if _, ok := f.svc.IdentityForUser(context.Background(), "some-user"); ok {
		t.Error("auth disabled must not produce a scoping identity")
	}
}

func TestIdentityForUserFalseForUnknownUser(t *testing.T) {
	f := newServiceFixture(t, false)
	setupEnabled(t, f)

	if _, ok := f.svc.IdentityForUser(context.Background(), "no-such-user"); ok {
		t.Error("an unknown user must not resolve")
	}
}

func TestIdentityForUserFalseForDisabledUser(t *testing.T) {
	f := newServiceFixture(t, false)
	setupEnabled(t, f)
	ctx := context.Background()
	member, err := f.svc.AdminCreateUser(ctx, "gone@x.dev", "memberpass123", "Gone", usermodels.RoleMember)
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	if _, err := f.svc.AdminSetRoleStatus(ctx, member.ID, usermodels.RoleMember, usermodels.StatusDisabled); err != nil {
		t.Fatalf("disable member: %v", err)
	}

	if _, ok := f.svc.IdentityForUser(ctx, member.ID); ok {
		t.Error("a disabled user must not resolve")
	}
}

func TestIdentityForUserFalseForEmptyUserID(t *testing.T) {
	f := newServiceFixture(t, false)
	setupEnabled(t, f)

	if _, ok := f.svc.IdentityForUser(context.Background(), ""); ok {
		t.Error("an empty user ID must not resolve")
	}
}
