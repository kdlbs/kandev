package automation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresStoreSchemaReplay(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := database.Exec(`
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			archived_at TIMESTAMPTZ
		);
		CREATE TABLE task_sessions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			is_primary INTEGER NOT NULL DEFAULT 0,
			state TEXT NOT NULL
		);
		CREATE TABLE task_session_messages (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			turn_id TEXT NOT NULL,
			author_type TEXT NOT NULL,
			content TEXT NOT NULL,
			type TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL
		);
		CREATE TABLE task_environments (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL
		);
		CREATE TABLE task_environment_repos (
			id TEXT PRIMARY KEY,
			task_environment_id TEXT NOT NULL,
			status TEXT NOT NULL
		)`); err != nil {
		t.Fatalf("create task prerequisites: %v", err)
	}
	if _, err := database.Exec(`
		INSERT INTO tasks (id, archived_at) VALUES ('task-1', NULL);
		INSERT INTO task_sessions (id, task_id, is_primary, state)
		VALUES ('session-1', 'task-1', 1, 'RUNNING');
		INSERT INTO task_environments (id, task_id) VALUES ('environment-1', 'task-1');
		INSERT INTO task_environment_repos (id, task_environment_id, status)
		VALUES ('environment-repo-1', 'environment-1', 'active')`); err != nil {
		t.Fatalf("seed task prerequisites: %v", err)
	}

	if _, err := NewStore(database, database); err != nil {
		t.Fatalf("first automation store schema init: %v", err)
	}
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("second automation store schema init: %v", err)
	}

	ctx := t.Context()
	automation := &Automation{
		ID:                "automation-postgres",
		WorkspaceID:       "ws-1",
		Name:              "PostgreSQL parity",
		WorkflowID:        "workflow-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "executor-1",
		Prompt:            "Review the change",
		Enabled:           true,
		MaxConcurrentRuns: 2,
		RepositoryMode:    RepositoryModeSelected,
		Repositories: []AutomationRepository{
			{RepositoryID: "repo-a", BaseBranch: "main"},
			{RepositoryID: "repo-b", BaseBranch: "develop"},
		},
	}
	if err := store.CreateAutomation(ctx, automation); err != nil {
		t.Fatalf("create automation: %v", err)
	}
	gotAutomation, err := store.GetAutomation(ctx, automation.ID)
	if err != nil || gotAutomation == nil || len(gotAutomation.Repositories) != 2 || gotAutomation.Repositories[1].BaseBranch != "develop" {
		t.Fatalf("get automation = %+v, err %v", gotAutomation, err)
	}
	if automations, err := store.ListAutomations(ctx, "ws-1"); err != nil || len(automations) != 1 {
		t.Fatalf("list automations = %+v, err %v", automations, err)
	}
	if enabled, err := store.ListEnabledByAgentProfile(ctx, "agent-1"); err != nil || len(enabled) != 1 {
		t.Fatalf("list enabled automations = %+v, err %v", enabled, err)
	}

	trigger := &AutomationTrigger{
		ID:           "trigger-postgres",
		AutomationID: automation.ID,
		Type:         TriggerTypeScheduled,
		Config:       json.RawMessage(`{"cron_expression":"0 9 * * *"}`),
		Enabled:      true,
	}
	if err := store.CreateTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	if gotTrigger, err := store.GetTrigger(ctx, trigger.ID); err != nil || gotTrigger == nil || string(gotTrigger.Config) != string(trigger.Config) {
		t.Fatalf("get trigger = %+v, err %v", gotTrigger, err)
	}
	if triggers, err := store.ListEnabledTriggersByType(ctx, TriggerTypeScheduled); err != nil || len(triggers) != 1 {
		t.Fatalf("list enabled triggers = %+v, err %v", triggers, err)
	}

	run := &AutomationRun{
		ID:           "run-postgres",
		AutomationID: automation.ID,
		TriggerID:    trigger.ID,
		TriggerType:  TriggerTypeScheduled,
		TaskID:       "task-1",
		Status:       RunStatusTaskCreated,
		DedupKey:     "dedup-postgres",
		TriggerData:  json.RawMessage(`{"source":"postgres"}`),
		SessionID:    "session-1",
		TurnID:       "turn-1",
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := database.Exec(`
		INSERT INTO task_session_messages (id, task_id, turn_id, author_type, content, type, created_at)
		VALUES ('message-1', 'task-1', 'turn-1', 'agent', 'PostgreSQL run completed', 'message', $1)`, time.Now().UTC()); err != nil {
		t.Fatalf("seed run summary: %v", err)
	}

	runs, err := store.ListRuns(ctx, automation.ID, 20)
	if err != nil || len(runs) != 1 || runs[0].Status != RunStatusTaskCreated || runs[0].Summary != "PostgreSQL run completed" {
		t.Fatalf("list runs = %+v, err %v", runs, err)
	}
	workspaceRuns, err := store.ListWorkspaceRuns(ctx, "ws-1", 20)
	if err != nil || len(workspaceRuns) != 1 || workspaceRuns[0].AutomationName != automation.Name {
		t.Fatalf("list workspace runs = %+v, err %v", workspaceRuns, err)
	}
	if openRuns, err := store.ListOpenRuns(ctx, automation.ID); err != nil || len(openRuns) != 1 {
		t.Fatalf("list open runs = %+v, err %v", openRuns, err)
	}
	if allOpenRuns, err := store.ListAllOpenRuns(ctx); err != nil || len(allOpenRuns) != 1 {
		t.Fatalf("list all open runs = %+v, err %v", allOpenRuns, err)
	}
	if summaries, err := store.ListAutomationSummaries(ctx, "ws-1"); err != nil || len(summaries) != 1 || summaries[0].OpenRuns != 1 {
		t.Fatalf("list automation summaries = %+v, err %v", summaries, err)
	}
	if count, err := store.CountActiveRuns(ctx, automation.ID); err != nil || count != 1 {
		t.Fatalf("count active runs = %d, err %v", count, err)
	}
	if found, err := store.HasRunWithDedupKey(ctx, automation.ID, run.DedupKey); err != nil || !found {
		t.Fatalf("has run with dedup key = %v, err %v", found, err)
	}
	if inUse, err := store.RunWorkspaceInUse(ctx, "task-1"); err != nil || !inUse {
		t.Fatalf("run workspace in use = %v, err %v", inUse, err)
	}
	if taskIDs, err := store.PrunableRunTaskIDs(ctx, "task-1", 0); err != nil || len(taskIDs) != 0 {
		t.Fatalf("prunable run task IDs = %v, err %v", taskIDs, err)
	}

	cleanupRun := &AutomationRun{
		ID:           "run-cleanup",
		AutomationID: automation.ID,
		TriggerID:    trigger.ID,
		TriggerType:  TriggerTypeScheduled,
		TaskID:       "task-cleanup",
		Status:       RunStatusSucceeded,
		TriggerData:  json.RawMessage(`{}`),
	}
	if err := store.CreateRun(ctx, cleanupRun); err != nil {
		t.Fatalf("create cleanup run: %v", err)
	}
	if jobs, err := store.DeleteAutomationWithCleanup(ctx, automation.ID, []string{"task-cleanup"}); err != nil || len(jobs) != 1 || jobs[0] != "task-cleanup" {
		t.Fatalf("delete automation with cleanup = %v, err %v", jobs, err)
	}
	jobs, err := store.ListCleanupJobs(ctx)
	if err != nil || len(jobs) != 1 || jobs[0].TaskID != "task-cleanup" {
		t.Fatalf("list cleanup jobs = %+v, err %v", jobs, err)
	}
	if err := store.UpdateCleanupJobError(ctx, "task-cleanup", "cleanup failed"); err != nil {
		t.Fatalf("update cleanup job error: %v", err)
	}
	if err := store.DeleteCleanupJob(ctx, "task-cleanup"); err != nil {
		t.Fatalf("delete cleanup job: %v", err)
	}
}
func TestPostgresRetryConcurrencyContracts(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("create automation store: %v", err)
	}
	ctx := context.Background()
	a := &Automation{ID: "pg-retry-automation", WorkspaceID: "pg-retry-workspace",
		Name: "PostgreSQL retry", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatalf("create automation: %v", err)
	}

	group := &RetryGroup{ID: "pg-retry-group", AutomationID: a.ID,
		Generation: 1, State: RetryGroupLive}
	if err := store.CreateRetryGroup(ctx, group); err != nil {
		t.Fatalf("create retry group: %v", err)
	}
	parent := &AutomationRun{ID: "pg-retry-parent", AutomationID: a.ID,
		TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
		RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
		AttemptNumber:       1,
		RetryPolicySnapshot: `{"mode":"finite","max_retries":"2","delay_seconds":"0","backoff":"fixed"}`}
	if err := store.CreateRun(ctx, parent); err != nil {
		t.Fatalf("create parent run: %v", err)
	}
	child, err := store.FinalizeRetryFailure(ctx, parent.ID, 1, errors.New("provider failed"), "launch")
	if err != nil {
		t.Fatalf("finalize retry: %v", err)
	}
	replayed, err := store.FinalizeRetryFailure(ctx, parent.ID, 1, errors.New("replayed"), "launch")
	if err != nil || replayed == nil || replayed.ID != child.ID {
		t.Fatalf("idempotent retry replay = %+v, err %v", replayed, err)
	}

	leaseRun := &AutomationRun{ID: "pg-retry-lease-run", AutomationID: a.ID,
		TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
		RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered, AttemptNumber: 3}
	if err := store.CreateRun(ctx, leaseRun); err != nil {
		t.Fatalf("create lease run: %v", err)
	}
	intent := &RetryTaskIntent{ID: "pg-retry-lease-intent", RunID: leaseRun.ID,
		GroupGeneration: 1, State: retryIntentAdmitted}
	if err := store.CreateRetryIntent(ctx, intent); err != nil {
		t.Fatalf("create lease intent: %v", err)
	}
	if err := store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "pg-retry-lease-operation", IntentID: intent.ID, RunID: leaseRun.ID,
		GroupGeneration: 1, Kind: retryTaskOperationKind, State: retryOperationRequested,
	}); err != nil {
		t.Fatalf("create lease operation: %v", err)
	}
	first, err := store.BeginRetryTaskOperation(ctx, leaseRun.ID, 1)
	if err != nil {
		t.Fatalf("first lease: %v", err)
	}
	if _, err := store.BeginRetryTaskOperation(ctx, leaseRun.ID, 1); !errors.Is(err, ErrRetryOperationUndispatchable) {
		t.Fatalf("unexpired lease error = %v", err)
	}
	if _, err := database.Exec(`UPDATE automation_run_operations SET lease_expires_at = $1 WHERE operation_id = $2`,
		time.Now().UTC().Add(-time.Minute), first.ID); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	reacquired, err := store.BeginRetryTaskOperation(ctx, leaseRun.ID, 1)
	if err != nil || reacquired.LeaseToken == first.LeaseToken {
		t.Fatalf("expired lease reacquisition = %+v, err %v", reacquired, err)
	}

	cancelGroup := &RetryGroup{ID: "pg-cancel-group", AutomationID: a.ID,
		Generation: 1, State: RetryGroupLive}
	if err := store.CreateRetryGroup(ctx, cancelGroup); err != nil {
		t.Fatalf("create cancel group: %v", err)
	}
	cancelRun := &AutomationRun{ID: "pg-cancel-run", AutomationID: a.ID,
		TriggerType: TriggerTypeManual, Status: RunStatusTaskCreated,
		TaskID: "pg-cancel-task", SessionID: "pg-cancel-session", TurnID: "pg-cancel-turn",
		RetryGroupID: cancelGroup.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
	if err := store.CreateRun(ctx, cancelRun); err != nil {
		t.Fatalf("create cancel run: %v", err)
	}
	if err := store.CancelRetryGroup(ctx, cancelGroup.ID, 1); err != nil {
		t.Fatalf("cancel retry group: %v", err)
	}
	if err := store.BindRun(ctx, cancelRun.ID, "new-task", "new-session", "new-turn", ThreadActionCreated, ""); !errors.Is(err, ErrRetryGenerationMismatch) {
		t.Fatalf("bind cancelled retry error = %v", err)
	}
	storedCancel, err := store.GetRun(ctx, cancelRun.ID)
	if err != nil || storedCancel.RetryState != RetryStateCancelled {
		t.Fatalf("cancelled retry run = %+v, err %v", storedCancel, err)
	}
}
