package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestCeilingReplaySynchronousReconciliationCallback(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "ceiling-replay-lock-order-task"
		sessionID = "ceiling-replay-lock-order-session"
		stepID    = "ceiling-replay-lock-order-step"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, sessionID, taskID, "ceiling-replay-lock-order-execution")

	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	task.WorkflowStepID = stepID
	require.NoError(t, repo.UpdateTask(ctx, task))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "profile-1"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf1", Name: "Replay step", Prompt: "continue",
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)

	var svc *Service
	promptEntered := make(chan struct{})
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptAgentFunc: func(_ context.Context, _ string, _ string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
			close(promptEntered)
			// A synchronous provider callback is allowed to reconcile the task
			// while replay is dispatching. This is the callback cycle that must
			// not inherit replay's task-admission ownership.
			svc.writeTaskReviewState(context.Background(), taskID, sessionID)
			return &executor.PromptResult{}, nil
		},
	}
	svc = createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
	svc.sessionCeiling = newSessionCeilingController(unlimitedSessionCeiling, nil, nil)

	entryIdentity := svc.workflowEntryIdentity(ctx, taskID)
	route := models.WorkflowSessionRoute{
		OperationID:       "ceiling-replay-lock-order-route",
		DestinationStepID: stepID,
		EntryIdentity:     entryIdentity,
		TargetKind:        "new_session",
		DestinationID:     sessionID,
		Phase:             "committed",
	}
	require.NoError(t, repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyWorkflowSessionRoute, route))
	deferral := models.CeilingDeferral{
		Kind: models.CeilingLaunchWorkflowStepEnsure,
		Payload: map[string]interface{}{
			metaKeySessionID:      sessionID,
			metaKeyWorkflowStepID: stepID,
			models.CeilingLaunchEntryBindingKey: map[string]interface{}{
				"workflow_id":            "wf1",
				"destination_step_id":    stepID,
				"route_operation_id":     route.OperationID,
				"entry_identity":         entryIdentity,
				"destination_session_id": sessionID,
			},
		},
		Origin:     string(launchOriginAutomatic),
		ReasonCode: ceilingReasonRefused,
		QueuedAt:   time.Now().UTC(),
	}

	done := make(chan ceilingReplayOutcome, 1)
	go func() {
		done <- svc.replayCeilingDeferral(ctx, &models.Task{ID: taskID}, deferral)
	}()

	select {
	case outcome := <-done:
		require.Equal(t, ceilingReplaySucceeded, outcome)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ceiling replay blocked while a synchronous dispatch callback reconciled task state")
	}

	select {
	case <-promptEntered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ceiling replay did not reach the provider dispatch boundary")
	}
}

func TestCeilingReplayRevalidatesAfterRuntimePreparation(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "ceiling-replay-preparation-task"
		sessionID = "ceiling-replay-preparation-session"
		stepID    = "ceiling-replay-preparation-step"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, sessionID, taskID, "ceiling-replay-preparation-execution")
	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	task.WorkflowStepID = stepID
	require.NoError(t, repo.UpdateTask(ctx, task))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "profile-1"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	step := &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf1", Name: "Replay step", Prompt: "continue",
	}
	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = step
	stepGetter.steps[stepID+"-successor"] = &wfmodels.WorkflowStep{
		ID: stepID + "-successor", WorkflowID: "wf1", Name: "Successor", Prompt: "new",
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
	svc.sessionCeiling = newSessionCeilingController(unlimitedSessionCeiling, nil, nil)

	entryIdentity := svc.workflowEntryIdentity(ctx, taskID)
	route := models.WorkflowSessionRoute{
		OperationID:       "ceiling-replay-preparation-route",
		DestinationStepID: stepID,
		EntryIdentity:     entryIdentity,
		TargetKind:        "new_session",
		DestinationID:     sessionID,
		Phase:             "committed",
	}
	require.NoError(t, repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyWorkflowSessionRoute, route))

	var stepReads int
	stepGetter.getStepFunc = func(stepCtx context.Context, requestedStepID string) (*wfmodels.WorkflowStep, error) {
		stepReads++
		if stepReads == 2 {
			successor := route
			successor.OperationID = "ceiling-replay-preparation-successor-route"
			successor.DestinationStepID = stepID + "-successor"
			successor.EntryIdentity = "successor-entry"
			require.NoError(t, repo.SetTaskMetadataKey(stepCtx, taskID, models.MetaKeyWorkflowSessionRoute, successor))
		}
		return stepGetter.steps[requestedStepID], nil
	}
	deferral := models.CeilingDeferral{
		Kind: models.CeilingLaunchWorkflowStepEnsure,
		Payload: map[string]interface{}{
			metaKeySessionID:      sessionID,
			metaKeyWorkflowStepID: stepID,
			models.CeilingLaunchEntryBindingKey: map[string]interface{}{
				"workflow_id":            "wf1",
				"destination_step_id":    stepID,
				"route_operation_id":     route.OperationID,
				"entry_identity":         entryIdentity,
				"destination_session_id": sessionID,
			},
		},
		Origin:     string(launchOriginAutomatic),
		ReasonCode: ceilingReasonRefused,
		QueuedAt:   time.Now().UTC(),
	}

	outcome := svc.replayCeilingDeferral(ctx, &models.Task{ID: taskID}, deferral)
	require.Equal(t, ceilingReplaySuperseded, outcome)
	agentMgr.mu.Lock()
	require.Empty(t, agentMgr.capturedPrompts)
	agentMgr.mu.Unlock()
}

func TestCeilingReplayPreservesReconciliationPrecedence(t *testing.T) {
	tests := []struct {
		name            string
		configure       func(*testing.T, *sqliterepo.Repository, string, string, string)
		wantState       v1.TaskState
		wantStateWrites int
	}{
		{
			name: "working sibling preserves active task",
			configure: func(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID, _ string) {
				now := time.Now().UTC()
				require.NoError(t, repo.CreateTaskSession(context.Background(), &models.TaskSession{
					ID: "reconciliation-working-sibling", TaskID: taskID,
					State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
				}))
			},
			wantState:       v1.TaskStateInProgress,
			wantStateWrites: 0,
		},
		{
			name: "valid destination preserves scheduling",
			configure: func(t *testing.T, repo *sqliterepo.Repository, taskID, _, destinationID string) {
				deferral := models.CeilingDeferral{
					Kind:    models.CeilingLaunchStartCreated,
					Payload: map[string]interface{}{metaKeySessionID: destinationID},
					Origin:  string(launchOriginAutomatic), ReasonCode: ceilingReasonRefused,
					QueuedAt: time.Now().UTC(),
				}
				require.NoError(t, repo.SetTaskMetadataKey(context.Background(), taskID,
					models.MetaKeyDeferredLaunch, models.CeilingRecordKeys(deferral)))
			},
			wantState:       v1.TaskStateScheduling,
			wantStateWrites: 1,
		},
		{
			name: "idle sibling permits review",
			configure: func(t *testing.T, repo *sqliterepo.Repository, taskID, _, _ string) {
				now := time.Now().UTC()
				require.NoError(t, repo.CreateTaskSession(context.Background(), &models.TaskSession{
					ID: "reconciliation-idle-sibling", TaskID: taskID,
					State: models.TaskSessionStateIdle, StartedAt: now, UpdatedAt: now,
				}))
			},
			wantState:       v1.TaskStateReview,
			wantStateWrites: 1,
		},
		{
			name: "failed sibling permits review",
			configure: func(t *testing.T, repo *sqliterepo.Repository, taskID, _, _ string) {
				now := time.Now().UTC()
				require.NoError(t, repo.CreateTaskSession(context.Background(), &models.TaskSession{
					ID: "reconciliation-failed-sibling", TaskID: taskID,
					State: models.TaskSessionStateFailed, StartedAt: now, UpdatedAt: now,
				}))
			},
			wantState:       v1.TaskStateReview,
			wantStateWrites: 1,
		},
		{
			name: "terminal destination falls back to review",
			configure: func(t *testing.T, repo *sqliterepo.Repository, taskID, _, destinationID string) {
				deferral := models.CeilingDeferral{
					Kind:    models.CeilingLaunchStartCreated,
					Payload: map[string]interface{}{metaKeySessionID: destinationID},
					Origin:  string(launchOriginAutomatic), ReasonCode: ceilingReasonRefused,
					QueuedAt: time.Now().UTC(),
				}
				require.NoError(t, repo.SetTaskMetadataKey(context.Background(), taskID,
					models.MetaKeyDeferredLaunch, models.CeilingRecordKeys(deferral)))
				require.NoError(t, repo.UpdateTaskSessionState(context.Background(), destinationID,
					models.TaskSessionStateCompleted, ""))
			},
			wantState:       v1.TaskStateReview,
			wantStateWrites: 1,
		},
		{
			name: "archived task is unchanged",
			configure: func(t *testing.T, repo *sqliterepo.Repository, taskID, _, _ string) {
				require.NoError(t, repo.ArchiveTask(context.Background(), taskID))
			},
			wantState:       v1.TaskStateInProgress,
			wantStateWrites: 0,
		},
		{
			name: "office task is unchanged",
			configure: func(t *testing.T, repo *sqliterepo.Repository, taskID, _, _ string) {
				task, err := repo.GetTask(context.Background(), taskID)
				require.NoError(t, err)
				task.ProjectID = "office-project"
				require.NoError(t, repo.UpdateTask(context.Background(), task))
			},
			wantState:       v1.TaskStateInProgress,
			wantStateWrites: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			taskID := "reconciliation-" + strings.ReplaceAll(tc.name, " ", "-")
			sessionID := taskID + "-session"
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
			tc.configure(t, repo, taskID, sessionID, sessionID)

			taskRepo := newMockTaskRepo()
			seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
			svc := createTestService(repo, newMockStepGetter(), taskRepo)
			svc.sessionCeiling = newSessionCeilingController(unlimitedSessionCeiling, nil, nil)
			svc.writeTaskReviewState(ctx, taskID, sessionID)

			taskRepo.mu.Lock()
			gotState := taskRepo.updatedStates[taskID]
			writes := taskRepo.stateWrites[taskID]
			taskRepo.mu.Unlock()
			if writes == 0 {
				gotState = v1.TaskStateInProgress
			}
			require.Equal(t, tc.wantState, gotState)
			require.Equal(t, tc.wantStateWrites, writes)
		})
	}
}

func TestCeilingReplayClaimAndShutdownProgress(t *testing.T) {
	ctx := context.Background()
	svc, repo := newServiceWithRealRepo(t)
	const (
		taskID    = "ceiling-replay-claim-shutdown-task"
		sessionID = "ceiling-replay-claim-shutdown-session"
	)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: taskID, Title: "claim and shutdown", State: v1.TaskStateScheduling,
		CreatedAt: now, UpdatedAt: now,
	}))
	deferral := models.CeilingDeferral{
		Kind:    models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{metaKeySessionID: sessionID},
		Origin:  string(launchOriginAutomatic), ReasonCode: ceilingReasonRefused,
		QueuedAt: now,
	}
	_, _, err := repo.SetTaskDeferredLaunchIfUnchanged(
		ctx, taskID, sqliterepo.AbsentDeferredLaunch(), models.CeilingRecordKeys(deferral),
	)
	require.NoError(t, err)

	// Both dispatcher classes may race for one durable record, but only one
	// receives the claim. The losing path must leave the record for retry.
	type claimResult struct {
		claim *ceilingDeferredLaunchClaim
		found bool
		err   error
	}
	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make(chan claimResult, 2)
	for _, owner := range []string{ceilingClaimOwnerReplay, ceilingClaimOwnerSendNow} {
		go func(owner string) {
			ready <- struct{}{}
			<-start
			claim, found, claimErr := svc.claimCeilingDeferredLaunch(ctx, taskID, sessionID, owner)
			results <- claimResult{claim: claim, found: found, err: claimErr}
		}(owner)
	}
	<-ready
	<-ready
	close(start)
	first := <-results
	second := <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.True(t, first.found)
	require.True(t, second.found)
	require.NotEqual(t, first.claim == nil, second.claim == nil)
	ownerClaim := first.claim
	if ownerClaim == nil {
		ownerClaim = second.claim
	}
	require.NotNil(t, ownerClaim)

	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	ownerClaim.releaseIfHeld(cancelledCtx)
	claimAfterCancellation, found, err := svc.claimCeilingDeferredLaunch(
		ctx, taskID, sessionID, ceilingClaimOwnerSendNow,
	)
	require.NoError(t, err)
	require.True(t, found)
	require.NotNil(t, claimAfterCancellation, "cancelled replay cleanup must return the record to the next dispatcher")
	claimAfterCancellation.releaseIfHeld(ctx)

	// Shutdown joins a sweep callback that is already in progress. Releasing
	// the callback barrier must let stop complete without a detached worker.
	svc.ceilingSweeper = newCeilingSweeper()
	svc.ceilingSweeper.interval = time.Hour
	tickStarted := make(chan struct{})
	releaseTick := make(chan struct{})
	require.True(t, svc.ceilingSweeper.start(ctx, func(context.Context) {
		close(tickStarted)
		<-releaseTick
	}))
	svc.ceilingSweeper.signalNow()
	select {
	case <-tickStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("sweeper did not enter its barrier-controlled callback")
	}
	stopped := make(chan struct{})
	go func() {
		svc.stopCeilingSweeper()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("sweeper shutdown returned before its callback released")
	default:
	}
	close(releaseTick)
	select {
	case <-stopped:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("sweeper shutdown remained blocked after callback release")
	}
}
