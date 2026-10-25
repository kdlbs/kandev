package inventoryrepair

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

func relocationFixture(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	root := filepath.Join(f.plan.TasksRoot, "child_root")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := storageworkspaces.WriteOwnershipMarker(root, storageworkspaces.OwnershipMarker{TaskID: "child", WorkspaceID: "workspace", TaskDirName: "child_root", LayoutVersion: 1}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "checkout")
	git(t, f.repo, "worktree", "move", f.plan.Repairs[0].SourcePath, path)
	f.plan.Repairs[0].SourcePath = path
	f.plan.Repairs[0].SourceRootTaskID = "child"
	execSQL(t, f.db, `UPDATE task_environment_repos SET worktree_path=? WHERE id='slot'`, path)
	execSQL(t, f.db, `INSERT INTO tasks(id,workspace_id,title,state,parent_id,created_at,updated_at) VALUES('child','workspace','Child','CANCELLED','owner',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	execSQL(t, f.db, `INSERT INTO task_sessions(id,task_id,agent_profile_id,state,task_environment_id,repository_id,workspace_path,started_at,updated_at) VALUES('child-session','child','profile','CANCELLED','env','repo',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, f.repo)
	execSQL(t, f.db, `UPDATE task_environments SET workspace_path=? WHERE id='env'`, f.repo)
	execSQL(t, f.db, `UPDATE task_sessions SET workspace_path=? WHERE id='session'`, f.repo)
	f.plan.Workspaces = []WorkspaceRepair{{EnvironmentID: "env", Path: filepath.Dir(f.plan.Repairs[0].Path), SessionIDs: []string{"child-session", "session"}}}
	return f
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.4
func TestPreviewSharedRelocationRequiresExactBindings(t *testing.T) {
	f := relocationFixture(t)
	if _, err := Preview(context.Background(), f.plan); err != nil {
		t.Fatal(err)
	}
	execSQL(t, f.db, `UPDATE task_sessions SET repository_id='foreign' WHERE id='child-session'`)
	if _, err := Preview(context.Background(), f.plan); err == nil {
		t.Fatal("parent-child relation alone authorized foreign-root adoption")
	}
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.4
func TestPreviewRelocationRequiresWorkspaceRepair(t *testing.T) {
	for _, omission := range []string{"workspace", "bound-session"} {
		t.Run(omission, func(t *testing.T) {
			f := relocationFixture(t)
			if _, err := Preview(context.Background(), f.plan); err != nil {
				t.Fatal(err)
			}
			if omission == "workspace" {
				f.plan.Workspaces = nil
			} else {
				f.plan.Workspaces[0].SessionIDs = []string{"session"}
			}
			_, err := Preview(context.Background(), f.plan)
			if err == nil || !strings.Contains(err.Error(), "workspace repair") {
				t.Fatalf("expected incomplete workspace repair to be refused, got %v", err)
			}
		})
	}
}

func cleanupFixture(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	execSQL(t, f.db, `UPDATE tasks SET archived_at='2026-09-28 12:00:00+00:00' WHERE id='owner'`)
	execSQL(t, f.db, `UPDATE task_environment_repos SET repository_id='' WHERE id='slot'`)
	snapshot := map[string]any{"workspace_id": "workspace", "worktrees": []map[string]any{{"id": "wt", "task_id": "owner", "task_environment_id": "env", "repository_id": "", "repository_path": "", "path": f.plan.Repairs[0].SourcePath, "branch": "feature/stale", "status": "active"}}, "worktree_head_oids": map[string]string{}, "orphan_reap_roots": []string{"previously-removed"}}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	execSQL(t, f.db, `INSERT INTO task_resource_cleanup_jobs(id,operation_id,task_id,trigger,state,resource_snapshot,attempts,last_error,created_at,updated_at) VALUES('job','old-operation','owner','cascade_archive','retry_wait',?,4,'incomplete identity',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, string(data))
	f.plan.CleanupJobs = []string{"job"}
	return f
}

func TestPreviewCleanupCannotChangeResourceScope(t *testing.T) {
	f := cleanupFixture(t)
	report, err := Preview(context.Background(), f.plan)
	if err != nil {
		t.Fatal(err)
	}
	if report.Plan.ExpectedRows["jobs:owner"] == "" {
		t.Fatal("cleanup predecessor was not fenced")
	}
	execSQL(t, f.db, `UPDATE task_resource_cleanup_jobs SET task_id='foreign' WHERE id='job'`)
	if _, err := Preview(context.Background(), f.plan); err == nil {
		t.Fatal("accepted cleanup outside selected task")
	}
}

func TestPreviewReportsNonterminalConsumer(t *testing.T) {
	f := newFixture(t)
	execSQL(t, f.db, `UPDATE task_sessions SET state='WAITING_FOR_INPUT' WHERE id='session'`)
	report, err := Preview(context.Background(), f.plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Blockers) != 1 {
		t.Fatalf("blockers=%v", report.Blockers)
	}
}
