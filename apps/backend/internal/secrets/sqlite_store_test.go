package secrets

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

func newTestSQLiteStore(t *testing.T) *sqliteStore {
	t.Helper()
	dir := t.TempDir()

	conn, err := db.OpenSQLite(filepath.Join(dir, "secrets.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")

	crypto, err := NewMasterKeyProvider(dir)
	if err != nil {
		t.Fatalf("master key: %v", err)
	}

	store, cleanup, err := Provide(sqlxDB, sqlxDB, crypto)
	if err != nil {
		t.Fatalf("provide store: %v", err)
	}
	t.Cleanup(func() {
		_ = cleanup()
		_ = sqlxDB.Close()
	})
	return store
}

// TestSQLiteStore_MissingID_IsErrNotFound proves the store signals an absent
// entry via the exported secrets.ErrNotFound sentinel (matchable with
// errors.Is), so consumers don't have to string-match the message.
func TestSQLiteStore_MissingID_IsErrNotFound(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	if _, err := store.Get(ctx, "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(missing) error = %v, want errors.Is ErrNotFound", err)
	}
	if _, err := store.Reveal(ctx, "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Reveal(missing) error = %v, want errors.Is ErrNotFound", err)
	}
	if err := store.Delete(ctx, "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete(missing) error = %v, want errors.Is ErrNotFound", err)
	}
}

// TestSQLiteStore_MissingID_MessageUnchanged confirms the human-readable text
// is still "secret not found: <id>" so existing logs are unchanged; only the
// detection mechanism (errors.Is) is new.
func TestSQLiteStore_MissingID_MessageUnchanged(t *testing.T) {
	store := newTestSQLiteStore(t)

	_, err := store.Get(context.Background(), "abc")
	if err == nil {
		t.Fatal("Get(missing) returned nil error")
	}
	if got, want := err.Error(), "secret not found: abc"; got != want {
		t.Errorf("error message = %q, want %q", got, want)
	}
}
