package automation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAutomationSchemaIncludesContinuationAndRunBindingColumns(t *testing.T) {
	store := setupTestStore(t)

	for _, column := range []string{"continuation_policy", "continuation_task_id"} {
		var count int
		require.NoError(t, store.db.Get(&count,
			`SELECT COUNT(*) FROM pragma_table_info('automations') WHERE name = ?`, column))
		require.Equal(t, 1, count, "automations.%s must be persisted", column)
	}
	for _, column := range []string{"session_id", "turn_id", "thread_action", "thread_reason", "display_title"} {
		var count int
		require.NoError(t, store.db.Get(&count,
			`SELECT COUNT(*) FROM pragma_table_info('automation_runs') WHERE name = ?`, column))
		require.Equal(t, 1, count, "automation_runs.%s must be persisted", column)
	}
}

func TestAutomationSchemaIncludesTargetModeColumns(t *testing.T) {
	store := setupTestStore(t)

	for _, column := range []string{"task_mode", "repository_mode"} {
		var count int
		require.NoError(t, store.db.Get(&count,
			`SELECT COUNT(*) FROM pragma_table_info('automations') WHERE name = ?`, column))
		require.Equal(t, 1, count, "automations.%s must be persisted", column)
	}
}

func TestCountActiveRunsIncludesAdmittedTriggeredRuns(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-1", Name: "A", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	require.NoError(t, store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusTriggered,
		TriggerData:  json.RawMessage(`{}`),
	}))

	count, err := store.CountActiveRuns(ctx, a.ID)

	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestAutomationContinuationPolicyDefaultsAndRoundTrips(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	defaulted := &Automation{WorkspaceID: "ws-1", Name: "default", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, defaulted))
	got, err := store.GetAutomation(ctx, defaulted.ID)
	require.NoError(t, err)
	require.Equal(t, ContinuationPolicyNewTask, got.ContinuationPolicy)

	reused := &Automation{
		WorkspaceID: "ws-1", Name: "reused", Enabled: true,
		MaxConcurrentRuns: 1, ContinuationPolicy: ContinuationPolicyReuseThread,
	}
	require.NoError(t, store.CreateAutomation(ctx, reused))
	got, err = store.GetAutomation(ctx, reused.ID)
	require.NoError(t, err)
	require.Equal(t, ContinuationPolicyReuseThread, got.ContinuationPolicy)

	reused.ContinuationTaskID = "task-1"
	_, err = store.db.Exec(`UPDATE automations SET continuation_task_id = ? WHERE id = ?`,
		reused.ContinuationTaskID, reused.ID)
	require.NoError(t, err)
	got, err = store.GetAutomation(ctx, reused.ID)
	require.NoError(t, err)
	require.Equal(t, "task-1", got.ContinuationTaskID)

	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "continuation_task_id")
}

func TestAutomationTargetModesPersistAndValidate(t *testing.T) {
	svc := newTestService(t)
	svc.SetRepositoryLookup(&fakeRepositoryLookup{repos: map[string]string{"repo-1": "ws-1"}})
	ctx := context.Background()

	hidden, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID:    "ws-1",
		Name:           "scratch",
		TaskMode:       TaskModeAutomationRun,
		RepositoryMode: RepositoryModeNone,
	})
	require.NoError(t, err)
	require.Equal(t, TaskModeAutomationRun, hidden.TaskMode)
	require.Equal(t, RepositoryModeNone, hidden.RepositoryMode)

	normal, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID:    "ws-1",
		Name:           "visible",
		TaskMode:       TaskModeNormalTask,
		WorkflowID:     "workflow-1",
		RepositoryMode: RepositoryModeNone,
	})
	require.NoError(t, err)
	require.Equal(t, TaskModeNormalTask, normal.TaskMode)
	require.Equal(t, RepositoryModeNone, normal.RepositoryMode)

	_, err = svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1",
		Name:        "missing workflow",
		TaskMode:    TaskModeNormalTask,
	})
	require.ErrorIs(t, err, ErrWorkflowRequired)

	got, err := svc.GetAutomation(ctx, normal.ID)
	require.NoError(t, err)
	require.Equal(t, TaskModeNormalTask, got.TaskMode)
	require.Equal(t, RepositoryModeNone, got.RepositoryMode)

	selected, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID:    "ws-1",
		Name:           "selected",
		RepositoryMode: RepositoryModeSelected,
		RepositoryIDs:  []string{"repo-1"},
	})
	require.NoError(t, err)
	mode := RepositoryModeNone
	updated, err := svc.UpdateAutomation(ctx, selected.ID, &UpdateAutomationRequest{RepositoryMode: &mode})
	require.NoError(t, err)
	require.Equal(t, RepositoryModeNone, updated.RepositoryMode)
	require.Empty(t, updated.RepositoryIDs)
}

func TestRepositoryFreeTargetAcceptsWorktreeExecutor(t *testing.T) {
	svc := newTestService(t)

	created, err := svc.CreateAutomation(context.Background(), &CreateAutomationRequest{
		WorkspaceID:       "ws-1",
		Name:              "worktree scratch",
		ExecutorProfileID: "worktree-profile",
		RepositoryMode:    RepositoryModeNone,
	})
	require.NoError(t, err)
	require.Equal(t, RepositoryModeNone, created.RepositoryMode)
}

func TestRepositoryFreeTargetDoesNotSelectWorkspaceRepositoryForTrigger(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.CreateAutomation(context.Background(), &CreateAutomationRequest{
		WorkspaceID:    "ws-1",
		Name:           "pr trigger",
		RepositoryMode: RepositoryModeNone,
		Triggers:       []CreateTriggerSpec{{Type: TriggerTypeGitHubPR, Config: json.RawMessage(`{}`)}},
	})
	require.NoError(t, err)

	_, err = svc.CreateAutomation(context.Background(), &CreateAutomationRequest{
		WorkspaceID:    "ws-1",
		Name:           "scheduled scratch",
		RepositoryMode: RepositoryModeNone,
		Triggers:       []CreateTriggerSpec{{Type: TriggerTypeScheduled, Config: json.RawMessage(`{}`)}},
	})
	require.NoError(t, err)
}

func TestAutomationRunPersistsExactBindingAndThreadMetadata(t *testing.T) {
	store := setupTestStore(t)
	run := &AutomationRun{
		AutomationID: "automation-1", TriggerID: "trigger-1", TriggerType: TriggerTypeScheduled,
		Status: RunStatusTaskCreated, TaskID: "task-1", SessionID: "session-1", TurnID: "turn-1",
		ThreadAction: ThreadActionResumed, ThreadReason: "", DisplayTitle: "Nightly report",
		TriggerData: json.RawMessage(`{"ok":true}`),
	}
	require.NoError(t, store.CreateRun(context.Background(), run))
	got, err := store.GetRun(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, run.SessionID, got.SessionID)
	require.Equal(t, run.TurnID, got.TurnID)
	require.Equal(t, run.ThreadAction, got.ThreadAction)
	require.Equal(t, run.DisplayTitle, got.DisplayTitle)
}

func TestAutomationRunBindsAndSettlesExactTurn(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-1", Name: "exact", MaxConcurrentRuns: 1}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	run := &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusTriggered,
		TriggerData:  json.RawMessage(`{}`),
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := store.BindRunTask(ctx, run.ID, "task-shared"); err != nil {
		t.Fatal(err)
	}
	if err := store.BindRun(ctx, run.ID, "task-shared", "session-shared", "turn-1", ThreadActionResumed, "reused continuation"); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRunTerminalByBinding(ctx, "task-shared", "session-shared", "turn-1", RunStatusSucceeded, ""); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != RunStatusSucceeded || got.TaskID != "task-shared" || got.SessionID != "session-shared" || got.TurnID != "turn-1" {
		t.Fatalf("settled exact run = %+v", got)
	}
	if got.ThreadAction != ThreadActionResumed || got.ThreadReason != "reused continuation" {
		t.Fatalf("thread metadata = action %q reason %q", got.ThreadAction, got.ThreadReason)
	}
}

func TestAutomationRunTerminalBindingDoesNotSettleAnotherTurn(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-1", Name: "exact", MaxConcurrentRuns: 1}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	for _, turnID := range []string{"turn-1", "turn-2"} {
		run := &AutomationRun{
			AutomationID: a.ID,
			TriggerType:  TriggerTypeScheduled,
			Status:       RunStatusTaskCreated,
			TaskID:       "task-shared",
			SessionID:    "session-shared",
			TurnID:       turnID,
			TriggerData:  json.RawMessage(`{}`),
		}
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.MarkRunTerminalByBinding(ctx, "task-shared", "session-shared", "turn-1", RunStatusFailed, "first failed"); err != nil {
		t.Fatal(err)
	}
	runs, err := store.ListRuns(ctx, a.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		if run.TurnID == "turn-1" && run.Status != RunStatusFailed {
			t.Errorf("turn-1 status = %q, want failed", run.Status)
		}
		if run.TurnID == "turn-2" && run.Status != RunStatusTaskCreated {
			t.Errorf("turn-2 status = %q, want task_created", run.Status)
		}
	}
}

func TestReuseThreadRequiresSingleConcurrencySlot(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.CreateAutomation(context.Background(), &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "invalid", ContinuationPolicy: ContinuationPolicyReuseThread,
		MaxConcurrentRuns: 2,
	})
	require.ErrorContains(t, err, "reuse_thread requires max_concurrent_runs = 1")

	a, err := svc.CreateAutomation(context.Background(), &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "valid", ContinuationPolicy: ContinuationPolicyReuseThread,
		MaxConcurrentRuns: 1,
	})
	require.NoError(t, err)
	tooMany := 2
	_, err = svc.UpdateAutomation(context.Background(), a.ID, &UpdateAutomationRequest{
		MaxConcurrentRuns: &tooMany,
	})
	require.ErrorContains(t, err, "reuse_thread requires max_concurrent_runs = 1")
}

func TestReuseThreadNormalizesNonPositiveConcurrencyOnUpdate(t *testing.T) {
	svc := newTestService(t)
	a, err := svc.CreateAutomation(context.Background(), &CreateAutomationRequest{
		WorkspaceID:       "ws-1",
		Name:              "normalize",
		MaxConcurrentRuns: 2,
	})
	require.NoError(t, err)

	policy := ContinuationPolicyReuseThread
	zero := 0
	updated, err := svc.UpdateAutomation(context.Background(), a.ID, &UpdateAutomationRequest{
		ContinuationPolicy: &policy,
		MaxConcurrentRuns:  &zero,
	})
	require.NoError(t, err)
	require.Equal(t, ContinuationPolicyReuseThread, updated.ContinuationPolicy)
	require.Equal(t, 1, updated.MaxConcurrentRuns)
}

func TestStoreUpdateNormalizesNonPositiveReuseConcurrency(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-1", Name: "normalize-store", Enabled: true, MaxConcurrentRuns: 2}
	require.NoError(t, store.CreateAutomation(ctx, a))

	policy := ContinuationPolicyReuseThread
	zero := 0
	require.NoError(t, store.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{
		ContinuationPolicy: &policy,
		MaxConcurrentRuns:  &zero,
	}))
	got, err := store.GetAutomation(ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, 1, got.MaxConcurrentRuns)
}

func TestDispatchRunBindsExactIdentityAndRejectsStoppedAdmission(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-1", Name: "dispatch", Enabled: true, MaxConcurrentRuns: 1}
	require.NoError(t, svc.store.CreateAutomation(ctx, a))

	run := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTriggered}
	require.NoError(t, svc.store.CreateRun(ctx, run))
	called := false
	require.NoError(t, svc.DispatchRun(ctx, run.ID, ThreadActionCreated, "created", func() (RunDispatch, error) {
		called = true
		return RunDispatch{TaskID: "task-1", SessionID: "session-1", TurnID: "turn-1"}, nil
	}))
	got, err := svc.store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, RunStatusTaskCreated, got.Status)
	require.Equal(t, "turn-1", got.TurnID)

	stopped := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTriggered}
	require.NoError(t, svc.store.CreateRun(ctx, stopped))
	require.NoError(t, svc.store.MarkRunTerminal(ctx, stopped.ID, "", "", RunStatusFailed, "stopped"))
	called = false
	err = svc.DispatchRun(ctx, stopped.ID, ThreadActionCreated, "created", func() (RunDispatch, error) {
		called = true
		return RunDispatch{TaskID: "task-2", SessionID: "session-2", TurnID: "turn-2"}, nil
	})
	require.ErrorIs(t, err, ErrAutomationRunNotDispatchable)
	require.False(t, called)
}
