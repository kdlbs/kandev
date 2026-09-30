package sqlite

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

// assertHandoffsJSONEqual compares two handoffs-array JSON strings
// structurally rather than byte-for-byte. PostgreSQL's jsonb column
// re-serializes on every read (for example inserting a space after each
// colon), so a value written as `[{"task_id":"t1"}]` comes back out as
// `[{"task_id": "t1"}]` — a real, dialect-specific difference this test
// exists to catch elsewhere, not one to assert on here.
func assertHandoffsJSONEqual(t *testing.T, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	var gotVal, wantVal interface{}
	if err := json.Unmarshal([]byte(got), &gotVal); err != nil {
		t.Fatalf("got = %q is not valid JSON: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("want = %q is not valid JSON: %v", want, err)
	}
	if !reflect.DeepEqual(gotVal, wantVal) {
		t.Fatalf("got = %q, want %q (structurally different)", got, want)
	}
}

// The handoffs CAS write is a per-dialect JSON statement (see
// metadataKeyUpdateQuery), so SQLite coverage in task_handoffs_cas_test.go
// says nothing about the PostgreSQL statement actually landing. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresSetTaskHandoffsIfUnchangedUsesJSONB(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo := newPostgresMetadataCASRepo(t, db)
	seedMetadataCASTask(t, repo, map[string]interface{}{"other_key": "keep me"})
	ctx := context.Background()

	raw, err := repo.GetTaskHandoffsRaw(ctx, casTaskID)
	if err != nil {
		t.Fatalf("GetTaskHandoffsRaw: %v", err)
	}
	if raw != "" {
		t.Fatalf("raw = %q, want empty before any handoff is recorded", raw)
	}

	stored, current, err := repo.SetTaskHandoffsIfUnchanged(ctx, casTaskID, "", `[{"task_id":"t1"}]`)
	if err != nil {
		t.Fatalf("SetTaskHandoffsIfUnchanged: %v", err)
	}
	if !stored {
		t.Fatal("expected the write to succeed against an empty expected value")
	}
	if current != `[{"task_id":"t1"}]` {
		t.Fatalf("current = %q, want the newly written value echoed back", current)
	}

	raw, err = repo.GetTaskHandoffsRaw(ctx, casTaskID)
	if err != nil {
		t.Fatalf("GetTaskHandoffsRaw after write: %v", err)
	}
	assertHandoffsJSONEqual(t, raw, `[{"task_id":"t1"}]`)
	if value, ok := metadataValue(t, repo, "other_key"); !ok || value != "keep me" {
		t.Fatalf("other_key = %#v (present=%v), want preserved value", value, ok)
	}

	stored, current, err = repo.SetTaskHandoffsIfUnchanged(ctx, casTaskID, "", `[{"task_id":"t2"}]`)
	if err != nil {
		t.Fatalf("SetTaskHandoffsIfUnchanged on stale expected: %v", err)
	}
	if stored {
		t.Fatal("expected the stale-expected write to be refused")
	}
	assertHandoffsJSONEqual(t, current, `[{"task_id":"t1"}]`)
}

// TestPostgresSetTaskHandoffsIfUnchangedLockBlocksConcurrentWriter proves the
// FOR UPDATE branch in lockTaskRowForHandoffs actually serializes concurrent
// appends under PostgreSQL: dialect.IsPostgres is false in every SQLite test,
// so nothing else in this package exercises that branch. It holds the same
// row lock a paused CAS attempt would hold, proves a concurrent
// SetTaskHandoffsIfUnchanged call blocks until the lock is released, and
// proves it observes the up-to-date value once it acquires the lock — the
// same shape as TestPostgresUpdateTaskWithWorkflowStepAdmissionIfAtStep_PreconditionReadLocksRow.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresSetTaskHandoffsIfUnchangedLockBlocksConcurrentWriter(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo := newPostgresMetadataCASRepo(t, db)
	seedMetadataCASTask(t, repo, map[string]interface{}{"other_key": "keep me"})
	ctx := context.Background()

	holder, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin holder tx: %v", err)
	}
	defer func() { _ = holder.Rollback() }()
	var locked string
	if err := holder.QueryRowxContext(ctx, db.Rebind(
		`SELECT metadata FROM tasks WHERE id = ? FOR UPDATE`), casTaskID,
	).Scan(&locked); err != nil {
		t.Fatalf("holder lock read: %v", err)
	}

	writeDone := make(chan struct {
		stored bool
		err    error
	}, 1)
	go func() {
		stored, _, err := repo.SetTaskHandoffsIfUnchanged(ctx, casTaskID, "", `[{"task_id":"concurrent"}]`)
		writeDone <- struct {
			stored bool
			err    error
		}{stored, err}
	}()

	select {
	case result := <-writeDone:
		t.Fatalf("expected the concurrent write to block while the row lock was held, but it completed (stored=%t err=%v)", result.stored, result.err)
	case <-time.After(200 * time.Millisecond):
		// Still blocked, as expected.
	}

	if err := holder.Commit(); err != nil {
		t.Fatalf("commit holder tx: %v", err)
	}

	select {
	case result := <-writeDone:
		if result.err != nil {
			t.Fatalf("SetTaskHandoffsIfUnchanged after lock release: %v", result.err)
		}
		if !result.stored {
			t.Fatal("expected the write to apply once it acquired the row lock")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("expected the blocked write to complete once the holder lock was released")
	}

	raw, err := repo.GetTaskHandoffsRaw(ctx, casTaskID)
	if err != nil {
		t.Fatalf("GetTaskHandoffsRaw: %v", err)
	}
	assertHandoffsJSONEqual(t, raw, `[{"task_id":"concurrent"}]`)
}
