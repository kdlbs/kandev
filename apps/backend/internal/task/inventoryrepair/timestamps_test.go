package inventoryrepair

import (
	"testing"
	"time"
)

func TestRepairCleanupTimestampsKeepDatabaseOrdering(t *testing.T) {
	f := cleanupFixture(t)
	p := observedPlan(t, f)
	if err := repairWithIsolatedHost(p, "apply"); err != nil {
		t.Fatal(err)
	}
	var created time.Time
	if err := f.db.QueryRow(`SELECT created_at FROM task_resource_cleanup_jobs WHERE state='pending'`).Scan(&created); err != nil {
		t.Fatal(err)
	}
	var beforeLaterJob bool
	if err := f.db.QueryRow(`SELECT created_at < ? FROM task_resource_cleanup_jobs WHERE state='pending'`, created.Add(time.Second)).Scan(&beforeLaterJob); err != nil {
		t.Fatal(err)
	}
	if !beforeLaterJob {
		t.Fatal("repair timestamp sorts after a later ordinary time.Time bind")
	}
}

func TestRepairRollbackPreservesCleanupTimestampText(t *testing.T) {
	f := cleanupFixture(t)
	execSQL(t, f.db, `UPDATE task_resource_cleanup_jobs SET updated_at='2026-09-28 12:00:00.123+01:00', completed_at='2026-09-28 12:30:00+01:00' WHERE id='job'`)
	var beforeUpdated, beforeCompleted, afterUpdated, afterCompleted string
	if err := f.db.QueryRow(`SELECT CAST(updated_at AS TEXT), CAST(completed_at AS TEXT) FROM task_resource_cleanup_jobs WHERE id='job'`).Scan(&beforeUpdated, &beforeCompleted); err != nil {
		t.Fatal(err)
	}
	p := observedPlan(t, f)
	if err := repairWithIsolatedHost(p, "apply"); err != nil {
		t.Fatal(err)
	}
	if err := repairWithIsolatedHost(p, "rollback"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT CAST(updated_at AS TEXT), CAST(completed_at AS TEXT) FROM task_resource_cleanup_jobs WHERE id='job'`).Scan(&afterUpdated, &afterCompleted); err != nil {
		t.Fatal(err)
	}
	if beforeUpdated != afterUpdated || beforeCompleted != afterCompleted {
		t.Fatalf("timestamp text changed: (%q, %q) -> (%q, %q)", beforeUpdated, beforeCompleted, afterUpdated, afterCompleted)
	}
}
