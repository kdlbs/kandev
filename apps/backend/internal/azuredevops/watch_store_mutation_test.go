package azuredevops

import (
	"errors"
	"testing"
	"time"
)

func newWatchStore(t *testing.T) *Store {
	t.Helper()
	db := newTestDB(t)
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func seedWorkItemWatch(t *testing.T, store *Store) *WorkItemWatch {
	t.Helper()
	watch := &WorkItemWatch{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		ProjectID: "project-1", WIQL: "SELECT [System.Id] FROM WorkItems",
		RepositoryID: "repo-1", BaseBranch: "main",
		AgentProfileID: "agent-1", ExecutorProfileID: "executor-1",
	}
	if err := store.CreateWorkItemWatch(t.Context(), watch); err != nil {
		t.Fatalf("create work-item watch: %v", err)
	}
	return watch
}

func seedPullRequestWatch(t *testing.T, store *Store) *PullRequestWatch {
	t.Helper()
	watch := &PullRequestWatch{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		ProjectID: "project-1", AzureRepositoryID: "azure-repo-1",
		RepositoryID: "repo-1", BaseBranch: "main",
		AgentProfileID: "agent-1", ExecutorProfileID: "executor-1",
	}
	if err := store.CreatePullRequestWatch(t.Context(), watch); err != nil {
		t.Fatalf("create pull-request watch: %v", err)
	}
	return watch
}

func TestStoreUpdatePullRequestWatchPersistsEveryMutableField(t *testing.T) {
	store := newWatchStore(t)
	watch := seedPullRequestWatch(t, store)

	watch.WorkflowID = "wf-2"
	watch.WorkflowStepID = "step-2"
	watch.ProjectID = "project-2"
	watch.AzureRepositoryID = "azure-repo-2"
	watch.Status = "completed"
	watch.CreatorID = "creator-1"
	watch.ReviewerID = "reviewer-1"
	watch.RepositoryID = "repo-2"
	watch.BaseBranch = "develop"
	watch.AgentProfileID = "agent-2"
	watch.ExecutorProfileID = "executor-2"
	watch.Prompt = "Review {{title}}"
	watch.Enabled = false
	watch.PollIntervalSeconds = 900
	watch.CleanupPolicy = CleanupPolicyNever
	watch.MaxInflightTasks = intPtr(4)
	if err := store.UpdatePullRequestWatch(t.Context(), watch); err != nil {
		t.Fatalf("update pull-request watch: %v", err)
	}

	got, err := store.GetPullRequestWatch(t.Context(), watch.ID)
	if err != nil || got == nil {
		t.Fatalf("get pull-request watch: %+v %v", got, err)
	}
	if got.WorkflowID != "wf-2" || got.WorkflowStepID != "step-2" || got.ProjectID != "project-2" ||
		got.AzureRepositoryID != "azure-repo-2" || got.Status != "completed" ||
		got.CreatorID != "creator-1" || got.ReviewerID != "reviewer-1" ||
		got.RepositoryID != "repo-2" || got.BaseBranch != "develop" ||
		got.AgentProfileID != "agent-2" || got.ExecutorProfileID != "executor-2" ||
		got.Prompt != "Review {{title}}" || got.Enabled ||
		got.PollIntervalSeconds != 900 || got.CleanupPolicy != CleanupPolicyNever ||
		got.MaxInflightTasks == nil || *got.MaxInflightTasks != 4 {
		t.Fatalf("persisted watch = %+v", got)
	}
	if !got.UpdatedAt.After(got.CreatedAt) && got.UpdatedAt.Before(got.CreatedAt) {
		t.Fatalf("updated_at = %v, created_at = %v", got.UpdatedAt, got.CreatedAt)
	}
	// The disabled watch is still listed for its workspace; only deletion hides it.
	list, err := store.ListPullRequestWatches(t.Context(), "ws-1")
	if err != nil || len(list) != 1 || list[0].ID != watch.ID {
		t.Fatalf("workspace list = %+v, %v", list, err)
	}
	if enabled, err := store.ListEnabledPullRequestWatches(t.Context()); err != nil || len(enabled) != 0 {
		t.Fatalf("enabled list = %+v, %v; want the disabled watch excluded", enabled, err)
	}
}

func TestStoreWatchUpdatesReportMissingRows(t *testing.T) {
	store := newWatchStore(t)
	work := &WorkItemWatch{ID: "missing", WorkspaceID: "ws-1", ProjectID: "project-1", WIQL: "SELECT"}
	if err := store.UpdateWorkItemWatch(t.Context(), work); !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("UpdateWorkItemWatch error = %v, want ErrWatchNotFound", err)
	}
	pr := &PullRequestWatch{ID: "missing", WorkspaceID: "ws-1", ProjectID: "project-1"}
	if err := store.UpdatePullRequestWatch(t.Context(), pr); !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("UpdatePullRequestWatch error = %v, want ErrWatchNotFound", err)
	}
	if got, err := store.GetWorkItemWatch(t.Context(), "missing"); err != nil || got != nil {
		t.Fatalf("GetWorkItemWatch = %+v, %v; want nil without error", got, err)
	}
	if got, err := store.GetPullRequestWatch(t.Context(), "missing"); err != nil || got != nil {
		t.Fatalf("GetPullRequestWatch = %+v, %v; want nil without error", got, err)
	}
}

func TestStoreWatchNormalizationRejectsInvalidInput(t *testing.T) {
	store := newWatchStore(t)
	negative := -1
	workItemCases := []struct {
		name  string
		watch *WorkItemWatch
	}{
		{"nil watch", nil},
		{"missing workspace", &WorkItemWatch{ProjectID: "project-1", WIQL: "SELECT"}},
		{"missing project", &WorkItemWatch{WorkspaceID: "ws-1", WIQL: "SELECT"}},
		{"missing wiql", &WorkItemWatch{WorkspaceID: "ws-1", ProjectID: "project-1"}},
		{"invalid cleanup policy", &WorkItemWatch{WorkspaceID: "ws-1", ProjectID: "project-1", WIQL: "SELECT", CleanupPolicy: "sometimes"}},
		{"negative max inflight", &WorkItemWatch{WorkspaceID: "ws-1", ProjectID: "project-1", WIQL: "SELECT", MaxInflightTasks: &negative}},
	}
	for _, tt := range workItemCases {
		t.Run("work item/"+tt.name, func(t *testing.T) {
			if err := store.CreateWorkItemWatch(t.Context(), tt.watch); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
		})
	}
	pullRequestCases := []struct {
		name  string
		watch *PullRequestWatch
	}{
		{"nil watch", nil},
		{"missing workspace", &PullRequestWatch{ProjectID: "project-1"}},
		{"missing project", &PullRequestWatch{WorkspaceID: "ws-1"}},
		{"invalid cleanup policy", &PullRequestWatch{WorkspaceID: "ws-1", ProjectID: "project-1", CleanupPolicy: "sometimes"}},
		{"negative max inflight", &PullRequestWatch{WorkspaceID: "ws-1", ProjectID: "project-1", MaxInflightTasks: &negative}},
	}
	for _, tt := range pullRequestCases {
		t.Run("pull request/"+tt.name, func(t *testing.T) {
			if err := store.CreatePullRequestWatch(t.Context(), tt.watch); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestStoreCreatePullRequestWatchDefaultsStatusAndInterval(t *testing.T) {
	store := newWatchStore(t)
	watch := &PullRequestWatch{WorkspaceID: "ws-1", ProjectID: "project-1"}
	if err := store.CreatePullRequestWatch(t.Context(), watch); err != nil {
		t.Fatalf("create pull-request watch: %v", err)
	}
	if watch.ID == "" || watch.Status != activePullRequestState || !watch.Enabled ||
		watch.Generation != 1 || watch.PollIntervalSeconds != defaultWatchPollIntervalSeconds ||
		watch.CleanupPolicy != CleanupPolicyAuto {
		t.Fatalf("normalized watch = %+v", watch)
	}
}

func TestStoreWatchErrorLifecycle(t *testing.T) {
	store := newWatchStore(t)
	work := seedWorkItemWatch(t, store)
	pr := seedPullRequestWatch(t, store)
	failedAt := time.Date(2026, 8, 12, 9, 30, 0, 0, time.UTC)

	if err := store.SetWorkItemWatchError(t.Context(), work.ID, "401 unauthorized", failedAt); err != nil {
		t.Fatalf("set work-item error: %v", err)
	}
	if err := store.SetPullRequestWatchError(t.Context(), pr.ID, "403 forbidden", failedAt); err != nil {
		t.Fatalf("set pull-request error: %v", err)
	}
	gotWork, err := store.GetWorkItemWatch(t.Context(), work.ID)
	if err != nil || gotWork.LastError != "401 unauthorized" ||
		gotWork.LastErrorAt == nil || !gotWork.LastErrorAt.Equal(failedAt) ||
		gotWork.LastPolledAt == nil || !gotWork.LastPolledAt.Equal(failedAt) {
		t.Fatalf("work-item watch after failure = %+v, %v", gotWork, err)
	}
	gotPR, err := store.GetPullRequestWatch(t.Context(), pr.ID)
	if err != nil || gotPR.LastError != "403 forbidden" || gotPR.LastErrorAt == nil {
		t.Fatalf("pull-request watch after failure = %+v, %v", gotPR, err)
	}

	polledAt := failedAt.Add(time.Minute)
	if err := store.ClearWorkItemWatchError(t.Context(), work.ID, polledAt); err != nil {
		t.Fatalf("clear work-item error: %v", err)
	}
	if err := store.ClearPullRequestWatchError(t.Context(), pr.ID, polledAt); err != nil {
		t.Fatalf("clear pull-request error: %v", err)
	}
	gotWork, _ = store.GetWorkItemWatch(t.Context(), work.ID)
	if gotWork.LastError != "" || gotWork.LastErrorAt != nil ||
		gotWork.LastPolledAt == nil || !gotWork.LastPolledAt.Equal(polledAt) {
		t.Fatalf("work-item watch after clear = %+v", gotWork)
	}
	gotPR, _ = store.GetPullRequestWatch(t.Context(), pr.ID)
	if gotPR.LastError != "" || gotPR.LastErrorAt != nil {
		t.Fatalf("pull-request watch after clear = %+v", gotPR)
	}
}

func TestStoreDisableWatchWithErrorStopsPolling(t *testing.T) {
	store := newWatchStore(t)
	work := seedWorkItemWatch(t, store)
	pr := seedPullRequestWatch(t, store)

	if err := store.DisableWorkItemWatchWithError(t.Context(), work.ID, "workflow step deleted"); err != nil {
		t.Fatalf("disable work-item watch: %v", err)
	}
	if err := store.DisablePullRequestWatchWithError(t.Context(), pr.ID, "repository unlinked"); err != nil {
		t.Fatalf("disable pull-request watch: %v", err)
	}
	gotWork, _ := store.GetWorkItemWatch(t.Context(), work.ID)
	if gotWork.Enabled || gotWork.LastError != "workflow step deleted" || gotWork.LastErrorAt == nil {
		t.Fatalf("disabled work-item watch = %+v", gotWork)
	}
	gotPR, _ := store.GetPullRequestWatch(t.Context(), pr.ID)
	if gotPR.Enabled || gotPR.LastError != "repository unlinked" {
		t.Fatalf("disabled pull-request watch = %+v", gotPR)
	}
	enabled, err := store.ListEnabledWorkItemWatches(t.Context())
	if err != nil || len(enabled) != 0 {
		t.Fatalf("enabled work-item watches = %+v, %v", enabled, err)
	}
	// A disabled watch also refuses new reservations.
	if reserved, err := store.ReserveWorkItemWatchTask(
		t.Context(), work.ID, work.Generation, "project-1", 101, "https://azure/101",
	); err != nil || reserved {
		t.Fatalf("reservation on a disabled watch: reserved=%v err=%v", reserved, err)
	}

	if err := store.DisableWorkItemWatchWithError(t.Context(), "missing", "gone"); !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("disable missing work-item watch error = %v, want ErrWatchNotFound", err)
	}
	if err := store.DisablePullRequestWatchWithError(t.Context(), "missing", "gone"); !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("disable missing pull-request watch error = %v, want ErrWatchNotFound", err)
	}
}

func TestStoreDeleteWatchRemovesReservationsAtomically(t *testing.T) {
	store := newWatchStore(t)
	work := seedWorkItemWatch(t, store)
	pr := seedPullRequestWatch(t, store)
	if ok, err := store.ReserveWorkItemWatchTask(t.Context(), work.ID, work.Generation, "project-1", 101, "https://azure/101"); err != nil || !ok {
		t.Fatalf("reserve work item: ok=%v err=%v", ok, err)
	}
	if ok, err := store.ReservePullRequestWatchTask(t.Context(), pr.ID, pr.Generation, "project-1", "azure-repo-1", 42, "https://azure/pr/42"); err != nil || !ok {
		t.Fatalf("reserve pull request: ok=%v err=%v", ok, err)
	}

	if err := store.DeleteWorkItemWatch(t.Context(), work.ID); err != nil {
		t.Fatalf("delete work-item watch: %v", err)
	}
	if err := store.DeletePullRequestWatch(t.Context(), pr.ID); err != nil {
		t.Fatalf("delete pull-request watch: %v", err)
	}
	if got, err := store.GetWorkItemWatch(t.Context(), work.ID); err != nil || got != nil {
		t.Fatalf("work-item watch after delete = %+v, %v", got, err)
	}
	rows, err := store.ListWorkItemWatchTasks(t.Context(), work.ID, work.Generation)
	if err != nil || len(rows) != 0 {
		t.Fatalf("work-item reservations after delete = %+v, %v", rows, err)
	}
	prRows, err := store.ListPullRequestWatchTasks(t.Context(), pr.ID, pr.Generation)
	if err != nil || len(prRows) != 0 {
		t.Fatalf("pull-request reservations after delete = %+v, %v", prRows, err)
	}

	if err := store.DeleteWorkItemWatch(t.Context(), work.ID); !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("repeat delete error = %v, want ErrWatchNotFound", err)
	}
	if err := store.DeletePullRequestWatch(t.Context(), "missing"); !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("delete missing pull-request watch error = %v, want ErrWatchNotFound", err)
	}
}

func TestStoreReleaseWatchTaskFreesTheReservationKey(t *testing.T) {
	store := newWatchStore(t)
	work := seedWorkItemWatch(t, store)
	pr := seedPullRequestWatch(t, store)
	if ok, err := store.ReserveWorkItemWatchTask(t.Context(), work.ID, work.Generation, "project-1", 101, "https://azure/101"); err != nil || !ok {
		t.Fatalf("reserve work item: ok=%v err=%v", ok, err)
	}
	if ok, err := store.ReservePullRequestWatchTask(t.Context(), pr.ID, pr.Generation, "project-1", "azure-repo-1", 42, "https://azure/pr/42"); err != nil || !ok {
		t.Fatalf("reserve pull request: ok=%v err=%v", ok, err)
	}

	if err := store.ReleaseWorkItemWatchTask(t.Context(), work.ID, work.Generation, "project-1", 101); err != nil {
		t.Fatalf("release work-item reservation: %v", err)
	}
	if err := store.ReleasePullRequestWatchTask(t.Context(), pr.ID, pr.Generation, "project-1", "azure-repo-1", 42); err != nil {
		t.Fatalf("release pull-request reservation: %v", err)
	}
	// Releasing must make the same identity claimable again.
	if ok, err := store.ReserveWorkItemWatchTask(t.Context(), work.ID, work.Generation, "project-1", 101, "https://azure/101"); err != nil || !ok {
		t.Fatalf("re-reservation after release: ok=%v err=%v", ok, err)
	}

	if err := store.ReleaseWorkItemWatchTask(t.Context(), work.ID, work.Generation, "project-1", 999); !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("release unknown work item error = %v, want ErrReservationNotFound", err)
	}
	if err := store.ReleasePullRequestWatchTask(t.Context(), pr.ID, pr.Generation, "project-1", "azure-repo-1", 999); !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("release unknown pull request error = %v, want ErrReservationNotFound", err)
	}
}

func TestStoreDeleteWatchTaskRowsByID(t *testing.T) {
	store := newWatchStore(t)
	work := seedWorkItemWatch(t, store)
	pr := seedPullRequestWatch(t, store)
	if ok, err := store.ReserveWorkItemWatchTask(t.Context(), work.ID, work.Generation, "project-1", 101, "https://azure/101"); err != nil || !ok {
		t.Fatalf("reserve work item: ok=%v err=%v", ok, err)
	}
	if ok, err := store.ReservePullRequestWatchTask(t.Context(), pr.ID, pr.Generation, "project-1", "azure-repo-1", 42, "https://azure/pr/42"); err != nil || !ok {
		t.Fatalf("reserve pull request: ok=%v err=%v", ok, err)
	}
	workRows, err := store.ListWorkItemWatchTasks(t.Context(), work.ID, work.Generation)
	if err != nil || len(workRows) != 1 || workRows[0].WorkItemID != 101 ||
		workRows[0].WorkItemURL != "https://azure/101" || workRows[0].TaskID != "" {
		t.Fatalf("work reservations = %+v, %v", workRows, err)
	}
	prRows, err := store.ListPullRequestWatchTasks(t.Context(), pr.ID, pr.Generation)
	if err != nil || len(prRows) != 1 || prRows[0].PullRequestID != 42 ||
		prRows[0].AzureRepositoryID != "azure-repo-1" {
		t.Fatalf("PR reservations = %+v, %v", prRows, err)
	}

	if err := store.DeleteWorkItemWatchTask(t.Context(), workRows[0].ID); err != nil {
		t.Fatalf("delete work-item reservation row: %v", err)
	}
	if err := store.DeletePullRequestWatchTask(t.Context(), prRows[0].ID); err != nil {
		t.Fatalf("delete pull-request reservation row: %v", err)
	}
	if rows, _ := store.ListWorkItemWatchTasks(t.Context(), work.ID, work.Generation); len(rows) != 0 {
		t.Fatalf("work reservations after row delete = %+v", rows)
	}
	if rows, _ := store.ListPullRequestWatchTasks(t.Context(), pr.ID, pr.Generation); len(rows) != 0 {
		t.Fatalf("PR reservations after row delete = %+v", rows)
	}
}

// TestStoreAssignWatchTaskIDDistinguishesOwnershipLossFromMissingReservation
// pins the two failure modes apart: a stale generation means the watch moved
// on (ownership lost), while a live generation with no row means the specific
// reservation is gone.
func TestStoreAssignWatchTaskIDDistinguishesOwnershipLossFromMissingReservation(t *testing.T) {
	store := newWatchStore(t)
	work := seedWorkItemWatch(t, store)
	if ok, err := store.ReserveWorkItemWatchTask(t.Context(), work.ID, work.Generation, "project-1", 101, "https://azure/101"); err != nil || !ok {
		t.Fatalf("reserve work item: ok=%v err=%v", ok, err)
	}
	if err := store.AssignWorkItemWatchTaskID(t.Context(), work.ID, work.Generation, "project-1", 101, "task-1"); err != nil {
		t.Fatalf("assign task id: %v", err)
	}
	rows, _ := store.ListWorkItemWatchTasks(t.Context(), work.ID, work.Generation)
	if len(rows) != 1 || rows[0].TaskID != "task-1" {
		t.Fatalf("reservation after assignment = %+v", rows)
	}
	if err := store.AssignWorkItemWatchTaskID(t.Context(), work.ID, work.Generation, "project-1", 999, "task-2"); !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("assign to unreserved item error = %v, want ErrReservationNotFound", err)
	}

	reset, err := store.BeginWorkItemWatchReset(t.Context(), work.ID)
	if err != nil {
		t.Fatalf("begin reset: %v", err)
	}
	if err := store.FinishWorkItemWatchReset(t.Context(), work.ID, reset.Generation); err != nil {
		t.Fatalf("finish reset: %v", err)
	}
	if err := store.AssignWorkItemWatchTaskID(t.Context(), work.ID, work.Generation, "project-1", 101, "task-3"); !errors.Is(err, ErrWatchOwnershipLost) {
		t.Fatalf("assign at stale generation error = %v, want ErrWatchOwnershipLost", err)
	}
	if err := store.AssignWorkItemWatchTaskID(t.Context(), "unknown-watch", reset.Generation, "project-1", 101, "task-4"); !errors.Is(err, ErrWatchOwnershipLost) {
		t.Fatalf("assign for unknown watch error = %v, want ErrWatchOwnershipLost", err)
	}
}

// TestStoreAssignPullRequestWatchTaskIDResolvesGenerationFromTheSecondTable
// covers watchGeneration falling through the work-item lookup to the
// pull-request table.
func TestStoreAssignPullRequestWatchTaskIDResolvesGenerationFromTheSecondTable(t *testing.T) {
	store := newWatchStore(t)
	pr := seedPullRequestWatch(t, store)
	if ok, err := store.ReservePullRequestWatchTask(t.Context(), pr.ID, pr.Generation, "project-1", "azure-repo-1", 42, "https://azure/pr/42"); err != nil || !ok {
		t.Fatalf("reserve pull request: ok=%v err=%v", ok, err)
	}
	if err := store.AssignPullRequestWatchTaskID(t.Context(), pr.ID, pr.Generation, "project-1", "azure-repo-1", 42, "task-42"); err != nil {
		t.Fatalf("assign task id: %v", err)
	}
	if err := store.AssignPullRequestWatchTaskID(t.Context(), pr.ID, pr.Generation, "project-1", "azure-repo-1", 99, "task-99"); !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("assign to unreserved PR error = %v, want ErrReservationNotFound", err)
	}
	if err := store.DisablePullRequestWatchWithError(t.Context(), pr.ID, "disabled"); err != nil {
		t.Fatalf("disable watch: %v", err)
	}
	if err := store.AssignPullRequestWatchTaskID(t.Context(), pr.ID, pr.Generation, "project-1", "azure-repo-1", 99, "task-99"); !errors.Is(err, ErrWatchOwnershipLost) {
		t.Fatalf("assign on disabled watch error = %v, want ErrWatchOwnershipLost", err)
	}
}

func TestStoreEnabledWatchListsSkipUnpolledAndDeletedRows(t *testing.T) {
	store := newWatchStore(t)
	first := seedWorkItemWatch(t, store)
	second := seedWorkItemWatch(t, store)
	polledAt := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	if err := store.ClearWorkItemWatchError(t.Context(), first.ID, polledAt); err != nil {
		t.Fatalf("mark first watch polled: %v", err)
	}

	watches, err := store.ListEnabledWorkItemWatches(t.Context())
	if err != nil || len(watches) != 2 {
		t.Fatalf("enabled watches = %+v, %v", watches, err)
	}
	// Never-polled watches sort first so a new watch is not starved.
	if watches[0].ID != second.ID || watches[0].LastPolledAt != nil {
		t.Fatalf("enabled ordering = %+v, want the never-polled watch first", watches)
	}
	if watches[1].ID != first.ID || watches[1].LastPolledAt == nil {
		t.Fatalf("enabled ordering = %+v", watches)
	}

	if err := store.DeleteWorkItemWatch(t.Context(), second.ID); err != nil {
		t.Fatalf("delete watch: %v", err)
	}
	watches, err = store.ListEnabledWorkItemWatches(t.Context())
	if err != nil || len(watches) != 1 || watches[0].ID != first.ID {
		t.Fatalf("enabled watches after delete = %+v, %v", watches, err)
	}
}
