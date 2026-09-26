package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

func TestUserStoreSetThenGetReturnsStoredValue(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	value := json.RawMessage(`{"text":"hello"}`)
	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", value, nil); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, _, found, err := store.Get(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note")
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

func TestUserStoreGetMissingReturnsNotFound(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	got, _, found, err := store.Get(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "missing")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found || got != nil {
		t.Fatalf("got (%q, %v), want (nil, false)", got, found)
	}
}

// TestUserStoreIsolatesByUser pins the core trust boundary of
// plugin_user_state: two different users writing the same
// plugin/scope/scopeID/key must never see each other's value (a different
// user's Get returns not-found, matching AC15's cross-user 404).
func TestUserStoreIsolatesByUser(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"alice's note"`), nil); err != nil {
		t.Fatalf("set user_1: %v", err)
	}

	got, _, found, err := store.Get(ctx, "kandev-plugin-notes", "user_2", "task", "task_xyz", "note")
	if err != nil {
		t.Fatalf("get user_2: %v", err)
	}
	if found {
		t.Fatalf("expected user_2 to not see user_1's note, got %q", got)
	}

	// user_1 still sees their own value.
	got, _, found, err = store.Get(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note")
	if err != nil {
		t.Fatalf("get user_1: %v", err)
	}
	if !found || string(got) != `"alice's note"` {
		t.Fatalf("got (%q, %v), want (\"alice's note\", true)", got, found)
	}
}

func TestUserStoreSetUpsertsOnRepeatedWrite(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"v1"`), nil); err != nil {
		t.Fatalf("first set: %v", err)
	}
	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"v2"`), nil); err != nil {
		t.Fatalf("second set: %v", err)
	}

	got, _, found, err := store.Get(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found || string(got) != `"v2"` {
		t.Fatalf("got (%q, %v), want (\"v2\", true) (upsert should overwrite)", got, found)
	}

	entries, err := store.List(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 entry after repeated Set, got %d: %+v", len(entries), entries)
	}
}

// TestUserStoreDistinctKeysAreIndependent pins AC27: writes to distinct keys
// never clobber one another.
func TestUserStoreDistinctKeysAreIndependent(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "a", json.RawMessage(`1`), nil); err != nil {
		t.Fatalf("set a: %v", err)
	}
	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "b", json.RawMessage(`2`), nil); err != nil {
		t.Fatalf("set b: %v", err)
	}

	gotA, _, _, err := store.Get(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "a")
	if err != nil {
		t.Fatalf("get a: %v", err)
	}
	gotB, _, _, err := store.Get(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "b")
	if err != nil {
		t.Fatalf("get b: %v", err)
	}
	if string(gotA) != "1" || string(gotB) != "2" {
		t.Fatalf("got a=%q b=%q, want a=1 b=2 (independent)", gotA, gotB)
	}
}

// TestUserStoreListReturnsOrderedByKey pins AC27's ordering contract, mirroring
// Store.List's ORDER BY state_key.
func TestUserStoreListReturnsOrderedByKey(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	for _, key := range []string{"zeta", "alpha", "mu"} {
		if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", key, json.RawMessage(`1`), nil); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}

	entries, err := store.List(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	want := []string{"alpha", "mu", "zeta"}
	for i, e := range entries {
		if e.Key != want[i] {
			t.Fatalf("entries[%d].Key = %q, want %q (order: %+v)", i, e.Key, want[i], entries)
		}
	}
}

func TestUserStoreListDoesNotLeakAcrossUsers(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"a"`), nil); err != nil {
		t.Fatalf("set user_1: %v", err)
	}

	entries, err := store.List(ctx, "kandev-plugin-notes", "user_2", "task", "task_xyz")
	if err != nil {
		t.Fatalf("list user_2: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries visible to user_2, got %+v", entries)
	}
}

func TestUserStoreDeleteRemovesEntry(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"a"`), nil); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Delete(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, _, found, err := store.Get(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if found {
		t.Fatalf("expected not found after delete")
	}
}

func TestUserStoreDeleteMissingIsNotAnError(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	if err := store.Delete(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "never_set"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
}

// TestUserStoreDeleteAllForPluginPurgesEveryUser pins AC20: uninstalling a
// plugin must purge plugin_user_state rows for every user, not just one.
func TestUserStoreDeleteAllForPluginPurgesEveryUser(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"a"`), nil); err != nil {
		t.Fatalf("set user_1: %v", err)
	}
	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_2", "task", "task_xyz", "note", json.RawMessage(`"b"`), nil); err != nil {
		t.Fatalf("set user_2: %v", err)
	}
	if _, err := store.Set(ctx, "kandev-plugin-other", "user_1", "task", "task_xyz", "note", json.RawMessage(`"c"`), nil); err != nil {
		t.Fatalf("set other plugin: %v", err)
	}

	if err := store.DeleteAllForPlugin(ctx, "kandev-plugin-notes"); err != nil {
		t.Fatalf("DeleteAllForPlugin: %v", err)
	}

	if entries, err := store.List(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz"); err != nil || len(entries) != 0 {
		t.Fatalf("user_1 entries after DeleteAllForPlugin = %v (err=%v), want none", entries, err)
	}
	if entries, err := store.List(ctx, "kandev-plugin-notes", "user_2", "task", "task_xyz"); err != nil || len(entries) != 0 {
		t.Fatalf("user_2 entries after DeleteAllForPlugin = %v (err=%v), want none", entries, err)
	}
	// A different plugin's state is untouched.
	if entries, err := store.List(ctx, "kandev-plugin-other", "user_1", "task", "task_xyz"); err != nil || len(entries) != 1 {
		t.Fatalf("kandev-plugin-other entries after deleting kandev-plugin-notes = %v (err=%v), want 1 (untouched)", entries, err)
	}
}

// TestUserStoreDeleteAllForUserPurgesEveryPlugin covers the DeleteAllForUser
// helper (R6: a future user-deletion cascade hook).
func TestUserStoreDeleteAllForUserPurgesEveryPlugin(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"a"`), nil); err != nil {
		t.Fatalf("set plugin-notes: %v", err)
	}
	if _, err := store.Set(ctx, "kandev-plugin-other", "user_1", "task", "task_xyz", "note", json.RawMessage(`"b"`), nil); err != nil {
		t.Fatalf("set plugin-other: %v", err)
	}
	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_2", "task", "task_xyz", "note", json.RawMessage(`"c"`), nil); err != nil {
		t.Fatalf("set user_2: %v", err)
	}

	if err := store.DeleteAllForUser(ctx, "user_1"); err != nil {
		t.Fatalf("DeleteAllForUser: %v", err)
	}

	if entries, err := store.List(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz"); err != nil || len(entries) != 0 {
		t.Fatalf("user_1/plugin-notes entries after DeleteAllForUser = %v (err=%v), want none", entries, err)
	}
	if entries, err := store.List(ctx, "kandev-plugin-other", "user_1", "task", "task_xyz"); err != nil || len(entries) != 0 {
		t.Fatalf("user_1/plugin-other entries after DeleteAllForUser = %v (err=%v), want none", entries, err)
	}
	if entries, err := store.List(ctx, "kandev-plugin-notes", "user_2", "task", "task_xyz"); err != nil || len(entries) != 1 {
		t.Fatalf("user_2 entries after deleting user_1 = %v (err=%v), want 1 (untouched)", entries, err)
	}
}

// TestUserStoreSetConditionalWriteConflict pins Approach H1 / AC28: a write
// carrying ifUnmodifiedSince older than the stored row's updated_at is
// rejected with ErrConflict and the stored value is left unchanged.
func TestUserStoreSetConditionalWriteConflict(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	firstUpdatedAt, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"v1"`), nil)
	if err != nil {
		t.Fatalf("first set: %v", err)
	}

	stale := firstUpdatedAt.Add(-time.Minute)
	_, err = store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"v2-conflict"`), &stale)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}

	got, _, _, err := store.Get(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != `"v1"` {
		t.Fatalf("stored value changed after a conflicting write: got %q, want \"v1\"", got)
	}
}

// TestUserStoreSetConditionalWriteAcceptsCurrent pins the non-conflict path:
// a write carrying the current stored updated_at (or later) succeeds.
func TestUserStoreSetConditionalWriteAcceptsCurrent(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	firstUpdatedAt, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"v1"`), nil)
	if err != nil {
		t.Fatalf("first set: %v", err)
	}

	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"v2"`), &firstUpdatedAt); err != nil {
		t.Fatalf("second set (current updatedAt): %v", err)
	}

	got, _, _, err := store.Get(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != `"v2"` {
		t.Fatalf("got %q, want \"v2\"", got)
	}
}

// TestUserStoreUpdatedAtRoundTripsWithSubSecondPrecision pins that the
// updated_at Set returns is the same instant Get reads back.
//
// updated_at is persisted as a string, so the layout it is formatted with
// decides the stored resolution. time.RFC3339 carries no fractional seconds,
// which silently truncates every timestamp to a whole second: Set would
// return 06:21:17.825462712Z while Get returned 06:21:17Z for the same row.
// That mismatch is not cosmetic — ifUnmodifiedSince compares against the
// stored value, so a coarser resolution directly widens the window in which
// userStoreHasNewerRow cannot see a modification (see the same-second test
// below).
func TestUserStoreUpdatedAtRoundTripsWithSubSecondPrecision(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	written, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"v1"`), nil)
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	_, read, found, err := store.Get(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note")
	if err != nil || !found {
		t.Fatalf("get: err=%v found=%v", err, found)
	}

	if !read.Equal(written) {
		t.Fatalf("updated_at lost precision in storage: Set returned %s, Get returned %s (delta %s)",
			written.Format(time.RFC3339Nano), read.Format(time.RFC3339Nano), written.Sub(read))
	}
}

// TestUserStateTimeLayoutPreservesLexicographicOrder pins the exact ordering
// bug found in review: time.RFC3339Nano trims trailing zeros from the
// fractional part, so a value landing on a "round" nanosecond (a trailing
// zero) formats shorter than one that doesn't — even when the round value is
// chronologically later. updated_at is compared as TEXT in the
// ifUnmodifiedSince WHERE clause, so that reordering would let a stale
// conditional write through. userStateTimeLayout's fixed width must not
// reproduce it.
func TestUserStateTimeLayoutPreservesLexicographicOrder(t *testing.T) {
	earlier := time.Date(2026, 8, 1, 10, 0, 0, 123456780, time.UTC) // trailing zero trims under RFC3339Nano
	later := time.Date(2026, 8, 1, 10, 0, 0, 123456781, time.UTC)   // 1ns later, no trailing zero

	naiveEarlier, naiveLater := earlier.Format(time.RFC3339Nano), later.Format(time.RFC3339Nano)
	if naiveLater >= naiveEarlier {
		t.Fatalf(
			"test setup invalid: expected time.RFC3339Nano to reproduce the ordering bug (later < earlier), got %q then %q",
			naiveEarlier, naiveLater,
		)
	}

	fixedEarlier, fixedLater := earlier.Format(userStateTimeLayout), later.Format(userStateTimeLayout)
	if fixedEarlier >= fixedLater {
		t.Fatalf("userStateTimeLayout does not preserve chronological order: %q then %q", fixedEarlier, fixedLater)
	}
}

// TestUserStoreSetConditionalWriteDetectsSameSecondModification is the
// regression test for the AC28 blind spot found in QA.
//
// TestUserStoreSetConditionalWriteConflict above passes only because it
// backdates ifUnmodifiedSince by a whole minute. The real scenario is much
// tighter: two of the same user's surfaces (task panel + kanban modal, or a
// debounced autosave firing every ~500ms) write within the same second.
//
// Sequence: tab A reads the note, tab B writes it, tab A then writes with the
// token it read. Tab B's write happened after tab A's read, so tab A must be
// told it lost the race (ErrConflict) rather than silently destroying tab B's
// edit. With second-resolution storage both writes share one timestamp, so
// `updatedAt.After(since)` is false and the guard never fires.
func TestUserStoreSetConditionalWriteDetectsSameSecondModification(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"v0"`), nil); err != nil {
		t.Fatalf("seed set: %v", err)
	}

	// Tab A reads; this timestamp is the token it will later submit.
	_, tabAToken, _, err := store.Get(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note")
	if err != nil {
		t.Fatalf("tab A get: %v", err)
	}

	// Tab B writes immediately afterwards — same wall-clock second.
	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"tabB-edit"`), nil); err != nil {
		t.Fatalf("tab B set: %v", err)
	}

	// Tab A writes with its now-stale token: must be rejected.
	_, err = store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"tabA-edit"`), &tabAToken)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("tab A's stale conditional write was accepted (err=%v); tab B's edit is silently lost", err)
	}

	stored, _, _, err := store.Get(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note")
	if err != nil {
		t.Fatalf("final get: %v", err)
	}
	if string(stored) != `"tabB-edit"` {
		t.Fatalf("stored value = %s, want \"tabB-edit\" preserved", stored)
	}
}

// TestUserStoreSetConditionalWriteAllowsFirstWrite pins that
// ifUnmodifiedSince is a no-op when no row exists yet — a plugin creating a
// document for the first time never needs a nil-vs-zero special case.
func TestUserStoreSetConditionalWriteAllowsFirstWrite(t *testing.T) {
	store := newTestUserStore(t)
	ctx := context.Background()

	since := time.Now().UTC()
	if _, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"v1"`), &since); err != nil {
		t.Fatalf("first set with ifUnmodifiedSince and no existing row: %v", err)
	}
}

func TestUserStoreInitSchemaIsIdempotent(t *testing.T) {
	conn := newUserStoreSQLite(t)
	if _, err := NewUserStore(db.NewPool(conn, conn)); err != nil {
		t.Fatalf("first NewUserStore: %v", err)
	}
	if _, err := NewUserStore(db.NewPool(conn, conn)); err != nil {
		t.Fatalf("second NewUserStore (re-init on existing schema): %v", err)
	}
}

// openIsolatedPostgresConcurrentPool is like testutil.OpenIsolatedPostgres
// but for tests that need real concurrent connections. OpenIsolatedPostgres
// isolates its schema with a session-level `SET search_path`, which only
// applies to the one connection it ran on; raising that *sqlx.DB's
// MaxOpenConns afterwards lets the pool open additional physical
// connections that never ran the SET and fall back to the default schema,
// so plugin_user_state "doesn't exist" the moment a query lands on a fresh
// connection (caught by actually running this test against a live
// container, not just trusting the retry-until-skip path). Baking
// search_path into the DSN via the `options` startup parameter instead
// applies it to every new physical connection at connect time.
func openIsolatedPostgresConcurrentPool(t *testing.T, dsn string, maxOpenConns int) *sqlx.DB {
	t.Helper()
	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	setup, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres (schema setup): %v", err)
	}
	if _, err := setup.Exec("CREATE SCHEMA " + schema); err != nil {
		_ = setup.Close()
		t.Fatalf("create postgres schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		_, _ = setup.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
		_ = setup.Close()
	})

	pooled, err := sqlx.Open("pgx", dsn+" options='-c search_path="+schema+"'")
	if err != nil {
		t.Fatalf("open postgres (pooled): %v", err)
	}
	pooled.SetMaxOpenConns(maxOpenConns)
	t.Cleanup(func() { _ = pooled.Close() })
	return pooled
}

// TestUserStorePostgresConditionalWriteIsRaceFreeUnderConcurrency is the
// regression test for review feedback on Set: a plain read-then-write
// (even wrapped in one transaction) only serializes against Postgres's
// default READ COMMITTED isolation if the read takes a row lock. A bare
// SELECT does not, so two concurrent transactions can each see the row as
// not-yet-conflicting and both proceed to write, silently clobbering one
// another. Set instead folds the ifUnmodifiedSince comparison into the
// upsert's own WHERE clause, making the check-and-write one atomic,
// row-locking statement. Racing many goroutines against the same
// ifUnmodifiedSince token must let exactly one succeed. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestUserStorePostgresConditionalWriteIsRaceFreeUnderConcurrency(t *testing.T) {
	conn := openIsolatedPostgresConcurrentPool(t, testutil.PostgresDSNFromEnv(t), 8)
	store, err := NewUserStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new user store: %v", err)
	}
	ctx := context.Background()

	seededAt, err := store.Set(ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note", json.RawMessage(`"v0"`), nil)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	const attempts = 20
	var wg sync.WaitGroup
	errs := make([]error, attempts)
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = store.Set(
				ctx, "kandev-plugin-notes", "user_1", "task", "task_xyz", "note",
				json.RawMessage(fmt.Sprintf(`"racer-%d"`, i)), &seededAt,
			)
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			// expected for every loser of the race
		default:
			t.Fatalf("unexpected error from concurrent conditional write: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly 1 of %d concurrent conditional writes (same ifUnmodifiedSince) to succeed, got %d",
			attempts, successes)
	}
}

func newTestUserStore(t *testing.T) *UserStore {
	t.Helper()
	conn := newUserStoreSQLite(t)
	store, err := NewUserStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new user store: %v", err)
	}
	return store
}

func newUserStoreSQLite(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
