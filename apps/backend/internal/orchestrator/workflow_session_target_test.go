package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

type failWorkflowRouteMetadataRepo struct {
	repoStore
	fail bool
}

func (r *failWorkflowRouteMetadataRepo) SetTaskMetadataKey(ctx context.Context, taskID, key string, value interface{}) error {
	if r.fail && key == models.MetaKeyWorkflowSessionRoute {
		return errors.New("workflow route metadata write failed")
	}
	return r.repoStore.SetTaskMetadataKey(ctx, taskID, key, value)
}

func TestValidateWorkflowSessionTargetSource(t *testing.T) {
	tests := []struct {
		name    string
		dest    *wfmodels.WorkflowStep
		source  *wfmodels.WorkflowStep
		wantErr string
	}{
		{
			name: "valid earlier direct profile",
			dest: &wfmodels.WorkflowStep{ID: "review", WorkflowID: "workflow", Position: 2},
			source: &wfmodels.WorkflowStep{
				ID: "implement", WorkflowID: "workflow", Position: 1, AgentProfileID: "profile-a",
			},
		},
		{
			name:    "foreign workflow",
			dest:    &wfmodels.WorkflowStep{ID: "review", WorkflowID: "workflow", Position: 2},
			source:  &wfmodels.WorkflowStep{ID: "implement", WorkflowID: "other", Position: 1, AgentProfileID: "profile-a"},
			wantErr: "belongs to another workflow",
		},
		{
			name:    "later source",
			dest:    &wfmodels.WorkflowStep{ID: "review", WorkflowID: "workflow", Position: 1},
			source:  &wfmodels.WorkflowStep{ID: "implement", WorkflowID: "workflow", Position: 2, AgentProfileID: "profile-a"},
			wantErr: "not earlier",
		},
		{
			name:    "source without direct profile",
			dest:    &wfmodels.WorkflowStep{ID: "review", WorkflowID: "workflow", Position: 2},
			source:  &wfmodels.WorkflowStep{ID: "implement", WorkflowID: "workflow", Position: 1},
			wantErr: "must use a direct agent profile",
		},
		{
			name: "indirect source",
			dest: &wfmodels.WorkflowStep{ID: "review", WorkflowID: "workflow", Position: 2},
			source: &wfmodels.WorkflowStep{
				ID: "implement", WorkflowID: "workflow", Position: 1, AgentProfileID: "profile-a",
				SessionTarget: &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetInitial},
			},
			wantErr: "must use a direct agent profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorkflowSessionTargetSource(tt.dest, tt.source)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSelectExplicitWorkflowStartSessionFallsBackFromTerminalTarget(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	terminal := fixture.current
	terminal.State = models.TaskSessionStateCompleted
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, terminal))

	step := &wfmodels.WorkflowStep{
		ID: "step-review", WorkflowID: "wf1", Position: 1,
		SessionTarget:             &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetInitial},
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	}
	selected, profileID, err := fixture.svc.selectExplicitWorkflowStartSession(ctx, "t1", step)

	require.NoError(t, err)
	require.Nil(t, selected)
	require.Equal(t, "profile-a", profileID)
}

func TestRecordWorkflowSourceBindingIgnoresDelayedEntryAfterTaskMoves(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	source := &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", AgentProfileID: "profile-a",
	}
	require.NoError(t, fixture.svc.recordWorkflowSourceBinding(ctx, "t1", source, fixture.current))

	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	task.WorkflowStepID = "step-b"
	require.NoError(t, fixture.repo.UpdateTask(ctx, task))

	stale := &models.TaskSession{ID: "stale-session", TaskID: "t1", AgentProfileID: "profile-a"}
	require.NoError(t, fixture.svc.recordWorkflowSourceBinding(ctx, "t1", source, stale))

	binding, err := fixture.repo.GetWorkflowSessionBinding(ctx, "t1", workflowSessionBindingTargetKey(source.ID))
	require.NoError(t, err)
	require.Equal(t, fixture.current.ID, binding.SessionID)
}

func TestRecordWorkflowSourceBindingDoesNotOverwriteRevisitedSourceEntry(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	source := &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", AgentProfileID: "profile-a",
	}
	second := &models.TaskSession{ID: "session-a-revisit", TaskID: "t1", AgentProfileID: "profile-a"}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, second))

	require.NoError(t, fixture.svc.recordWorkflowSourceBinding(ctx, "t1", source, fixture.current, 41))
	require.NoError(t, fixture.svc.recordWorkflowSourceBinding(ctx, "t1", source, second, 42))
	// E1 resumes after E2 has already committed while the task is back on the
	// same source step. Its current-step guard alone is insufficient; the
	// immutable entry operation must lose the conditional write.
	require.NoError(t, fixture.svc.recordWorkflowSourceBinding(ctx, "t1", source, fixture.current, 41))

	binding, err := fixture.repo.GetWorkflowSessionBinding(ctx, "t1", workflowSessionBindingTargetKey(source.ID))
	require.NoError(t, err)
	require.Equal(t, second.ID, binding.SessionID)
	require.Equal(t, "workflow-step-entry-v2:entry:00000000000000000042", binding.OperationID)
}

func TestLoadRecordedWorkflowSessionRouteReturnsCommittedDestination(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	target := &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetStep, StepID: "step-a"}
	step := &wfmodels.WorkflowStep{
		ID: "step-review", WorkflowID: "wf1", Position: 1, SessionTarget: target,
	}
	operationID := workflowSessionRouteID("t1", step.ID, "", target, models.WorkflowProfileSessionStartPolicyReuse)
	require.NoError(t, fixture.repo.SetTaskMetadataKey(ctx, "t1", models.MetaKeyWorkflowSessionRoute, models.WorkflowSessionRoute{
		OperationID:       operationID,
		DestinationStepID: step.ID,
		TargetKind:        string(target.Kind),
		TargetStepID:      target.StepID,
		AgentProfileID:    "profile-a",
		DestinationID:     fixture.current.ID,
		Phase:             workflowSessionRouteCommitted,
	}))

	route, session, err := fixture.svc.loadRecordedWorkflowSessionRoute(ctx, "t1", operationID, step, "profile-a")

	require.NoError(t, err)
	require.NotNil(t, route)
	require.Equal(t, workflowSessionRouteCommitted, route.Phase)
	require.NotNil(t, session)
	require.Equal(t, fixture.current.ID, session.ID)
}

func TestLoadRecordedWorkflowSessionRouteRejectsDeletedDestination(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	target := &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetInitial}
	step := &wfmodels.WorkflowStep{ID: "step-review", WorkflowID: "wf1", Position: 1, SessionTarget: target}
	operationID := workflowSessionRouteID("t1", step.ID, "entry:00000000000000000007", target, models.WorkflowProfileSessionStartPolicyReuse)
	require.NoError(t, fixture.repo.SetTaskMetadataKey(ctx, "t1", models.MetaKeyWorkflowSessionRoute, models.WorkflowSessionRoute{
		OperationID:       operationID,
		DestinationStepID: step.ID,
		TargetKind:        string(target.Kind),
		AgentProfileID:    "profile-a",
		DestinationID:     "deleted-destination",
		Phase:             workflowSessionRoutePrepared,
	}))

	_, _, err := fixture.svc.loadRecordedWorkflowSessionRoute(ctx, "t1", operationID, step, "profile-a")
	require.ErrorContains(t, err, "recorded workflow session route destination")
}

func TestLoadRecordedWorkflowSessionRouteAllowsFreshFallbackFromTerminalDestination(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	fixture.current.State = models.TaskSessionStateCompleted
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, fixture.current))
	target := &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetInitial}
	step := &wfmodels.WorkflowStep{ID: "step-review", WorkflowID: "wf1", Position: 1, SessionTarget: target}
	operationID := workflowSessionRouteID("t1", step.ID, "entry:00000000000000000008", target, models.WorkflowProfileSessionStartPolicyReuse)
	require.NoError(t, fixture.repo.SetTaskMetadataKey(ctx, "t1", models.MetaKeyWorkflowSessionRoute, models.WorkflowSessionRoute{
		OperationID:       operationID,
		DestinationStepID: step.ID,
		TargetKind:        string(target.Kind),
		AgentProfileID:    "profile-a",
		DestinationID:     fixture.current.ID,
		Phase:             workflowSessionRouteCommitted,
	}))

	route, session, err := fixture.svc.loadRecordedWorkflowSessionRoute(ctx, "t1", operationID, step, "profile-a")
	require.NoError(t, err)
	require.NotNil(t, route)
	require.Nil(t, session)
}

func TestCreateTaskSessionWithWorkflowRoutePersistsPreparedDestinationAtomically(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	route := &models.WorkflowSessionRoute{
		OperationID:       "workflow-session:t1:step-review:entry:00000000000000000009:initial:",
		DestinationStepID: "step-review",
		TargetKind:        string(wfmodels.WorkflowSessionTargetInitial),
		AgentProfileID:    "profile-a",
		Phase:             workflowSessionRoutePrepared,
	}
	destination := &models.TaskSession{
		ID:             "workflow-route-destination",
		TaskID:         "t1",
		AgentProfileID: "profile-a",
		State:          models.TaskSessionStateCreated,
	}
	require.NoError(t, fixture.repo.CreateTaskSessionWithWorkflowSessionRoute(ctx, destination, route))

	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	recorded, ok := models.LoadWorkflowSessionRoute(task.Metadata)
	require.True(t, ok)
	require.Equal(t, destination.ID, recorded.DestinationID)
	require.Equal(t, workflowSessionRoutePrepared, recorded.Phase)
}

func TestReusePreparedWorkflowRouteCommitsWhenDestinationIsAlreadyPrimary(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	target := &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetInitial}
	step := &wfmodels.WorkflowStep{ID: "step-review", WorkflowID: "wf1", Position: 1, SessionTarget: target}
	route := models.WorkflowSessionRoute{
		OperationID:       workflowSessionRouteID("t1", step.ID, "entry:00000000000000000011", target, models.WorkflowProfileSessionStartPolicyReuse),
		DestinationStepID: step.ID,
		TargetKind:        string(target.Kind),
		AgentProfileID:    "profile-a",
		DestinationID:     fixture.current.ID,
		Phase:             workflowSessionRoutePrepared,
	}
	require.NoError(t, fixture.repo.SetTaskMetadataKey(ctx, "t1", models.MetaKeyWorkflowSessionRoute, route))

	reused, switched, err := fixture.svc.reuseRecordedWorkflowSession(ctx, "t1", fixture.current, &route, fixture.current, models.WorkflowProfileSessionEndPolicyPark)
	require.NoError(t, err)
	require.False(t, switched)
	require.Equal(t, fixture.current.ID, reused.ID)

	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	committed, ok := models.LoadWorkflowSessionRoute(task.Metadata)
	require.True(t, ok)
	require.Equal(t, workflowSessionRouteCommitted, committed.Phase)
}

func TestWorkflowRouteRetryRecoversAfterLegacyCommitMetadataFailure(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	destination := &models.TaskSession{
		ID:             "workflow-route-retry-destination",
		TaskID:         "t1",
		AgentProfileID: "profile-a",
		State:          models.TaskSessionStateCreated,
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, destination))
	target := &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetInitial}
	route := models.WorkflowSessionRoute{
		OperationID:       workflowSessionRouteID("t1", "step-review", "entry:00000000000000000013", target, models.WorkflowProfileSessionStartPolicyReuse),
		DestinationStepID: "step-review",
		TargetKind:        string(target.Kind),
		AgentProfileID:    "profile-a",
		DestinationID:     destination.ID,
		Phase:             workflowSessionRoutePrepared,
	}
	require.NoError(t, fixture.repo.SetTaskMetadataKey(ctx, "t1", models.MetaKeyWorkflowSessionRoute, route))

	failedRepo := &failWorkflowRouteMetadataRepo{repoStore: fixture.repo, fail: true}
	fixture.svc.repo = failedRepo
	_, err := fixture.svc.promoteWorkflowSessionRoute(ctx, "t1", destination, &route)
	require.ErrorContains(t, err, "workflow route metadata write failed")

	promoted, err := fixture.repo.GetTaskSession(ctx, destination.ID)
	require.NoError(t, err)
	require.True(t, promoted.IsPrimary)
	require.Equal(t, models.TaskSessionStateRunning, fixtureSessionState(t, fixture.repo, fixture.current.ID))

	// A retry after reloading the primary must use the prepared destination,
	// commit the existing route, and never allocate another session.
	fixture.svc.repo = fixture.repo
	recordedTask, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	recordedRoute, ok := models.LoadWorkflowSessionRoute(recordedTask.Metadata)
	require.True(t, ok)
	reused, switched, err := fixture.svc.reuseRecordedWorkflowSession(
		ctx, "t1", fixture.current, &recordedRoute, promoted, models.WorkflowProfileSessionEndPolicyPark,
	)
	require.NoError(t, err)
	require.True(t, switched)
	require.Equal(t, destination.ID, reused.ID)

	sessions, err := fixture.repo.ListTaskSessions(ctx, "t1")
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	reloadedTask, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	committed, ok := models.LoadWorkflowSessionRoute(reloadedTask.Metadata)
	require.True(t, ok)
	require.Equal(t, workflowSessionRouteCommitted, committed.Phase)
}

func fixtureSessionState(t *testing.T, repo interface {
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
}, sessionID string) models.TaskSessionState {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	return session.State
}
