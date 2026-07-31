package backendapp

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/auth"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// newSSOTestAuthService builds an auth service with features.auth on, which
// leaves it in ModeSetup (no admin identity yet) — non-disabled, so
// ssoProvidersForBoot gets past its first guard and reaches the plugin lookup.
func newSSOTestAuthService(t *testing.T) *auth.Service {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	users, cleanup, err := userstore.Provide(conn, conn)
	if err != nil {
		t.Fatalf("user store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	st, err := authstore.New(conn, conn)
	if err != nil {
		t.Fatalf("auth store: %v", err)
	}
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	svc, err := auth.NewService(context.Background(), auth.Deps{Cfg: cfg, Store: st, Users: users})
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	if svc.Mode() == auth.ModeDisabled {
		t.Fatalf("test auth service is disabled; the plugin branch would be unreachable")
	}
	return svc
}

// TestSSOProvidersForBootGuards covers ssoProvidersForBoot now that the plugins
// feature flag is gone. The function used to short-circuit on
// !p.features.Plugins; with that clause removed, the remaining guards are the
// only thing standing between the pre-auth login screen and a nil dereference,
// so each one gets explicit coverage.
//
// The provider mapping itself is covered by plugins' own sso_test.go
// (TestSSOProviders); what matters here is that an auth-enabled instance with
// no auth-capable plugin yields no providers rather than panicking.
func TestSSOProvidersForBootGuards(t *testing.T) {
	t.Run("no auth service", func(t *testing.T) {
		if got := ssoProvidersForBoot(routeParams{}); got != nil {
			t.Fatalf("ssoProvidersForBoot() with nil authSvc = %v, want nil", got)
		}
	})

	t.Run("auth disabled short-circuits before the plugin lookup", func(t *testing.T) {
		// A zero-value service reports ModeDisabled, and services is nil here:
		// reaching the plugin branch at all would panic, so a nil return proves
		// the disabled check still comes first.
		if got := ssoProvidersForBoot(routeParams{authSvc: &auth.Service{}}); got != nil {
			t.Fatalf("ssoProvidersForBoot() with disabled auth = %v, want nil", got)
		}
	})

	t.Run("auth enabled but no services", func(t *testing.T) {
		got := ssoProvidersForBoot(routeParams{authSvc: newSSOTestAuthService(t)})
		if got != nil {
			t.Fatalf("ssoProvidersForBoot() with nil services = %v, want nil", got)
		}
	})

	t.Run("auth enabled but no plugin service", func(t *testing.T) {
		got := ssoProvidersForBoot(routeParams{
			authSvc:  newSSOTestAuthService(t),
			services: &Services{},
		})
		if got != nil {
			t.Fatalf("ssoProvidersForBoot() with nil Plugins service = %v, want nil", got)
		}
	})

	t.Run("auth enabled, plugin service present, no auth-capable plugin", func(t *testing.T) {
		svc := newTestPluginsService(t)
		installTestPluginForBoot(t, svc, "kandev-plugin-hello", true)

		got := ssoProvidersForBoot(routeParams{
			authSvc:  newSSOTestAuthService(t),
			services: &Services{Plugins: svc},
		})
		if got != nil {
			t.Fatalf("ssoProvidersForBoot() with no auth-capable plugin = %v, want nil", got)
		}
	})
}
