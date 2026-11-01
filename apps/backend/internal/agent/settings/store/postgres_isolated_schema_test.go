package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresIsolatedSchemaIsAppliedToEveryPoolConnection(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(2)

	first, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("open first pooled connection: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })

	second, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("open second pooled connection: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	var firstSchema, secondSchema string
	if err := first.QueryRowContext(context.Background(), "SELECT current_schema()").Scan(&firstSchema); err != nil {
		t.Fatalf("read first connection schema: %v", err)
	}
	if err := second.QueryRowContext(context.Background(), "SELECT current_schema()").Scan(&secondSchema); err != nil {
		t.Fatalf("read second connection schema: %v", err)
	}
	if firstSchema == "public" || secondSchema != firstSchema {
		t.Fatalf("pooled connection schemas = %q and %q, want the same isolated schema", firstSchema, secondSchema)
	}
}

func openPostgresProfileOrderRaceRepo(t testing.TB) *sqliteRepository {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(8)
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize profile order schema: %v", err)
	}
	return repo
}
