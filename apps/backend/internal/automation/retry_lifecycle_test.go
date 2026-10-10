package automation

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestRetryDelayZeroBaseIsConstantTimeForHugeAttempt(t *testing.T) {
	done := make(chan struct{})
	go func() {
		delay, err := RetryDelay(RetryPolicy{
			Mode: RetryModeInfinite, DelaySeconds: "0", Backoff: RetryBackoffExponential,
		}, math.MaxInt64)
		require.NoError(t, err)
		require.Zero(t, delay)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("zero-base exponential delay did not complete in constant time")
	}
}

func TestSanitizeAutomationFailureUsesAllowlistedMessages(t *testing.T) {
	failure := SanitizeAutomationFailure(errors.New(
		`Authorization: Bearer abc.def.ghi {"access_token":"json-secret"} bearer raw-secret`,
	), "launch", nil)

	require.Equal(t, "automation launch failed", failure.Message)
	require.Equal(t, "launch", failure.FailureClass)
	require.NotContains(t, failure.Message, "raw-secret")
	require.NotContains(t, failure.Message, "json-secret")
}

func TestFinalizeRetrySchedulingOverflowDoesNotEnqueueOutbox(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-overflow", Name: "overflow", Enabled: true,
		RetryPolicy: RetryPolicy{Mode: RetryModeFinite, MaxRetries: "3",
			DelaySeconds: "9223372036854775807", Backoff: RetryBackoffExponential}}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-overflow", AutomationID: a.ID, Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "run-overflow", AutomationID: a.ID, TriggerType: TriggerTypeManual,
		Status: RunStatusTriggered, RetryGroupID: group.ID, AttemptNumber: 2,
		RetryState: RetryStateTriggered, RetryGroupGeneration: 1,
		RetryPolicySnapshot: `{"mode":"finite","max_retries":"3","delay_seconds":"9223372036854775807","backoff":"exponential"}`}
	require.NoError(t, store.CreateRun(ctx, run))

	child, err := store.FinalizeRetryFailure(ctx, run.ID, 1, errors.New("provider failed"), "launch")
	require.NoError(t, err)
	require.Equal(t, RunStatusRetrySchedulingFailed, child.Status)
	require.Equal(t, RetryStateSchedulingFailed, child.RetryState)

	var outboxCount int
	require.NoError(t, store.db.Get(&outboxCount,
		`SELECT COUNT(*) FROM automation_retry_outbox WHERE run_id = ?`, child.ID))
	require.Zero(t, outboxCount)
}
func TestFinalizedRetryChildCanBeClaimedAndPromoted(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{ID: "automation-child-claim", WorkspaceID: "ws-child-claim", Name: "child claim", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-child-claim", AutomationID: a.ID, Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	parent := &AutomationRun{
		ID: "run-child-claim", AutomationID: a.ID, TriggerType: TriggerTypeManual,
		Status: RunStatusTriggered, RetryGroupID: group.ID, RetryGroupGeneration: 1,
		AttemptNumber: 1, RetryState: RetryStateTriggered,
		RetryPolicySnapshot: `{"mode":"finite","max_retries":"2","delay_seconds":"0","backoff":"fixed"}`,
	}
	require.NoError(t, store.CreateRun(ctx, parent))
	child, err := store.FinalizeRetryFailure(ctx, parent.ID, 1, errors.New("attempt failed"), "launch")
	require.NoError(t, err)

	claimed, token, err := store.ClaimDueRetry(ctx, time.Now().UTC().Add(time.Second), time.Minute)
	require.NoError(t, err)
	require.Equal(t, child.ID, claimed.ID)
	require.NotEmpty(t, token)
	require.NoError(t, store.PromoteClaimedRetry(ctx, child.ID, token, 1))
	promoted, err := store.GetRun(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, RunStatusTriggered, promoted.Status)
	require.Equal(t, RetryStateTriggered, promoted.RetryState)
}

func TestRetrySnapshotInterpolatesBeforeSafeProjection(t *testing.T) {
	store := setupTestStore(t)
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	svc := NewService(store, bus.NewMemoryEventBus(log), log)
	a := &Automation{WorkspaceID: "ws-snapshot", Name: "snapshot", Enabled: true,
		Prompt: "Review {{pr.title}} (#{{pr.number}})", TaskTitleTemplate: "Retry {{pr.title}}",
		RetryPolicy: RetryPolicy{Mode: RetryModeFinite, MaxRetries: "1", DelaySeconds: "0"}}
	require.NoError(t, store.CreateAutomation(context.Background(), a))
	trigger := &AutomationTrigger{ID: "trigger-snapshot", AutomationID: a.ID,
		Type: TriggerTypeGitHubPR, Enabled: true}
	require.NoError(t, store.CreateTrigger(context.Background(), trigger))

	result, err := svc.FireTrigger(context.Background(), a.ID, trigger.ID, TriggerTypeGitHubPR,
		json.RawMessage(`{"number":42,"title":"Fix retry safety","repo":"acme/app"}`), DedupKey("pr-42"))
	require.NoError(t, err)
	run, err := store.GetRun(context.Background(), result.RunID)
	require.NoError(t, err)
	require.Equal(t, "Review Fix retry safety (#42)", run.RetryResolvedPrompt)
	require.Equal(t, "Retry Fix retry safety", run.RetryResolvedTitle)
	require.Contains(t, run.RetryTriggerSnapshot, "Fix retry safety")
	child, err := store.FinalizeRetryFailure(context.Background(), run.ID, 1, errors.New("provider failed"), "launch")
	require.NoError(t, err)
	require.Equal(t, run.RetryResolvedPrompt, child.RetryResolvedPrompt)
	require.Equal(t, run.RetryResolvedTitle, child.RetryResolvedTitle)
}

func TestFinalizeRetryFailureRefreshesContinuationSnapshot(t *testing.T) {
	store := setupTestStore(t)
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	svc := NewService(store, bus.NewMemoryEventBus(log), log)
	ctx := context.Background()
	automation := &Automation{
		ID: "automation-continuation-retry", WorkspaceID: "workspace-continuation-retry",
		Name: "continuation retry", Enabled: true, MaxConcurrentRuns: 1,
		ContinuationPolicy: ContinuationPolicyReuseThread,
		RetryPolicy:        RetryPolicy{Mode: RetryModeFinite, MaxRetries: "1", DelaySeconds: "0"},
	}
	require.NoError(t, store.CreateAutomation(ctx, automation))
	trigger := &AutomationTrigger{
		ID: "trigger-continuation-retry", AutomationID: automation.ID,
		Type: TriggerTypeManual, Enabled: true,
	}
	require.NoError(t, store.CreateTrigger(ctx, trigger))
	result, err := svc.FireTrigger(ctx, automation.ID, trigger.ID, trigger.Type, nil, DedupKey("continuation-retry"))
	require.NoError(t, err)
	parent, err := store.GetRun(ctx, result.RunID)
	require.NoError(t, err)
	require.NoError(t, store.SetContinuationTaskID(ctx, automation.ID, "original-task"))

	child, err := store.FinalizeRetryFailure(ctx, result.RunID, 1, errors.New("provider failed"), "launch")
	require.NoError(t, err)
	require.NotNil(t, child)
	require.Equal(t, parent.RetryLaunchConfigSnapshot, child.RetryLaunchConfigSnapshot)
	require.Equal(t, parent.RetryContinuationSnapshot, child.RetryContinuationSnapshot)
	snapshot, err := DecodeRetryLaunchConfigSnapshot(
		child.RetryLaunchConfigSnapshot, child.RetryLaunchConfigVersion,
	)
	require.NoError(t, err)
	require.Empty(t, snapshot.ContinuationTaskID)
}

func TestFinalizeRetryFailureRejectsTerminalParentCAS(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{ID: "automation-finalize-cas", WorkspaceID: "ws-finalize-cas",
		Name: "finalize cas", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-finalize-cas", AutomationID: a.ID,
		Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	parent := &AutomationRun{
		ID: "run-finalize-cas", AutomationID: a.ID, TriggerType: TriggerTypeManual,
		Status: RunStatusFailed, RetryGroupID: group.ID, RetryGroupGeneration: 1,
		RetryState: RetryStateCompleted, AttemptNumber: 1,
		RetryPolicySnapshot: `{"mode":"finite","max_retries":"2","delay_seconds":"0","backoff":"fixed"}`,
	}
	require.NoError(t, store.CreateRun(ctx, parent))

	child, err := store.FinalizeRetryFailure(ctx, parent.ID, 1, errors.New("late failure"), "launch")
	require.Nil(t, child)
	require.ErrorIs(t, err, ErrRetryGenerationMismatch)
	var childCount int
	require.NoError(t, store.db.Get(&childCount,
		`SELECT COUNT(*) FROM automation_runs WHERE retry_parent_run_id = ?`, parent.ID))
	require.Zero(t, childCount)
}

func TestBindRunTaskRejectsCancelledRetryGeneration(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-fence", Name: "fence", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-fence", AutomationID: a.ID, Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "run-fence", AutomationID: a.ID, TriggerType: TriggerTypeManual,
		Status: RunStatusTriggered, RetryGroupID: group.ID, RetryGroupGeneration: 1,
		RetryState: RetryStateTriggered}
	require.NoError(t, store.CreateRun(ctx, run))
	require.NoError(t, store.CancelRetryGroup(ctx, group.ID, 1))

	require.ErrorIs(t, store.BindRunTask(ctx, run.ID, "task-created", ""), ErrRetryGenerationMismatch)
}

func TestBindRunTaskIsIdempotentForAlreadyBoundRun(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{ID: "bind-idempotent-automation", WorkspaceID: "bind-idempotent-workspace", Name: "bind idempotent", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	run := &AutomationRun{
		ID: "bind-idempotent-run", AutomationID: a.ID, Status: RunStatusTaskCreated,
		TaskID: "task-already-bound", SessionID: "session-1", TurnID: "turn-1",
	}
	require.NoError(t, store.CreateRun(ctx, run))

	require.NoError(t, store.BindRunTask(ctx, run.ID, run.TaskID, ""))
}

func TestReplayPendingRetryEventsRevokesSchedulingFailedRows(t *testing.T) {
	store := setupTestStore(t)
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	busImpl := bus.NewMemoryEventBus(log)
	eventsSeen := make(chan struct{}, 1)
	_, err = busImpl.Subscribe(events.AutomationTriggered, func(context.Context, *bus.Event) error {
		eventsSeen <- struct{}{}
		return nil
	})
	require.NoError(t, err)
	svc := NewService(store, busImpl, log)
	a := &Automation{WorkspaceID: "ws-recovery", Name: "recovery", Enabled: true}
	require.NoError(t, store.CreateAutomation(context.Background(), a))
	run := &AutomationRun{ID: "run-recovery", AutomationID: a.ID, TriggerType: TriggerTypeManual,
		Status: RunStatusRetrySchedulingFailed, RetryState: RetryStateSchedulingFailed,
		RetryGroupID: "group-recovery", RetryGroupGeneration: 1}
	require.NoError(t, store.CreateRun(context.Background(), run))
	group := &RetryGroup{ID: run.RetryGroupID, AutomationID: a.ID, Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(context.Background(), group))
	require.NoError(t, store.CreateRetryOutbox(context.Background(), &RetryOutbox{
		EventID: "run-recovery:1", RunID: run.ID, SnapshotVersion: 1, State: "pending",
	}))

	require.NoError(t, svc.ReplayPendingRetryEvents(context.Background()))
	select {
	case <-eventsSeen:
		t.Fatal("scheduling-failed retry was replayed")
	default:
	}
	var state string
	require.NoError(t, store.db.Get(&state,
		`SELECT state FROM automation_retry_outbox WHERE event_id = ?`, "run-recovery:1"))
	require.Equal(t, "revoked", state)
}
func TestReplayPendingRetryEventsRevokesAbandonedOperations(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	busImpl := bus.NewMemoryEventBus(log)
	eventsSeen := make(chan struct{}, 1)
	_, err = busImpl.Subscribe(events.AutomationTriggered, func(context.Context, *bus.Event) error {
		eventsSeen <- struct{}{}
		return nil
	})
	require.NoError(t, err)
	svc := NewService(store, busImpl, log)
	a := &Automation{ID: "automation-abandoned", WorkspaceID: "ws-abandoned",
		Name: "abandoned", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-abandoned", AutomationID: a.ID,
		Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "run-abandoned", AutomationID: a.ID,
		TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
		RetryState: RetryStateTriggered, RetryGroupID: group.ID,
		RetryGroupGeneration: 1}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{ID: "intent-abandoned", RunID: run.ID,
		GroupGeneration: 1, State: retryIntentAdmitted}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "operation-abandoned", IntentID: intent.ID, RunID: run.ID,
		GroupGeneration: 1, Kind: retryTaskOperationKind,
		State: retryOperationAbandoned,
	}))
	require.NoError(t, store.CreateRetryOutbox(ctx, &RetryOutbox{
		EventID: "run-abandoned:1", RunID: run.ID, SnapshotVersion: 1,
		State: "pending",
	}))

	require.NoError(t, svc.ReplayPendingRetryEvents(ctx))
	select {
	case <-eventsSeen:
		t.Fatal("abandoned retry operation was replayed")
	default:
	}
	var state string
	require.NoError(t, store.db.Get(&state,
		`SELECT state FROM automation_retry_outbox WHERE event_id = ?`,
		"run-abandoned:1"))
	require.Equal(t, "revoked", state)
}

func TestRetryDelayAndTitleUseCheckedArithmetic(t *testing.T) {
	policy := RetryPolicy{Mode: RetryModeFinite, MaxRetries: "10", DelaySeconds: "2", Backoff: RetryBackoffExponential}
	delay, err := RetryDelay(policy, 3)
	require.NoError(t, err)
	require.Equal(t, 8*time.Second, delay)

	_, err = RetryDelay(RetryPolicy{Mode: RetryModeInfinite, DelaySeconds: "9223372036854775807", Backoff: RetryBackoffExponential}, 2)
	require.ErrorIs(t, err, ErrRetryDelayOverflow)

	title := FormatRetryTitle(strings.Repeat("x", 100), 12)
	require.Equal(t, 60, runeCount(title))
	require.True(t, strings.HasPrefix(title, "Retry 12: "))
}

func TestRetryFailureSanitizesSecretsAndCreatesOneChild(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-retry", Name: "retry", Enabled: true, MaxConcurrentRuns: 1,
		RetryPolicy: RetryPolicy{Mode: RetryModeFinite, MaxRetries: "2", DelaySeconds: "0"}}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-1", AutomationID: a.ID, TriggerID: "trigger-1", Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "run-1", AutomationID: a.ID, TriggerID: "trigger-1", TriggerType: TriggerTypeManual,
		Status: RunStatusTriggered, RetryGroupID: group.ID, AttemptNumber: 1, RetryState: RetryStateTriggered,
		RetryGroupGeneration: 1, RetryBaseTitle: "Root", RetryPolicySnapshot: `{"mode":"finite","max_retries":"2","delay_seconds":"0","backoff":"fixed","history_mode":"attempts"}`}
	require.NoError(t, store.CreateRun(ctx, run))

	child, err := store.FinalizeRetryFailure(ctx, run.ID, 1, errors.New("token=secret prompt=private /home/user"), "launch")
	require.NoError(t, err)
	require.NotNil(t, child)
	require.Equal(t, int64(2), child.AttemptNumber)
	require.Equal(t, RunStatusScheduledRetry, child.Status)
	require.Equal(t, RetryStateScheduled, child.RetryState)
	require.NotContains(t, child.ErrorMessage, "secret")

	replay, err := store.FinalizeRetryFailure(ctx, run.ID, 1, errors.New("different"), "launch")
	require.NoError(t, err)
	require.Equal(t, child.ID, replay.ID)
	var count int
	require.NoError(t, store.db.Get(&count, `SELECT COUNT(*) FROM automation_runs WHERE retry_group_id = ?`, group.ID))
	require.Equal(t, 2, count)
	var intents, operations int
	require.NoError(t, store.db.Get(&intents,
		`SELECT COUNT(*) FROM automation_run_task_intents WHERE run_id = ?`, child.ID))
	require.NoError(t, store.db.Get(&operations,
		`SELECT COUNT(*) FROM automation_run_operations WHERE run_id = ?`, child.ID))
	require.Equal(t, 1, intents)
	require.Equal(t, 1, operations)
}

func TestRetryClaimLeaseIsSingleUseAndGenerationFenced(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-claim", Name: "claim", Enabled: true, MaxConcurrentRuns: 1}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-claim", AutomationID: a.ID, Generation: 4, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	due := time.Now().UTC().Add(-time.Second)
	run := &AutomationRun{ID: "run-claim", AutomationID: a.ID, Status: RunStatusScheduledRetry, RetryGroupID: group.ID,
		AttemptNumber: 2, RetryState: RetryStateScheduled, RetryScheduledAt: &due, RetryGroupGeneration: 4}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{ID: "intent-claim", RunID: run.ID,
		GroupGeneration: 4, State: retryIntentAdmitted}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "operation-claim", IntentID: intent.ID, RunID: run.ID,
		GroupGeneration: 4, Kind: retryTaskOperationKind,
		State: retryOperationRequested,
	}))

	claimed, token, err := store.ClaimDueRetry(ctx, time.Now().UTC(), time.Minute)
	require.NoError(t, err)
	require.Equal(t, run.ID, claimed.ID)
	require.NotEmpty(t, token)
	_, _, err = store.ClaimDueRetry(ctx, time.Now().UTC(), time.Minute)
	require.ErrorIs(t, err, ErrNoDueRetry)

	require.NoError(t, store.CancelRetryGroup(ctx, group.ID, 4))
	require.ErrorIs(t, store.ReleaseRetryClaim(ctx, run.ID, token, 4), ErrRetryGenerationMismatch)

	var state string
	require.NoError(t, store.db.Get(&state, `SELECT retry_state FROM automation_runs WHERE id = ?`, run.ID))
	require.Equal(t, string(RetryStateCancelled), state)
}
func TestRecoverRetryLedgerKeepsAdmittedRetryClaimableAfterDue(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	automation := &Automation{
		ID:          "automation-recovery-admitted",
		WorkspaceID: "workspace-recovery-admitted",
		Name:        "recovery admitted",
		Enabled:     true,
	}
	require.NoError(t, store.CreateAutomation(ctx, automation))
	group := &RetryGroup{
		ID:           "group-recovery-admitted",
		AutomationID: automation.ID,
		Generation:   1,
		State:        RetryGroupLive,
	}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	due := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	run := &AutomationRun{
		ID:                   "run-recovery-admitted",
		AutomationID:         automation.ID,
		Status:               RunStatusScheduledRetry,
		RetryGroupID:         group.ID,
		RetryGroupGeneration: 1,
		RetryState:           RetryStateScheduled,
		RetryScheduledAt:     &due,
	}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{
		ID:              "intent-recovery-admitted",
		RunID:           run.ID,
		GroupGeneration: 1,
		State:           retryIntentAdmitted,
	}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{
		ID:              "operation-recovery-admitted",
		IntentID:        intent.ID,
		RunID:           run.ID,
		GroupGeneration: 1,
		Kind:            retryTaskOperationKind,
		State:           retryOperationRequested,
	}))

	restartedAt := due.Add(-time.Minute)
	require.NoError(t, store.RecoverRetryLedger(ctx, restartedAt))
	var state string
	require.NoError(t, store.db.Get(&state,
		`SELECT state FROM automation_run_task_intents WHERE intent_id = ?`, intent.ID))
	require.Equal(t, retryIntentAdmitted, state)
	_, _, err := store.ClaimDueRetry(ctx, restartedAt, time.Minute)
	require.ErrorIs(t, err, ErrNoDueRetry)

	claimed, _, err := store.ClaimDueRetry(ctx, due.Add(time.Second), time.Minute)
	require.NoError(t, err)
	require.Equal(t, run.ID, claimed.ID)
}

func TestRetryAdmissionPersistsImmutableIntentAndSafeEvent(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	eventBus := bus.NewMemoryEventBus(log)
	eventsSeen := make(chan *AutomationTriggeredEvent, 1)
	_, err = eventBus.Subscribe(events.AutomationTriggered, func(_ context.Context, event *bus.Event) error {
		evt, ok := event.Data.(*AutomationTriggeredEvent)
		if ok {
			eventsSeen <- evt
		}
		return nil
	})
	require.NoError(t, err)
	svc := NewService(store, eventBus, log)
	a := &Automation{WorkspaceID: "ws-admission", Name: "safe", Enabled: true,
		RetryPolicy: RetryPolicy{Mode: RetryModeFinite, MaxRetries: "1", DelaySeconds: "0"}}
	require.NoError(t, store.CreateAutomation(ctx, a))
	trigger := &AutomationTrigger{ID: "trigger", AutomationID: a.ID, Type: TriggerTypeManual, Enabled: true}
	require.NoError(t, store.CreateTrigger(ctx, trigger))
	result, err := svc.FireTrigger(ctx, a.ID, "trigger", TriggerTypeManual, []byte(`{"secret":"do-not-publish"}`), DedupKey("manual-1"))
	if err != nil {
		t.Fatalf("fire trigger: %v", err)
	}
	require.NotEmpty(t, result.RunID)
	evt := <-eventsSeen
	require.Equal(t, result.RunID, evt.RunID)
	require.Equal(t, RetryTaskExternalID(result.RunID, 1), evt.RetryExternalID)
	require.Empty(t, evt.TriggerData)
	require.Empty(t, evt.AutomationID)
	var intents, outbox int
	require.NoError(t, store.db.Get(&intents, `SELECT COUNT(*) FROM automation_run_task_intents WHERE run_id = ?`, result.RunID))
	require.NoError(t, store.db.Get(&outbox, `SELECT COUNT(*) FROM automation_retry_outbox WHERE run_id = ?`, result.RunID))
	require.Equal(t, 1, intents)
	run, err := store.GetRun(ctx, result.RunID)
	require.NoError(t, err)
	require.NotContains(t, run.RetryTriggerSnapshot, "do-not-publish")
	require.Equal(t, 1, outbox)
}
func TestRetryOperationLeasesAndCommitsExternalTask(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{ID: "automation-operation", WorkspaceID: "ws-operation",
		Name: "operation", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-operation", AutomationID: a.ID,
		Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "run-operation", AutomationID: a.ID,
		Status: RunStatusTriggered, RetryGroupID: group.ID,
		RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{ID: "intent-operation", RunID: run.ID,
		GroupGeneration: 1, State: retryIntentAdmitted}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "operation-operation", IntentID: intent.ID, RunID: run.ID,
		GroupGeneration: 1, Kind: "create_task", State: retryOperationRequested,
	}))

	leased, err := store.BeginRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, retryOperationLeased, leased.State)
	require.NotEmpty(t, leased.LeaseToken)
	require.NoError(t, store.CommitRetryTaskOperation(ctx, run.ID, 1,
		leased.LeaseToken, "task-recovered"))

	require.ErrorIs(t, store.CommitRetryTaskOperation(ctx, run.ID, 1, leased.LeaseToken, "stale-task"), ErrRetryGenerationMismatch)
	committed, err := store.GetRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, retryOperationCommitted, committed.State)
	require.Equal(t, "task-recovered", committed.ExternalTaskID)
}

func TestRetryContinuationOperationPersistsAcceptedTurnIdentity(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{ID: "automation-continuation-operation", WorkspaceID: "ws-continuation-operation",
		Name: "continuation operation", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	require.NoError(t, store.CreateRetryGroup(ctx, &RetryGroup{
		ID: "group-continuation-operation", AutomationID: a.ID,
		Generation: 1, State: RetryGroupLive,
	}))
	run := &AutomationRun{ID: "run-continuation-operation", AutomationID: a.ID,
		Status: RunStatusTriggered, RetryGroupID: "group-continuation-operation",
		RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{ID: "intent-continuation-operation", RunID: run.ID,
		GroupGeneration: 1, State: retryIntentAdmitted}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "operation-continuation-operation", IntentID: intent.ID, RunID: run.ID,
		GroupGeneration: 1, Kind: retryTaskOperationKind, State: retryOperationRequested,
	}))

	leased, err := store.BeginRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	require.NoError(t, store.CommitRetryContinuationOperation(ctx, run.ID, 1,
		leased.LeaseToken, RunDispatch{
			TaskID: "continuation-task", SessionID: "continuation-session", TurnID: "continuation-turn",
		}))

	committed, err := store.GetRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, retryOperationCommitted, committed.State)
	require.Equal(t, "continuation-task", committed.ExternalTaskID)
	require.Equal(t, "continuation-session", committed.ExternalSessionID)
	require.Equal(t, "continuation-turn", committed.ExternalTurnID)
}
func TestBeginRetryTaskOperationTreatsUnexpiredLeaseAsBusy(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{ID: "automation-operation-busy", WorkspaceID: "ws-operation-busy",
		Name: "operation busy", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-operation-busy", AutomationID: a.ID,
		Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "run-operation-busy", AutomationID: a.ID,
		Status: RunStatusTriggered, RetryGroupID: group.ID,
		RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{ID: "intent-operation-busy", RunID: run.ID,
		GroupGeneration: 1, State: retryIntentAdmitted}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "operation-operation-busy", IntentID: intent.ID, RunID: run.ID,
		GroupGeneration: 1, Kind: retryTaskOperationKind, State: retryOperationRequested,
	}))

	first, err := store.BeginRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	second, err := store.BeginRetryTaskOperation(ctx, run.ID, 1)
	require.Nil(t, second)
	require.ErrorIs(t, err, ErrRetryOperationUndispatchable)

	expired := time.Now().UTC().Add(-time.Minute)
	_, err = store.db.Exec(`UPDATE automation_run_operations SET lease_expires_at = ? WHERE operation_id = ?`,
		expired, first.ID)
	require.NoError(t, err)
	reacquired, err := store.BeginRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	require.NotEqual(t, first.LeaseToken, reacquired.LeaseToken)
}

func TestBeginRetryTaskOperationRejectsCancelledGeneration(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{ID: "automation-cancelled-operation", WorkspaceID: "ws-cancelled-operation",
		Name: "cancelled operation", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-cancelled-operation", AutomationID: a.ID,
		Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "run-cancelled-operation", AutomationID: a.ID,
		Status: RunStatusTriggered, RetryGroupID: group.ID,
		RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{ID: "intent-cancelled-operation", RunID: run.ID,
		GroupGeneration: 1, State: retryIntentAdmitted}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "operation-cancelled-operation", IntentID: intent.ID, RunID: run.ID,
		GroupGeneration: 1, Kind: retryTaskOperationKind,
		State: retryOperationRequested,
	}))
	require.NoError(t, store.CreateRetryOutbox(ctx, &RetryOutbox{
		EventID: "run-cancelled-operation:1", RunID: run.ID,
		SnapshotVersion: 1, State: "pending",
	}))
	require.NoError(t, store.CancelRetryGroup(ctx, group.ID, 1))
	var operationState, outboxState string
	require.NoError(t, store.db.Get(&operationState,
		`SELECT state FROM automation_run_operations WHERE operation_id = ?`,
		"operation-cancelled-operation"))
	require.NoError(t, store.db.Get(&outboxState,
		`SELECT state FROM automation_retry_outbox WHERE event_id = ?`,
		"run-cancelled-operation:1"))
	require.Equal(t, retryOperationAbandoned, operationState)
	require.Equal(t, "revoked", outboxState)
	var intentState string
	require.NoError(t, store.db.Get(&intentState,
		`SELECT state FROM automation_run_task_intents WHERE intent_id = ?`,
		"intent-cancelled-operation"))
	require.Equal(t, retryIntentAbandoned, intentState)

	_, err := store.BeginRetryTaskOperation(ctx, run.ID, 1)
	require.ErrorIs(t, err, ErrRetryGenerationMismatch)
}
func TestListAutomationTaskIDsIncludesCommittedRetryTask(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{ID: "automation-cleanup-retry", WorkspaceID: "ws-cleanup-retry",
		Name: "cleanup retry", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-cleanup-retry", AutomationID: a.ID,
		Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "run-cleanup-retry", AutomationID: a.ID,
		Status: RunStatusTriggered, RetryGroupID: group.ID,
		RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{ID: "intent-cleanup-retry", RunID: run.ID,
		GroupGeneration: 1, State: retryIntentAdmitted}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "operation-cleanup-retry", IntentID: intent.ID, RunID: run.ID,
		GroupGeneration: 1, Kind: retryTaskOperationKind,
		State: retryOperationCommitted, ExternalTaskID: "task-committed-before-bind",
	}))

	taskIDs, err := store.ListAutomationTaskIDs(ctx, a.ID)
	require.NoError(t, err)
	require.Contains(t, taskIDs, "task-committed-before-bind")
}

func TestClaimDueRetryTerminalizesMissingOperation(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{ID: "automation-poison-retry", WorkspaceID: "ws-poison-retry",
		Name: "poison retry", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-poison-retry", AutomationID: a.ID,
		Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	due := time.Now().UTC().Add(-time.Second)
	run := &AutomationRun{ID: "run-poison-retry", AutomationID: a.ID,
		Status: RunStatusScheduledRetry, RetryState: RetryStateScheduled,
		RetryGroupID: group.ID, RetryGroupGeneration: 1,
		RetryScheduledAt: &due}
	require.NoError(t, store.CreateRun(ctx, run))

	_, _, err := store.ClaimDueRetry(ctx, time.Now().UTC(), time.Minute)
	require.ErrorIs(t, err, ErrNoDueRetry)
	stored, err := store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, RunStatusRetrySchedulingFailed, stored.Status)
	require.Equal(t, RetryStateSchedulingFailed, stored.RetryState)
}

func TestRetryTaskExternalIDIsStableAcrossRecovery(t *testing.T) {
	require.Equal(t, RetryTaskExternalID("run-stable", 7),
		RetryTaskExternalID("run-stable", 7))
	require.NotEqual(t, RetryTaskExternalID("run-stable", 7),
		RetryTaskExternalID("run-stable", 8))
}

func TestCancelRetryGroupsByAutomationPreservesOrdinaryRunHistory(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{ID: "automation-cancel-ordinary", WorkspaceID: "ws-cancel-ordinary", Name: "ordinary", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	run := &AutomationRun{
		ID: "ordinary-run", AutomationID: a.ID, Status: RunStatusTaskCreated,
		RetryState: RetryStateNone,
	}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{
		ID: "ordinary-intent", RunID: run.ID, State: retryIntentAdmitted,
	}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "ordinary-operation", IntentID: intent.ID, RunID: run.ID,
		Kind: retryTaskOperationKind, State: retryOperationRequested,
	}))
	require.NoError(t, store.CreateRetryOutbox(ctx, &RetryOutbox{
		EventID: "ordinary-outbox", RunID: run.ID, SnapshotVersion: 1,
		State: retryOutboxPending,
	}))

	require.NoError(t, store.CancelRetryGroupsByAutomation(ctx, a.ID))

	reloadedRun, err := store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, RunStatusTaskCreated, reloadedRun.Status)
	require.Equal(t, RetryStateNone, reloadedRun.RetryState)
	var intentState, operationState, outboxState string
	require.NoError(t, store.db.Get(&intentState,
		`SELECT state FROM automation_run_task_intents WHERE intent_id = ?`, intent.ID))
	require.NoError(t, store.db.Get(&operationState,
		`SELECT state FROM automation_run_operations WHERE operation_id = ?`, "ordinary-operation"))
	require.NoError(t, store.db.Get(&outboxState,
		`SELECT state FROM automation_retry_outbox WHERE event_id = ?`, "ordinary-outbox"))
	require.Equal(t, retryIntentAdmitted, intentState)
	require.Equal(t, retryOperationRequested, operationState)
	require.Equal(t, retryOutboxPending, outboxState)
}
func TestReplayThenReconcilePreservesReplayableUnboundRetry(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	eventBus := bus.NewMemoryEventBus(log)
	eventsSeen := make(chan *AutomationTriggeredEvent, 1)
	_, err = eventBus.Subscribe(events.AutomationTriggered, func(_ context.Context, event *bus.Event) error {
		if evt, ok := event.Data.(*AutomationTriggeredEvent); ok {
			eventsSeen <- evt
		}
		return nil
	})
	require.NoError(t, err)
	svc := NewService(store, eventBus, log)
	automation := &Automation{ID: "replay-order-automation", WorkspaceID: "replay-order-workspace",
		Name: "replay order", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, automation))
	group := &RetryGroup{ID: "replay-order-group", AutomationID: automation.ID,
		Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "replay-order-run", AutomationID: automation.ID,
		TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
		RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{ID: "replay-order-intent", RunID: run.ID,
		GroupGeneration: 1, State: retryIntentAdmitted}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "replay-order-operation", IntentID: intent.ID, RunID: run.ID,
		GroupGeneration: 1, Kind: retryTaskOperationKind, State: retryOperationRequested,
	}))
	require.NoError(t, store.CreateRetryOutbox(ctx, &RetryOutbox{
		EventID: "replay-order-event", RunID: run.ID, SnapshotVersion: 1,
		State: retryOutboxPending,
	}))

	require.NoError(t, svc.ReplayPendingRetryEvents(ctx))
	select {
	case event := <-eventsSeen:
		require.Equal(t, run.ID, event.RunID)
	default:
		t.Fatal("expected replayable retry event")
	}
	require.NoError(t, svc.ReconcileOpenRuns(ctx))

	stored, err := store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, RunStatusTriggered, stored.Status)
}
