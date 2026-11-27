package inventoryrepair

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	dbutil "github.com/kandev/kandev/internal/db"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type fixture struct {
	plan Plan
	db   *sqlx.DB
	repo string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	home := t.TempDir()
	repo := filepath.Join(home, "source")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.email", "repair@example.invalid")
	git(t, repo, "config", "user.name", "Repair test")
	if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("keep\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "tracked")
	git(t, repo, "commit", "-m", "initial")
	root := filepath.Join(home, "tasks", "owner_root")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := storageworkspaces.WriteOwnershipMarker(root, storageworkspaces.OwnershipMarker{
		TaskID: "owner", WorkspaceID: "workspace", TaskDirName: "owner_root", LayoutVersion: 1,
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "checkout")
	git(t, repo, "worktree", "add", "-b", "feature/actual", path)
	git(t, repo, "branch", "feature/stale")
	dbPath := filepath.Join(home, "data.db")
	conn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := tasksqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('workspace','Workspace',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	execSQL(t, db, `INSERT INTO tasks(id,workspace_id,title,state,created_at,updated_at) VALUES('owner','workspace','Owner','FAILED',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	execSQL(t, db, `INSERT INTO repositories(id,workspace_id,name,source_type,local_path,created_at,updated_at) VALUES('repo','workspace','Source','local',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, repo)
	execSQL(t, db, `INSERT INTO task_environments(id,task_id,executor_type,status,task_dir_name,workspace_path,created_at,updated_at) VALUES('env','owner','worktree','ready','owner_root',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, root)
	execSQL(t, db, `INSERT INTO task_environment_repos(id,task_environment_id,repository_id,worktree_id,worktree_path,worktree_branch,created_at,updated_at) VALUES('slot','env','repo','wt',?,'feature/stale',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, path)
	execSQL(t, db, `INSERT INTO task_sessions(id,task_id,agent_profile_id,state,task_environment_id,workspace_path,started_at,updated_at) VALUES('session','owner','profile','FAILED','env',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, root)
	return fixture{Plan{Version: 1, OperationID: "repair-test", Home: home, Database: dbPath,
		Driver: "sqlite", TasksRoot: filepath.Join(home, "tasks"), Repairs: []Repair{{
			WorktreeID: "wt", EnvironmentID: "env", RepositoryID: "repo", RepositoryPath: repo,
			SourcePath: path, Path: path, Branch: "feature/actual", HeadOID: git(t, path, "rev-parse", "HEAD"), SourceRootTaskID: "owner",
		}}}, db, repo}
}

func execSQL(t *testing.T, db *sqlx.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
	return strings.TrimSpace(string(out))
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.1
func TestPreviewBranchRepairPreservesDatabaseAndRefs(t *testing.T) {
	f := newFixture(t)
	before := git(t, f.repo, "show-ref")
	report, err := Preview(context.Background(), f.plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Changes) != 1 || report.Changes[0].OldBranch != "feature/stale" {
		t.Fatalf("unexpected preview: %+v", report)
	}
	if len(report.Plan.ExpectedRows) == 0 || report.Plan.ExpectedGit["wt"].ContentSHA256 == "" {
		t.Fatal("preview omitted application evidence")
	}
	var branch string
	if err := f.db.Get(&branch, `SELECT worktree_branch FROM task_environment_repos WHERE id='slot'`); err != nil {
		t.Fatal(err)
	}
	if branch != "feature/stale" || git(t, f.repo, "show-ref") != before {
		t.Fatal("preview changed inventory or refs")
	}
	if _, err := Preview(context.Background(), report.Plan); err != nil {
		t.Fatalf("preview of exact observations: %v", err)
	}
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.2, AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.1
func TestPreviewRefusesChangedOrForeignIdentity(t *testing.T) {
	for _, name := range []string{"branch", "repository", "marker", "row", "remote executor", "symlink"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			report, err := Preview(context.Background(), f.plan)
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "branch":
				git(t, f.plan.Repairs[0].Path, "checkout", "-b", "feature/other")
			case "repository":
				execSQL(t, f.db, `UPDATE repositories SET local_path=? WHERE id='repo'`, t.TempDir())
			case "marker":
				p := filepath.Join(filepath.Dir(f.plan.Repairs[0].Path), ".kandev-workspace.json")
				if err := os.WriteFile(p, []byte(`{"task_id":"foreign","workspace_id":"workspace","task_dir_name":"owner_root","layout_version":1}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "row":
				execSQL(t, f.db, `UPDATE task_environment_repos SET worktree_branch='feature/new' WHERE id='slot'`)
			case "remote executor":
				execSQL(t, f.db, `UPDATE task_environments SET executor_type='ssh' WHERE id='env'`)
			case "symlink":
				path := f.plan.Repairs[0].Path
				if err := os.Rename(path, path+"-moved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+"-moved", path); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := Preview(context.Background(), report.Plan); err == nil {
				t.Fatal("accepted changed identity")
			}
		})
	}
}

func TestPreviewMissingRepositoryUsesExactRegistration(t *testing.T) {
	f := newFixture(t)
	execSQL(t, f.db, `UPDATE task_environment_repos SET repository_id='' WHERE id='slot'`)
	report, err := Preview(context.Background(), f.plan)
	if err != nil {
		t.Fatal(err)
	}
	if report.Changes[0].OldRepositoryID != "" {
		t.Fatal("missing legacy identity was not reported")
	}
}

func TestDecodeRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	f := newFixture(t)
	data, err := json.Marshal(f.plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(strings.NewReader(string(data))); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{strings.Replace(string(data), `"version":1`, `"version":1,"force":true`, 1), string(data) + "{}"} {
		if _, err := Decode(strings.NewReader(input)); err == nil {
			t.Fatal("accepted malformed repair plan")
		}
	}
}
