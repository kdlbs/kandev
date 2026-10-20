package main

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandRequiresUnambiguousExplicitPlan(t *testing.T) {
	for _, args := range [][]string{nil, {"--apply"}, {"--plan", "absent", "--apply", "--rollback"}, {"--plan", "absent", "--unknown"}} {
		if err := run(args, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestDefaultPreviewDoesNotCreateMissingDatabase(t *testing.T) {
	dir := t.TempDir()
	plan := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(plan, []byte(`{"version":1,"operation_id":"preview","driver":"sqlite","home":"`+dir+`","database":"`+dir+`/missing.db","tasks_root":"`+dir+`/tasks","repairs":[{"worktree_id":"wt","environment_id":"env","repository_id":"repo","source_root_task_id":"task","repository_path":"`+dir+`/repo","source_path":"`+dir+`/tasks/task/repo","path":"`+dir+`/tasks/task/repo","branch":"branch","head_oid":"head"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--plan", plan}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("preview accepted absent database")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("preview created files: %v", entries)
	}
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.1
func TestAliasDiscoveryCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, q := range []string{
		`CREATE TABLE task_environments(id TEXT, task_id TEXT, executor_type TEXT)`,
		`CREATE TABLE task_environment_repos(id TEXT, task_environment_id TEXT, repository_id TEXT, branch_slug TEXT, worktree_id TEXT, worktree_path TEXT, worktree_branch TEXT, status TEXT, deleted_at TEXT)`,
		`INSERT INTO task_environments VALUES('env','owner','worktree')`,
		`INSERT INTO task_environment_repos VALUES('slot','env','repo','actual','wt','/uninspected/checkout','feature/actual','active',NULL),('alias','env','repo','','wt','/uninspected','feature/actual','active',NULL)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	err = run([]string{"--discover", "--home", dir, "--database", path, "--tasks-root", filepath.Join(dir, "tasks")}, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"alias"`) || !strings.Contains(out.String(), `"slot"`) {
		t.Fatalf("missing complete claims: %s", out.String())
	}
}
