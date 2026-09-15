package alertsource

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// newTestStore opens a single-connection in-memory SQLite Store, mirroring
// the internal/sentry/store_test.go convention: one connection so writes
// serialize the same way a real single-writer SQLite deployment does.
func newTestStore(t *testing.T) (*Store, *sqlx.DB) {
	t.Helper()
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store, db
}

// newConcurrentTestStore opens a shared-cache in-memory SQLite database with
// multiple connections and a busy timeout, so AC1(ii)'s concurrent-reserve
// test exercises genuine multi-connection contention rather than the
// pool-level serialization newTestStore's single connection would hide.
func newConcurrentTestStore(t *testing.T) (*Store, *sqlx.DB) {
	t.Helper()
	raw, err := sql.Open("sqlite3", "file::memory:?cache=shared&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(8)
	db := sqlx.NewDb(raw, "sqlite3")
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store, db
}

// insertTestSource and insertTestWatch seed the FK-referenced parent rows
// directly: source/watch CRUD is T06 (D19), so Store exposes no create
// method for either and tests populate them with raw SQL.
func insertTestSource(t *testing.T, db *sqlx.DB, id, workspaceID, name string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := db.Exec(db.Rebind(`
		INSERT INTO alert_sources (id, workspace_id, type, name, created_at, updated_at)
		VALUES (?, ?, 'datadog', ?, ?, ?)`), id, workspaceID, name, now, now)
	if err != nil {
		t.Fatalf("insert test source: %v", err)
	}
}

func insertTestWatch(t *testing.T, db *sqlx.DB, id, sourceID, workspaceID string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := db.Exec(db.Rebind(`
		INSERT INTO alert_watches (id, workspace_id, source_id, workflow_id, workflow_step_id, created_at, updated_at)
		VALUES (?, ?, ?, 'wf-1', 'step-1', ?, ?)`), id, workspaceID, sourceID, now, now)
	if err != nil {
		t.Fatalf("insert test watch: %v", err)
	}
}

// seedSourceAndWatch is the common fixture most reservation tests need: one
// source, one watch bound to it.
func seedSourceAndWatch(t *testing.T, db *sqlx.DB) {
	t.Helper()
	insertTestSource(t, db, "src-1", "ws-1", "s1")
	insertTestWatch(t, db, "watch-1", "src-1", "ws-1")
}

func testFingerprint(seed string) string {
	return Fingerprint(map[string]string{"seed": seed}, []string{"seed"})
}

// --- AC1(ii): concurrent ReserveFingerprint race ---

func TestStore_ReserveFingerprint_ConcurrentRace_ExactlyOneWins(t *testing.T) {
	store, db := newConcurrentTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("race")

	const n = 20
	var wg sync.WaitGroup
	results := make([]bool, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			reserved, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil)
			results[i] = reserved
			errs[i] = err
		}(i)
	}
	wg.Wait()

	wins := 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: unexpected error: %v", i, err)
		}
		if results[i] {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly 1 winner among %d goroutines, got %d", n, wins)
	}

	var count int
	if err := db.Get(&count, db.Rebind(
		`SELECT COUNT(*) FROM alert_reservations WHERE watch_id = ? AND fingerprint = ?`),
		"watch-1", fp); err != nil {
		t.Fatalf("count reservations: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 live row, got %d", count)
	}
}

// --- (a) ReserveFingerprint ---

func TestStore_ReserveFingerprint_EmptyWatchID(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	if _, err := store.ReserveFingerprint(context.Background(), "", testFingerprint("x"), nil); err == nil {
		t.Fatal("expected error for empty watchID")
	}
}

func TestStore_ReserveFingerprint_InvalidFingerprint(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	for _, fp := range []string{"", "too-short", "UPPERCASEUPPERCASEUPPERCASEUPPERCASEUPPERCASEUPPERCASEUPPERCASE"} {
		if _, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil); err == nil {
			t.Fatalf("expected error for fingerprint %q", fp)
		}
	}
}

func TestStore_ReserveFingerprint_FirstReserveWinsSecondLoses(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("dup")

	reserved, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, []byte(`{"a":1}`))
	if err != nil || !reserved {
		t.Fatalf("first reserve: reserved=%v err=%v", reserved, err)
	}
	reserved, err = store.ReserveFingerprint(context.Background(), "watch-1", fp, nil)
	if err != nil || reserved {
		t.Fatalf("second reserve: expected reserved=false err=nil, got reserved=%v err=%v", reserved, err)
	}
}

func TestStore_ReserveFingerprint_NilAlertRawStoredAsEmptyString(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("nil-raw")
	if _, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	var raw sql.NullString
	if err := db.Get(&raw, db.Rebind(`SELECT alert_raw FROM alert_reservations WHERE watch_id = ? AND fingerprint = ?`),
		"watch-1", fp); err != nil {
		t.Fatalf("read alert_raw: %v", err)
	}
	if !raw.Valid || raw.String != "" {
		t.Fatalf("expected alert_raw = '', got %+v", raw)
	}
}

// --- (b) AttachReservationTaskID ---

func TestStore_AttachReservationTaskID_EmptyArgs(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("attach-empty")
	if err := store.AttachReservationTaskID(context.Background(), "", fp, "task-1"); err == nil {
		t.Fatal("expected error for empty watchID")
	}
	if err := store.AttachReservationTaskID(context.Background(), "watch-1", "not-a-fingerprint", "task-1"); err == nil {
		t.Fatal("expected error for invalid fingerprint")
	}
	if err := store.AttachReservationTaskID(context.Background(), "watch-1", fp, ""); err == nil {
		t.Fatal("expected error for empty taskID")
	}
}

func TestStore_AttachReservationTaskID_FreshAttachSucceeds(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("attach-fresh")
	if _, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.AttachReservationTaskID(context.Background(), "watch-1", fp, "task-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	var taskID string
	if err := db.Get(&taskID, db.Rebind(`SELECT task_id FROM alert_reservations WHERE watch_id = ? AND fingerprint = ?`),
		"watch-1", fp); err != nil {
		t.Fatalf("read task_id: %v", err)
	}
	if taskID != "task-1" {
		t.Fatalf("expected task_id = task-1, got %q", taskID)
	}
}

func TestStore_AttachReservationTaskID_IdempotentForOwnTaskID(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("attach-idempotent")
	if _, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.AttachReservationTaskID(context.Background(), "watch-1", fp, "task-1"); err != nil {
		t.Fatalf("first attach: %v", err)
	}
	// Simulates a retry after an ambiguous commit: same taskID, must succeed
	// again rather than being classified as AttachedElsewhere.
	if err := store.AttachReservationTaskID(context.Background(), "watch-1", fp, "task-1"); err != nil {
		t.Fatalf("retry attach with same taskID: %v", err)
	}
}

func TestStore_AttachReservationTaskID_DifferentTaskIDReturnsAttachedElsewhere(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("attach-conflict")
	if _, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.AttachReservationTaskID(context.Background(), "watch-1", fp, "task-1"); err != nil {
		t.Fatalf("first attach: %v", err)
	}
	err := store.AttachReservationTaskID(context.Background(), "watch-1", fp, "task-2")
	if !errors.Is(err, ErrReservationAttachedElsewhere) {
		t.Fatalf("expected ErrReservationAttachedElsewhere, got %v", err)
	}
}

func TestStore_AttachReservationTaskID_NeverReservedReturnsGone(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("attach-never-reserved")
	err := store.AttachReservationTaskID(context.Background(), "watch-1", fp, "task-1")
	if !errors.Is(err, ErrReservationGone) {
		t.Fatalf("expected ErrReservationGone, got %v", err)
	}
}

func TestStore_AttachReservationTaskID_ReleasedReturnsGone(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("attach-released")
	if _, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.DeleteReservation(context.Background(), "watch-1", fp); err != nil {
		t.Fatalf("delete: %v", err)
	}
	err := store.AttachReservationTaskID(context.Background(), "watch-1", fp, "task-1")
	if !errors.Is(err, ErrReservationGone) {
		t.Fatalf("expected ErrReservationGone, got %v", err)
	}
}

// TestStore_AttachReservationTaskID_DiagnosticSelectFailureIsNeverGone
// covers A5: a store/transport error on the classification SELECT must be
// returned wrapped, never as ErrReservationGone — T06 retries a plain error
// but stops (and releases) on the sentinel, so misclassifying a transient
// failure as Gone would suppress the fingerprint permanently and silently.
// The diagnostic SELECT runs against the store's read handle; closing it
// after the write handle has done its job forces exactly that failure.
func TestStore_AttachReservationTaskID_DiagnosticSelectFailureIsNeverGone(t *testing.T) {
	writerRaw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	writer := sqlx.NewDb(writerRaw, "sqlite3")
	t.Cleanup(func() { _ = writer.Close() })
	if _, err := writer.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	readerRaw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	reader := sqlx.NewDb(readerRaw, "sqlite3")
	t.Cleanup(func() { _ = reader.Close() })

	store, err := NewStore(writer, reader)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	seedSourceAndWatch(t, writer)
	fp := testFingerprint("attach-select-fails")

	// Break the read handle only. The UPDATE (on writer) affects zero rows
	// because nothing was ever reserved, so AttachReservationTaskID reaches
	// classifyAttachFailure's SELECT, which must now fail.
	if err := reader.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}

	err = store.AttachReservationTaskID(context.Background(), "watch-1", fp, "task-1")
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, ErrReservationGone) {
		t.Fatalf("a store error on the diagnostic SELECT must never be reported as ErrReservationGone, got %v", err)
	}
	if errors.Is(err, ErrReservationAttachedElsewhere) {
		t.Fatalf("a store error on the diagnostic SELECT must never be reported as ErrReservationAttachedElsewhere, got %v", err)
	}
}

// --- (c) DeleteReservation ---

func TestStore_DeleteReservation_EmptyArgs(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	if err := store.DeleteReservation(context.Background(), "", testFingerprint("x")); err == nil {
		t.Fatal("expected error for empty watchID")
	}
	if err := store.DeleteReservation(context.Background(), "watch-1", "bad"); err == nil {
		t.Fatal("expected error for invalid fingerprint")
	}
}

func TestStore_DeleteReservation_DeletesLiveReservation(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("delete-live")
	if _, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.DeleteReservation(context.Background(), "watch-1", fp); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var count int
	if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM alert_reservations WHERE watch_id = ? AND fingerprint = ?`),
		"watch-1", fp); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 rows after delete, got %d", count)
	}
}

func TestStore_DeleteReservation_NoOpOnMissingRow(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	if err := store.DeleteReservation(context.Background(), "watch-1", testFingerprint("never-existed")); err != nil {
		t.Fatalf("expected nil (no-op), got %v", err)
	}
}

// TestStore_DeleteReservation_NoOpOnAttachedReservation guards the
// task_id = ” condition against a concurrent AttachReservationTaskID: once a
// reservation is attached to a task, DeleteReservation must leave it in
// place rather than deleting the record a task now depends on.
func TestStore_DeleteReservation_NoOpOnAttachedReservation(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("delete-attached")
	if _, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.AttachReservationTaskID(context.Background(), "watch-1", fp, "task-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := store.DeleteReservation(context.Background(), "watch-1", fp); err != nil {
		t.Fatalf("expected nil (no-op), got %v", err)
	}
	var count int
	if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM alert_reservations WHERE watch_id = ? AND fingerprint = ?`),
		"watch-1", fp); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected the attached reservation to survive DeleteReservation, got %d rows", count)
	}
}

// --- (d) ReleaseReservationForTask ---

func TestStore_ReleaseReservationForTask_EmptyTaskIDDoesNotReleaseUnattachedRows(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("release-empty-taskid")
	if _, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.ReleaseReservationForTask(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty taskID")
	}
	var releasedAt sql.NullTime
	if err := db.Get(&releasedAt, db.Rebind(`SELECT released_at FROM alert_reservations WHERE watch_id = ? AND fingerprint = ?`),
		"watch-1", fp); err != nil {
		t.Fatalf("read released_at: %v", err)
	}
	if releasedAt.Valid {
		t.Fatal("expected the unattached reservation to remain live")
	}
}

func TestStore_ReleaseReservationForTask_ReleasesAttachedReservation(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("release-attached")
	if _, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.AttachReservationTaskID(context.Background(), "watch-1", fp, "task-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := store.ReleaseReservationForTask(context.Background(), "task-1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	var releasedAt sql.NullTime
	if err := db.Get(&releasedAt, db.Rebind(`SELECT released_at FROM alert_reservations WHERE watch_id = ? AND fingerprint = ?`),
		"watch-1", fp); err != nil {
		t.Fatalf("read released_at: %v", err)
	}
	if !releasedAt.Valid {
		t.Fatal("expected released_at to be set")
	}
}

func TestStore_ReleaseReservationForTask_NoOpWhenNoMatch(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	if err := store.ReleaseReservationForTask(context.Background(), "task-does-not-exist"); err != nil {
		t.Fatalf("expected nil (no-op), got %v", err)
	}
}

func TestStore_ReleaseReservationForTask_IdempotentAgainstRedeliveredClose(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("release-idempotent")
	if _, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.AttachReservationTaskID(context.Background(), "watch-1", fp, "task-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := store.ReleaseReservationForTask(context.Background(), "task-1"); err != nil {
		t.Fatalf("first release: %v", err)
	}
	if err := store.ReleaseReservationForTask(context.Background(), "task-1"); err != nil {
		t.Fatalf("redelivered release: %v", err)
	}
}

// --- (e) ReleaseOrphanedReservation ---

func TestStore_ReleaseOrphanedReservation_EmptyArgs(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	if err := store.ReleaseOrphanedReservation(context.Background(), "", testFingerprint("x")); err == nil {
		t.Fatal("expected error for empty watchID")
	}
	if err := store.ReleaseOrphanedReservation(context.Background(), "watch-1", "bad"); err == nil {
		t.Fatal("expected error for invalid fingerprint")
	}
}

func TestStore_ReleaseOrphanedReservation_ReleasesOnlyUnattachedRow(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("release-orphan")
	if _, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.ReleaseOrphanedReservation(context.Background(), "watch-1", fp); err != nil {
		t.Fatalf("release orphaned: %v", err)
	}
	var releasedAt sql.NullTime
	var taskID string
	if err := db.QueryRow(db.Rebind(`SELECT released_at, task_id FROM alert_reservations WHERE watch_id = ? AND fingerprint = ?`),
		"watch-1", fp).Scan(&releasedAt, &taskID); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if !releasedAt.Valid || taskID != "" {
		t.Fatalf("expected released_at set and task_id empty, got released_at.Valid=%v task_id=%q", releasedAt.Valid, taskID)
	}
}

func TestStore_ReleaseOrphanedReservation_CannotReleaseAttachedReservation(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("release-orphan-guard")
	if _, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.AttachReservationTaskID(context.Background(), "watch-1", fp, "task-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := store.ReleaseOrphanedReservation(context.Background(), "watch-1", fp); err != nil {
		t.Fatalf("release orphaned: %v", err)
	}
	var releasedAt sql.NullTime
	if err := db.Get(&releasedAt, db.Rebind(`SELECT released_at FROM alert_reservations WHERE watch_id = ? AND fingerprint = ?`),
		"watch-1", fp); err != nil {
		t.Fatalf("read released_at: %v", err)
	}
	if releasedAt.Valid {
		t.Fatal("expected the attached reservation to remain live")
	}
}

// --- AC3: introspective exact column list ---

type sqliteColumn struct {
	Cid       int            `db:"cid"`
	Name      string         `db:"name"`
	Type      string         `db:"type"`
	NotNull   int            `db:"notnull"`
	DfltValue sql.NullString `db:"dflt_value"`
	PK        int            `db:"pk"`
}

func tableColumns(t *testing.T, db *sqlx.DB, table string) []sqliteColumn {
	t.Helper()
	var cols []sqliteColumn
	if err := db.Select(&cols, `PRAGMA table_info(`+table+`)`); err != nil {
		t.Fatalf("table_info(%s): %v", table, err)
	}
	return cols
}

func TestStore_Schema_AlertSourcesColumns(t *testing.T) {
	_, db := newTestStore(t)
	got := tableColumns(t, db, "alert_sources")
	want := []sqliteColumn{
		{Name: "id", Type: "TEXT", NotNull: 0, PK: 1},
		{Name: "workspace_id", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "type", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "name", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "config_json", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "enabled", Type: "BOOLEAN", NotNull: 1, PK: 0},
		{Name: "health_status", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "last_error", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "last_error_at", Type: "DATETIME", NotNull: 0, PK: 0},
		{Name: "created_at", Type: "DATETIME", NotNull: 1, PK: 0},
		{Name: "updated_at", Type: "DATETIME", NotNull: 1, PK: 0},
	}
	assertColumnsEqual(t, "alert_sources", want, got)
}

func TestStore_Schema_AlertWatchesColumns(t *testing.T) {
	_, db := newTestStore(t)
	got := tableColumns(t, db, "alert_watches")
	want := []sqliteColumn{
		{Name: "id", Type: "TEXT", NotNull: 0, PK: 1},
		{Name: "workspace_id", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "source_id", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "workflow_id", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "workflow_step_id", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "repository_id", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "base_branch", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "filter_json", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "agent_profile_id", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "executor_profile_id", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "prompt", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "enabled", Type: "BOOLEAN", NotNull: 1, PK: 0},
		{Name: "poll_interval_seconds", Type: "INTEGER", NotNull: 1, PK: 0},
		{Name: "max_inflight_tasks", Type: "INTEGER", NotNull: 0, PK: 0},
		{Name: "last_polled_at", Type: "DATETIME", NotNull: 0, PK: 0},
		{Name: "last_error", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "last_error_at", Type: "DATETIME", NotNull: 0, PK: 0},
		{Name: "created_at", Type: "DATETIME", NotNull: 1, PK: 0},
		{Name: "updated_at", Type: "DATETIME", NotNull: 1, PK: 0},
	}
	assertColumnsEqual(t, "alert_watches", want, got)
}

func TestStore_Schema_AlertReservationsColumns(t *testing.T) {
	_, db := newTestStore(t)
	got := tableColumns(t, db, "alert_reservations")
	want := []sqliteColumn{
		{Name: "id", Type: "TEXT", NotNull: 0, PK: 1},
		{Name: "watch_id", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "fingerprint", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "task_id", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "alert_raw", Type: "TEXT", NotNull: 1, PK: 0},
		{Name: "created_at", Type: "DATETIME", NotNull: 1, PK: 0},
		{Name: "released_at", Type: "DATETIME", NotNull: 0, PK: 0},
	}
	assertColumnsEqual(t, "alert_reservations", want, got)

	for _, c := range got {
		if c.Name == "updated_at" {
			t.Fatal("alert_reservations must not have an updated_at column (D14)")
		}
	}
}

func assertColumnsEqual(t *testing.T, table string, want, got []sqliteColumn) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: expected %d columns, got %d: %+v", table, len(want), len(got), got)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Name != w.Name || g.Type != w.Type || g.NotNull != w.NotNull || g.PK != w.PK {
			t.Fatalf("%s: column %d mismatch: want {Name:%s Type:%s NotNull:%d PK:%d}, got {Name:%s Type:%s NotNull:%d PK:%d}",
				table, i, w.Name, w.Type, w.NotNull, w.PK, g.Name, g.Type, g.NotNull, g.PK)
		}
	}
}

// --- AC3: behavioral DDL assertions ---

func TestStore_Schema_SourceDeleteRestrictedWhileWatchExists(t *testing.T) {
	store, db := newTestStore(t)
	_ = store
	seedSourceAndWatch(t, db)
	if _, err := db.Exec(db.Rebind(`DELETE FROM alert_sources WHERE id = ?`), "src-1"); err == nil {
		t.Fatal("expected the delete to be rejected by ON DELETE RESTRICT")
	}
	var count int
	if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM alert_sources WHERE id = ?`), "src-1"); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatal("expected the source row to still be present")
	}
}

func TestStore_Schema_WatchDeleteCascadesToReservations(t *testing.T) {
	store, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	fp := testFingerprint("cascade")
	if _, err := store.ReserveFingerprint(context.Background(), "watch-1", fp, nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`DELETE FROM alert_watches WHERE id = ?`), "watch-1"); err != nil {
		t.Fatalf("delete watch: %v", err)
	}
	var count int
	if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM alert_reservations WHERE watch_id = ?`), "watch-1"); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatal("expected the watch's reservations to be removed by ON DELETE CASCADE")
	}
}

func TestStore_Schema_HealthStatusCheckRejectsBogusValue(t *testing.T) {
	_, db := newTestStore(t)
	now := time.Now().UTC()
	_, err := db.Exec(db.Rebind(`
		INSERT INTO alert_sources (id, workspace_id, type, name, health_status, created_at, updated_at)
		VALUES (?, ?, 'datadog', 's-bogus', 'bogus', ?, ?)`), "src-bogus", "ws-1", now, now)
	if err == nil {
		t.Fatal("expected the health_status CHECK to reject 'bogus'")
	}
}

func TestStore_Schema_FingerprintWidthCheckRejectsShortValue(t *testing.T) {
	_, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	now := time.Now().UTC()
	full := testFingerprint("width")
	short := full[:63]
	_, err := db.Exec(db.Rebind(`
		INSERT INTO alert_reservations (id, watch_id, fingerprint, task_id, alert_raw, created_at)
		VALUES (?, ?, ?, '', '', ?)`), "res-short", "watch-1", short, now)
	if err == nil {
		t.Fatal("expected the fingerprint width CHECK to reject a 63-character value")
	}
}

func TestStore_Schema_UniqueWorkspaceAndName(t *testing.T) {
	_, db := newTestStore(t)
	insertTestSource(t, db, "src-a", "ws-1", "dup-name")
	now := time.Now().UTC()
	_, err := db.Exec(db.Rebind(`
		INSERT INTO alert_sources (id, workspace_id, type, name, created_at, updated_at)
		VALUES (?, ?, 'datadog', ?, ?, ?)`), "src-b", "ws-1", "dup-name", now, now)
	if err == nil {
		t.Fatal("expected UNIQUE(workspace_id, name) to reject a duplicate")
	}
}

func TestStore_Schema_Defaults(t *testing.T) {
	_, db := newTestStore(t)
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO alert_sources (id, workspace_id, type, name, created_at, updated_at)
		VALUES (?, ?, 'datadog', 's1', ?, ?)`), "src-defaults", "ws-1", now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	var configJSON, healthStatus string
	var enabled bool
	if err := db.QueryRow(db.Rebind(`SELECT config_json, health_status, enabled FROM alert_sources WHERE id = ?`),
		"src-defaults").Scan(&configJSON, &healthStatus, &enabled); err != nil {
		t.Fatalf("read source defaults: %v", err)
	}
	if configJSON != "{}" || healthStatus != "unknown" || !enabled {
		t.Fatalf("source defaults: config_json=%q health_status=%q enabled=%v", configJSON, healthStatus, enabled)
	}

	if _, err := db.Exec(db.Rebind(`
		INSERT INTO alert_watches (id, workspace_id, source_id, workflow_id, workflow_step_id, created_at, updated_at)
		VALUES (?, ?, ?, 'wf-1', 'step-1', ?, ?)`), "watch-defaults", "ws-1", "src-defaults", now, now); err != nil {
		t.Fatalf("insert watch: %v", err)
	}
	var pollInterval, maxInflight sql.NullInt64
	var watchEnabled bool
	if err := db.QueryRow(db.Rebind(`SELECT poll_interval_seconds, max_inflight_tasks, enabled FROM alert_watches WHERE id = ?`),
		"watch-defaults").Scan(&pollInterval, &maxInflight, &watchEnabled); err != nil {
		t.Fatalf("read watch defaults: %v", err)
	}
	if !pollInterval.Valid || pollInterval.Int64 != 300 {
		t.Fatalf("expected poll_interval_seconds default 300, got %+v", pollInterval)
	}
	if !maxInflight.Valid || maxInflight.Int64 != 5 {
		t.Fatalf("expected max_inflight_tasks default 5, got %+v", maxInflight)
	}
	if !watchEnabled {
		t.Fatal("expected watch enabled default true")
	}

	if _, err := db.Exec(db.Rebind(`
		INSERT INTO alert_reservations (id, watch_id, fingerprint, created_at)
		VALUES (?, ?, ?, ?)`), "res-defaults", "watch-defaults", testFingerprint("defaults"), now); err != nil {
		t.Fatalf("insert reservation: %v", err)
	}
	var taskID, alertRaw string
	if err := db.QueryRow(db.Rebind(`SELECT task_id, alert_raw FROM alert_reservations WHERE id = ?`),
		"res-defaults").Scan(&taskID, &alertRaw); err != nil {
		t.Fatalf("read reservation defaults: %v", err)
	}
	if taskID != "" || alertRaw != "" {
		t.Fatalf("expected task_id and alert_raw to default to '', got task_id=%q alert_raw=%q", taskID, alertRaw)
	}
}

// TestStore_Schema_CaseSensitiveCollation inserts directly below the store
// API (isFingerprint would reject the uppercase value at the Go boundary)
// to assert that two fingerprints differing only in case are admitted as
// two distinct live rows under the partial unique index — a
// case-insensitive default collation would collapse them (D14, D16.3).
func TestStore_Schema_CaseSensitiveCollation(t *testing.T) {
	_, db := newTestStore(t)
	seedSourceAndWatch(t, db)
	now := time.Now().UTC()
	lower := testFingerprint("collation")
	upper := make([]byte, len(lower))
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if c >= 'a' && c <= 'f' {
			c -= 'a' - 'A'
		}
		upper[i] = c
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO alert_reservations (id, watch_id, fingerprint, created_at)
		VALUES (?, ?, ?, ?)`), "res-lower", "watch-1", lower, now); err != nil {
		t.Fatalf("insert lowercase: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO alert_reservations (id, watch_id, fingerprint, created_at)
		VALUES (?, ?, ?, ?)`), "res-upper", "watch-1", string(upper), now); err != nil {
		t.Fatalf("insert uppercase: %v", err)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM alert_reservations WHERE released_at IS NULL`); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 distinct live rows under case-sensitive collation, got %d", count)
	}
}
