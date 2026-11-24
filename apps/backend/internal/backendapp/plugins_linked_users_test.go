package backendapp

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	authhttpmw "github.com/kandev/kandev/internal/auth/httpmw"
	userservice "github.com/kandev/kandev/internal/user/service"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// With authentication disabled, the only linkable user is the synthetic local
// user. Any other user ID must resolve to false so no receipt is minted for it.
func TestPluginLinkedUserResolverDisabledAuthResolvesOnlyLocalUser(t *testing.T) {
	resolve := pluginLinkedUserResolver(nil, nil)
	local := authhttpmw.SyntheticIdentity()

	human, ok := resolve(context.Background(), local.UserID)
	if !ok {
		t.Fatal("local user did not resolve while authentication is disabled")
	}
	if human.Identity.UserID != local.UserID {
		t.Fatalf("identity user = %q, want %q", human.Identity.UserID, local.UserID)
	}
	if human.DisplayName != "" {
		t.Fatalf("display name = %q without a users service, want empty", human.DisplayName)
	}

	if _, ok := resolve(context.Background(), "someone-else"); ok {
		t.Fatal("a non-local user resolved while authentication is disabled")
	}
}

// A zero-value auth service reports disabled, so it resolves like no service.
func TestPluginLinkedUserResolverZeroValueAuthServiceIsDisabled(t *testing.T) {
	resolve := pluginLinkedUserResolver(&auth.Service{}, nil)
	local := authhttpmw.SyntheticIdentity()

	if _, ok := resolve(context.Background(), local.UserID); !ok {
		t.Fatal("local user did not resolve with a disabled auth service")
	}
	if _, ok := resolve(context.Background(), "someone-else"); ok {
		t.Fatal("a non-local user resolved with a disabled auth service")
	}
}

// With authentication enabled the synthetic local user shortcut is gone: only
// an account the auth service resolves to an active identity resolves.
func TestPluginLinkedUserResolverEnabledAuthRejectsUnknownUsers(t *testing.T) {
	resolve := pluginLinkedUserResolver(newSSOTestAuthService(t), nil)

	for _, userID := range []string{"missing-user", ""} {
		if _, ok := resolve(context.Background(), userID); ok {
			t.Fatalf("user %q resolved although the auth service has no such active account", userID)
		}
	}
}

func newLinkedUserTestUsers(t *testing.T) (*userservice.Service, *sqlx.DB) {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	repo, cleanup, err := userstore.Provide(conn, conn)
	if err != nil {
		t.Fatalf("user store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	return userservice.NewService(repo, nil, newTestLogger()), conn
}

// The display name comes from the user record; a failed lookup keeps the
// identity and leaves the name empty.
func TestPluginLinkedUserResolverDisplayName(t *testing.T) {
	users, conn := newLinkedUserTestUsers(t)
	local := authhttpmw.SyntheticIdentity()
	resolve := pluginLinkedUserResolver(nil, users)

	stored, err := users.GetCurrentUser(authn.WithIdentity(context.Background(), local))
	if err != nil || stored == nil {
		t.Fatalf("local user record = %+v err=%v", stored, err)
	}
	human, ok := resolve(context.Background(), local.UserID)
	if !ok || human.DisplayName != stored.DisplayName {
		t.Fatalf("resolved = %+v ok=%v, want display name %q", human, ok, stored.DisplayName)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("close user database: %v", err)
	}
	human, ok = resolve(context.Background(), local.UserID)
	if !ok || human.Identity.UserID != local.UserID || human.DisplayName != "" {
		t.Fatalf("resolved after lookup failure = %+v ok=%v, want the identity with an empty name", human, ok)
	}
}
