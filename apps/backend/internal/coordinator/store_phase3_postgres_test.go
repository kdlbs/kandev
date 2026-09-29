package coordinator

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

// newMultiConnStorePostgres opens a store over a pool of several connections
// that all resolve one private schema, so two transactions really run
// concurrently and a lock, not the pool, is what serialises them.
func newMultiConnStorePostgres(t *testing.T) *Store {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	boot := testutil.OpenIsolatedPostgres(t, dsn)
	if _, err := boot.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _, _ = boot.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE") })
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	raw, err := internaldb.OpenPostgres(u.String(), 8, 2)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	pool := sqlx.NewDb(raw, "pgx")
	t.Cleanup(func() { _ = pool.Close() })
	store, err := NewStore(pool, pool)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

// assertSerialised runs two WithWakeLock callers for one coordinator; the
// second must observe the first's committed write and the bodies must never
// overlap.
func assertSerialised(t *testing.T, store *Store, idA string) {
	t.Helper()
	var inside, overlap atomic.Int32
	run := func(name string) error {
		return store.WithWakeLock(context.Background(), idA, func(tx coordinatorExec) error {
			if inside.Add(1) > 1 {
				overlap.Add(1)
			}
			time.Sleep(50 * time.Millisecond) // hold the lock so an overlap is observable
			var n int
			if err := tx.QueryRowContext(context.Background(), store.db.Rebind(`SELECT COUNT(*) FROM coordinator_class_changes WHERE coordinator_id = ?`), idA).Scan(&n); err != nil {
				return err
			}
			_, err := tx.ExecContext(context.Background(), store.db.Rebind(`INSERT INTO coordinator_class_changes (id, coordinator_id, class, from_value, to_value, changed_at) VALUES (?, ?, 'c', ?, ?, ?)`),
				name, idA, "seen", string(rune('0'+n)), time.Now().UTC())
			inside.Add(-1)
			return err
		})
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, name := range []string{"first", "second"} {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = run(name) }()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if overlap.Load() != 0 {
		t.Fatal("two WithWakeLock bodies for one coordinator overlapped")
	}
	rows, err := store.db.Query(store.db.Rebind(`SELECT to_value FROM coordinator_class_changes WHERE coordinator_id = ? ORDER BY to_value`), idA)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var seen []string
	for rows.Next() {
		var v string
		_ = rows.Scan(&v)
		seen = append(seen, v)
	}
	if len(seen) != 2 || seen[0] != "0" || seen[1] != "1" {
		t.Fatalf("the second caller did not see the first's write: %v", seen)
	}
}

func TestWithWakeLock_Postgres_SerialisesOneCoordinator(t *testing.T) {
	store := newMultiConnStorePostgres(t)
	c := newTestCoordinator(t, store, "ws-1")
	assertSerialised(t, store, c.ID)
}

func TestWithWakeLock_Postgres_DifferentCoordinatorsAreIndependent(t *testing.T) {
	store := newMultiConnStorePostgres(t)
	a := newTestCoordinator(t, store, "ws-1")
	b := newTestCoordinator(t, store, "ws-1")
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- store.WithWakeLock(context.Background(), a.ID, func(coordinatorExec) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := store.WithWakeLock(ctx, b.ID, func(coordinatorExec) error { return nil }); err != nil {
		t.Fatalf("a caller for a different coordinator was blocked: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWithWakeLock_Postgres_MissingRowAndRollback(t *testing.T) {
	store := newTestStorePostgres(t)
	c := newTestCoordinator(t, store, "ws-1")
	ran := false
	if err := store.WithWakeLock(context.Background(), "nope", func(coordinatorExec) error { ran = true; return nil }); !errors.Is(err, ErrNotFound) || ran {
		t.Fatalf("missing row: err = %v ran = %v", err, ran)
	}
	boom := context.Canceled
	err := store.WithWakeLock(context.Background(), c.ID, func(tx coordinatorExec) error {
		if _, err := tx.ExecContext(context.Background(), store.db.Rebind(`INSERT INTO coordinator_class_changes (id, coordinator_id, class, from_value, to_value, changed_at) VALUES ('x', ?, 'a', 'b', 'c', ?)`), c.ID, time.Now().UTC()); err != nil {
			return err
		}
		return boom
	})
	if err != boom || count(t, store, "coordinator_class_changes") != 0 {
		t.Fatalf("err = %v", err)
	}
}

func TestPatchAutonomyOff_Postgres_SupersedesUnderWakeLock(t *testing.T) {
	store := newMultiConnStorePostgres(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	insertWake(t, store, c, "w1", "pending")
	off := false
	if _, _, err := store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{AutonomyEnabled: &off}, nil); err != nil {
		t.Fatal(err)
	}
	if got := wakeStatus(t, store, "w1"); got != "superseded" {
		t.Fatalf("wake = %q", got)
	}
	// While another transaction holds the wake lock, an autonomy-off PATCH waits.
	held := make(chan struct{})
	release := make(chan struct{})
	holder := make(chan error, 1)
	go func() {
		holder <- store.WithWakeLock(ctx, c.ID, func(coordinatorExec) error { close(held); <-release; return nil })
	}()
	<-held
	patched := make(chan error, 1)
	go func() {
		_, _, err := store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{AutonomyEnabled: &off}, nil)
		patched <- err
	}()
	select {
	case <-patched:
		t.Fatal("autonomy-off PATCH did not wait for the wake lock")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-holder; err != nil {
		t.Fatal(err)
	}
	if err := <-patched; err != nil {
		t.Fatal(err)
	}
}

func TestPrunePhase3_Postgres(t *testing.T) {
	store := newTestStorePostgres(t)
	c := newTestCoordinator(t, store, "ws-1")
	now := time.Now().UTC()
	insertWakeAt(t, store, c, "old", "delivered", now.Add(-31*24*time.Hour))
	insertWakeAt(t, store, c, "pending", "pending", now.Add(-400*24*time.Hour))
	fin := now.Add(-100 * 24 * time.Hour)
	insertTurn(t, store, c, "t", &fin)
	insertDenial(t, store, "t", "p")
	turns, wakes, err := store.PruneWakeState(context.Background(), now)
	if err != nil || turns != 1 || wakes != 1 {
		t.Fatalf("prune = %d, %d, %v", turns, wakes, err)
	}
	if count(t, store, "coordinator_unattended_denials") != 0 {
		t.Fatal("denials remain")
	}
}

func TestDeletePhase3_Postgres(t *testing.T) {
	store := newTestStorePostgres(t)
	a := newTestCoordinator(t, store, "ws-1")
	b := newTestCoordinator(t, store, "ws-2")
	seedPhase3Rows(t, store, a)
	seedPhase3Rows(t, store, b)
	if err := store.DeleteWorkspaceState(context.Background(), "ws-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteCoordinator(context.Background(), "ws-2", b.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range phase3Tables {
		if count(t, store, table) != 0 {
			t.Fatalf("%s has rows", table)
		}
	}
}

func TestPhase3Schema_Postgres_UpgradeAndOpenTurnIndex(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	pg := testutil.OpenIsolatedPostgres(t, dsn)
	upgradeFromPhase1(t, pg, true)
	store, err := NewStore(pg, pg)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	c := &Coordinator{ID: "c1", WorkspaceID: "w1"}
	insertTurn(t, store, c, "open-1", nil)
	fin := time.Now().UTC()
	insertTurn(t, store, c, "done-1", &fin)
	_, err = pg.Exec(pg.Rebind(`INSERT INTO coordinator_unattended_turns (id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, started_at) VALUES ('open-2', 'c1', 'c', 's', 1, 1, ?)`), time.Now().UTC())
	if err == nil {
		t.Fatal("a second open turn for one coordinator must be refused on PostgreSQL")
	}
	var autonomy int
	if err := pg.QueryRow(`SELECT autonomy_enabled FROM coordinators WHERE id = 'c1'`).Scan(&autonomy); err != nil || autonomy != 0 {
		t.Fatalf("upgraded autonomy_enabled = %d, %v", autonomy, err)
	}
	for _, table := range phase3Tables {
		if len(tableColumns(t, pg, table)) == 0 {
			t.Fatalf("table %s missing after upgrade", table)
		}
	}
}
