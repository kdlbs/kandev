package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type ceilingDispatchFixture struct {
	svc      *Service
	repo     *sqliterepo.Repository
	agent    *mockAgentManager
	steps    *mockStepGetter
	task     *models.Task
	route    models.WorkflowSessionRoute
	binding  models.CeilingWorkflowEntryBinding
	deferral models.CeilingDeferral
}

func newCeilingDispatchFixture(t *testing.T) *ceilingDispatchFixture {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "dispatch-task", "dispatch-session", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "dispatch-session", "dispatch-task", "dispatch-execution")
	task, err := repo.GetTask(ctx, "dispatch-task")
	require.NoError(t, err)
	task.WorkflowStepID = "dispatch-step"
	require.NoError(t, repo.UpdateTask(ctx, task))
	session, err := repo.GetTaskSession(ctx, "dispatch-session")
	require.NoError(t, err)
	session.AgentProfileID = "profile-1"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	steps := newMockStepGetter()
	steps.steps[task.WorkflowStepID] = &wfmodels.WorkflowStep{
		ID: task.WorkflowStepID, WorkflowID: "wf1", Name: "Dispatch", Prompt: "continue",
	}
	tasks := newMockTaskRepo()
	seedMockTaskState(tasks, task.ID, v1.TaskStateInProgress)
	agent := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, steps, tasks, agent)
	svc.sessionCeiling = newSessionCeilingController(unlimitedSessionCeiling, nil, nil)
	route := models.WorkflowSessionRoute{
		OperationID: "dispatch-route", DestinationStepID: task.WorkflowStepID,
		EntryIdentity: svc.workflowEntryIdentity(ctx, task.ID), TargetKind: "new_session",
		DestinationID: session.ID, Phase: "committed",
	}
	require.NoError(t, repo.SetTaskMetadataKey(ctx, task.ID, models.MetaKeyWorkflowSessionRoute, route))
	binding := models.CeilingWorkflowEntryBinding{
		WorkflowID: "wf1", DestinationStepID: route.DestinationStepID,
		RouteOperationID: route.OperationID, EntryIdentity: route.EntryIdentity, DestinationSessionID: session.ID,
	}
	deferral := models.CeilingDeferral{
		Kind: models.CeilingLaunchWorkflowStepEnsure, Origin: string(launchOriginAutomatic),
		ReasonCode: ceilingReasonRefused, QueuedAt: time.Now().UTC(),
		Payload: map[string]interface{}{
			metaKeySessionID: session.ID, metaKeyWorkflowStepID: task.WorkflowStepID,
			models.CeilingLaunchEntryBindingKey: ceilingEntryBindingValue(binding),
		},
	}
	require.NoError(t, repo.SetTaskMetadataKey(ctx, task.ID, models.MetaKeyDeferredLaunch, models.CeilingRecordKeys(deferral)))
	return &ceilingDispatchFixture{svc, repo, agent, steps, task, route, binding, deferral}
}

// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.6
func TestCeilingReplayRejectsSuccessorClaimAfterPreparation(t *testing.T) {
	for _, changeRoute := range []bool{false, true} {
		t.Run(fmt.Sprintf("route_changed=%t", changeRoute), func(t *testing.T) {
			testCeilingReplaySuccessorClaim(t, changeRoute)
		})
	}
}

func testCeilingReplaySuccessorClaim(t *testing.T, changeRoute bool) {
	f := newCeilingDispatchFixture(t)
	ctx := context.Background()
	replaced := false
	f.steps.getStepFunc = func(ctx context.Context, id string) (*wfmodels.WorkflowStep, error) {
		record, _, err := f.repo.GetTaskDeferredLaunch(ctx, f.task.ID)
		require.NoError(t, err)
		if _, _, claimed := models.ReadCeilingLaunchClaim(record); claimed && !replaced {
			replaced = true
			record[models.CeilingLaunchClaimKey] = map[string]interface{}{
				"id": "successor-claim", "owner": ceilingClaimOwnerSendNow,
				models.CeilingLaunchClaimExpiresAtKey: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano),
			}
			require.NoError(t, f.repo.SetTaskMetadataKey(ctx, f.task.ID, models.MetaKeyDeferredLaunch, record))
			if changeRoute {
				successor := f.route
				successor.OperationID = "successor-route"
				require.NoError(t, f.repo.SetTaskMetadataKey(ctx, f.task.ID, models.MetaKeyWorkflowSessionRoute, successor))
			}
		}
		return f.steps.steps[id], nil
	}
	f.svc.retryOneDeferredCeilingLaunch(ctx, f.task)
	require.True(t, replaced)
	require.Empty(t, f.agent.capturedPrompts, "the replaced claim must not dispatch")
	record, _, err := f.repo.GetTaskDeferredLaunch(ctx, f.task.ID)
	require.NoError(t, err)
	id, _, exists := models.ReadCeilingLaunchClaim(record)
	require.True(t, exists)
	require.Equal(t, "successor-claim", id)
}

func TestCeilingPromptRevalidatesAfterDispatchReceipt(t *testing.T) {
	f := newCeilingDispatchFixture(t)
	ctx := withCeilingEntryKind(withCeilingEntryBinding(context.Background(), &f.binding), f.deferral.Kind)
	var afterAdmissionCalled bool
	_, err := f.svc.promptTask(ctx, f.task.ID, f.route.DestinationID, "old prompt", "", false, nil, false,
		launchOriginAutomatic, promptTaskOptions{beforeDispatch: func() error {
			successor := f.route
			successor.OperationID = "successor-route"
			return f.svc.persistWorkflowSessionRoute(context.Background(), f.task.ID, successor)
		}, afterDispatchAdmission: func() error {
			afterAdmissionCalled = true
			return nil
		}})
	require.ErrorIs(t, err, ErrCeilingLaunchSuperseded)
	require.Empty(t, f.agent.capturedPrompts)
	require.False(t, afterAdmissionCalled, "post-admission hooks must not run for a stale claim")
}

func TestCeilingModelSwitchRevalidatesAfterDispatchReceipt(t *testing.T) {
	f := newCeilingDispatchFixture(t)
	ctx := withCeilingEntryKind(withCeilingEntryBinding(context.Background(), &f.binding), f.deferral.Kind)
	session, err := f.repo.GetTaskSession(ctx, f.route.DestinationID)
	require.NoError(t, err)
	session.AgentProfileSnapshot = map[string]interface{}{"model": "old-model"}
	_, _, err = f.svc.attemptModelSwitchForPrompt(ctx, f.task.ID, session.ID, "new-model", "old prompt", session, &foregroundDispatch{}, func() error {
		successor := f.route
		successor.OperationID = "successor-route"
		return f.svc.persistWorkflowSessionRoute(context.Background(), f.task.ID, successor)
	}, nil)
	require.ErrorIs(t, err, ErrCeilingLaunchSuperseded)
	require.Empty(t, f.agent.capturedPrompts)
}

type ceilingRenewalBarrier struct {
	*sqliterepo.Repository
	entered, release chan struct{}
}

func (r *ceilingRenewalBarrier) SetTaskDeferredLaunchIfUnchanged(ctx context.Context, id string, prior interface{}, record map[string]interface{}) (bool, bool, error) {
	close(r.entered)
	<-r.release
	return r.Repository.SetTaskDeferredLaunchIfUnchanged(ctx, id, prior, record)
}

// Reviewer-requested contract coverage: every kind uses the same final local
// admission boundary, whose claim renewal serializes with route mutation.
func TestCeilingDispatchAdmissionSerializesRouteMutation(t *testing.T) {
	for _, kind := range []models.CeilingLaunchKind{
		models.CeilingLaunchStart, models.CeilingLaunchStartCreated, models.CeilingLaunchPromptEnsure,
		models.CeilingLaunchWorkflowStepEnsure, models.CeilingLaunchQueueDrainEnsure,
		models.CeilingLaunchResume, models.CeilingLaunchDynamicRelaunch,
	} {
		t.Run(string(kind), func(t *testing.T) {
			testCeilingDispatchAdmissionSerialization(t, kind)
		})
	}
}

func testCeilingDispatchAdmissionSerialization(t *testing.T, kind models.CeilingLaunchKind) {
	f := newCeilingDispatchFixture(t)
	ctx := context.Background()
	f.deferral.Kind = kind
	require.NoError(t, f.repo.SetTaskMetadataKey(ctx, f.task.ID, models.MetaKeyDeferredLaunch, models.CeilingRecordKeys(f.deferral)))
	claim, found, err := f.svc.claimCeilingDeferredLaunch(ctx, f.task.ID, f.route.DestinationID, ceilingClaimOwnerReplay)
	require.NoError(t, err)
	require.True(t, found)
	require.NotNil(t, claim)
	before, _, err := f.repo.GetTaskDeferredLaunch(ctx, f.task.ID)
	require.NoError(t, err)
	oldClaim, ok := models.ReadCeilingLaunchClaimDetails(before)
	require.True(t, ok)
	barrier := &ceilingRenewalBarrier{Repository: f.repo, entered: make(chan struct{}), release: make(chan struct{})}
	f.svc.repo = barrier
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(barrier.release) }) })
	admitted := make(chan error, 1)
	go func() { admitted <- f.svc.admitCeilingDispatch(withCeilingDispatchClaim(ctx, claim), f.task.ID) }()
	awaitCeilingProgress(t, barrier.entered, "final claim renewal")
	routeChanged := make(chan error, 1)
	successor := f.route
	successor.OperationID = "successor-route"
	go func() { routeChanged <- f.svc.persistWorkflowSessionRoute(ctx, f.task.ID, successor) }()
	require.Eventually(t, func() bool {
		f.svc.ceilingEntryAdmissionLocksMu.Lock()
		defer f.svc.ceilingEntryAdmissionLocksMu.Unlock()
		entry := f.svc.ceilingEntryAdmissionLocks[f.task.ID]
		return entry != nil && entry.refs == 2
	}, time.Second, time.Millisecond, "route mutation must wait for final claim renewal")
	release.Do(func() { close(barrier.release) })
	require.NoError(t, <-admitted)
	require.NoError(t, <-routeChanged)
	after, _, err := f.repo.GetTaskDeferredLaunch(ctx, f.task.ID)
	require.NoError(t, err)
	newClaim, ok := models.ReadCeilingLaunchClaimDetails(after)
	require.True(t, ok)
	require.Equal(t, oldClaim.ID, newClaim.ID)
	require.True(t, newClaim.ExpiresAt.After(oldClaim.ExpiresAt))
	current, err := f.repo.GetTask(ctx, f.task.ID)
	require.NoError(t, err)
	route, ok := models.LoadWorkflowSessionRoute(current.Metadata)
	require.True(t, ok)
	require.Equal(t, successor.OperationID, route.OperationID)
}
