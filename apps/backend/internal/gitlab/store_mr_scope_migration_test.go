package gitlab

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

// legacySwitches names the five task-wide booleans as they were stored
// before this change, so each test reads as the configuration it is
// simulating rather than as five positional bools.
type legacySwitches struct {
	autoFix, autoMerge, review, merged, closed bool
}

// writeLegacyTaskMROptions puts a task back into the pre-scope-migration
// shape: task-wide switch values on gitlab_task_mr_options with the
// mr_scope_migrated_at marker cleared, exactly what an installation upgrading
// across this change starts from.
func writeLegacyTaskMROptions(t *testing.T, store *Store, taskID string, s legacySwitches) {
	t.Helper()
	if _, err := store.db.Exec(`
		INSERT INTO gitlab_task_mr_options (
			task_id, auto_fix_enabled, auto_merge_enabled, prompt_on_review_requested,
			prompt_on_merged, prompt_on_closed, mr_scope_migrated_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(task_id) DO UPDATE SET
			auto_fix_enabled = excluded.auto_fix_enabled,
			auto_merge_enabled = excluded.auto_merge_enabled,
			prompt_on_review_requested = excluded.prompt_on_review_requested,
			prompt_on_merged = excluded.prompt_on_merged,
			prompt_on_closed = excluded.prompt_on_closed,
			mr_scope_migrated_at = NULL`,
		taskID, s.autoFix, s.autoMerge, s.review, s.merged, s.closed); err != nil {
		t.Fatalf("write legacy options for %s: %v", taskID, err)
	}
}

// TestStore_MigrateTaskMROptionsToMRScope_FansOutToEveryLinkedMR covers the
// upgrade path: a task that had the switches on task-wide keeps them on for
// every MR it had linked at the time, so nobody's configured automation
// silently stops working across the upgrade.
func TestStore_MigrateTaskMROptionsToMRScope_FansOutToEveryLinkedMR(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")

	for _, mr := range []*TaskMR{
		newTestMR("task-1", "", "group/a", 1),
		newTestMR("task-1", "", "group/b", 2),
	} {
		if err := store.UpsertTaskMR(ctx, mr); err != nil {
			t.Fatalf("upsert MR %s: %v", mr.ProjectPath, err)
		}
	}
	writeLegacyTaskMROptions(t, store, "task-1", legacySwitches{autoFix: true, merged: true})

	if err := store.migrateTaskMROptionsToMRScope(); err != nil {
		t.Fatalf("migrateTaskMROptionsToMRScope: %v", err)
	}

	stored, err := store.ListTaskMRAutomationOptions(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListTaskMRAutomationOptions: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("expected one row per linked MR, got %+v", stored)
	}
	for _, row := range stored {
		if !row.AutoFixEnabled || !row.PromptOnMerged {
			t.Errorf("MR %s did not inherit the legacy switches: %+v", row.ProjectPath, row)
		}
		if row.AutoMergeEnabled || row.PromptOnReviewRequested || row.PromptOnClosed {
			t.Errorf("MR %s gained a switch the task never had on: %+v", row.ProjectPath, row)
		}
	}
}

// TestStore_MigrateTaskMROptionsToMRScope_DoesNotReplayOverADeliberateDisable
// is what the mr_scope_migrated_at marker exists for: once the fan-out has
// run, a later boot must not re-enable a switch the user has since turned off
// for one specific MR.
func TestStore_MigrateTaskMROptionsToMRScope_DoesNotReplayOverADeliberateDisable(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	first := mrIdentity("group/a", 1)

	if err := store.UpsertTaskMR(ctx, newTestMR("task-1", "", "group/a", 1)); err != nil {
		t.Fatalf("upsert MR: %v", err)
	}
	writeLegacyTaskMROptions(t, store, "task-1", legacySwitches{merged: true})
	if err := store.migrateTaskMROptionsToMRScope(); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	setMRSwitches(t, store, "task-1", first, TaskMRAutomationSwitchPatch{PromptOnMerged: boolPtr(false)})

	// Simulates the next process start.
	if err := store.migrateTaskMROptionsToMRScope(); err != nil {
		t.Fatalf("replayed migration: %v", err)
	}

	got, err := store.GetTaskMRAutomationOptionsForMR(ctx, "task-1", first)
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptionsForMR: %v", err)
	}
	if got.PromptOnMerged {
		t.Fatalf("replayed migration re-enabled a deliberately disabled switch: %+v", got)
	}
}

// TestStore_MigrateTaskMROptionsToMRScope_LaterLinkedMRStartsAllOff pins the
// other half of the marker's job: an MR linked *after* the fan-out must start
// with every switch off rather than inheriting the task's legacy values.
func TestStore_MigrateTaskMROptionsToMRScope_LaterLinkedMRStartsAllOff(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")

	if err := store.UpsertTaskMR(ctx, newTestMR("task-1", "", "group/a", 1)); err != nil {
		t.Fatalf("upsert first MR: %v", err)
	}
	writeLegacyTaskMROptions(t, store, "task-1", legacySwitches{autoFix: true, autoMerge: true, merged: true, closed: true})
	if err := store.migrateTaskMROptionsToMRScope(); err != nil {
		t.Fatalf("first migration: %v", err)
	}

	if err := store.UpsertTaskMR(ctx, newTestMR("task-1", "", "group/late", 9)); err != nil {
		t.Fatalf("upsert later MR: %v", err)
	}
	if err := store.migrateTaskMROptionsToMRScope(); err != nil {
		t.Fatalf("replayed migration: %v", err)
	}

	got, err := store.GetTaskMRAutomationOptionsForMR(ctx, "task-1", mrIdentity("group/late", 9))
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptionsForMR: %v", err)
	}
	if got.AutoFixEnabled || got.AutoMergeEnabled || got.PromptOnMerged || got.PromptOnClosed {
		t.Fatalf("an MR linked after the migration inherited legacy switches: %+v", got)
	}
}

// TestStore_MigrateTaskMROptionsToMRScope_UpgradesADBWithoutTheMarkerColumn
// runs the whole boot path against a database whose gitlab_task_mr_options
// predates mr_scope_migrated_at: the ALTER must add the column and the
// fan-out must then run against the rows already there.
func TestStore_MigrateTaskMROptionsToMRScope_UpgradesADBWithoutTheMarkerColumn(t *testing.T) {
	tmp := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmp, "gitlab-legacy-scope.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	if _, err := sqlxDB.Exec(`
		CREATE TABLE workspaces (id TEXT PRIMARY KEY);
		CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', archived_at DATETIME);
		INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1');
		CREATE TABLE gitlab_task_mr_options (
			task_id TEXT PRIMARY KEY,
			auto_fix_enabled BOOLEAN NOT NULL DEFAULT 0,
			auto_merge_enabled BOOLEAN NOT NULL DEFAULT 0,
			auto_fix_prompt_override TEXT,
			prompt_on_review_requested BOOLEAN NOT NULL DEFAULT 0,
			prompt_on_merged BOOLEAN NOT NULL DEFAULT 0,
			prompt_on_closed BOOLEAN NOT NULL DEFAULT 0,
			review_reviewer_username TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
		INSERT INTO gitlab_task_mr_options (
			task_id, auto_merge_enabled, prompt_on_closed, created_at, updated_at
		) VALUES ('task-1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("NewStore against legacy DB: %v", err)
	}
	ctx := context.Background()
	// The MR is linked before the *second* boot, so the first boot's fan-out
	// found nothing to seed — the marker is already stamped, which is exactly
	// the "starts all-off" contract.
	if err := store.UpsertTaskMR(ctx, newTestMR("task-1", "", "group/a", 1)); err != nil {
		t.Fatalf("upsert MR: %v", err)
	}
	if _, err := NewStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("second boot: %v", err)
	}

	columns, err := store.tableColumns("gitlab_task_mr_options")
	if err != nil {
		t.Fatalf("tableColumns: %v", err)
	}
	if _, ok := columns["mr_scope_migrated_at"]; !ok {
		t.Fatalf("mr_scope_migrated_at missing after upgrading a legacy DB")
	}
	got, err := store.GetTaskMRAutomationOptionsForMR(ctx, "task-1", mrIdentity("group/a", 1))
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptionsForMR: %v", err)
	}
	if got.AutoMergeEnabled || got.PromptOnClosed {
		t.Fatalf("an MR linked after the marker was stamped inherited legacy switches: %+v", got)
	}
}

// TestStore_MigrateTaskMROptionsToMRScope_FansOutOnFirstBootOfALegacyDB is the
// companion to the test above: when the MR was already linked, the very first
// boot after the upgrade must carry the task's switches onto it.
func TestStore_MigrateTaskMROptionsToMRScope_FansOutOnFirstBootOfALegacyDB(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	if err := store.UpsertTaskMR(ctx, newTestMR("task-1", "repo-1", "group/a", 1)); err != nil {
		t.Fatalf("upsert MR: %v", err)
	}
	writeLegacyTaskMROptions(t, store, "task-1", legacySwitches{autoMerge: true, review: true})

	if err := store.migrateTaskMROptionsToMRScope(); err != nil {
		t.Fatalf("migrateTaskMROptionsToMRScope: %v", err)
	}

	got, err := store.GetTaskMRAutomationOptionsForMR(ctx, "task-1",
		MRIdentity{RepositoryID: "repo-1", ProjectPath: "group/a", MRIID: 1})
	if err != nil {
		t.Fatalf("GetTaskMRAutomationOptionsForMR: %v", err)
	}
	if !got.AutoMergeEnabled || !got.PromptOnReviewRequested {
		t.Fatalf("legacy switches did not reach the linked MR: %+v", got)
	}
	if got.RepositoryID != "repo-1" {
		t.Fatalf("fan-out lost the MR's repository identity: %+v", got)
	}
}
