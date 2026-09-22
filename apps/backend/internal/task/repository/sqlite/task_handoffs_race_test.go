package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestFullMetadataWrite_PreservesLiveHandoffProvenance proves the fix for a
// correctness gap in the handoffs-array CAS: SetTaskHandoffsIfUnchanged
// (task_handoffs_cas.go) is a correct key-scoped compare-and-set, but every
// full-row task write (UpdateTask, UpdateTaskIfWorkflowMatches,
// UpdateTaskWithExplicitPosition, UpdateTaskPreservingDeferredLaunch)
// marshals whatever metadata is already in the caller's in-memory
// *models.Task and overwrites the whole metadata column. If that in-memory
// snapshot was read before a concurrent handoffs CAS append committed, the
// full-row write must not silently revert the handoffs (or handoff_source)
// key back to its stale pre-append value — an ordinary unrelated PATCH
// racing a handoff append must never lose the reverse-link provenance the
// CAS just wrote. UpdateTaskPreservingDeferredLaunch additionally strips and
// re-splices deferred_launch (buildTaskUpdateQuery's protectDeferredLaunch
// branch), which must operate on the already-live-merged payload rather than
// rebuilding from the stale in-memory snapshot, or it silently discards this
// same handoff-provenance merge a second time.
func TestFullMetadataWrite_PreservesLiveHandoffProvenance(t *testing.T) {
	writers := map[string]func(repo *Repository, task *models.Task) error{
		"UpdateTask": func(repo *Repository, task *models.Task) error {
			return repo.UpdateTask(context.Background(), task)
		},
		"UpdateTaskWithExplicitPosition": func(repo *Repository, task *models.Task) error {
			return repo.UpdateTaskWithExplicitPosition(context.Background(), task)
		},
		"UpdateTaskIfWorkflowMatches": func(repo *Repository, task *models.Task) error {
			return repo.UpdateTaskIfWorkflowMatches(context.Background(), task, "wf-cas")
		},
		"UpdateTaskPreservingDeferredLaunch": func(repo *Repository, task *models.Task) error {
			return repo.UpdateTaskPreservingDeferredLaunch(context.Background(), task)
		},
	}

	for name, write := range writers {
		t.Run(name, func(t *testing.T) {
			repo := newRepoForMetadataCASTests(t)
			seedMetadataCASTask(t, repo, map[string]interface{}{
				"other_key":                 "keep me",
				models.MetaKeyHandoffSource: map[string]interface{}{"source_task_id": "origin-task"},
			})
			ctx := context.Background()

			stored, _, err := repo.SetTaskHandoffsIfUnchanged(ctx, casTaskID, "", `[{"task_id":"t1"}]`)
			if err != nil || !stored {
				t.Fatalf("seed handoff append failed: stored=%v err=%v", stored, err)
			}

			// Simulate the service's request-time snapshot: read once, then
			// mutate in memory, exactly as Service.UpdateTask does before its
			// own write transaction opens.
			staleTask, err := repo.GetTask(ctx, casTaskID)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}

			// A second handoff commits after the snapshot above was taken but
			// before the unrelated write below runs.
			stored, _, err = repo.SetTaskHandoffsIfUnchanged(ctx, casTaskID, `[{"task_id":"t1"}]`, `[{"task_id":"t1"},{"task_id":"t2"}]`)
			if err != nil || !stored {
				t.Fatalf("concurrent handoff append failed: stored=%v err=%v", stored, err)
			}

			staleTask.Description = "updated by an unrelated write"
			if err := write(repo, staleTask); err != nil {
				t.Fatalf("write: %v", err)
			}

			raw, err := repo.GetTaskHandoffsRaw(ctx, casTaskID)
			if err != nil {
				t.Fatalf("GetTaskHandoffsRaw: %v", err)
			}
			if raw != `[{"task_id":"t1"},{"task_id":"t2"}]` {
				t.Fatalf("raw = %q, want the concurrently-appended value preserved, not the stale snapshot's", raw)
			}

			updated, err := repo.GetTask(ctx, casTaskID)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if updated.Description != "updated by an unrelated write" {
				t.Fatalf("Description = %q, want the unrelated field's write to still apply", updated.Description)
			}
			if _, ok := updated.Metadata[models.MetaKeyHandoffSource]; !ok {
				t.Fatal("handoff_source was dropped by the unrelated write, want it preserved")
			}
			if v, ok := updated.Metadata["other_key"]; !ok || v != "keep me" {
				t.Fatalf("other_key = %v (ok=%v), want untouched neighbouring metadata preserved", v, ok)
			}
		})
	}
}

// TestFullMetadataWrite_HandoffProvenanceAbsentStaysAbsent proves the
// preservation logic does not resurrect a handoffs/handoff_source key that
// has never been written: a task with no provenance yet must not gain one
// from an unrelated full-row write.
func TestFullMetadataWrite_HandoffProvenanceAbsentStaysAbsent(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{"other_key": "keep me"})
	ctx := context.Background()

	task, err := repo.GetTask(ctx, casTaskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	task.Description = "first write"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	raw, err := repo.GetTaskHandoffsRaw(ctx, casTaskID)
	if err != nil {
		t.Fatalf("GetTaskHandoffsRaw: %v", err)
	}
	if raw != "" {
		t.Fatalf("raw = %q, want still absent", raw)
	}

	updated, err := repo.GetTask(ctx, casTaskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if _, ok := updated.Metadata[models.MetaKeyHandoffSource]; ok {
		t.Fatal("handoff_source appeared from nowhere, want it to remain absent")
	}
}
