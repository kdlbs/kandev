package provideraccess

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func TestExportedBearerRemainsResidualAfterLeaseExpiryAndFailedRevoke(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider-access.db")
	open := func() (*sqlx.DB, *Store) {
		t.Helper()
		conn, err := sqlx.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL")
		if err != nil {
			t.Fatal(err)
		}
		store, err := NewStore(conn)
		if err != nil {
			_ = conn.Close()
			t.Fatal(err)
		}
		return conn, store
	}
	ctx := context.Background()
	conn, store := open()
	grant := testGrant("grant-1")
	if err := store.ReplaceGrant(ctx, &grant); err != nil {
		t.Fatal(err)
	}
	claim := testLeaseClaim(grant)
	lease, err := store.IssueLease(ctx, claim)
	if err != nil {
		t.Fatal(err)
	}
	providerExpiry := time.Now().UTC().Add(45 * time.Minute)
	if err := store.RecordExposureReceipt(ctx, ExposureReceipt{
		LeaseID: lease.ID, GrantID: grant.ID, Provider: "github",
		ProviderPrincipalID: "installation:42", RepositoryID: grant.RepositoryID,
		PermissionProfile: "github_actions_rerun", ProviderExpiresAt: providerExpiry,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRevocationResult(ctx, lease.ID, claim.ExpiresAt, false); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	conn, store = open() // Process memory, including exact-token revocation material, is gone.
	defer func() { _ = conn.Close() }()
	state, err := store.ExposureStateAt(ctx, lease.ID, claim.ExpiresAt.Add(time.Second))
	if err != nil || state != ExposureResidual {
		t.Fatalf("after failed revoke and restart: state = %s, err = %v, want residual", state, err)
	}
	state, err = store.ExposureStateAt(ctx, lease.ID, providerExpiry.Add(time.Second))
	if err != nil || state != ExposureProviderExpired {
		t.Fatalf("after provider expiry: state = %s, err = %v, want expired", state, err)
	}
}

func TestConfirmedProviderRevocationEndsExposure(t *testing.T) {
	store := newGrantTestStore(t)
	ctx := context.Background()
	grant := testGrant("grant-1")
	if err := store.ReplaceGrant(ctx, &grant); err != nil {
		t.Fatal(err)
	}
	lease, err := store.IssueLease(ctx, testLeaseClaim(grant))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordExposureReceipt(ctx, ExposureReceipt{
		LeaseID: lease.ID, GrantID: grant.ID, Provider: "github",
		ProviderPrincipalID: "installation:42", RepositoryID: grant.RepositoryID,
		PermissionProfile: "github_actions_rerun",
		ProviderExpiresAt: time.Now().UTC().Add(45 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRevocationResult(ctx, lease.ID, time.Now().UTC(), true); err != nil {
		t.Fatal(err)
	}
	state, err := store.ExposureStateAt(ctx, lease.ID, time.Now().UTC())
	if err != nil || state != ExposureRevokedAtProvider {
		t.Fatalf("confirmed revoke: state = %s, err = %v", state, err)
	}
}
