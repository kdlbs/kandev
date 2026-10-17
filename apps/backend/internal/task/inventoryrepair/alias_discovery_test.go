package inventoryrepair

import (
	"context"
	"fmt"
	"testing"
)

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.1
func TestAliasDiscoveryCompletePages(t *testing.T) {
	f := newFixture(t)
	aliasPlanInput(t, f)
	execSQL(t, f.db, `INSERT INTO task_environment_repos(id,task_environment_id,repository_id,branch_slug,worktree_id,status,created_at,updated_at) VALUES('other1','env','repo','other1','zz','active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('other2','env','repo','other2','zz','active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('old','env','repo','old','wt','deleted',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	opts := DiscoveryOptions{Home: f.plan.Home, Database: f.plan.Database, TasksRoot: f.plan.TasksRoot, Limit: 1}
	first, err := Discover(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Groups) != 1 || len(first.Groups[0].Claims) != 2 || first.Next != "wt" {
		t.Fatalf("bad page: %+v", first)
	}
	opts.After = first.Next
	last, err := Discover(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(last.Groups) != 1 || last.Groups[0].WorktreeID != "zz" || last.Next != "" {
		t.Fatalf("bad continuation: %+v", last)
	}
}

func TestAliasDiscoveryOversizedGroup(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 256; i++ {
		execSQL(t, f.db, `INSERT INTO task_environment_repos(id,task_environment_id,repository_id,branch_slug,worktree_id,status,created_at,updated_at) VALUES(?,'env','repo',?,'wt','active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, fmt.Sprint(i), fmt.Sprint(i))
	}
	report, err := Discover(context.Background(), DiscoveryOptions{Home: f.plan.Home, Database: f.plan.Database, TasksRoot: f.plan.TasksRoot})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Groups) != 1 || !report.Groups[0].Oversized || len(report.Groups[0].Claims) != 0 {
		t.Fatalf("partial oversized claims: %+v", report)
	}
}
