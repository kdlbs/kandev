package inventoryrepair

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

func TestAliasDiscoveryEncodedByteLimit(t *testing.T) {
	f := newFixture(t)
	for group := range 3 {
		for claim := range 200 {
			id := fmt.Sprintf("group-%d-claim-%03d", group, claim)
			execSQL(t, f.db, `INSERT INTO task_environment_repos(id,task_environment_id,repository_id,branch_slug,worktree_id,status,created_at,updated_at) VALUES(?,'env','repo',?,?,'active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, id, id, fmt.Sprintf("group-%d", group))
		}
	}
	opts := DiscoveryOptions{Home: f.plan.Home, Database: f.plan.Database, TasksRoot: f.plan.TasksRoot, Limit: 2}
	report, err := Discover(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	compact, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	padding := strings.Repeat("x", ((1<<20)-4096-len(compact))/400)
	execSQL(t, f.db, `UPDATE task_environment_repos SET worktree_path=? WHERE worktree_id LIKE 'group-%'`, padding)
	seen := map[string]bool{}
	for {
		report, err = Discover(t.Context(), opts)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if len(encoded)+1 > 1<<20 {
			t.Fatalf("encoded page is %d bytes, exceeds 1 MiB", len(encoded)+1)
		}
		for _, group := range report.Groups {
			if group.Oversized || len(group.Claims) != 200 || seen[group.WorktreeID] {
				t.Fatalf("incomplete or repeated group: %s", group.WorktreeID)
			}
			seen[group.WorktreeID] = true
		}
		if report.Next == "" {
			break
		}
		opts.After = report.Next
	}
	if len(seen) != 3 {
		t.Fatalf("discovered %d groups, want 3", len(seen))
	}
}

func TestAliasDiscoveryByteOversizedGroupContinues(t *testing.T) {
	f := newFixture(t)
	aliasPlanInput(t, f)
	execSQL(t, f.db, `UPDATE task_environment_repos SET worktree_path=? WHERE worktree_id='wt'`, strings.Repeat("x", 1<<20))
	execSQL(t, f.db, `INSERT INTO task_environment_repos(id,task_environment_id,repository_id,branch_slug,worktree_id,status,created_at,updated_at) VALUES('later1','env','repo','later1','zz','active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('later2','env','repo','later2','zz','active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	opts := DiscoveryOptions{Home: f.plan.Home, Database: f.plan.Database, TasksRoot: f.plan.TasksRoot, Limit: 1}
	report, err := Discover(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Groups) != 1 || !report.Groups[0].Oversized || len(report.Groups[0].Claims) != 0 || report.Next != "wt" {
		t.Fatalf("missing oversized continuation: %+v", report)
	}
	opts.After = report.Next
	report, err = Discover(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Groups) != 1 || report.Groups[0].WorktreeID != "zz" || len(report.Groups[0].Claims) != 2 || report.Next != "" {
		t.Fatalf("missing later group: %+v", report)
	}
}
