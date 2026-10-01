package main

import (
	"bytes"
	"os"
	"path/filepath"
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
