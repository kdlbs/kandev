package testutil

import (
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	internaldb "github.com/kandev/kandev/internal/db"
)

// OpenIsolatedPostgres opens dsn with a unique schema on every pooled connection.
// It lets package tests share one Postgres database without racing on
// DROP SCHEMA public when Go runs packages in parallel.
func OpenIsolatedPostgres(t testing.TB, dsn string) *sqlx.DB {
	t.Helper()

	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatal("parse postgres URL")
		}
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		dsn = parsed.String()
	} else {
		dsn += " search_path=" + schema
	}
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
	t.Cleanup(func() {
		_, _ = db.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
		_ = db.Close()
	})

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
