package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresControlServerRecordRoundTrips is the PostgreSQL counterpart to
// TestUpsertControlServerRecordThenGetRoundTrips/TestUpsertControlServerRecordIsSingleton
// (SQLite): control_server_records is a new table reached through raw SQL
// (an ON CONFLICT(id) DO UPDATE upsert plus a JSON-encoded capabilities
// column), so schema replay alone does not prove the upsert, singleton, and
// CreatedAt-preservation behavior actually holds on this dialect too.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresControlServerRecordRoundTrips(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	if _, err := repo.GetControlServerRecord(ctx); err == nil {
		t.Fatalf("expected ErrControlServerRecordNotFound before any write")
	}

	first := &models.ControlServerRecord{
		Endpoint:           "127.0.0.1:41123",
		ServerIdentity:     "server-identity-1",
		CredentialSecretID: "secret-ref-1",
		Capabilities:       []string{"resume", "diagnostics"},
		DiagnosticLogPath:  "/home/kandev/logs/agentctl-diagnostic.log",
	}
	if err := repo.UpsertControlServerRecord(ctx, first); err != nil {
		t.Fatalf("UpsertControlServerRecord(first): %v", err)
	}
	firstCreatedAt := first.CreatedAt

	got, err := repo.GetControlServerRecord(ctx)
	if err != nil {
		t.Fatalf("GetControlServerRecord: %v", err)
	}
	if got.Endpoint != first.Endpoint || got.ServerIdentity != first.ServerIdentity ||
		got.CredentialSecretID != first.CredentialSecretID || got.DiagnosticLogPath != first.DiagnosticLogPath {
		t.Fatalf("got = %#v, want match of %#v", got, first)
	}
	if len(got.Capabilities) != 2 || got.Capabilities[0] != "resume" || got.Capabilities[1] != "diagnostics" {
		t.Fatalf("Capabilities = %#v, want [resume diagnostics]", got.Capabilities)
	}

	time.Sleep(2 * time.Millisecond)

	second := &models.ControlServerRecord{
		Endpoint:           "127.0.0.1:52222",
		ServerIdentity:     "server-identity-2",
		CredentialSecretID: "secret-ref-2",
		DiagnosticLogPath:  "/home/kandev/logs/agentctl-diagnostic.log",
	}
	if err := repo.UpsertControlServerRecord(ctx, second); err != nil {
		t.Fatalf("UpsertControlServerRecord(second): %v", err)
	}

	got2, err := repo.GetControlServerRecord(ctx)
	if err != nil {
		t.Fatalf("GetControlServerRecord(after rewrite): %v", err)
	}
	if got2.Endpoint != second.Endpoint || got2.ServerIdentity != second.ServerIdentity {
		t.Fatalf("got2 = %#v, want the rewritten (second) record", got2)
	}
	// Compared at microsecond granularity because PostgreSQL TIMESTAMP stores
	// microseconds while Go's time.Time carries nanoseconds, so the first
	// write's in-memory stamp reads back truncated (.330254029 -> .330254).
	// That truncation is a storage property, not the contract under test: the
	// regression this guards is an upsert that RESETS created_at to now, which
	// the 2ms sleep above puts milliseconds away and which a microsecond
	// comparison still catches with room to spare.
	if !got2.CreatedAt.Truncate(time.Microsecond).Equal(firstCreatedAt.Truncate(time.Microsecond)) {
		t.Fatalf("CreatedAt = %v, want preserved original %v", got2.CreatedAt, firstCreatedAt)
	}
	if !got2.UpdatedAt.After(got2.CreatedAt) {
		t.Fatalf("UpdatedAt = %v, want after CreatedAt %v", got2.UpdatedAt, got2.CreatedAt)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM control_server_records`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1", count)
	}
}
