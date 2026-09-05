package azuredevops

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresStoreSchemaReplay(t *testing.T) {
	ctx := context.Background()
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := database.Exec(`
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL
		);
		INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1')`); err != nil {
		t.Fatalf("create task prerequisite: %v", err)
	}

	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("first Azure DevOps store schema init: %v", err)
	}
	replayed, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("second Azure DevOps store schema init: %v", err)
	}

	config := &Config{
		WorkspaceID:        "ws-1",
		OrganizationURL:    "https://dev.azure.com/acme",
		DefaultProjectID:   "project-1",
		DefaultProjectName: "Platform",
		AuthMethod:         AuthMethodPAT,
	}
	if err := replayed.UpsertConfig(ctx, config); err != nil {
		t.Fatalf("upsert Azure DevOps config: %v", err)
	}
	checkedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := replayed.UpdateAuthHealth(ctx, "ws-1", true, "", checkedAt); err != nil {
		t.Fatalf("update Azure DevOps auth health: %v", err)
	}
	if err := replayed.PutSavedViewsJSON(ctx, "ws-1", `[{"id":"view-1"}]`); err != nil {
		t.Fatalf("put Azure DevOps saved views: %v", err)
	}
	if err := replayed.PutWorkspaceSettingsJSON(ctx, "ws-1", `{"workItemActions":[]}`); err != nil {
		t.Fatalf("put Azure DevOps workspace settings: %v", err)
	}
	snapshot, err := replayed.GetWorkspaceSettingsSnapshot(ctx, "ws-1")
	if err != nil || snapshot.JSON != `{"workItemActions":[]}` || snapshot.Version != 1 {
		t.Fatalf("workspace settings snapshot = %+v, err=%v", snapshot, err)
	}
	updated, err := replayed.PutWorkspaceSettingsJSONIfVersion(ctx, "ws-1", `{"pullRequestActions":[]}`, snapshot.Version)
	if err != nil || !updated {
		t.Fatalf("conditional workspace settings update = %v, err=%v", updated, err)
	}
	updated, err = replayed.PutWorkspaceSettingsJSONIfVersion(ctx, "ws-1", `{}`, snapshot.Version)
	if err != nil || updated {
		t.Fatalf("stale workspace settings update = %v, err=%v", updated, err)
	}
	gotConfig, err := replayed.GetConfig(ctx, "ws-1")
	if err != nil || gotConfig == nil || !gotConfig.LastOK || gotConfig.LastCheckedAt == nil {
		t.Fatalf("get Azure DevOps config = %+v, err=%v", gotConfig, err)
	}
	savedViews, err := replayed.GetSavedViewsJSON(ctx, "ws-1")
	if err != nil || savedViews != `[{"id":"view-1"}]` {
		t.Fatalf("saved views = %q, err=%v", savedViews, err)
	}
	if err := replayed.ResetAuthHealth(ctx, "ws-1"); err != nil {
		t.Fatalf("reset Azure DevOps auth health: %v", err)
	}

	taskPR := &TaskPR{
		TaskID:            "task-1",
		RepositoryID:      "repo-1",
		OrganizationURL:   config.OrganizationURL,
		ProjectID:         "project-1",
		AzureRepositoryID: "azure-repo-1",
		PullRequestID:     42,
		PullRequestURL:    "https://dev.azure.com/acme/project/_git/repo/pullrequest/42",
		Title:             "Initial PR",
		SourceBranch:      "feature",
		TargetBranch:      "main",
		AuthorID:          "author-1",
		AuthorName:        "Ada",
		Status:            "active",
		IsDraft:           true,
	}
	if err := replayed.UpsertTaskPR(ctx, taskPR); err != nil {
		t.Fatalf("upsert Azure DevOps task PR: %v", err)
	}
	firstPRID, firstPRCreatedAt := taskPR.ID, taskPR.CreatedAt
	taskPR.Title = "Updated PR"
	if err := replayed.UpsertTaskPR(ctx, taskPR); err != nil {
		t.Fatalf("refresh Azure DevOps task PR: %v", err)
	}
	prs, err := replayed.ListTaskPRsByTask(ctx, "task-1")
	if err != nil || len(prs) != 1 || prs[0].ID != firstPRID || !prs[0].CreatedAt.Equal(firstPRCreatedAt) || prs[0].Title != "Updated PR" || !prs[0].IsDraft {
		t.Fatalf("Azure DevOps task PRs = %+v, err=%v", prs, err)
	}
	prsByWorkspace, err := replayed.ListTaskPRsByWorkspace(ctx, "ws-1")
	if err != nil || len(prsByWorkspace["task-1"]) != 1 {
		t.Fatalf("Azure DevOps task PRs by workspace = %+v, err=%v", prsByWorkspace, err)
	}

	workItem := &TaskWorkItem{
		TaskID:      "task-1",
		WorkspaceID: "ws-1",
		ProjectID:   "project-1",
		WorkItemID:  101,
		WorkItemURL: "https://dev.azure.com/acme/project/_workitems/edit/101",
		Title:       "Initial work item",
		State:       "Active",
		Type:        "User Story",
	}
	if err := replayed.UpsertTaskWorkItem(ctx, workItem); err != nil {
		t.Fatalf("upsert Azure DevOps task work item: %v", err)
	}
	itemsByWorkspace, err := replayed.ListTaskWorkItemsByWorkspace(ctx, "ws-1")
	if err != nil || len(itemsByWorkspace["task-1"]) != 1 {
		t.Fatalf("Azure DevOps task work items = %+v, err=%v", itemsByWorkspace, err)
	}
	belongs, err := replayed.TaskBelongsToWorkspace(ctx, "task-1", "ws-1")
	if err != nil || !belongs {
		t.Fatalf("task workspace ownership = %v, err=%v", belongs, err)
	}

	workWatch := &WorkItemWatch{
		ID:                  "work-watch-1",
		WorkspaceID:         "ws-1",
		WorkflowID:          "workflow-1",
		WorkflowStepID:      "step-1",
		ProjectID:           "project-1",
		WIQL:                "SELECT [System.Id] FROM WorkItems",
		RepositoryID:        "repo-1",
		BaseBranch:          "main",
		AgentProfileID:      "agent-1",
		ExecutorProfileID:   "executor-1",
		Prompt:              "Fix {{title}}",
		CleanupPolicy:       CleanupPolicyAuto,
		MaxInflightTasks:    intPtr(2),
		PollIntervalSeconds: 60,
	}
	if err := replayed.CreateWorkItemWatch(ctx, workWatch); err != nil {
		t.Fatalf("create Azure DevOps work-item watch: %v", err)
	}
	if got, err := replayed.GetWorkItemWatch(ctx, workWatch.ID); err != nil || got == nil || got.WIQL != workWatch.WIQL {
		t.Fatalf("get Azure DevOps work-item watch = %+v, err=%v", got, err)
	}
	if enabled, err := replayed.ListEnabledWorkItemWatches(ctx); err != nil || len(enabled) != 1 {
		t.Fatalf("enabled Azure DevOps work-item watches = %d, err=%v", len(enabled), err)
	}
	if ok, err := replayed.ReserveWorkItemWatchTask(ctx, workWatch.ID, workWatch.Generation, "project-1", 101, workItem.WorkItemURL); err != nil || !ok {
		t.Fatalf("reserve Azure DevOps work item = %v, err=%v", ok, err)
	}
	if ok, err := replayed.ReserveWorkItemWatchTask(ctx, workWatch.ID, workWatch.Generation, "project-1", 101, workItem.WorkItemURL); err != nil || ok {
		t.Fatalf("duplicate Azure DevOps work item reservation = %v, err=%v", ok, err)
	}
	if err := replayed.AssignWorkItemWatchTaskID(ctx, workWatch.ID, workWatch.Generation, "project-1", 101, "task-1"); err != nil {
		t.Fatalf("assign Azure DevOps work item reservation: %v", err)
	}
	reset, err := replayed.BeginWorkItemWatchReset(ctx, workWatch.ID)
	if err != nil || reset.Generation != workWatch.Generation+1 || len(reset.TaskIDs) != 1 || reset.TaskIDs[0] != "task-1" {
		t.Fatalf("Azure DevOps work-item watch reset = %+v, err=%v", reset, err)
	}
	if err := replayed.FinishWorkItemWatchReset(ctx, workWatch.ID, reset.Generation); err != nil {
		t.Fatalf("finish Azure DevOps work-item watch reset: %v", err)
	}
	if ok, err := replayed.ReserveWorkItemWatchTask(ctx, workWatch.ID, reset.Generation, "project-1", 101, workItem.WorkItemURL); err != nil || !ok {
		t.Fatalf("reserve Azure DevOps work item after reset = %v, err=%v", ok, err)
	}

	pullRequestWatch := &PullRequestWatch{
		ID:                  "pr-watch-1",
		WorkspaceID:         "ws-1",
		WorkflowID:          "workflow-1",
		WorkflowStepID:      "step-1",
		ProjectID:           "project-1",
		AzureRepositoryID:   "azure-repo-1",
		Status:              "active",
		RepositoryID:        "repo-1",
		BaseBranch:          "main",
		AgentProfileID:      "agent-1",
		ExecutorProfileID:   "executor-1",
		CleanupPolicy:       CleanupPolicyAuto,
		PollIntervalSeconds: 60,
	}
	if err := replayed.CreatePullRequestWatch(ctx, pullRequestWatch); err != nil {
		t.Fatalf("create Azure DevOps pull-request watch: %v", err)
	}
	if ok, err := replayed.ReservePullRequestWatchTask(ctx, pullRequestWatch.ID, pullRequestWatch.Generation, "project-1", "azure-repo-1", 42, taskPR.PullRequestURL); err != nil || !ok {
		t.Fatalf("reserve Azure DevOps pull request = %v, err=%v", ok, err)
	}
	if ok, err := replayed.ReservePullRequestWatchTask(ctx, pullRequestWatch.ID, pullRequestWatch.Generation, "project-1", "azure-repo-1", 42, taskPR.PullRequestURL); err != nil || ok {
		t.Fatalf("duplicate Azure DevOps pull request reservation = %v, err=%v", ok, err)
	}
	if err := replayed.AssignPullRequestWatchTaskID(ctx, pullRequestWatch.ID, pullRequestWatch.Generation, "project-1", "azure-repo-1", 42, "task-1"); err != nil {
		t.Fatalf("assign Azure DevOps pull request reservation: %v", err)
	}
	if err := replayed.DeleteWatchesByWorkspace(ctx, "ws-1"); err != nil {
		t.Fatalf("delete Azure DevOps watches by workspace: %v", err)
	}
	if rows, err := replayed.ListWorkItemWatchTasks(ctx, workWatch.ID, reset.Generation); err != nil || len(rows) != 0 {
		t.Fatalf("work-item reservations after workspace deletion = %+v, err=%v", rows, err)
	}
	if rows, err := replayed.ListPullRequestWatchTasks(ctx, pullRequestWatch.ID, pullRequestWatch.Generation); err != nil || len(rows) != 0 {
		t.Fatalf("pull-request reservations after workspace deletion = %+v, err=%v", rows, err)
	}

	if err := replayed.DeleteTaskPRsByTask(ctx, "task-1"); err != nil {
		t.Fatalf("delete Azure DevOps task PRs: %v", err)
	}
	if err := replayed.DeleteTaskWorkItemsByTask(ctx, "task-1"); err != nil {
		t.Fatalf("delete Azure DevOps task work items: %v", err)
	}
	if err := replayed.DeleteConfig(ctx, "ws-1"); err != nil {
		t.Fatalf("delete Azure DevOps config: %v", err)
	}
	if got, err := store.GetConfig(ctx, "ws-1"); err != nil || got != nil {
		t.Fatalf("config after delete = %+v, err=%v", got, err)
	}
	if err := replayed.ReleaseWorkItemWatchTask(ctx, workWatch.ID, reset.Generation, "project-1", 101); !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("released deleted work-item reservation error = %v, want ErrReservationNotFound", err)
	}
}
