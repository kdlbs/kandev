package store

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresFreshSchemaInitializes(t *testing.T) {
	dsn := os.Getenv("KANDEV_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set KANDEV_TEST_POSTGRES_DSN to run Postgres schema initialization test")
	}

	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset postgres schema: %v", err)
	}

	if _, err := newSQLiteRepositoryWithDB(db, db, nil); err != nil {
		t.Fatalf("init fresh postgres schema: %v", err)
	}
}
