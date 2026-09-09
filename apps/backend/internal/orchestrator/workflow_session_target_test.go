package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

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
