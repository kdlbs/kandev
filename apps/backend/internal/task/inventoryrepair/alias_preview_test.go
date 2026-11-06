package inventoryrepair

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func aliasPlanInput(t *testing.T, f fixture) string {
	t.Helper()
	execSQL(t, f.db, `UPDATE task_environment_repos SET branch_slug='actual', worktree_branch='feature/actual' WHERE id='slot'`)
	execSQL(t, f.db, `INSERT INTO task_environment_repos(id,task_environment_id,repository_id,worktree_id,worktree_path,worktree_branch,created_at,updated_at) VALUES('alias','env','repo','wt',?,'feature/actual',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, filepath.Dir(f.plan.Repairs[0].Path))
	b, err := json.Marshal(f.plan)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Replace(strings.Replace(string(b), `"version":1`, `"version":2`, 1), `"repairs":`, `"aliases":[{"worktree_id":"wt","canonical_row_id":"slot","alias_row_ids":["alias"]}],"repairs":`, 1)
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.2
func TestAliasPreviewCanonicalProof(t *testing.T) {
	f := newFixture(t)
	p, err := Decode(strings.NewReader(aliasPlanInput(t, f)))
	if err != nil {
		t.Fatal(err)
	}
	report, err := Preview(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Blockers) != 0 || len(report.Plan.ExpectedRows) == 0 || report.Plan.ExpectedGit["wt"].Head == "" {
		t.Fatalf("incomplete preview: %+v", report)
	}
	var n int
	if err := f.db.Get(&n, `SELECT count(*) FROM task_environment_repos WHERE worktree_id='wt'`); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatal("preview mutated aliases")
	}
	execSQL(t, f.db, `UPDATE task_environment_repos SET worktree_branch='foreign' WHERE id='alias'`)
	if _, err := Preview(context.Background(), report.Plan); err == nil {
		t.Fatal("accepted changed claim")
	}
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.4
func TestAliasPreviewSharedEnvironment(t *testing.T) {
	f := sharedAliasFixture(t)
	report, err := Preview(context.Background(), f.plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Blockers) != 0 {
		t.Fatalf("unexpected blockers: %v", report.Blockers)
	}
	execSQL(t, f.db, `INSERT INTO task_environment_repos(id,task_environment_id,repository_id,branch_slug,worktree_id,worktree_path,worktree_branch,status,created_at,updated_at) VALUES('independent','child-env','repo','other','other','/foreign','other','failed',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	if _, err := Preview(context.Background(), f.plan); err == nil {
		t.Fatal("retired environment with independent failed slot")
	}
}

func sharedAliasFixture(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	p, err := Decode(strings.NewReader(aliasPlanInput(t, f)))
	if err != nil {
		t.Fatal(err)
	}
	f.plan = p
	execSQL(t, f.db, `CREATE TABLE task_workspace_groups(id TEXT PRIMARY KEY, workspace_id TEXT, owner_task_id TEXT, materialized_environment_id TEXT)`)
	execSQL(t, f.db, `CREATE TABLE task_workspace_group_members(workspace_group_id TEXT,task_id TEXT,role TEXT,released_at TEXT)`)
	execSQL(t, f.db, `INSERT INTO task_workspace_groups VALUES('group','workspace','owner','env')`)
	execSQL(t, f.db, `INSERT INTO task_workspace_group_members VALUES('group','owner','owner',NULL),('group','child','member',NULL)`)
	execSQL(t, f.db, `INSERT INTO tasks(id,workspace_id,title,state,parent_id,metadata,created_at,updated_at) VALUES('child','workspace','Child','FAILED','owner','{"workspace":{"mode":"inherit_parent","group_id":"group"}}',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	execSQL(t, f.db, `INSERT INTO task_environments(id,task_id,executor_type,status,task_dir_name,workspace_path,created_at,updated_at) VALUES('child-env','child','worktree','ready','owner_root',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, filepath.Dir(p.Repairs[0].Path))
	execSQL(t, f.db, `INSERT INTO task_environment_repos(id,task_environment_id,repository_id,worktree_id,worktree_path,worktree_branch,created_at,updated_at) VALUES('copy','child-env','repo','wt',?,'feature/actual',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, p.Repairs[0].Path)
	execSQL(t, f.db, `INSERT INTO task_sessions(id,task_id,agent_profile_id,state,task_environment_id,workspace_path,started_at,updated_at) VALUES('child-session','child','profile','WAITING_FOR_INPUT','child-env',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, filepath.Dir(p.Repairs[0].Path))
	f.plan.Aliases[0].AliasRowIDs = append(f.plan.Aliases[0].AliasRowIDs, "copy")
	f.plan.RedundantEnvironments = []EnvironmentRebind{{EnvironmentID: "child-env", CanonicalEnvironmentID: "env", GroupID: "group", SessionIDs: []string{"child-session"}}}
	return f
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.2, AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.6
func TestAliasPreviewRefusals(t *testing.T) {
	for _, name := range []string{"forged canonical", "omitted claim", "foreign path", "broken pointer", "unknown reference", "cleanup", "live session", "mixed claims"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			p, err := Decode(strings.NewReader(aliasPlanInput(t, f)))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = Preview(context.Background(), p); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "forged canonical":
				p.Aliases[0].CanonicalRowID = "alias"
				p.Aliases[0].AliasRowIDs = []string{"slot"}
			case "omitted claim":
				execSQL(t, f.db, `INSERT INTO task_environment_repos (
 id, task_environment_id, repository_id, branch_slug, worktree_id,
 worktree_path, worktree_branch, worktree_branch_owner, worktree_integration_ref,
 worktree_recovery_head_sha, worktree_source_clone_path, worktree_source_common_dir,
 worktree_branch_compacted_at, position, error_message, status,
 created_at, updated_at, merged_at, deleted_at
) SELECT 'third', task_environment_id, repository_id, 'third', worktree_id,
 worktree_path, worktree_branch, worktree_branch_owner, worktree_integration_ref,
 worktree_recovery_head_sha, worktree_source_clone_path, worktree_source_common_dir,
 worktree_branch_compacted_at, position, error_message, status,
 created_at, updated_at, merged_at, deleted_at
FROM task_environment_repos WHERE id='alias'`)
			case "foreign path":
				execSQL(t, f.db, `UPDATE task_environment_repos SET worktree_path='/foreign' WHERE id='alias'`)
			case "broken pointer":
				if err = os.WriteFile(filepath.Join(p.Repairs[0].Path, ".git"), []byte("gitdir: /absent"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unknown reference":
				execSQL(t, f.db, `CREATE TABLE unknown_dependency(id TEXT PRIMARY KEY, owner TEXT REFERENCES task_environments(id))`)
				execSQL(t, f.db, `INSERT INTO unknown_dependency VALUES('foreign','env')`)
			case "cleanup":
				execSQL(t, f.db, `INSERT INTO task_resource_cleanup_jobs(id,operation_id,task_id,trigger,state,resource_snapshot,created_at,updated_at) VALUES('job','cleanup-op','owner','archive','failed','{}',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
			case "live session":
				execSQL(t, f.db, `UPDATE task_sessions SET state='RUNNING' WHERE id='session'`)
			case "mixed claims":
				execSQL(t, f.db, `UPDATE task_environment_repos SET status='failed' WHERE id='alias'`)
			}
			report, err := Preview(context.Background(), p)
			if err == nil && len(report.Blockers) == 0 {
				t.Fatal("unsafe preview accepted")
			}
		})
	}
}

func TestAliasDecodeRejectsOversizedInput(t *testing.T) {
	f := newFixture(t)
	input := aliasPlanInput(t, f)
	if _, err := Decode(strings.NewReader(input + strings.Repeat(" ", 1<<20))); err == nil {
		t.Fatal("accepted oversized plan")
	}
}

func TestAliasPreviewBorrowerCleanupBlocks(t *testing.T) {
	f := sharedAliasFixture(t)
	execSQL(t, f.db, `UPDATE task_sessions SET task_environment_id='env' WHERE id='child-session'`)
	execSQL(t, f.db, `DELETE FROM task_environment_repos WHERE id='copy'`)
	execSQL(t, f.db, `DELETE FROM task_environments WHERE id='child-env'`)
	f.plan.Aliases[0].AliasRowIDs = []string{"alias"}
	f.plan.RedundantEnvironments = nil
	if report, err := Preview(context.Background(), f.plan); err != nil || len(report.Blockers) != 0 {
		t.Fatalf("positive control: %v %+v", err, report)
	}
	execSQL(t, f.db, `INSERT INTO task_resource_cleanup_jobs(id,operation_id,task_id,trigger,state,resource_snapshot,created_at,updated_at) VALUES('job','cleanup-op','child','archive','failed','{}',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	report, err := Preview(context.Background(), f.plan)
	if err == nil && len(report.Blockers) == 0 {
		t.Fatal("ignored canonical borrower cleanup")
	}
}

func TestAliasPreviewMultipleRepositories(t *testing.T) {
	f := newFixture(t)
	p, err := Decode(strings.NewReader(aliasPlanInput(t, f)))
	if err != nil {
		t.Fatal(err)
	}
	other := newFixture(t)
	path := filepath.Join(filepath.Dir(p.Repairs[0].Path), "second")
	git(t, other.repo, "worktree", "add", "-b", "feature/second", path)
	execSQL(t, f.db, `INSERT INTO repositories(id,workspace_id,name,source_type,local_path,created_at,updated_at) VALUES('repo2','workspace','Second','local',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, other.repo)
	execSQL(t, f.db, `INSERT INTO task_environment_repos(id,task_environment_id,repository_id,branch_slug,worktree_id,worktree_path,worktree_branch,created_at,updated_at) VALUES('slot2','env','repo2','second','wt2',?,'feature/second',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('alias2','env','repo2','','wt2',?,'feature/second',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, path, filepath.Dir(path))
	p.Repairs = append(p.Repairs, Repair{WorktreeID: "wt2", EnvironmentID: "env", RepositoryID: "repo2", RepositoryPath: other.repo, SourcePath: path, Path: path, Branch: "feature/second", HeadOID: git(t, path, "rev-parse", "HEAD"), SourceRootTaskID: "owner"})
	p.Aliases = append(p.Aliases, AliasSelection{WorktreeID: "wt2", CanonicalRowID: "slot2", AliasRowIDs: []string{"alias2"}})
	if report, err := Preview(context.Background(), p); err != nil || len(report.Blockers) != 0 {
		t.Fatalf("multi-repository preview: %v %+v", err, report)
	}
	p.Repairs = p.Repairs[:1]
	p.Aliases = p.Aliases[:1]
	if _, err := Preview(context.Background(), p); err == nil {
		t.Fatal("accepted incomplete connected selection")
	}
}

func TestAliasPreviewSessionLimit(t *testing.T) {
	f := aliasFixture(t)
	for i := 0; i < 256; i++ {
		execSQL(t, f.db, `INSERT INTO task_sessions(id,task_id,agent_profile_id,state,task_environment_id,started_at,updated_at) VALUES(?,'owner','profile','FAILED','env',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, fmt.Sprintf("session-%03d", i))
	}
	if _, err := Preview(context.Background(), f.plan); err == nil {
		t.Fatal("accepted more than 256 affected sessions")
	}
}
