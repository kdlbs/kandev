package testutil

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// OpenIsolatedPostgres opens dsn with a unique schema on a single connection.
// It lets package tests share one Postgres database without racing on
// DROP SCHEMA public when Go runs packages in parallel.
func OpenIsolatedPostgres(t testing.TB, dsn string) *sqlx.DB {
	t.Helper()

	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec("CREATE SCHEMA " + schema); err != nil {
		_ = db.Close()
		t.Fatalf("create postgres schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
		_ = db.Close()
	})

	if _, err := db.Exec("SET search_path TO " + schema); err != nil {
		t.Fatalf("set postgres search_path %s: %v", schema, err)
	}
	return db
}

func PostgresDSNFromEnv(t testing.TB) string {
	t.Helper()
	dsn := os.Getenv("KANDEV_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set KANDEV_TEST_POSTGRES_DSN to run Postgres tests")
	}
	return dsn
}
