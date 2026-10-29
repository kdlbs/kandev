package changes

import (
	"context"
	"errors"
	"testing"
	"time"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestCoordinatorPersistsPolicyAndCapturesBeforeDispatch(t *testing.T) {
	ctx := context.Background()
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, Revision: 8,
		ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	client := &coordinatorCheckpointClient{}
	coordinator := NewCoordinator(store, coordinatorTurnReader{}, policy, NewContentService(store, nil), func() time.Time {
		return time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	})
	admission := coordinatorAdmission()

	require.NoError(t, coordinator.Admit(ctx, admission, client))
	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	require.True(t, changeSet.CaptureEnabled)
	require.Equal(t, "user-initiator", changeSet.SettingsUserID)
	require.EqualValues(t, 8, changeSet.SettingsRevision)
	require.EqualValues(t, 9, changeSet.RouteGeneration)
	require.Equal(t, "profile-routed", changeSet.ExecutionProfileID)
	require.Equal(t, "user-initiator", policy.userID)
	require.Equal(t, 1, client.scopeCalls)
	require.Equal(t, 1, client.startCalls)
	require.Equal(t, turnchanges.CheckpointStart, client.checkpointRequests[0].Boundary)
	require.True(t, changeSet.StartAccepted)

	require.NoError(t, coordinator.Finish(ctx, Terminal{Admission: admission, At: time.Now(), Outcome: "end_turn"}, client))
	changeSet = store.changeSets[changeSet.ID]
	require.NotNil(t, changeSet.TerminalAt)
	require.Equal(t, "end_turn", changeSet.TerminalOutcome)
	require.EqualValues(t, 1, changeSet.FileCount)
	require.EqualValues(t, 2, *changeSet.AddedLines)
	require.EqualValues(t, 1, *changeSet.DeletedLines)
	require.True(t, changeSet.SummaryComplete)
	require.True(t, changeSet.ContentComplete)
	require.True(t, changeSet.Complete)
	require.Equal(t, 1, client.endCalls)
	require.Equal(t, 1, client.compareCalls)
	require.Equal(t, 1, client.exportCalls)

	require.NoError(t, coordinator.Finish(ctx, Terminal{Admission: admission, At: time.Now()}, client))
	require.Equal(t, 1, client.endCalls, "a duplicate terminal event must reuse the accepted result")
}

func TestCoordinatorAcceptsTerminalEndpointsBeforeSummaryProcessing(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	client := &coordinatorCheckpointClient{}
	coordinator := NewCoordinator(store, coordinatorTurnReader{}, policy, NewContentService(store, nil), nil)
	admission := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	terminal := Terminal{Admission: admission, At: time.Now().UTC(), Outcome: "end_turn", FinalAssistantMessageID: "reply-final"}

	require.NoError(t, coordinator.CaptureTerminalEndpoints(context.Background(), terminal, client))
	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	require.Nil(t, changeSet.TerminalAt, "comparison and export have not finalized the summary yet")
	require.Equal(t, "reply-final", changeSet.TerminalCaptureFinalMessageID)
	require.Equal(t, "commit-end", store.repositoryRows[changeSet.ID][0].EndCommitOID)
	require.Zero(t, client.compareCalls)
	require.Zero(t, client.exportCalls)

	require.NoError(t, coordinator.ProcessTerminal(context.Background(), terminal, client))
	changeSet = store.changeSets[changeSet.ID]
	require.NotNil(t, changeSet.TerminalAt)
	require.True(t, changeSet.Complete)
	require.Equal(t, 1, client.compareCalls)
	require.Equal(t, 1, client.exportCalls)
}

func TestCoordinatorFinalizesUnavailableWhenTerminalAgentctlClientIsMissing(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	startClient := &coordinatorCheckpointClient{}
	coordinator := NewCoordinator(store, coordinatorTurnReader{}, policy, NewContentService(store, nil), nil)
	admission := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), admission, startClient))

	var missingAgentctl *agentctlclient.Client
	err := coordinator.Finish(context.Background(), Terminal{
		Admission: admission, At: time.Now().UTC(), Outcome: "dispatch_error",
	}, missingAgentctl)

	require.NoError(t, err)
	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	require.NotNil(t, changeSet.TerminalAt)
	require.Equal(t, models.TurnChangeAvailabilityUnavailable, changeSet.Availability)
	require.Equal(t, models.TurnChangeReasonCheckoutUnavailable, changeSet.Reason)
	require.False(t, changeSet.Complete)
	require.Len(t, store.repositoryRows[changeSet.ID], 1)
	repositoryChange := store.repositoryRows[changeSet.ID][0]
	require.Equal(t, models.TurnChangeAvailabilityUnavailable, repositoryChange.Availability)
	require.Equal(t, models.TurnChangeReasonCheckoutUnavailable, repositoryChange.Reason)
	require.Empty(t, repositoryChange.EndCommitOID)
	require.Zero(t, startClient.endCalls)
	require.Zero(t, startClient.compareCalls)
}

func TestLoadUnfinishedRepositoryRowsUsesOneBatchQuery(t *testing.T) {
	store := newCoordinatorStore()
	first := &models.TurnChangeSet{ID: "set-first"}
	second := &models.TurnChangeSet{ID: "set-second"}
	store.repositoryRows[first.ID] = []*models.TurnRepositoryChangeSet{{ID: "row-first", TurnChangeSetID: first.ID}}
	store.repositoryRows[second.ID] = []*models.TurnRepositoryChangeSet{{ID: "row-second", TurnChangeSetID: second.ID}}

	rows, err := NewCoordinator(store, nil, nil, nil, nil).loadUnfinishedRepositoryRows(context.Background(), []*models.TurnChangeSet{first, nil, second, first})

	require.NoError(t, err)
	require.Equal(t, 1, store.batchListCalls)
	require.Zero(t, store.singleListCalls)
	require.Equal(t, "row-first", rows[first.ID][0].ID)
	require.Equal(t, "row-second", rows[second.ID][0].ID)
}

func TestTerminalOverlapClosureFailureIsLogged(t *testing.T) {
	store := newCoordinatorStore()
	changeSet := &models.TurnChangeSet{ID: "set-overlap-log", TaskID: "task", TaskSessionID: "session"}
	startedAt := time.Now().Add(-time.Minute)
	store.changeSets[changeSet.ID] = changeSet
	store.repositoryRows[changeSet.ID] = []*models.TurnRepositoryChangeSet{{
		ID: "row-overlap-log", TurnChangeSetID: changeSet.ID, CheckoutID: "checkout-1", StartCapturedAt: &startedAt,
	}}
	store.writerIntervalErr = errors.New("writer interval query failed")
	core, logs := observer.New(zap.WarnLevel)
	coordinator := NewCoordinator(store, nil, nil, nil, nil)
	coordinator.SetLogger(zap.New(core))

	_, err := coordinator.loadTerminalRepositoryRows(context.Background(), changeSet, Terminal{}, &coordinatorCheckpointClient{})

	require.NoError(t, err)
	require.Equal(t, 1, logs.Len())
	require.Equal(t, "failed to reconcile turn-change overlap intervals", logs.All()[0].Message)
	require.Equal(t, changeSet.ID, logs.All()[0].ContextMap()["change_set_id"])
}

func TestCoordinatorDisabledPolicyDoesNotCallExecutorAndStillFinalizes(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "default", Enabled: false, ResolutionKind: turnchanges.PolicyDefaultUser,
	}}
	client := &coordinatorCheckpointClient{}
	coordinator := NewCoordinator(store, coordinatorTurnReader{}, policy, NewContentService(store, nil), nil)
	admission := coordinatorAdmission()

	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	require.False(t, changeSet.CaptureEnabled)
	require.Equal(t, models.TurnChangeReasonCaptureDisabled, changeSet.Reason)
	require.Zero(t, client.scopeCalls)
	require.Zero(t, client.startCalls)
	require.Zero(t, client.endCalls)
	require.Zero(t, client.compareCalls)
	require.Zero(t, client.exportCalls)

	require.NoError(t, coordinator.Finish(context.Background(), Terminal{Admission: admission, At: time.Now()}, client))
	require.NotNil(t, store.changeSets[changeSet.ID].TerminalAt)
	require.Zero(t, client.endCalls)
}

func TestCoordinatorPolicyReadFailureDisablesCaptureWithoutBlockingPrompt(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{err: errors.New("settings unavailable")}
	client := &coordinatorCheckpointClient{}
	coordinator := NewCoordinator(store, coordinatorTurnReader{}, policy, nil, nil)
	admission := coordinatorAdmission()

	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	require.False(t, changeSet.CaptureEnabled)
	require.Equal(t, turnchanges.PolicyReadFailed, changeSet.ResolutionKind)
	require.Equal(t, models.TurnChangeReasonPolicyReadFailed, changeSet.Reason)
	require.Zero(t, client.scopeCalls)
	require.Zero(t, client.startCalls)
}

func TestCoordinatorPersistsResolvedRootScopeForSingleCheckout(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	client := &coordinatorCheckpointClient{scopes: []string{""}}
	coordinator := NewCoordinator(store, coordinatorTurnReader{}, policy, NewContentService(store, nil), nil)
	admission := coordinatorAdmission()
	admission.Checkouts[0].RepositorySubpath = "E2E-Repo"

	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	rows := store.repositoryRows[turnChangeSetID(admission.TurnID)]
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].RepositorySubpath)
	require.Empty(t, client.checkpointRequests[0].Repo)

	require.NoError(t, coordinator.Finish(context.Background(), Terminal{Admission: admission, At: time.Now()}, client))
	require.Equal(t, turnchanges.CheckpointStart, client.checkpointRequests[0].Boundary)
	require.Equal(t, turnchanges.CheckpointEnd, client.checkpointRequests[1].Boundary)
	require.Empty(t, client.checkpointRequests[1].Repo)
}

func TestCoordinatorKeepsBaselineForValidSameTurnContinuation(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	client := &coordinatorCheckpointClient{}
	coordinator := NewCoordinator(store, coordinatorTurnReader{}, policy, NewContentService(store, nil), nil)
	admission := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	continuation := admission
	continuation.PromptGeneration++
	require.NoError(t, coordinator.Admit(context.Background(), continuation, client))
	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	require.EqualValues(t, continuation.PromptGeneration, changeSet.PromptGeneration)
	require.Equal(t, int64(1), policy.calls, "same-turn continuation must reuse the resolved policy")
	require.Equal(t, 1, client.startCalls, "same-turn continuation must retain its original start endpoint")
	require.NoError(t, coordinator.Finish(context.Background(), Terminal{Admission: continuation, At: time.Now()}, client))
	require.Equal(t, 1, client.endCalls)
}

func TestCoordinatorSettlesCaptureDeadlineAndPersistenceFailuresBeforeSuccessorAdmission(t *testing.T) {
	for _, stage := range []string{"end_capture", "comparison", "export", "persistence"} {
		t.Run(stage, func(t *testing.T) {
			turns := coordinatorTurnReaderMap{
				"turn-1": {ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1"},
				"turn-2": {ID: "turn-2", TaskID: "task-1", TaskSessionID: "session-1"},
			}
			store := newCoordinatorStore()
			policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
				SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
			}}
			client := &coordinatorCheckpointClient{}
			switch stage {
			case "end_capture":
				client.waitCaptureBoundary = turnchanges.CheckpointEnd
			case "comparison":
				client.waitCompare = true
			case "export":
				client.waitExport = true
			case "persistence":
				store.finalizeFailuresRemaining = 1
			}
			coordinator := NewCoordinator(store, turns, policy, NewContentService(store, nil), nil)
			admission := coordinatorAdmission()
			require.NoError(t, coordinator.Admit(context.Background(), admission, client))

			terminalCtx := context.Background()
			cancel := func() {}
			if stage != "persistence" {
				terminalCtx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
			}
			err := coordinator.Finish(terminalCtx, Terminal{Admission: admission, At: time.Now().UTC(), Outcome: "end_turn", FinalAssistantMessageID: "reply-1"}, client)
			cancel()
			require.NoError(t, err)
			changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
			require.NotNil(t, changeSet.TerminalAt, "bounded failure must not strand the terminal row")
			require.Equal(t, "reply-1", changeSet.FinalAssistantMessageID)
			require.NotEqual(t, models.TurnChangeAvailabilityPending, changeSet.Availability)
			if stage == "comparison" || stage == "export" || stage == "persistence" {
				require.Equal(t, "commit-end", store.repositoryRows[changeSet.ID][0].EndCommitOID,
					"accepted end endpoint must survive comparison/export/persistence failure")
			}

			successor := admission
			successor.TurnID = "turn-2"
			successor.PromptGeneration++
			require.NoError(t, coordinator.Admit(context.Background(), successor, client), "settled terminal must admit the next turn")
		})
	}
}

func TestCoordinatorRestartReconciliationDoesNotCaptureAnEndEndpoint(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	client := &coordinatorCheckpointClient{}
	completedAt := time.Now().UTC().Add(-time.Minute)
	turns := coordinatorTurnReaderMap{"turn-1": {
		ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1", CompletedAt: &completedAt,
	}}
	coordinator := NewCoordinator(store, turns, policy, NewContentService(store, nil), nil)
	admission := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	require.NoError(t, coordinator.ReconcileUnfinished(context.Background()))
	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	require.NotNil(t, changeSet.TerminalAt)
	require.Equal(t, models.TurnChangeAvailabilityUnavailable, changeSet.Availability)
	require.Equal(t, "restart_unavailable", changeSet.TerminalOutcome)
	require.Zero(t, client.endCalls, "restart recovery must not invent an end snapshot")
}

func TestCoordinatorRetriesOwnedCheckpointCleanupOnExecutorReconnect(t *testing.T) {
	turns := coordinatorTurnReaderMap{
		"turn-1": {ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1"},
		"turn-2": {ID: "turn-2", TaskID: "task-1", TaskSessionID: "session-1"},
	}
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	client := &coordinatorCheckpointClient{deleteErr: errors.New("executor unavailable")}
	coordinator := NewCoordinator(store, turns, policy, NewContentService(store, nil), nil)
	first := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), first, client))
	require.NoError(t, coordinator.Finish(context.Background(), Terminal{Admission: first, At: time.Now().UTC()}, client))
	firstSetID := turnChangeSetID(first.TurnID)
	require.True(t, store.repositoryRows[firstSetID][0].CleanupPending)

	client.deleteErr = nil
	second := first
	second.TurnID = "turn-2"
	second.PromptGeneration++
	require.NoError(t, coordinator.Admit(context.Background(), second, client))
	require.False(t, store.repositoryRows[firstSetID][0].CleanupPending)
	require.GreaterOrEqual(t, len(client.deleteRequests), 3, "reconnect should retry the durable cleanup intent")
}

func TestCoordinatorRecordsOnlyOverlappingActualCheckoutAliases(t *testing.T) {
	for _, test := range []struct {
		name          string
		worktreeID    string
		environmentID string
		overlap       bool
	}{
		{name: "worktree alias", worktreeID: "worktree-shared", environmentID: "env-repo-alias", overlap: true},
		{name: "environment repository alias", environmentID: "env-repo-1", overlap: true},
		{name: "distinct worktrees in one repository", worktreeID: "worktree-distinct", environmentID: "env-repo-1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			turns := coordinatorTurnReaderMap{
				"turn-1": {ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1"},
				"turn-2": {ID: "turn-2", TaskID: "task-1", TaskSessionID: "session-2"},
			}
			store := newCoordinatorStore()
			policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
				SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
			}}
			client := &coordinatorCheckpointClient{}
			coordinator := NewCoordinator(store, turns, policy, NewContentService(store, nil), nil)
			first := coordinatorAdmission()
			first.Checkouts[0].WorktreeID = "worktree-shared"
			require.NoError(t, coordinator.Admit(context.Background(), first, client))
			second := coordinatorAdmission()
			second.TurnID = "turn-2"
			second.SessionID = "session-2"
			second.ExecutionID = "execution-2"
			second.StartupAttemptID = "startup-2"
			second.TaskEnvironmentID = "env-2"
			second.Checkouts[0].ID = "checkout-alias"
			second.Checkouts[0].EnvironmentRepoID = test.environmentID
			second.Checkouts[0].WorktreeID = test.worktreeID
			require.NoError(t, coordinator.Admit(context.Background(), second, client))

			firstSet := store.changeSets[turnChangeSetID(first.TurnID)]
			secondSet := store.changeSets[turnChangeSetID(second.TurnID)]
			if test.overlap {
				require.Len(t, firstSet.OverlapIntervals, 1)
				require.Len(t, secondSet.OverlapIntervals, 1)
				require.Equal(t, secondSet.ID, firstSet.OverlapIntervals[0].ChangeSetID)
				require.Equal(t, firstSet.ID, secondSet.OverlapIntervals[0].ChangeSetID)
				require.NoError(t, coordinator.Finish(context.Background(), Terminal{Admission: second, At: time.Now().UTC()}, client))
				firstSet = store.changeSets[firstSet.ID]
				secondSet = store.changeSets[secondSet.ID]
				require.NotNil(t, firstSet.OverlapIntervals[0].EndedAt)
				require.NotNil(t, secondSet.OverlapIntervals[0].EndedAt)
			} else {
				require.Empty(t, firstSet.OverlapIntervals)
				require.Empty(t, secondSet.OverlapIntervals)
			}
		})
	}
}

func TestCoordinatorAdmitsAutomatedSyntheticTurnAndKeepsExplicitFalsePolicy(t *testing.T) {
	turn := &models.Turn{
		ID: "automated-turn", TaskID: "task-1", TaskSessionID: "session-1",
		Metadata: map[string]interface{}{
			"task_launch_scope":                        "automation",
			models.TurnMetaKeyTurnChangeSyntheticActor: true,
		},
	}
	reader := coordinatorTurnReaderMap{"automated-turn": turn}
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "default-user", Enabled: true, ResolutionKind: turnchanges.PolicyDefaultUser,
	}}
	client := &coordinatorCheckpointClient{}
	coordinator := NewCoordinator(store, reader, policy, NewContentService(store, nil), nil)
	automated := coordinatorAdmission()
	automated.TurnID = "automated-turn"
	require.NoError(t, coordinator.Admit(context.Background(), automated, client))
	require.Equal(t, 1, client.startCalls)
	require.Equal(t, "default-user", store.changeSets[turnChangeSetID(automated.TurnID)].SettingsUserID)

	storeFalse := newCoordinatorStore()
	falsePolicy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "default-user", Enabled: false, ResolutionKind: turnchanges.PolicyDefaultUser,
	}}
	falseCoordinator := NewCoordinator(storeFalse, reader, falsePolicy, nil, nil)
	client = &coordinatorCheckpointClient{}
	require.NoError(t, falseCoordinator.Admit(context.Background(), automated, client))
	require.Zero(t, client.startCalls)
	require.Equal(t, models.TurnChangeReasonCaptureDisabled, storeFalse.changeSets[turnChangeSetID(automated.TurnID)].Reason)
}

func TestCoordinatorRejectsTerminalFromMismatchedOwnership(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Admission)
		admit  bool
	}{
		{name: "runtime execution", admit: true, mutate: func(admission *Admission) { admission.ExecutionID = "replacement-execution" }},
		{name: "startup attempt", admit: true, mutate: func(admission *Admission) { admission.StartupAttemptID = "replacement-startup" }},
		{name: "task environment", admit: true, mutate: func(admission *Admission) { admission.TaskEnvironmentID = "replacement-environment" }},
		{name: "prompt generation", admit: false, mutate: func(admission *Admission) { admission.PromptGeneration++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newCoordinatorStore()
			policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
				SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
			}}
			client := &coordinatorCheckpointClient{}
			coordinator := NewCoordinator(store, coordinatorTurnReader{}, policy, NewContentService(store, nil), nil)
			admission := coordinatorAdmission()
			require.NoError(t, coordinator.Admit(context.Background(), admission, client))
			mismatched := admission
			test.mutate(&mismatched)
			if test.admit {
				require.NoError(t, coordinator.Admit(context.Background(), mismatched, client))
			} else {
				require.NoError(t, coordinator.Finish(context.Background(), Terminal{Admission: mismatched, At: time.Now()}, client))
			}
			changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
			require.NotNil(t, changeSet.TerminalAt)
			require.Equal(t, models.TurnChangeAvailabilityUnavailable, changeSet.Availability)
			require.Equal(t, 1, client.startCalls)
			require.Zero(t, client.endCalls, "mismatched ownership must not capture an end endpoint")
			require.Equal(t, int64(1), policy.calls, "mismatched reuse must not re-resolve policy")
		})
	}
}

func TestCoordinatorKeepsComparisonFileSummariesWhenExportFails(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	added, deleted := int64(4), int64(2)
	client := &coordinatorCheckpointClient{
		comparisonFiles: []turnchanges.CheckpointFile{{
			PathBytes: []byte("src/export-failed.go"), Kind: "modified", Added: &added, Deleted: &deleted,
		}}, exportErr: errors.New("executor transport lost"),
	}
	coordinator := NewCoordinator(store, coordinatorTurnReader{}, policy, NewContentService(store, nil), nil)
	admission := coordinatorAdmission()
	require.NoError(t, coordinator.Admit(context.Background(), admission, client))
	require.NoError(t, coordinator.Finish(context.Background(), Terminal{Admission: admission, At: time.Now()}, client))
	changeSet := store.changeSets[turnChangeSetID(admission.TurnID)]
	require.EqualValues(t, 1, changeSet.FileCount)
	require.False(t, changeSet.ContentComplete)
	require.Len(t, store.files, 1)
	file := store.files[0].File
	require.Equal(t, "src/export-failed.go", file.Path)
	require.Equal(t, "modified", file.Kind)
	require.EqualValues(t, 4, *file.AddedLines)
	require.EqualValues(t, 2, *file.DeletedLines)
	require.Equal(t, models.TurnChangeAvailabilityUnavailable, file.ContentAvailability)
}

func TestCoordinatorRejectsUnmatchedMultiCheckoutScope(t *testing.T) {
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser}}
	client := &coordinatorCheckpointClient{scopes: []string{"frontend"}}
	coordinator := NewCoordinator(store, coordinatorTurnReader{}, policy, nil, nil)
	admission := coordinatorAdmission()
	admission.Checkouts = append(admission.Checkouts, Checkout{
		ID: "checkout-two", EnvironmentRepoID: "env-repo-two", TaskRepositoryID: "task-repo-two",
		RepositoryID: "repo-two", RepositorySubpath: "backend", RepositorySubpathKnown: true,
	})

	err := coordinator.Admit(context.Background(), admission, client)
	require.ErrorContains(t, err, "checkout \"checkout-two\" is absent from registered repository scopes")
	require.Equal(t, 1, client.startCalls)
	require.Equal(t, turnchanges.CheckpointStart, client.checkpointRequests[0].Boundary)
	rows := store.repositoryRows[turnChangeSetID(admission.TurnID)]
	require.Len(t, rows, 2)
	require.Equal(t, models.TurnChangeAvailabilityUnavailable, rows[1].Availability)
	require.Equal(t, models.TurnChangeReasonCheckoutUnavailable, rows[1].Reason)
}
