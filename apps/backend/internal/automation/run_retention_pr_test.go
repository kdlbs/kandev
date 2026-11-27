package automation

import (
	"context"
	"testing"
)

// A reuse_per_pr task keeps its checkout while its PR is open, however many
// newer runs exist; removing the mapping restores ordinary retention.
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.3
func TestPrunableRunTaskIDs_ProtectsOpenPRTasks(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	a := newRetentionAutomation(t, store, "per-PR review")
	ctx := context.Background()

	tasks := seedTerminalRunSeries(t, store, a.ID, "task", DefaultRunWorktreeRetention+2)
	mapped := tasks[0]
	if err := store.UpsertPRContinuation(ctx, &PRContinuation{
		AutomationID: a.ID, PRSubject: "acme/widget#1", RepoOwner: "acme", RepoName: "widget",
		PRNumber: 1, TaskID: mapped,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := store.PrunableRunTaskIDs(ctx, tasks[len(tasks)-1], DefaultRunWorktreeRetention)
	if err != nil {
		t.Fatal(err)
	}
	assertTaskIDs(t, got, []string{tasks[1]})
	inUse, err := store.RunWorkspaceInUse(ctx, mapped)
	if err != nil || !inUse {
		t.Fatalf("mapped task must report its workspace in use: inUse=%v err=%v", inUse, err)
	}

	if err := store.DeletePRContinuation(ctx, a.ID, "acme/widget#1", mapped); err != nil {
		t.Fatal(err)
	}
	got, err = store.PrunableRunTaskIDs(ctx, tasks[len(tasks)-1], DefaultRunWorktreeRetention)
	if err != nil {
		t.Fatal(err)
	}
	assertTaskIDs(t, got, []string{tasks[1], tasks[0]})
}
