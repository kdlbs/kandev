package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/store"
	usermodels "github.com/kandev/kandev/internal/user/models"
	userstore "github.com/kandev/kandev/internal/user/store"
)

const testOIDCProvider = "oidc:https://issuer.example"

var errBoomSSO = errors.New("transient store failure")

// erroringUsers overrides GetUserByEmail to return a non-not-found error; every
// other AccountRepository method is left nil (this path never calls them).
type erroringUsers struct {
	userstore.AccountRepository
	err error
}

func (e erroringUsers) GetUserByEmail(context.Context, string) (*usermodels.User, error) {
	return nil, e.err
}

func TestAuthenticateExternalRejectedWhenDisabled(t *testing.T) {
	f := newServiceFixture(t, false)
	ext := ExternalIdentity{Provider: testOIDCProvider, Subject: "sub-1", Email: "a@b.dev"}
	if _, _, err := f.svc.AuthenticateExternal(context.Background(), ext, "", ""); !errors.Is(err, ErrSSONotEnabled) {
		t.Fatalf("disabled mode: got %v, want ErrSSONotEnabled", err)
	}
}

func TestAuthenticateExternalInvalidAssertions(t *testing.T) {
	f := newServiceFixture(t, false)
	setupEnabled(t, f)
	ctx := context.Background()

	cases := map[string]ExternalIdentity{
		"empty provider":    {Provider: "", Subject: "s", Email: "a@b.dev"},
		"local provider":    {Provider: "local", Subject: "s", Email: "a@b.dev"},
		"empty subject":     {Provider: testOIDCProvider, Subject: "", Email: "a@b.dev"},
		"new without email": {Provider: testOIDCProvider, Subject: "s-noemail"},
	}
	for name, ext := range cases {
		if _, _, err := f.svc.AuthenticateExternal(ctx, ext, "", ""); !errors.Is(err, ErrSSOInvalidAssertion) {
			t.Fatalf("%s: got %v, want ErrSSOInvalidAssertion", name, err)
		}
	}
}

func TestAuthenticateExternalProvisionsMember(t *testing.T) {
	f := newServiceFixture(t, false)
	setupEnabled(t, f)
	ctx := context.Background()

	ext := ExternalIdentity{Provider: testOIDCProvider, Subject: "sub-new", Email: "New.User@Example.com", DisplayName: "New User"}
	user, token, err := f.svc.AuthenticateExternal(ctx, ext, "ua", "1.2.3.4")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if user.Email != "new.user@example.com" || user.Role != usermodels.RoleMember {
		t.Fatalf("provisioned user = %+v, want normalized member", user)
	}
	// The minted session authenticates as a non-admin member.
	identity, ok := f.svc.ResolveSessionToken(ctx, token, "")
	if !ok || identity.UserID != user.ID || identity.IsAdmin() {
		t.Fatalf("session identity = %+v ok=%v, want member %s", identity, ok, user.ID)
	}

	// A second login for the same (provider, subject) reuses the account —
	// no duplicate provisioning.
	again, _, err := f.svc.AuthenticateExternal(ctx, ext, "ua", "1.2.3.4")
	if err != nil {
		t.Fatalf("second authenticate: %v", err)
	}
	if again.ID != user.ID {
		t.Fatalf("second login created a new user %s (first %s)", again.ID, user.ID)
	}
}

func TestAuthenticateExternalRefusesAdminLink(t *testing.T) {
	f := newServiceFixture(t, false)
	setupEnabled(t, f) // creates admin@x.dev (admin)
	ctx := context.Background()

	// Asserting the admin's email must NOT auto-link / log in as the admin.
	ext := ExternalIdentity{Provider: testOIDCProvider, Subject: "sub-admin", Email: "admin@x.dev"}
	if _, _, err := f.svc.AuthenticateExternal(ctx, ext, "ua", "1.2.3.4"); !errors.Is(err, ErrSSOAdminLinkForbidden) {
		t.Fatalf("admin auto-link: got %v, want ErrSSOAdminLinkForbidden", err)
	}
	// No identity row was created for the admin.
	if _, err := f.svc.store.GetIdentityByProviderSubject(ctx, testOIDCProvider, "sub-admin"); err == nil {
		t.Fatal("admin auto-link must not create an identity row")
	}
}

func TestAuthenticateExternalLinksExistingMemberByEmail(t *testing.T) {
	f := newServiceFixture(t, false)
	setupEnabled(t, f)
	ctx := context.Background()

	// A local-password member adopting SSO is the intended JIT-linking case.
	if _, err := f.svc.AdminCreateUser(ctx, "member@x.dev", "memberpass123", "Member", usermodels.RoleMember); err != nil {
		t.Fatalf("create member: %v", err)
	}
	ext := ExternalIdentity{Provider: testOIDCProvider, Subject: "sub-mem", Email: "member@x.dev"}
	user, token, err := f.svc.AuthenticateExternal(ctx, ext, "ua", "1.2.3.4")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	identity, ok := f.svc.ResolveSessionToken(ctx, token, "")
	if !ok || identity.UserID != user.ID || identity.IsAdmin() {
		t.Fatalf("linked member session = %+v ok=%v, want non-admin member", identity, ok)
	}
	// The external identity now resolves directly (no email needed).
	stored, err := f.svc.store.GetIdentityByProviderSubject(ctx, testOIDCProvider, "sub-mem")
	if err != nil || stored.UserID != user.ID {
		t.Fatalf("identity not linked: %+v err=%v", stored, err)
	}
}

func TestProvisionExternalUserRollsBackOnIdentityConflict(t *testing.T) {
	f := newServiceFixture(t, false)
	setupEnabled(t, f)
	ctx := context.Background()

	// A "winner" already owns the (provider, subject) identity — simulating a
	// concurrent callback that committed first.
	winner, err := f.svc.AdminCreateUser(ctx, "winner@x.dev", "winnerpass123", "Winner", usermodels.RoleMember)
	if err != nil {
		t.Fatalf("create winner: %v", err)
	}
	if err := f.svc.store.CreateIdentity(ctx, &store.LoginIdentity{
		UserID: winner.ID, Provider: testOIDCProvider, Subject: "shared-sub",
	}); err != nil {
		t.Fatalf("seed winner identity: %v", err)
	}

	// Provisioning the same subject with a different email must fail to link,
	// roll back the new user, and resolve to the winner instead.
	got, err := f.svc.provisionExternalUser(ctx, testOIDCProvider, "shared-sub", "loser@x.dev", "Loser")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if got.ID != winner.ID {
		t.Fatalf("resolved user = %s, want winner %s", got.ID, winner.ID)
	}
	// The rolled-back account must not linger reserving loser@x.dev.
	if _, err := f.svc.users.GetUserByEmail(ctx, "loser@x.dev"); err == nil {
		t.Fatal("orphan user was not rolled back")
	}
}

func TestResolveExternalUserPropagatesStoreError(t *testing.T) {
	f := newServiceFixture(t, false)
	// Empty auth store → unknown identity, so resolution reaches the
	// link-by-email lookup; a transient error there must propagate, not be
	// masked as "no such user" and fall through to provisioning.
	f.svc.users = erroringUsers{err: errBoomSSO}
	if _, err := f.svc.resolveExternalUser(context.Background(), testOIDCProvider, "sub-x", "x@y.dev", "X"); !errors.Is(err, errBoomSSO) {
		t.Fatalf("got %v, want propagated store error", err)
	}
}

func TestAuthenticateExternalRejectsDisabledUser(t *testing.T) {
	f := newServiceFixture(t, false)
	setupEnabled(t, f)
	ctx := context.Background()

	// Provision, then disable the account.
	ext := ExternalIdentity{Provider: testOIDCProvider, Subject: "sub-dis", Email: "dis@x.dev"}
	user, _, err := f.svc.AuthenticateExternal(ctx, ext, "", "")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if _, err := f.svc.AdminSetRoleStatus(ctx, user.ID, usermodels.RoleMember, usermodels.StatusDisabled); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, _, err := f.svc.AuthenticateExternal(ctx, ext, "", ""); err == nil {
		t.Fatal("expected disabled user to be rejected")
	}
}
