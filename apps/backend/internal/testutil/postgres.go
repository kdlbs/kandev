package testutil

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	internaldb "github.com/kandev/kandev/internal/db"
)

// OpenIsolatedPostgres opens dsn with a unique schema as each connection's
// search_path. It lets package tests share one Postgres database without
// racing on DROP SCHEMA public when Go runs packages in parallel.
func OpenIsolatedPostgres(t testing.TB, dsn string) *sqlx.DB {
	t.Helper()

	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	raw, err := internaldb.OpenPostgres(dsn, 1, 1)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	db := sqlx.NewDb(raw, "pgx")
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec("CREATE SCHEMA " + schema); err != nil {
		_ = db.Close()
		t.Fatalf("create postgres schema %s: %v", schema, err)
	}
	isolatedRaw, err := internaldb.OpenPostgresWithRuntimeParams(dsn, 1, 1, map[string]string{"search_path": schema})
	if err != nil {
		_, _ = db.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
		_ = db.Close()
		t.Fatalf("open postgres with isolated schema %s: %v", schema, err)
	}
	if err := db.Close(); err != nil {
		_, _ = isolatedRaw.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
		_ = isolatedRaw.Close()
		t.Fatalf("close postgres schema setup connection: %v", err)
	}
	isolatedDB := sqlx.NewDb(isolatedRaw, "pgx")
	t.Cleanup(func() {
		_, _ = isolatedDB.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
		_ = isolatedDB.Close()
	})
	return isolatedDB
}

func PostgresDSNFromEnv(t testing.TB) string {
	t.Helper()
	dsn := os.Getenv("KANDEV_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set KANDEV_TEST_POSTGRES_DSN to run Postgres tests")
	}
	return dsn
}
