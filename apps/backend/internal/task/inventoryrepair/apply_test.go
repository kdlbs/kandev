package inventoryrepair

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/backendapp/ownershiplock"
)

func observedPlan(t *testing.T, f fixture) Plan {
	t.Helper()
	report, err := Preview(context.Background(), f.plan)
	if err != nil {
		t.Fatal(err)
	}
	return report.Plan
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.4
func TestApplyRelocatesWithoutChangingContentAndRollsBack(t *testing.T) {
	f := relocationFixture(t)
	r := f.plan.Repairs[0]
	if err := os.WriteFile(filepath.Join(r.SourcePath, "ignored-secret"), []byte("preserve me"), 0600); err != nil {
		t.Fatal(err)
	}
	p := observedPlan(t, f)
	if err := repairWithIsolatedHost(p, "apply"); err != nil {
		t.Fatal(err)
	}
	if err := repairWithIsolatedHost(p, "verify"); err != nil {
		t.Fatal(err)
	}
	var path, branch string
	if err := f.db.QueryRow(`SELECT worktree_path,worktree_branch FROM task_environment_repos WHERE id='slot'`).Scan(&path, &branch); err != nil {
		t.Fatal(err)
	}
	if path != r.Path || branch != r.Branch {
		t.Fatalf("path=%s branch=%s", path, branch)
	}
	assertWorkspacePaths(t, f, filepath.Dir(r.Path))
	if _, err := readMarker(p.TasksRoot, filepath.Dir(r.SourcePath)); err != nil {
		t.Fatal(err)
	}
	if err := repairWithIsolatedHost(p, "apply"); err != nil {
		t.Fatalf("idempotent apply: %v", err)
	}
	if err := repairWithIsolatedHost(p, "rollback"); err != nil {
		t.Fatal(err)
	}
	assertWorkspacePaths(t, f, f.repo)
	if _, err := Preview(context.Background(), p); err != nil {
		t.Fatalf("original state not restored: %v", err)
	}
	if err := CheckPending(p.Home, p.Driver, p.Database); err != nil {
		t.Fatal(err)
	}
}

func TestApplyRefusesLiveBackendAndChangedEvidence(t *testing.T) {
	for _, mode := range []string{"backend", "content", "row", "missing-preview"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			p := observedPlan(t, f)
			switch mode {
			case "backend":
				targets, err := ownershiplock.Targets(p.Home, p.Driver, p.Database)
				if err != nil {
					t.Fatal(err)
				}
				owner, err := ownershiplock.Acquire(targets)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = owner.Close() }()
			case "content":
				if err := os.WriteFile(filepath.Join(p.Repairs[0].SourcePath, "new-file"), []byte("new"), 0600); err != nil {
					t.Fatal(err)
				}
			case "row":
				execSQL(t, f.db, `UPDATE task_environment_repos SET worktree_branch='changed' WHERE id='slot'`)
			case "missing-preview":
				p.ExpectedRows = nil
			}
			if err := repairWithIsolatedHost(p, "apply"); err == nil {
				t.Fatal("unsafe application accepted")
			}
			if err := CheckPending(p.Home, p.Driver, p.Database); err != nil {
				t.Fatalf("refusal left journal fence: %v", err)
			}
		})
	}
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.2
func TestApplyCleanupPreservesPredecessorAndProgress(t *testing.T) {
	f := cleanupFixture(t)
	p := observedPlan(t, f)
	var original string
	if err := f.db.QueryRow(`SELECT resource_snapshot FROM task_resource_cleanup_jobs WHERE id='job'`).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if err := repairWithIsolatedHost(p, "apply"); err != nil {
		t.Fatal(err)
	}
	var state, old, successor string
	if err := f.db.QueryRow(`SELECT state,resource_snapshot FROM task_resource_cleanup_jobs WHERE id='job'`).Scan(&state, &old); err != nil {
		t.Fatal(err)
	}
	if state != "cancelled" || old != original {
		t.Fatal("predecessor evidence changed")
	}
	if err := f.db.QueryRow(`SELECT resource_snapshot FROM task_resource_cleanup_jobs WHERE state='pending'`).Scan(&successor); err != nil {
		t.Fatal(err)
	}
	var s map[string]json.RawMessage
	if err := json.Unmarshal([]byte(successor), &s); err != nil {
		t.Fatal(err)
	}
	var heads map[string]string
	if err := json.Unmarshal(s["worktree_head_oids"], &heads); err != nil {
		t.Fatal(err)
	}
	if heads["wt"] != p.Repairs[0].HeadOID || string(s["orphan_reap_roots"]) != `["previously-removed"]` || len(s["inventory_repair"]) == 0 {
		t.Fatalf("incomplete successor: %s", successor)
	}
	if err := repairWithIsolatedHost(p, "rollback"); err != nil {
		t.Fatal(err)
	}
	if _, err := Preview(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

// The host process census is tested separately; application fixtures isolate it
// from unrelated processes and ptrace restrictions on the test machine.
func repairWithIsolatedHost(p Plan, mode string) error {
	if mode == "verify" {
		return Verify(context.Background(), p)
	}
	return runRepair(context.Background(), p, mode, func(context.Context, Plan) error { return nil })
}

func assertWorkspacePaths(t *testing.T, f fixture, want string) {
	t.Helper()
	rows, err := queryRows(context.Background(), f.db, `SELECT workspace_path FROM task_environments WHERE id='env' UNION ALL SELECT workspace_path FROM task_sessions WHERE task_environment_id='env'`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected environment and both sessions, got %d rows", len(rows))
	}
	for _, r := range rows {
		if got := stringValue(r, "workspace_path"); got != want {
			t.Fatalf("workspace path=%q, want %q", got, want)
		}
	}
}
