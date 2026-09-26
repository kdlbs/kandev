package gitlab

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresStoreSchemaReplay(t *testing.T) {
	ctx := context.Background()
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := database.Exec(`
		CREATE TABLE workspaces (id TEXT PRIMARY KEY);
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			archived_at TIMESTAMPTZ
		);
		INSERT INTO workspaces (id) VALUES ('ws-1');
		INSERT INTO tasks (id, workspace_id, archived_at) VALUES ('task-1', 'ws-1', NULL)`); err != nil {
		t.Fatalf("create prerequisite tables: %v", err)
	}

	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("first GitLab store schema init: %v", err)
	}
	if _, err := NewStore(database, database); err != nil {
		t.Fatalf("second GitLab store schema init: %v", err)
	}

	config := &GitLabConfig{
		Host:       "https://gitlab.example.com",
		AuthMethod: "pat",
		Username:   "alice",
	}
	if err := store.UpsertConfigForWorkspace(ctx, "ws-1", config); err != nil {
		t.Fatalf("upsert GitLab config: %v", err)
	}
	gotConfig, err := store.GetConfigForWorkspace(ctx, "ws-1")
	if err != nil || gotConfig == nil || gotConfig.Host != config.Host {
		t.Fatalf("get GitLab config: config=%+v err=%v", gotConfig, err)
	}
	if updated, err := store.UpdateConfigHealthForRevision(
		ctx, "ws-1", "alice", true, "", time.Now().UTC(), gotConfig.Revision,
	); err != nil || !updated {
		t.Fatalf("update GitLab config health: updated=%v err=%v", updated, err)
	}

	if err := store.UpsertMentionScope(ctx, &MentionScope{
		WorkspaceID: "ws-1",
		Host:        "https://gitlab.example.com",
		Projects:    []MentionProjectScope{{ID: 7, Path: "group/project"}},
	}); err != nil {
		t.Fatalf("upsert GitLab mention scope: %v", err)
	}
	gotScope, err := store.GetMentionScope(ctx, "ws-1")
	if err != nil || gotScope == nil || len(gotScope.Projects) != 1 {
		t.Fatalf("get GitLab mention scope: scope=%+v err=%v", gotScope, err)
	}

	mergeRequest := &TaskMR{
		TaskID:      "task-1",
		ProjectPath: "group/project",
		MRIID:       42,
		MRURL:       "https://gitlab.example.com/group/project/-/merge_requests/42",
		MRTitle:     "Parity",
		HeadBranch:  "feature/parity",
		BaseBranch:  "main",
		Draft:       true,
	}
	if err := store.UpsertTaskMR(ctx, mergeRequest); err != nil {
		t.Fatalf("upsert GitLab task MR: %v", err)
	}
	gotMR, err := store.GetTaskMR(ctx, "task-1", "", "group/project", 42)
	if err != nil || gotMR == nil || !gotMR.Draft {
		t.Fatalf("get GitLab task MR: mr=%+v err=%v", gotMR, err)
	}
	if err := store.UpdateTaskMRUnresolvedDiscussions(ctx, mergeRequest.ID, 3); err != nil {
		t.Fatalf("update GitLab task MR discussions: %v", err)
	}

	identity := MRIdentity{ProjectPath: "group/project", MRIID: 42}
	trueValue := true
	options, err := store.UpdateTaskMRAutomationOptionsForMR(ctx, "task-1", identity, TaskMRAutomationSwitchPatch{
		AutoMergeEnabled:        &trueValue,
		PromptOnReviewRequested: &trueValue,
	})
	if err != nil || options == nil || !options.AutoMergeEnabled || !options.PromptOnReviewRequested {
		t.Fatalf("update GitLab MR automation: options=%+v err=%v", options, err)
	}
	if err := store.SetTaskMRReviewRequestState(ctx, "task-1", "", "group/project", 42, true); err != nil {
		t.Fatalf("set GitLab MR review state: %v", err)
	}
	if err := store.SetTaskMRObservedState(ctx, "task-1", "", "group/project", 42, gitlabStateMerged); err != nil {
		t.Fatalf("set GitLab MR observed state: %v", err)
	}
	if err := store.RecordTaskMRLifecyclePrompt(ctx, TaskMRLifecyclePrompt{
		TaskID:          "task-1",
		ProjectPath:     "group/project",
		MRIID:           42,
		Event:           mrLifecycleEventReviewRequested,
		ReviewRequested: true,
	}); err != nil {
		t.Fatalf("record GitLab MR lifecycle prompt: %v", err)
	}
	if _, err := store.GetTaskMRLifecycleState(ctx, "task-1", "", "group/project", 42); err != nil {
		t.Fatalf("get GitLab MR lifecycle state: %v", err)
	}
	if subscribed, err := store.ListAutomationSubscribedTaskMRs(ctx); err != nil || len(subscribed) != 1 {
		t.Fatalf("list GitLab automation MRs: mrs=%d err=%v", len(subscribed), err)
	}

	refreshWatch := &MRWatch{
		SessionID:   "session-1",
		TaskID:      "task-1",
		ProjectPath: "group/project",
		MRIID:       42,
		Branch:      "feature/parity",
	}
	if err := store.CreateMRWatch(ctx, refreshWatch); err != nil {
		t.Fatalf("create GitLab MR watch: %v", err)
	}
	if got, err := store.GetMRWatchBySession(ctx, "session-1"); err != nil || got == nil {
		t.Fatalf("get GitLab MR watch: watch=%+v err=%v", got, err)
	}

	reviewWatch := &ReviewWatch{
		WorkspaceID:       "ws-1",
		WorkflowID:        "workflow-1",
		WorkflowStepID:    "step-1",
		Projects:          []ProjectFilter{{Path: "group/project"}},
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "executor-1",
		Enabled:           true,
	}
	if err := store.CreateReviewWatch(ctx, reviewWatch); err != nil {
		t.Fatalf("create GitLab review watch: %v", err)
	}
	if _, err := store.ListEnabledReviewWatchesForWorkspace(ctx, "ws-1"); err != nil {
		t.Fatalf("list GitLab review watches: %v", err)
	}
	reserved, err := store.ReserveReviewMRTask(ctx, reviewWatch.ID, "group/project", 43, "https://gitlab.example.com/mr/43")
	if err != nil || !reserved {
		t.Fatalf("reserve GitLab review MR task: reserved=%v err=%v", reserved, err)
	}
	if err := store.AssignReviewMRTaskID(ctx, reviewWatch.ID, "group/project", 43, "task-1"); err != nil {
		t.Fatalf("assign GitLab review MR task: %v", err)
	}

	issueWatch := &IssueWatch{
		WorkspaceID:       "ws-1",
		WorkflowID:        "workflow-1",
		WorkflowStepID:    "step-1",
		Projects:          []ProjectFilter{{Path: "group/project"}},
		Labels:            []string{"bug"},
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "executor-1",
		Enabled:           true,
	}
	if err := store.CreateIssueWatch(ctx, issueWatch); err != nil {
		t.Fatalf("create GitLab issue watch: %v", err)
	}
	if _, err := store.ListEnabledIssueWatchesForWorkspace(ctx, "ws-1"); err != nil {
		t.Fatalf("list GitLab issue watches: %v", err)
	}
	reserved, err = store.ReserveIssueWatchTask(ctx, issueWatch.ID, "group/project", 9, "https://gitlab.example.com/issue/9")
	if err != nil || !reserved {
		t.Fatalf("reserve GitLab issue task: reserved=%v err=%v", reserved, err)
	}
	if err := store.AssignIssueWatchTaskID(ctx, issueWatch.ID, "group/project", 9, "task-1"); err != nil {
		t.Fatalf("assign GitLab issue task: %v", err)
	}

	if err := store.UpsertActionPresets(ctx, &ActionPresets{
		WorkspaceID: "ws-1",
		MR:          DefaultMRActionPresets(),
		Issue:       DefaultIssueActionPresets(),
	}); err != nil {
		t.Fatalf("upsert GitLab action presets: %v", err)
	}
	if _, err := store.GetActionPresets(ctx, "ws-1"); err != nil {
		t.Fatalf("get GitLab action presets: %v", err)
	}
}
