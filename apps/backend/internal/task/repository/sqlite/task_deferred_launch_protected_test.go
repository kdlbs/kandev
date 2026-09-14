package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestUpdateTaskPreservesWinningDeferredLaunchAgainstStaleUpdate pins the
// same race TestUpdateTaskPreservesWinningTitleAgainstStaleUpdate already
// pins for agent_title_pending, but for deferred_launch: the session ceiling's
// own compare-and-set writers (SetTaskDeferredLaunchIfUnchanged) own that key,
// so the service layer's request-driven UpdateTaskPreservingDeferredLaunch —
// started before their write and committed after it, still holding the
// pre-CAS in-memory snapshot — must not resurrect or clobber whatever they
// wrote. Before this fix, that path wrote the whole metadata column verbatim
// from the stale snapshot. Plain UpdateTask deliberately keeps its old
// unprotected behavior: many orchestrator-internal callers read a task fresh
// immediately before mutating deferred_launch themselves and writing it back,
// and are not racing the ceiling's CAS writers the way a request-scoped
// snapshot can.
func TestUpdateTaskPreservesWinningDeferredLaunchAgainstStaleUpdate(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-deferred-race")
	if _, err := repo.db.ExecContext(ctx, `
		UPDATE tasks SET metadata = ? WHERE id = ?
	`, `{"deferred_launch":{"prompt":"original","ceiling_launch_kind":"start_created"}}`, "task-deferred-race"); err != nil {
		t.Fatalf("seed deferred launch: %v", err)
	}

	stale, err := repo.GetTask(ctx, "task-deferred-race")
	if err != nil {
		t.Fatalf("load stale task: %v", err)
	}

	// Models the ceiling machinery's own CAS write landing after the stale
	// read above but before the ordinary update below commits.
	_, prior, err := repo.GetTaskDeferredLaunch(ctx, "task-deferred-race")
	if err != nil {
		t.Fatalf("read deferred launch prior: %v", err)
	}
	stored, lostCompare, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "task-deferred-race", prior,
		map[string]interface{}{"prompt": "original", "ceiling_launch_kind": "start_created", "ceiling_deferred": true})
	if err != nil || lostCompare || !stored {
		t.Fatalf("simulate concurrent ceiling write: stored=%v lostCompare=%v err=%v", stored, lostCompare, err)
	}

	// This models a request-scoped task update that started before the CAS
	// write and commits after it, still holding the pre-CAS deferred_launch
	// value.
	stale.Description = "updated concurrently"
	stale.Metadata["stale_change"] = "retained"
	if err := repo.UpdateTaskPreservingDeferredLaunch(ctx, stale); err != nil {
		t.Fatalf("stale UpdateTaskPreservingDeferredLaunch: %v", err)
	}

	current, err := repo.GetTask(ctx, "task-deferred-race")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	deferred, ok := current.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	if !ok {
		t.Fatalf("deferred_launch missing or wrong shape: %#v", current.Metadata[models.MetaKeyDeferredLaunch])
	}
	if deferred["ceiling_deferred"] != true {
		t.Fatalf("deferred_launch = %#v, want the concurrent CAS winner retained", deferred)
	}
	if current.Description != "updated concurrently" {
		t.Fatalf("description = %q, want stale update to retain its unrelated change", current.Description)
	}
	if current.Metadata["stale_change"] != "retained" {
		t.Fatalf("metadata = %#v, want unrelated stale metadata change retained", current.Metadata)
	}
}

// TestUpdateTaskStillDeletesUnrelatedMetadataKeysByOmission guards the fix
// above against the regression it could easily introduce: protecting
// deferred_launch must not turn UpdateTaskPreservingDeferredLaunch's metadata
// write into a general merge. Every other key must keep behaving like a
// plain replace, including a caller deleting one by simply omitting it —
// the pattern service_tasks.go and the workflow engine rely on throughout
// (agent_title_pending, workflow_move_pending, queued_move_exit_pending, …).
func TestUpdateTaskStillDeletesUnrelatedMetadataKeysByOmission(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-omission-delete")
	if _, err := repo.db.ExecContext(ctx, `
		UPDATE tasks SET metadata = ? WHERE id = ?
	`, `{"some_pending_marker":true,"keep":"value"}`, "task-omission-delete"); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}

	task, err := repo.GetTask(ctx, "task-omission-delete")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	delete(task.Metadata, "some_pending_marker")
	if err := repo.UpdateTaskPreservingDeferredLaunch(ctx, task); err != nil {
		t.Fatalf("UpdateTaskPreservingDeferredLaunch: %v", err)
	}

	current, err := repo.GetTask(ctx, "task-omission-delete")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, present := current.Metadata["some_pending_marker"]; present {
		t.Fatalf("metadata = %#v, want the omitted key deleted, not preserved", current.Metadata)
	}
	if current.Metadata["keep"] != "value" {
		t.Fatalf("metadata = %#v, want the untouched key preserved", current.Metadata)
	}
}
