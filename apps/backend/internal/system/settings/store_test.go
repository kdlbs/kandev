package settings

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
)

func TestStoreSaveAndGet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	value, found, err := store.Get(ctx, "missing")
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	if found || value != nil {
		t.Fatalf("missing value = (%q, %v), want nil false", value, found)
	}

	if err := store.Save(ctx, "system_metrics", []byte(`{"interval_seconds":5}`)); err != nil {
		t.Fatalf("save: %v", err)
	}
	value, found, err = store.Get(ctx, "system_metrics")
	if err != nil {
		t.Fatalf("get saved: %v", err)
	}
	if !found || string(value) != `{"interval_seconds":5}` {
		t.Fatalf("saved value = (%q, %v)", value, found)
	}
}

func TestStoreMigratesLegacySystemSettings(t *testing.T) {
	conn := newSQLite(t)
	if _, err := conn.Exec(`
		CREATE TABLE system_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		INSERT INTO system_settings (key, value, updated_at)
		VALUES ('system_metrics', '{"interval_seconds":5}', CURRENT_TIMESTAMP);
	`); err != nil {
		t.Fatalf("seed legacy table: %v", err)
	}

	store, err := NewStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	value, found, err := store.Get(context.Background(), "system_metrics")
	if err != nil {
		t.Fatalf("get migrated: %v", err)
	}
	if !found || string(value) != `{"interval_seconds":5}` {
		t.Fatalf("migrated value = (%q, %v)", value, found)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	conn := newSQLite(t)
	store, err := NewStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func newSQLite(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
