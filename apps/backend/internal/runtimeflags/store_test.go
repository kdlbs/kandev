package runtimeflags

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func TestSQLiteStoreRoundTripsFalseOverride(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.SetOverride(ctx, "features.office", false); err != nil {
		t.Fatalf("SetOverride false: %v", err)
	}
	overrides, err := store.ListOverrides(ctx)
	if err != nil {
		t.Fatalf("ListOverrides: %v", err)
	}
	value, ok := overrides["features.office"]
	if !ok {
		t.Fatal("features.office override missing")
	}
	if value {
		t.Fatal("features.office override = true, want false")
	}
}

func TestSQLiteStoreDeleteOverrideRestoresMissing(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.SetOverride(ctx, "features.office", true); err != nil {
		t.Fatalf("SetOverride true: %v", err)
	}
	if err := store.DeleteOverride(ctx, "features.office"); err != nil {
		t.Fatalf("DeleteOverride: %v", err)
	}
	overrides, err := store.ListOverrides(ctx)
	if err != nil {
		t.Fatalf("ListOverrides: %v", err)
	}
	if _, ok := overrides["features.office"]; ok {
		t.Fatal("features.office override still present after delete")
	}
}

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewSQLiteStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	return store
}
