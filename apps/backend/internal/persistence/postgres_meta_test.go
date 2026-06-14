package persistence

import (
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresLatestVersionMetaRoundTrip(t *testing.T) {
	dsn := os.Getenv("KANDEV_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set KANDEV_TEST_POSTGRES_DSN to run Postgres meta test")
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
	if err := ensureMetaTable(db); err != nil {
		t.Fatalf("ensure meta table: %v", err)
	}

	checkedAt := time.Unix(123, 0).UTC()
	if err := WriteLatestVersion(db, "v1.2.3", "https://example.test/release", checkedAt); err != nil {
		t.Fatalf("write latest version: %v", err)
	}
	version, url, gotCheckedAt, err := ReadLatestVersion(db)
	if err != nil {
		t.Fatalf("read latest version: %v", err)
	}
	if version != "v1.2.3" || url != "https://example.test/release" || !gotCheckedAt.Equal(checkedAt) {
		t.Fatalf("meta round-trip = (%q, %q, %s)", version, url, gotCheckedAt)
	}
}
