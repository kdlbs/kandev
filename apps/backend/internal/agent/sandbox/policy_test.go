package sandbox

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/testutil"
)

func operatorContext(t *testing.T) context.Context {
	t.Helper()
	return authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", Role: authn.RoleAdmin, SessionID: "browser-session", OrgID: "org"})
}

func policyFixture(t *testing.T) Policy {
	t.Helper()
	return Policy{ID: "policy", Name: "Work", InstallationID: "installation", ProviderKey: "fixture-alpha", Grants: contract.Grants{Filesystem: []contract.FilesystemGrant{{Path: t.TempDir(), Writable: true}}}}
}

func sqlitePolicyDB(t *testing.T) *sqlx.DB {
	t.Helper()
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "sandbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	database := sqlx.NewDb(raw, "sqlite3")
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestSandboxAuthorityPublish(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{"absent", t.Context()},
		{"synthetic", authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", Role: authn.RoleAdmin, Synthetic: true, SessionID: "fake"})},
		{"member", authn.WithIdentity(t.Context(), authn.Identity{UserID: "member", Role: authn.RoleMember, SessionID: "browser"})},
		{"owner without browser", authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", Role: authn.RoleAdmin})},
		{"PAT", authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", Role: authn.RoleAdmin, TokenID: "pat"})},
		{"instance operator", authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", Role: authn.RoleAdmin, SessionID: "browser", Instance: true})},
		{"external MCP", origin.WithTrustedExternalTransport(operatorContext(t))},
		{"internal dispatch", origin.WithTrustedInternalCall(operatorContext(t))},
		{"agent with owner", scope.WithPrincipal(operatorContext(t), scope.Principal{CallerTaskID: "task", CallerSessionID: "session"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewService(sqlitePolicyDB(t), true, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Publish(tc.ctx, policyFixture(t), 0); !errors.Is(err, ErrAuthority) {
				t.Fatalf("unauthorized publish: %v", err)
			}
		})
	}
	s, err := NewService(sqlitePolicyDB(t), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(operatorContext(t), policyFixture(t), 0); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled publish: %v", err)
	}
}

func TestSandboxPolicySQLite(t *testing.T) { testPolicyPersistence(t, sqlitePolicyDB(t)) }
func TestSandboxPolicyPostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	database.SetMaxOpenConns(4)
	testPolicyPersistence(t, database)
}

func testPolicyPersistence(t *testing.T, database *sqlx.DB) {
	t.Helper()
	ctx := operatorContext(t)
	s, err := NewService(database, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := policyFixture(t)
	first, err := s.Publish(ctx, policy, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || first.ID == "" || first.Number != 1 || first.Digest == "" || first.OperatorID != "operator" {
		t.Fatalf("missing revision provenance: %+v", first)
	}
	policy.Name = "Changed"
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := s.Publish(ctx, policy, 1); out <- err })
	}
	wg.Wait()
	close(out)
	success, conflict := 0, 0
	for err := range out {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrConflict):
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent publish: success=%d conflict=%d", success, conflict)
	}
	reloaded, err := NewService(database, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reloaded.GetRevision(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Policy.Name != "Work" || got.Digest != first.Digest {
		t.Fatal("existing revision changed")
	}
	foreign := authn.WithIdentity(t.Context(), authn.Identity{UserID: "other", OrgID: "foreign", Role: authn.RoleAdmin, SessionID: "browser"})
	if _, err := reloaded.GetRevision(foreign, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org policy visible: %v", err)
	}
}
