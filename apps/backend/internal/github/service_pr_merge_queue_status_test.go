package github

import (
	"context"
	"testing"
	"time"
)

func TestSyncTaskPRPersistsMergeQueueObservationAndPublishesChange(t *testing.T) {
	svc, store, eventBus := setupSyncTest(t)
	ctx := context.Background()
	position := 3
	estimate := 125
	taskPR := &TaskPR{
		TaskID: "task-queue-sync", Owner: "owner", Repo: "repo", PRNumber: 42,
		PRURL: "https://example.test/42", PRTitle: "queued", State: "open",
		MergeQueueState: "", CreatedAt: nowUTC(),
	}
	if err := store.CreateTaskPR(ctx, taskPR); err != nil {
		t.Fatalf("seed TaskPR: %v", err)
	}
	status := &PRStatus{
		PR:              &PR{Number: 42, State: "open", RepoOwner: "owner", RepoName: "repo"},
		MergeQueueState: "queued", MergeQueuePosition: &position,
		MergeQueueEstimatedTimeToMergeSeconds: &estimate, mergeQueuePopulated: true,
	}
	if err := svc.SyncTaskPR(ctx, taskPR.TaskID, status); err != nil {
		t.Fatalf("SyncTaskPR: %v", err)
	}
	got := mustGetTaskPR(t, store, ctx, taskPR.ID)
	assertTaskPRMergeQueueFields(t, "SyncTaskPR", got, "queued", &position, &estimate)
	if eventBus.publishedCount() != 1 {
		t.Fatalf("published events = %d, want 1", eventBus.publishedCount())
	}
}

func TestSyncTaskPRClearsQueueOnQueueUnawareRead(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	position := 6
	estimate := 240
	taskPR := &TaskPR{
		TaskID: "task-queue-preserve", Owner: "owner", Repo: "repo", PRNumber: 43,
		PRURL: "https://example.test/43", PRTitle: "queued", State: "open",
		MergeQueueState: "queued", MergeQueuePosition: &position,
		MergeQueueEstimatedTimeToMergeSeconds: &estimate, CreatedAt: nowUTC(),
	}
	if err := store.CreateTaskPR(ctx, taskPR); err != nil {
		t.Fatalf("seed TaskPR: %v", err)
	}
	if err := svc.SyncTaskPR(ctx, taskPR.TaskID, &PRStatus{
		PR: &PR{Number: 43, State: "open", RepoOwner: "owner", RepoName: "repo"},
	}); err != nil {
		t.Fatalf("queue-unaware SyncTaskPR: %v", err)
	}
	assertTaskPRMergeQueueFields(t, "queue-unaware SyncTaskPR", mustGetTaskPR(t, store, ctx, taskPR.ID), "", nil, nil)
}

func TestSyncTaskPRClearsQueueOnAuthoritativeExitAndTerminalState(t *testing.T) {
	tests := []struct {
		name  string
		state string
	}{
		{name: "queue exit", state: "open"},
		{name: "merged", state: "merged"},
		{name: "closed", state: "closed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, store, _ := setupSyncTest(t)
			ctx := context.Background()
			position := 2
			estimate := 60
			taskPR := &TaskPR{
				TaskID: "task-queue-clear-" + test.name, Owner: "owner", Repo: "repo", PRNumber: 44,
				PRURL: "https://example.test/44", PRTitle: "queued", State: "open",
				MergeQueueState: "queued", MergeQueuePosition: &position,
				MergeQueueEstimatedTimeToMergeSeconds: &estimate, CreatedAt: nowUTC(),
			}
			if err := store.CreateTaskPR(ctx, taskPR); err != nil {
				t.Fatalf("seed TaskPR: %v", err)
			}
			status := &PRStatus{
				PR: &PR{Number: 44, State: test.state, RepoOwner: "owner", RepoName: "repo"},
			}
			if test.name == "queue exit" {
				status.mergeQueuePopulated = true
			}
			if err := svc.SyncTaskPR(ctx, taskPR.TaskID, status); err != nil {
				t.Fatalf("SyncTaskPR: %v", err)
			}
			assertTaskPRMergeQueueFields(t, test.name, mustGetTaskPR(t, store, ctx, taskPR.ID), "", nil, nil)
		})
	}
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
