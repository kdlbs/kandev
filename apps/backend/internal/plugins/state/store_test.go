package state

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
)

func TestStoreSetThenGetReturnsStoredValue(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	value := json.RawMessage(`{"synced":true}`)
	if err := store.Set(ctx, "kandev-plugin-jira", "task", "task_xyz", "sync_status", value); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, found, err := store.Get(ctx, "kandev-plugin-jira", "task", "task_xyz", "sync_status")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found {
		t.Fatalf("expected found = true")
	}
	if string(got) != string(value) {
		t.Fatalf("got %q, want %q", got, value)
	}
}

func TestStoreGetMissingReturnsNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	got, found, err := store.Get(ctx, "kandev-plugin-jira", "task", "task_xyz", "missing_key")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found || got != nil {
		t.Fatalf("got (%q, %v), want (nil, false)", got, found)
	}
}

// TestStoreSetUpsertsOnRepeatedWrite pins the UNIQUE(plugin_id, scope,
// scope_id, state_key) upsert contract: a second Set for the same tuple must
// update the existing row in place, not create a duplicate.
func TestStoreSetUpsertsOnRepeatedWrite(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "kandev-plugin-jira", "task", "task_xyz", "sync_status", json.RawMessage(`"PROJ-1"`)); err != nil {
		t.Fatalf("first set: %v", err)
	}
	if err := store.Set(ctx, "kandev-plugin-jira", "task", "task_xyz", "sync_status", json.RawMessage(`"PROJ-2"`)); err != nil {
		t.Fatalf("second set: %v", err)
	}

	got, found, err := store.Get(ctx, "kandev-plugin-jira", "task", "task_xyz", "sync_status")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found {
		t.Fatalf("expected found = true")
	}
	if string(got) != `"PROJ-2"` {
		t.Fatalf("got %q, want %q (upsert should overwrite)", got, `"PROJ-2"`)
	}

	entries, err := store.List(ctx, "kandev-plugin-jira", "task", "task_xyz")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 entry after repeated Set, got %d: %+v", len(entries), entries)
	}
}

// TestStoreInstanceScopeUpsertsWithEmptyScopeID pins scope_id NULL handling
// for instance-scoped state (scope_id == ""). SQLite's UNIQUE index treats
// each NULL as distinct, so a naive INSERT ... ON CONFLICT with a literal
// NULL scope_id would silently insert a duplicate row on the second Set
// instead of updating. The store must normalize "" consistently so repeated
// writes at instance scope still upsert.
func TestStoreInstanceScopeUpsertsWithEmptyScopeID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "kandev-plugin-jira", "instance", "", "install_id", json.RawMessage(`"abc"`)); err != nil {
		t.Fatalf("first set: %v", err)
	}
	if err := store.Set(ctx, "kandev-plugin-jira", "instance", "", "install_id", json.RawMessage(`"def"`)); err != nil {
		t.Fatalf("second set: %v", err)
	}

	got, found, err := store.Get(ctx, "kandev-plugin-jira", "instance", "", "install_id")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found || string(got) != `"def"` {
		t.Fatalf("got (%q, %v), want (\"def\", true)", got, found)
	}

	entries, err := store.List(ctx, "kandev-plugin-jira", "instance", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 instance-scoped entry, got %d: %+v", len(entries), entries)
	}
}

func TestStoreDeleteRemovesEntry(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "kandev-plugin-jira", "task", "task_xyz", "sync_status", json.RawMessage(`"PROJ-1"`)); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Delete(ctx, "kandev-plugin-jira", "task", "task_xyz", "sync_status"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, found, err := store.Get(ctx, "kandev-plugin-jira", "task", "task_xyz", "sync_status")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if found {
		t.Fatalf("expected not found after delete")
	}
}

// TestStoreDeleteAllRemovesEveryRowForPlugin pins the Uninstall cleanup
// contract (docs/specs/plugins/requirements/plugins.md "plugin_state"): a plugin's entire
// state footprint — across every scope and scope_id — must be removable in
// one call, so a reinstalled or id-reused plugin never inherits stale state.
func TestStoreDeleteAllRemovesEveryRowForPlugin(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "kandev-plugin-jira", "instance", "", "install_id", json.RawMessage(`"abc"`)); err != nil {
		t.Fatalf("set instance-scoped: %v", err)
	}
	if err := store.Set(ctx, "kandev-plugin-jira", "task", "task_1", "sync_status", json.RawMessage(`"PROJ-1"`)); err != nil {
		t.Fatalf("set task_1-scoped: %v", err)
	}
	if err := store.Set(ctx, "kandev-plugin-jira", "task", "task_2", "sync_status", json.RawMessage(`"PROJ-2"`)); err != nil {
		t.Fatalf("set task_2-scoped: %v", err)
	}

	if err := store.DeleteAll(ctx, "kandev-plugin-jira"); err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}

	if entries, err := store.List(ctx, "kandev-plugin-jira", "instance", ""); err != nil || len(entries) != 0 {
		t.Fatalf("instance-scoped entries after DeleteAll = %v (err=%v), want none", entries, err)
	}
	if entries, err := store.List(ctx, "kandev-plugin-jira", "task", "task_1"); err != nil || len(entries) != 0 {
		t.Fatalf("task_1-scoped entries after DeleteAll = %v (err=%v), want none", entries, err)
	}
	if entries, err := store.List(ctx, "kandev-plugin-jira", "task", "task_2"); err != nil || len(entries) != 0 {
		t.Fatalf("task_2-scoped entries after DeleteAll = %v (err=%v), want none", entries, err)
	}
}

// TestStoreDeleteAllDoesNotTouchOtherPlugins pins the plugin-isolation
// invariant: DeleteAll for one plugin must never remove another plugin's
// state.
func TestStoreDeleteAllDoesNotTouchOtherPlugins(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "kandev-plugin-jira", "task", "task_1", "sync_status", json.RawMessage(`"PROJ-1"`)); err != nil {
		t.Fatalf("set jira: %v", err)
	}
	if err := store.Set(ctx, "kandev-plugin-slack", "task", "task_1", "channel", json.RawMessage(`"#dev"`)); err != nil {
		t.Fatalf("set slack: %v", err)
	}

	if err := store.DeleteAll(ctx, "kandev-plugin-jira"); err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}

	entries, err := store.List(ctx, "kandev-plugin-slack", "task", "task_1")
	if err != nil {
		t.Fatalf("list slack: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("kandev-plugin-slack entries after deleting kandev-plugin-jira = %d, want 1 (untouched)", len(entries))
	}
}

// TestStoreDeleteAllMissingPluginIsNotAnError pins that DeleteAll on a
// plugin with no stored state is a safe no-op.
func TestStoreDeleteAllMissingPluginIsNotAnError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.DeleteAll(ctx, "kandev-plugin-never-had-state"); err != nil {
		t.Fatalf("DeleteAll on a plugin with no state: %v", err)
	}
}

func TestStoreDeleteMissingIsNotAnError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.Delete(ctx, "kandev-plugin-jira", "task", "task_xyz", "never_set"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestStoreListReturnsOnlyMatchingScope(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "kandev-plugin-jira", "task", "task_1", "a", json.RawMessage(`1`)); err != nil {
		t.Fatalf("set task_1/a: %v", err)
	}
	if err := store.Set(ctx, "kandev-plugin-jira", "task", "task_1", "b", json.RawMessage(`2`)); err != nil {
		t.Fatalf("set task_1/b: %v", err)
	}
	if err := store.Set(ctx, "kandev-plugin-jira", "task", "task_2", "a", json.RawMessage(`3`)); err != nil {
		t.Fatalf("set task_2/a: %v", err)
	}

	entries, err := store.List(ctx, "kandev-plugin-jira", "task", "task_1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for task_1, got %d: %+v", len(entries), entries)
	}
	keys := map[string]bool{}
	for _, e := range entries {
		keys[e.Key] = true
		if e.UpdatedAt.IsZero() {
			t.Errorf("entry %q has zero UpdatedAt", e.Key)
		}
	}
	if !keys["a"] || !keys["b"] {
		t.Fatalf("expected keys a and b, got %+v", entries)
	}
}

// TestStorePluginsCannotReadEachOthersState pins the spec invariant that
// plugin state is always filtered by plugin_id (docs/specs/plugins/requirements/plugins.md
// "Plugins cannot read others' state").
func TestStorePluginsCannotReadEachOthersState(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "kandev-plugin-jira", "task", "task_xyz", "sync_status", json.RawMessage(`"PROJ-1"`)); err != nil {
		t.Fatalf("set: %v", err)
	}

	_, found, err := store.Get(ctx, "kandev-plugin-slack", "task", "task_xyz", "sync_status")
	if err != nil {
		t.Fatalf("get from other plugin: %v", err)
	}
	if found {
		t.Fatalf("expected another plugin's state to be invisible")
	}

	entries, err := store.List(ctx, "kandev-plugin-slack", "task", "task_xyz")
	if err != nil {
		t.Fatalf("list from other plugin: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries visible to a different plugin, got %+v", entries)
	}
}

func TestStoreSetStampsUpdatedAtAsRFC3339UTC(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	before := time.Now().UTC()
	if err := store.Set(ctx, "kandev-plugin-jira", "task", "task_xyz", "sync_status", json.RawMessage(`"PROJ-1"`)); err != nil {
		t.Fatalf("set: %v", err)
	}
	after := time.Now().UTC()

	entries, err := store.List(ctx, "kandev-plugin-jira", "task", "task_xyz")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	got := entries[0].UpdatedAt
	if got.Location() != time.UTC {
		t.Errorf("expected UpdatedAt location UTC, got %v", got.Location())
	}
	if got.Before(before.Add(-time.Second)) || got.After(after.Add(time.Second)) {
		t.Errorf("UpdatedAt %v not within expected window [%v, %v]", got, before, after)
	}
}

// TestStoreClaimFirstCallWins pins Claim's atomic-insert contract: the first
// caller for a (plugin, scope, scope_id, key) tuple gets claimed=true and its
// value is persisted; Get then reflects that value.
func TestStoreClaimFirstCallWins(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	claimed, err := store.Claim(ctx, "kandev-plugin-coordinator", "occurrence", "ws-1/coordinator", "wake:cycle:1", json.RawMessage(`{"claimed":true}`))
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !claimed {
		t.Fatal("expected claimed = true for the first call")
	}

	got, found, err := store.Get(ctx, "kandev-plugin-coordinator", "occurrence", "ws-1/coordinator", "wake:cycle:1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found {
		t.Fatal("expected the claimed value to be persisted")
	}
	if string(got) != `{"claimed":true}` {
		t.Fatalf("got %q", got)
	}
}

// TestStoreClaimSecondCallLoses pins the duplicate-occurrence rejection: a
// second Claim for the same tuple returns claimed=false and does not
// overwrite the first caller's value (unlike Set's upsert semantics).
func TestStoreClaimSecondCallLoses(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Claim(ctx, "p", "occurrence", "ws-1/coordinator", "wake:cycle:1", json.RawMessage(`"first"`)); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	claimed, err := store.Claim(ctx, "p", "occurrence", "ws-1/coordinator", "wake:cycle:1", json.RawMessage(`"second"`))
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if claimed {
		t.Fatal("expected claimed = false for the second call")
	}

	got, _, err := store.Get(ctx, "p", "occurrence", "ws-1/coordinator", "wake:cycle:1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != `"first"` {
		t.Fatalf("second claim must not overwrite the first caller's value, got %q", got)
	}
}

// TestStoreClaimIsScopedPerPluginAndKey confirms two independent tuples
// (different plugin, or different key) can each be claimed independently.
func TestStoreClaimIsScopedPerPluginAndKey(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if claimed, err := store.Claim(ctx, "plugin-a", "occurrence", "ws-1/coordinator", "wake:1", nil); err != nil || !claimed {
		t.Fatalf("plugin-a claim: claimed=%v err=%v", claimed, err)
	}
	if claimed, err := store.Claim(ctx, "plugin-b", "occurrence", "ws-1/coordinator", "wake:1", nil); err != nil || !claimed {
		t.Fatalf("plugin-b claim (different plugin, same key) should succeed: claimed=%v err=%v", claimed, err)
	}
	if claimed, err := store.Claim(ctx, "plugin-a", "occurrence", "ws-1/coordinator", "wake:2", nil); err != nil || !claimed {
		t.Fatalf("plugin-a claim (different key) should succeed: claimed=%v err=%v", claimed, err)
	}
}

// TestStoreClaimConcurrentCallersExactlyOneWins is the durability/atomicity
// regression: N goroutines race to Claim the same occurrence key against a
// single shared SQLite connection pool. Only one may observe claimed=true —
// this is what makes a scheduled coordinator wake safe to dispatch from
// multiple goroutines (or, via the same DB file, multiple processes)
// without ever double-firing a turn.
func TestStoreClaimConcurrentCallersExactlyOneWins(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	const attempts = 20
	results := make([]bool, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			claimed, err := store.Claim(ctx, "kandev-plugin-coordinator", "occurrence", "ws-1/coordinator", "wake:cycle:concurrent", nil)
			if err != nil {
				t.Errorf("claim %d: %v", idx, err)
				return
			}
			results[idx] = claimed
		}(i)
	}
	wg.Wait()

	wins := 0
	for _, r := range results {
		if r {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly 1 winning claim out of %d concurrent attempts, got %d", attempts, wins)
	}
}

// TestStoreClaimSurvivesRestart proves durability: a claim persisted to a
// file-backed SQLite database is still honored by a brand-new Store instance
// opened against the same file — modeling a backend restart between the
// scheduler claiming an occurrence and the process going down before
// dispatch. An in-memory adapter (the pre-fix implementation) would lose
// this and let a restarted scheduler double-fire the occurrence.
func TestStoreClaimSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	dsn := filepath.Join(dir, "plugin_state.db")
	ctx := context.Background()

	conn1, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open first connection: %v", err)
	}
	store1, err := NewStore(db.NewPool(conn1, conn1))
	if err != nil {
		_ = conn1.Close()
		t.Fatalf("new store 1: %v", err)
	}
	claimed, err := store1.Claim(ctx, "kandev-plugin-coordinator", "occurrence", "ws-1/coordinator", "wake:daily:2026-08-17", json.RawMessage(`{"claimed":true}`))
	if err != nil {
		_ = conn1.Close()
		t.Fatalf("claim before restart: %v", err)
	}
	if !claimed {
		_ = conn1.Close()
		t.Fatalf("expected the first claim to win")
	}
	if err := conn1.Close(); err != nil {
		t.Fatalf("close first connection: %v", err)
	}

	// Simulate a restart: fresh connection, fresh Store, same DB file.
	conn2, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open second connection: %v", err)
	}
	t.Cleanup(func() { _ = conn2.Close() })
	store2, err := NewStore(db.NewPool(conn2, conn2))
	if err != nil {
		t.Fatalf("new store 2 (post-restart): %v", err)
	}
	claimedAgain, err := store2.Claim(ctx, "kandev-plugin-coordinator", "occurrence", "ws-1/coordinator", "wake:daily:2026-08-17", json.RawMessage(`{"claimed":true}`))
	if err != nil {
		t.Fatalf("claim after restart: %v", err)
	}
	if claimedAgain {
		t.Fatal("expected the occurrence claim to survive a restart — a second claim must lose")
	}
}

func TestStoreInitSchemaIsIdempotent(t *testing.T) {
	conn := newSQLite(t)
	if _, err := NewStore(db.NewPool(conn, conn)); err != nil {
		t.Fatalf("first NewStore: %v", err)
	}
	if _, err := NewStore(db.NewPool(conn, conn)); err != nil {
		t.Fatalf("second NewStore (re-init on existing schema): %v", err)
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
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
