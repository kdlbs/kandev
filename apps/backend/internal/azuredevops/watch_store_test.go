package azuredevops

import (
	"context"
	"errors"
	"testing"
)

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
