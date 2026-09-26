package azuredevops

import (
	"context"
	"errors"
	"testing"
)

func TestCleanupCandidateListsExcludeHistoricalTasks(t *testing.T) {
	store, err := NewStore(newTestDB(t), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		archived_at DATETIME
	)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	ctx := context.Background()
	workWatch := &WorkItemWatch{ID: "work-cleanup", WorkspaceID: "ws-1", ProjectID: "project-1", WIQL: "SELECT"}
	if err := store.CreateWorkItemWatch(ctx, workWatch); err != nil {
		t.Fatalf("create work-item watch: %v", err)
	}
	otherWorkWatch := &WorkItemWatch{ID: "work-other", WorkspaceID: "ws-2", ProjectID: "project-2", WIQL: "SELECT"}
	if err := store.CreateWorkItemWatch(ctx, otherWorkWatch); err != nil {
		t.Fatalf("create other work-item watch: %v", err)
	}
	pullRequestWatch := &PullRequestWatch{ID: "pr-cleanup", WorkspaceID: "ws-1", ProjectID: "project-1", AzureRepositoryID: "repo-1"}
	if err := store.CreatePullRequestWatch(ctx, pullRequestWatch); err != nil {
		t.Fatalf("create pull-request watch: %v", err)
	}
	otherPullRequestWatch := &PullRequestWatch{ID: "pr-other", WorkspaceID: "ws-2", ProjectID: "project-2", AzureRepositoryID: "repo-2"}
	if err := store.CreatePullRequestWatch(ctx, otherPullRequestWatch); err != nil {
		t.Fatalf("create other pull-request watch: %v", err)
	}

	for _, task := range []struct {
		id        string
		workspace string
	}{
		{id: "work-active", workspace: "ws-1"},
		{id: "work-archived", workspace: "ws-1"},
		{id: "pr-active", workspace: "ws-1"},
		{id: "pr-archived", workspace: "ws-1"},
		{id: "other-work-active", workspace: "ws-2"},
		{id: "other-pr-active", workspace: "ws-2"},
	} {
		if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES (?, ?)`, task.id, task.workspace); err != nil {
			t.Fatalf("seed task %s: %v", task.id, err)
		}
	}
	for _, taskID := range []string{"work-archived", "pr-archived"} {
		if _, err := store.db.Exec(`UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id = ?`, taskID); err != nil {
			t.Fatalf("archive task %s: %v", taskID, err)
		}
	}

	reserveWork := func(watchID string, generation int64, projectID string, itemID int, taskID string) {
		t.Helper()
		reserved, err := store.ReserveWorkItemWatchTask(ctx, watchID, generation, projectID, itemID, "https://azure/item")
		if err != nil || !reserved {
			t.Fatalf("reserve work item %d: reserved=%v err=%v", itemID, reserved, err)
		}
		if taskID != "" {
			if err := store.AssignWorkItemWatchTaskID(ctx, watchID, generation, projectID, itemID, taskID); err != nil {
				t.Fatalf("assign work item %d: %v", itemID, err)
			}
		}
	}
	reservePullRequest := func(watchID string, generation int64, projectID, repositoryID string, pullRequestID int, taskID string) {
		t.Helper()
		reserved, err := store.ReservePullRequestWatchTask(ctx, watchID, generation, projectID, repositoryID, pullRequestID, "https://azure/pr")
		if err != nil || !reserved {
			t.Fatalf("reserve pull request %d: reserved=%v err=%v", pullRequestID, reserved, err)
		}
		if taskID != "" {
			if err := store.AssignPullRequestWatchTaskID(ctx, watchID, generation, projectID, repositoryID, pullRequestID, taskID); err != nil {
				t.Fatalf("assign pull request %d: %v", pullRequestID, err)
			}
		}
	}
	reserveWork(workWatch.ID, workWatch.Generation, workWatch.ProjectID, 101, "work-active")
	reserveWork(workWatch.ID, workWatch.Generation, workWatch.ProjectID, 102, "work-archived")
	reserveWork(workWatch.ID, workWatch.Generation, workWatch.ProjectID, 103, "work-missing")
	reserveWork(workWatch.ID, workWatch.Generation, workWatch.ProjectID, 104, "")
	reserveWork(otherWorkWatch.ID, otherWorkWatch.Generation, otherWorkWatch.ProjectID, 105, "other-work-active")
	reservePullRequest(pullRequestWatch.ID, pullRequestWatch.Generation, pullRequestWatch.ProjectID, pullRequestWatch.AzureRepositoryID, 201, "pr-active")
	reservePullRequest(pullRequestWatch.ID, pullRequestWatch.Generation, pullRequestWatch.ProjectID, pullRequestWatch.AzureRepositoryID, 202, "pr-archived")
	reservePullRequest(pullRequestWatch.ID, pullRequestWatch.Generation, pullRequestWatch.ProjectID, pullRequestWatch.AzureRepositoryID, 203, "pr-missing")
	reservePullRequest(pullRequestWatch.ID, pullRequestWatch.Generation, pullRequestWatch.ProjectID, pullRequestWatch.AzureRepositoryID, 204, "")
	reservePullRequest(otherPullRequestWatch.ID, otherPullRequestWatch.Generation, otherPullRequestWatch.ProjectID, otherPullRequestWatch.AzureRepositoryID, 205, "other-pr-active")

	assertWorkRows := func(name string, rows []*WorkItemWatchTask, want map[string]bool) {
		t.Helper()
		got := make(map[string]bool, len(rows))
		for _, row := range rows {
			got[row.TaskID] = true
		}
		if len(rows) != len(want) {
			t.Fatalf("%s rows = %d (%v), want %d (%v)", name, len(rows), got, len(want), want)
		}
		for taskID := range want {
			if !got[taskID] {
				t.Errorf("%s missing task %q in %v", name, taskID, got)
			}
		}
	}
	assertPullRequestRows := func(name string, rows []*PullRequestWatchTask, want map[string]bool) {
		t.Helper()
		got := make(map[string]bool, len(rows))
		for _, row := range rows {
			got[row.TaskID] = true
		}
		if len(rows) != len(want) {
			t.Fatalf("%s rows = %d (%v), want %d (%v)", name, len(rows), got, len(want), want)
		}
		for taskID := range want {
			if !got[taskID] {
				t.Errorf("%s missing task %q in %v", name, taskID, got)
			}
		}
	}

	workRows, err := store.ListWorkItemWatchTasks(ctx, workWatch.ID, workWatch.Generation)
	if err != nil {
		t.Fatalf("list work-item cleanup rows: %v", err)
	}
	assertWorkRows("work-item current generation", workRows, map[string]bool{"work-active": true, "": true})
	wrongGenerationRows, err := store.ListWorkItemWatchTasks(ctx, workWatch.ID, workWatch.Generation+1)
	if err != nil {
		t.Fatalf("list work-item wrong generation: %v", err)
	}
	if len(wrongGenerationRows) != 0 {
		t.Fatalf("work-item wrong generation rows = %v, want empty", wrongGenerationRows)
	}
	pullRequestRows, err := store.ListPullRequestWatchTasks(ctx, pullRequestWatch.ID, pullRequestWatch.Generation)
	if err != nil {
		t.Fatalf("list pull-request cleanup rows: %v", err)
	}
	assertPullRequestRows("pull-request current generation", pullRequestRows, map[string]bool{"pr-active": true, "": true})
}

func TestAzureWatchStoreRoundTripAndGenerationReservations(t *testing.T) {
	store, err := NewStore(newTestDB(t), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	watch := &WorkItemWatch{
		WorkspaceID:         "ws-1",
		WorkflowID:          "wf-1",
		WorkflowStepID:      "step-1",
		ProjectID:           "project-1",
		WIQL:                "SELECT [System.Id] FROM WorkItems",
		RepositoryID:        "repo-1",
		BaseBranch:          "main",
		AgentProfileID:      "agent-1",
		ExecutorProfileID:   "executor-1",
		Prompt:              "Fix {{title}}",
		Enabled:             true,
		CleanupPolicy:       CleanupPolicyAuto,
		MaxInflightTasks:    intPtr(2),
		PollIntervalSeconds: 10,
	}
	if err := store.CreateWorkItemWatch(context.Background(), watch); err != nil {
		t.Fatalf("create work-item watch: %v", err)
	}
	if watch.ID == "" || watch.Generation != 1 || watch.PollIntervalSeconds != minWatchPollIntervalSeconds {
		t.Fatalf("normalized watch = %+v", watch)
	}
	got, err := store.GetWorkItemWatch(context.Background(), watch.ID)
	if err != nil {
		t.Fatalf("get work-item watch: %v", err)
	}
	if got == nil || got.WorkspaceID != "ws-1" || got.WIQL != watch.WIQL || got.MaxInflightTasks == nil || *got.MaxInflightTasks != 2 {
		t.Fatalf("round-trip watch = %+v", got)
	}

	reserved, err := store.ReserveWorkItemWatchTask(context.Background(), watch.ID, watch.Generation, "project-1", 101, "https://azure/item/101")
	if err != nil || !reserved {
		t.Fatalf("first reservation: reserved=%v err=%v", reserved, err)
	}
	reserved, err = store.ReserveWorkItemWatchTask(context.Background(), watch.ID, watch.Generation, "project-1", 101, "https://azure/item/101")
	if err != nil || reserved {
		t.Fatalf("duplicate reservation: reserved=%v err=%v", reserved, err)
	}
	seedAzureTask(t, store, "task-1", "ws-1")
	if err := store.AssignWorkItemWatchTaskID(context.Background(), watch.ID, watch.Generation, "project-1", 101, "task-1"); err != nil {
		t.Fatalf("attach task: %v", err)
	}

	reset, err := store.BeginWorkItemWatchReset(context.Background(), watch.ID)
	if err != nil {
		t.Fatalf("begin reset: %v", err)
	}
	if reset.Generation != watch.Generation+1 || len(reset.TaskIDs) != 1 || reset.TaskIDs[0] != "task-1" {
		t.Fatalf("reset = %+v", reset)
	}
	if err := store.FinishWorkItemWatchReset(context.Background(), watch.ID, reset.Generation); err != nil {
		t.Fatalf("finish reset: %v", err)
	}
	reserved, err = store.ReserveWorkItemWatchTask(context.Background(), watch.ID, reset.Generation, "project-1", 101, "https://azure/item/101")
	if err != nil || !reserved {
		t.Fatalf("reservation after reset: reserved=%v err=%v", reserved, err)
	}
}

func TestAzurePullRequestWatchStoreIsWorkspaceScoped(t *testing.T) {
	store, err := NewStore(newTestDB(t), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	first := &PullRequestWatch{WorkspaceID: "ws-1", ProjectID: "project-1", AzureRepositoryID: "azure-repo-1", Status: "active"}
	second := &PullRequestWatch{WorkspaceID: "ws-2", ProjectID: "project-2", AzureRepositoryID: "azure-repo-2", Status: "active"}
	for _, watch := range []*PullRequestWatch{first, second} {
		if err := store.CreatePullRequestWatch(context.Background(), watch); err != nil {
			t.Fatalf("create pull-request watch: %v", err)
		}
	}
	list, err := store.ListPullRequestWatches(context.Background(), "ws-1")
	if err != nil || len(list) != 1 || list[0].ID != first.ID {
		t.Fatalf("workspace watch list = %+v err=%v", list, err)
	}
	if _, err := store.GetPullRequestWatch(context.Background(), second.ID); err != nil {
		t.Fatalf("get second watch: %v", err)
	}
	if err := store.AssignPullRequestWatchTaskID(context.Background(), first.ID, first.Generation, "project-1", "azure-repo-1", 42, "task-42"); !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("attach without reservation error = %v, want ErrReservationNotFound", err)
	}
}

func TestAzureWatchStoreWorkspaceDeletionRemovesReservations(t *testing.T) {
	store, err := NewStore(newTestDB(t), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	work := &WorkItemWatch{WorkspaceID: "ws-delete", ProjectID: "project-1", WIQL: "SELECT"}
	pr := &PullRequestWatch{WorkspaceID: "ws-delete", ProjectID: "project-1", AzureRepositoryID: "repo-1"}
	if err := store.CreateWorkItemWatch(t.Context(), work); err != nil {
		t.Fatalf("create work watch: %v", err)
	}
	if err := store.CreatePullRequestWatch(t.Context(), pr); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}
	if ok, err := store.ReserveWorkItemWatchTask(t.Context(), work.ID, work.Generation, work.ProjectID, 101, "item"); err != nil || !ok {
		t.Fatalf("reserve work item: %v %v", ok, err)
	}
	if ok, err := store.ReservePullRequestWatchTask(t.Context(), pr.ID, pr.Generation, pr.ProjectID, pr.AzureRepositoryID, 42, "pr"); err != nil || !ok {
		t.Fatalf("reserve pull request: %v %v", ok, err)
	}
	if err := store.DeleteWatchesByWorkspace(t.Context(), "ws-delete"); err != nil {
		t.Fatalf("delete workspace watches: %v", err)
	}
	workRows, err := store.ListWorkItemWatchTasks(t.Context(), work.ID, work.Generation)
	if err != nil || len(workRows) != 0 {
		t.Fatalf("work reservations after workspace deletion = %v, %v", workRows, err)
	}
	prRows, err := store.ListPullRequestWatchTasks(t.Context(), pr.ID, pr.Generation)
	if err != nil || len(prRows) != 0 {
		t.Fatalf("PR reservations after workspace deletion = %v, %v", prRows, err)
	}
}

func intPtr(value int) *int { return &value }
