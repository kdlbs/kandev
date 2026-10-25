package inventoryrepair

import (
	"context"
	"os"
	"testing"
)

func assertLifecycleCascade(t *testing.T, f fixture, operation string) {
	t.Helper()
	var count int
	query := `SELECT COUNT(*) FROM tasks WHERE id IN ('owner','child')`
	want := 0
	if operation == "archive" {
		query += ` AND archived_at IS NOT NULL`
		want = 2
	}
	if err := f.db.Get(&count, query); err != nil || count != want {
		t.Fatalf("cascade persistence: %d %v", count, err)
	}
	if operation == "archive" {
		// A terminal borrower session still protects the shared checkout.
		if err := f.db.Get(&count, `SELECT COUNT(*) FROM task_sessions WHERE task_id='child' AND task_environment_id='env'`); err != nil || count != 1 {
			t.Fatalf("archive lost borrower association: %d %v", count, err)
		}
		if err := f.db.Get(&count, `SELECT COUNT(*) FROM task_resource_cleanup_jobs WHERE state='waiting_for_clean'`); err != nil || count != 1 {
			t.Fatalf("archive did not defer reclaim: %d %v", count, err)
		}
		return
	}
	// Group cleanup runs after task deletion. Its ID lookup cannot use the
	// deleted inventory; the durable task cleanup job owns disk removal.
	var status, reason string
	if err := f.db.QueryRow(`SELECT cleanup_status,cleanup_error FROM task_workspace_groups WHERE id='group'`).Scan(&status, &reason); err != nil {
		t.Fatal(err)
	}
	if status != "cleanup_failed" || reason != "worktree wt: worktree not found" {
		t.Fatalf("group cleanup evidence: %s %s", status, reason)
	}
	if err := f.db.Get(&count, `SELECT COUNT(*) FROM task_resource_cleanup_jobs WHERE trigger='cascade_delete' AND state='succeeded'`); err != nil || count != 2 {
		t.Fatalf("durable delete cleanup: %d %v", count, err)
	}
}

// The canonical control distinguishes existing lifecycle outcomes from repair
// regressions. It starts with one owner and one borrower, without running repair.
func TestLegacyAliasLifecycleCanonicalCascade(t *testing.T) {
	for _, operation := range []string{"archive", "delete"} {
		t.Run(operation, func(t *testing.T) {
			f := lifecycleAliasFixture(t, true)
			execSQL(t, f.db, `DELETE FROM task_environment_repos WHERE id IN ('alias','copy')`)
			execSQL(t, f.db, `UPDATE task_sessions SET task_environment_id='env' WHERE id='child-session'`)
			execSQL(t, f.db, `UPDATE task_session_git_snapshots SET task_environment_id='env' WHERE id='snapshot'`)
			execSQL(t, f.db, `DELETE FROM task_environments WHERE id='child-env'`)
			svc, handoff, router := lifecycleServices(t, f)
			if operation == "archive" {
				if _, err := handoff.ArchiveTaskTree(context.Background(), "owner", true); err != nil {
					t.Fatal(err)
				}
			} else {
				lifecycleDelete(t, router, "owner", true, false)
			}
			if err := svc.ResumeTaskResourceCleanupJobs(context.Background()); err != nil {
				t.Fatal(err)
			}
			assertLifecycleCascade(t, f, operation)
			_, err := os.Stat(f.plan.Repairs[0].Path)
			if operation == "archive" && err != nil {
				t.Fatalf("borrowed checkout removed: %v", err)
			}
			if operation == "delete" && !os.IsNotExist(err) {
				t.Fatalf("durable cleanup left checkout: %v", err)
			}
		})
	}
}
