package inventoryrepair

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewRejectsStoppedRowWithLivePID(t *testing.T) {
	f := newFixture(t)
	execSQL(t, f.db, `INSERT INTO executors_running(id,session_id,task_id,executor_id,status,pid,created_at,updated_at) VALUES('runtime','session','owner','exec-worktree','stopped',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, os.Getpid())
	report, err := Preview(context.Background(), f.plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Blockers) == 0 {
		t.Fatal("stopped database state hid a live process")
	}
}

func TestRepairRefusesExistingBackupDestination(t *testing.T) {
	f := newFixture(t)
	p := observedPlan(t, f)
	dir := filepath.Dir(journalPath(p))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(p.Home, "untouched")
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "before.db")); err != nil {
		t.Fatal(err)
	}
	if err := repairWithIsolatedHost(p, "apply"); err == nil {
		t.Fatal("existing backup destination accepted")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatal("backup followed symlink")
	}
	if err = CheckPending(p.Home, p.Driver, p.Database); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewRejectsCompetingPathInventory(t *testing.T) {
	f := newFixture(t)
	execSQL(t, f.db, `INSERT INTO task_environment_repos(id,task_environment_id,repository_id,worktree_id,worktree_path,worktree_branch,branch_slug,created_at,updated_at) VALUES('competing','env','repo','other-worktree',?,'feature/stale','other',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, f.plan.Repairs[0].SourcePath)
	if _, err := Preview(context.Background(), f.plan); err == nil {
		t.Fatal("two inventory slots claimed the same checkout")
	}
}

func TestApplyPreservesUnrelatedColumnBytes(t *testing.T) {
	f := newFixture(t)
	p := observedPlan(t, f)
	var before, after string
	if err := f.db.QueryRow(`SELECT CAST(created_at AS TEXT) FROM task_environment_repos WHERE id='slot'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := repairWithIsolatedHost(p, "apply"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT CAST(created_at AS TEXT) FROM task_environment_repos WHERE id='slot'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("unrelated created_at representation changed: %q -> %q", before, after)
	}
}
