package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
)

func newInstanceStoreTest(t *testing.T) *InstanceStore {
	t.Helper()
	conn, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "instance-state.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	pool := db.NewPool(sqlx.NewDb(conn, "sqlite3"), sqlx.NewDb(conn, "sqlite3"))
	t.Cleanup(func() { _ = pool.Close() })
	store, err := NewInstanceStore(pool)
	if err != nil {
		t.Fatalf("NewInstanceStore: %v", err)
	}
	return store
}

func TestInstanceStoreUsesRevisionedConditionalWrites(t *testing.T) {
	store := newInstanceStoreTest(t)
	ctx := context.Background()
	first, err := store.Set(ctx, "instance-1", "filters", json.RawMessage(`{"group":"step"}`), ptrInt64(0), "browser")
	if err != nil {
		t.Fatalf("initial Set: %v", err)
	}
	if first.Revision != 1 {
		t.Fatalf("initial revision = %d, want 1", first.Revision)
	}
	second, err := store.Set(ctx, "instance-1", "filters", json.RawMessage(`{"group":"workflow"}`), ptrInt64(1), "agent")
	if err != nil {
		t.Fatalf("conditional Set: %v", err)
	}
	if second.Revision != 2 {
		t.Fatalf("second revision = %d, want 2", second.Revision)
	}
	_, err = store.Set(ctx, "instance-1", "filters", json.RawMessage(`{"group":"stale"}`), ptrInt64(1), "browser")
	var conflict *ConflictError
	if !errors.As(err, &conflict) || conflict.CurrentRevision != 2 {
		t.Fatalf("stale Set = %v, conflict = %+v", err, conflict)
	}
	got, found, err := store.Get(ctx, "instance-1", "filters")
	if err != nil || !found || string(got.Value) != `{"group":"workflow"}` || got.Revision != 2 {
		t.Fatalf("Get = %+v, found=%v, err=%v", got, found, err)
	}
}

func TestInstanceStoreDeleteRetainsTombstoneRevision(t *testing.T) {
	store := newInstanceStoreTest(t)
	ctx := context.Background()
	if _, err := store.Set(ctx, "instance-1", "draft", json.RawMessage(`1`), ptrInt64(0), "browser"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	revision, err := store.Delete(ctx, "instance-1", "draft", ptrInt64(1), "browser")
	if err != nil || revision != 2 {
		t.Fatalf("Delete = %d, %v; want revision 2", revision, err)
	}
	entry, found, err := store.Get(ctx, "instance-1", "draft")
	if err != nil || found || entry.Revision != 2 {
		t.Fatalf("Get tombstone = %+v, found=%v, err=%v", entry, found, err)
	}
	var conflict *ConflictError
	if _, err := store.Set(ctx, "instance-1", "draft", json.RawMessage(`2`), ptrInt64(1), "browser"); !errors.As(err, &conflict) {
		t.Fatalf("stale recreate = %v, want conflict", err)
	}
}

func TestInstanceStoreDeleteInstanceRemovesAllState(t *testing.T) {
	store := newInstanceStoreTest(t)
	ctx := context.Background()
	for _, key := range []string{"one", "two"} {
		if _, err := store.Set(ctx, "instance-1", key, json.RawMessage(`{"ok":true}`), ptrInt64(0), "agent"); err != nil {
			t.Fatalf("Set(%s): %v", key, err)
		}
	}
	if err := store.DeleteInstance(ctx, "instance-1"); err != nil {
		t.Fatalf("DeleteInstance: %v", err)
	}
	entries, err := store.List(ctx, "instance-1")
	if err != nil {
		t.Fatalf("List after DeleteInstance: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("state after DeleteInstance = %+v, want empty", entries)
	}
}

func ptrInt64(value int64) *int64 { return &value }
