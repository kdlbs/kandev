package secrets

import (
	"context"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresStoreRoundTrip(t *testing.T) {
	dsn := os.Getenv("KANDEV_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set KANDEV_TEST_POSTGRES_DSN to run Postgres secret store test")
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

	crypto, err := NewMasterKeyProvider(t.TempDir())
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	store, cleanup, err := Provide(db, db, crypto)
	if err != nil {
		t.Fatalf("provide store: %v", err)
	}
	t.Cleanup(func() {
		_ = cleanup()
	})

	ctx := context.Background()
	secret := &SecretWithValue{Secret: Secret{Name: "token"}, Value: "s3cr3t"}
	if err := store.Create(ctx, secret); err != nil {
		t.Fatalf("create secret: %v", err)
	}
	got, err := store.Reveal(ctx, secret.ID)
	if err != nil {
		t.Fatalf("reveal secret: %v", err)
	}
	if got != "s3cr3t" {
		t.Fatalf("revealed value = %q, want %q", got, "s3cr3t")
	}
}
