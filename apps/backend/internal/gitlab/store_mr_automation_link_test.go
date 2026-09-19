package gitlab

import (
	"context"
	"strings"
	"testing"
)

func TestStore_UpdateTaskMRAutomationOptionsForMR_LocksLinkBeforeWritingOptions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "")
	id := mrIdentity("group/linked", 7)
	if err := store.UpsertTaskMR(ctx, newTestMR("task-1", "", id.ProjectPath, id.MRIID)); err != nil {
		t.Fatalf("link MR: %v", err)
	}

	// A concurrent unlink must be serialized behind the link validation write.
	// This trigger stands in for a database-side conflict: the old read-only
	// validation never touches the trigger and would incorrectly create an
	// options row, while the locking validation surfaces the conflict before
	// it can write one.
	if _, err := store.db.Exec(`
		CREATE TRIGGER reject_link_update BEFORE UPDATE ON gitlab_task_mrs
		BEGIN SELECT RAISE(ABORT, 'simulated concurrent unlink'); END`); err != nil {
		t.Fatalf("create link conflict trigger: %v", err)
	}

	_, err := store.UpdateTaskMRAutomationOptionsForMR(
		ctx, "task-1", id, TaskMRAutomationSwitchPatch{AutoMergeEnabled: boolPtr(true)},
	)
	if err == nil || !strings.Contains(err.Error(), "simulated concurrent unlink") {
		t.Fatalf("expected link conflict before options write, got %v", err)
	}
	options, err := store.ListTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("list options: %v", err)
	}
	if len(options) != 0 {
		t.Fatalf("link conflict created automation options: %+v", options)
	}
}
