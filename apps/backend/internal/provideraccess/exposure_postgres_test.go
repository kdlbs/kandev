package provideraccess

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/testutil"
)

// This needs two real PostgreSQL connections. A single-connection pool would
// serialize the operations without testing the grant-row lock.
func TestPostgresExposureAdmissionWaitsForGrantRevocationLock(t *testing.T) {
	store := newPostgresExposureTestStore(t)
	ctx := context.Background()
	grant := testGrant("grant-1")
	if err := store.ReplaceGrant(ctx, &grant); err != nil {
		t.Fatal(err)
	}
	lease, err := store.IssueLease(ctx, testLeaseClaim(grant))
	if err != nil {
		t.Fatal(err)
	}
	receipt := ExposureReceipt{
		LeaseID: lease.ID, GrantID: grant.ID, Provider: "github",
		ProviderPrincipalID: "installation:42", RepositoryID: grant.RepositoryID,
		PermissionProfile: "github_actions_rerun",
		ProviderExpiresAt: time.Now().UTC().Add(45 * time.Minute),
	}
	holder, err := store.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback() }()
	if _, err := holder.ExecContext(ctx, `UPDATE provider_access_grants
  SET updated_at = updated_at WHERE id = $1`, grant.ID); err != nil {
		t.Fatal(err)
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	revocations := 0
	err = store.RecordExposureOrRevoke(deadlineCtx, receipt, func(context.Context) error {
		revocations++
		return nil
	})
	if err == nil || revocations != 1 {
		t.Fatalf("exposure crossed held grant lock: err = %v, revocations = %d", err, revocations)
	}
	if _, err := holder.ExecContext(ctx, `UPDATE provider_access_grants
  SET revoked_at = $1 WHERE id = $2`, time.Now().Unix(), grant.ID); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	err = store.RecordExposureOrRevoke(ctx, receipt, func(context.Context) error { return nil })
	if !errors.Is(err, ErrGrantUnavailable) {
		t.Fatalf("exposure after committed revocation = %v, want ErrGrantUnavailable", err)
	}
	state, err := store.ExposureStateAt(ctx, lease.ID, time.Now().UTC())
	if err != nil || state != ExposureUnknown {
		t.Fatalf("exposure row after losing race = %s, err = %v", state, err)
	}
}

func newPostgresExposureTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	setup, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := setup.Exec("CREATE SCHEMA " + schema); err != nil {
		_ = setup.Close()
		t.Fatal(err)
	}
	_ = setup.Close()
	t.Cleanup(func() {
		if cleanup, err := sqlx.Open("pgx", dsn); err == nil {
			_, _ = cleanup.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
			_ = cleanup.Close()
		}
	})
	var scopedDSN string
	if strings.Contains(dsn, "://") {
		separator := "?"
		if strings.Contains(dsn, "?") {
			separator = "&"
		}
		scopedDSN = dsn + separator + "options=" + url.QueryEscape("-c search_path="+schema)
	} else {
		scopedDSN = dsn + " options='-c search_path=" + schema + "'"
	}
	db, err := sqlx.Open("pgx", scopedDSN)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
