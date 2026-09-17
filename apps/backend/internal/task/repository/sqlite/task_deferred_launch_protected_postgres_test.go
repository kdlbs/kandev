package sqlite

// Postgres parity coverage for updateTaskTx's protectDeferredLaunch metadata
// merge (UpdateTaskPreservingDeferredLaunch), whose `?::jsonb ||
// jsonb_strip_nulls(...)` expression is dialect-sensitive: the JSON payload
// marshaled from a nil in-memory Task.Metadata is the literal scalar `null`,
// and Postgres's jsonb `||` concatenates a scalar with an object into a
// two-element array rather than merging into one, corrupting the metadata
// column. Skips unless KANDEV_TEST_POSTGRES_DSN is set; CI runs these in
// postgres-boot.

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresUpdateTaskPreservingDeferredLaunchWithNilMetadataStaysAnObject
// pins the Postgres-only counterpart of
// TestUpdateTaskPreservingDeferredLaunchWithNilMetadataStaysAnObject: a task
// update carrying nil in-memory Metadata must still leave the row's metadata
// column a well-formed JSON object with the row's own deferred_launch value
// intact, not a corrupted two-element array.
func TestPostgresUpdateTaskPreservingDeferredLaunchWithNilMetadataStaysAnObject(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const taskID = "task-nil-metadata-pg"
	seedPostgresTask(t, repo, taskID)

	stored, lostCompare, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, AbsentDeferredLaunch(),
		map[string]interface{}{"prompt": "original"})
	if err != nil || lostCompare || !stored {
		t.Fatalf("seed deferred launch: stored=%v lostCompare=%v err=%v", stored, lostCompare, err)
	}

	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.Metadata = nil
	task.Description = "updated with nil metadata"
	task.UpdatedAt = time.Now().UTC()
	if err := repo.UpdateTaskPreservingDeferredLaunch(ctx, task); err != nil {
		t.Fatalf("UpdateTaskPreservingDeferredLaunch with nil metadata: %v", err)
	}

	current, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if current.Description != "updated with nil metadata" {
		t.Fatalf("description = %q, want the nil-metadata update to still apply", current.Description)
	}
	deferred, ok := current.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	if !ok {
		t.Fatalf("deferred_launch missing or wrong shape after a nil-metadata update (metadata column corrupted?): %#v",
			current.Metadata)
	}
	if deferred["prompt"] != "original" {
		t.Fatalf("deferred_launch = %#v, want the row's own value preserved", deferred)
	}
}
