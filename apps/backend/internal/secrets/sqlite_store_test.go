package secrets

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/auth/authn"
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
	t.Cleanup(func() {
		_ = sqlxDB.Close()
	})

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

// Per-user secret scoping (opt-in auth): owned secrets are invisible to other
// users; unowned (pre-auth) rows stay shared; internal/synthetic callers are
// unscoped; ClaimUnowned hands legacy rows to the setup-wizard admin.
func TestSQLiteStore_PerUserScoping(t *testing.T) {
	store := newTestSQLiteStore(t)
	internalCtx := context.Background()
	userA := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-a", Role: authn.RoleMember})
	userB := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-b", Role: authn.RoleMember})

	shared := &SecretWithValue{Secret: Secret{Name: "SHARED"}, Value: "legacy"}
	if err := store.Create(internalCtx, shared); err != nil {
		t.Fatalf("create shared: %v", err)
	}
	mine := &SecretWithValue{Secret: Secret{Name: "MINE"}, Value: "a-only"}
	if err := store.Create(userA, mine); err != nil {
		t.Fatalf("create owned: %v", err)
	}

	// Owner + internal see the owned secret; another user does not.
	if _, err := store.Reveal(userA, mine.ID); err != nil {
		t.Fatalf("owner reveal: %v", err)
	}
	if _, err := store.Reveal(internalCtx, mine.ID); err != nil {
		t.Fatalf("internal reveal: %v", err)
	}
	if _, err := store.Reveal(userB, mine.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign reveal: %v, want ErrNotFound", err)
	}
	if err := store.Delete(userB, mine.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign delete: %v, want ErrNotFound", err)
	}

	// Unowned rows are visible to everyone.
	if _, err := store.Reveal(userB, shared.ID); err != nil {
		t.Fatalf("shared reveal: %v", err)
	}

	// List is scoped: A sees both, B sees only the shared one.
	itemsA, err := store.List(userA)
	if err != nil || len(itemsA) != 2 {
		t.Fatalf("list A: %v (%d items, want 2)", err, len(itemsA))
	}
	itemsB, err := store.List(userB)
	if err != nil || len(itemsB) != 1 {
		t.Fatalf("list B: %v (%d items, want 1)", err, len(itemsB))
	}

	// Setup-wizard claim: shared secret becomes admin-owned, invisible to B.
	if err := store.ClaimUnowned(internalCtx, "user-a"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := store.Reveal(userB, shared.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("post-claim foreign reveal: %v, want ErrNotFound", err)
	}
}
