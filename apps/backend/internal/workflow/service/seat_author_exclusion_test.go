package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
)

func seatStepFixture(workflowID, id string) *models.WorkflowStep {
	return &models.WorkflowStep{
		ID: id, WorkflowID: workflowID, Name: "Review", Position: 1, Color: "yellow",
		Events: models.StepEvents{OnEnter: []models.OnEnterAction{
			{Type: models.OnEnterClearDecisions},
			{Type: models.OnEnterEnsureParticipantSeat, Config: map[string]interface{}{"role": "reviewer"}},
		}},
	}
}

func TestSetSeatAuthorExclusion_TogglesTheSeatActionAndReturnsUpdatedSteps(t *testing.T) {
	svc, _, provider := setupTestServiceWithProvider(t)
	provider.addWorkflow("wf-1", "ws-1", "Office Default")
	createStep(t, svc, seatStepFixture("wf-1", "step-review"))
	ctx := context.Background()

	updated, err := svc.SetSeatAuthorExclusion(ctx, "ws-1", []SeatAuthorExclusionChange{
		{StepID: "step-review", Role: "reviewer", Enabled: true},
	})
	require.NoError(t, err)
	require.Len(t, updated, 1)
	assert.Equal(t, "step-review", updated[0].ID)

	stored, err := svc.GetStep(ctx, "step-review")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"reviewer": true}, models.ParticipantSeatRoles(stored.Events))
	assert.Equal(t, models.OnEnterClearDecisions, stored.Events.OnEnter[0].Type, "other actions are kept")

	_, err = svc.SetSeatAuthorExclusion(ctx, "ws-1", []SeatAuthorExclusionChange{
		{StepID: "step-review", Role: "reviewer", Enabled: false},
	})
	require.NoError(t, err)
	stored, err = svc.GetStep(ctx, "step-review")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"reviewer": false}, models.ParticipantSeatRoles(stored.Events))
}

func TestSetSeatAuthorExclusion_RejectsWithoutWriting(t *testing.T) {
	cases := []struct {
		name    string
		change  SeatAuthorExclusionChange
		wantErr error
	}{
		{"step in another workspace", SeatAuthorExclusionChange{StepID: "step-foreign", Role: "reviewer", Enabled: true}, ErrSeatStepNotInWorkspace},
		{"role without a seat action", SeatAuthorExclusionChange{StepID: "step-ok", Role: "approver", Enabled: true}, ErrSeatRoleNotFound},
		{"read-only workflow", SeatAuthorExclusionChange{StepID: "step-synced", Role: "reviewer", Enabled: true}, ErrWorkflowReadOnly},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, provider := setupTestServiceWithProvider(t)
			provider.addWorkflow("wf-1", "ws-1", "Office Default")
			provider.addWorkflow("wf-other", "ws-2", "Elsewhere")
			provider.addWorkflow("wf-synced", "ws-1", "Synced")
			provider.workflows[2].Source = taskmodels.WorkflowSourceGitHub
			createStep(t, svc, seatStepFixture("wf-1", "step-ok"))
			createStep(t, svc, seatStepFixture("wf-other", "step-foreign"))
			createSyncedStepFixture(t, svc, seatStepFixture("wf-synced", "step-synced"))
			ctx := context.Background()

			_, err := svc.SetSeatAuthorExclusion(ctx, "ws-1", []SeatAuthorExclusionChange{
				{StepID: "step-ok", Role: "reviewer", Enabled: true},
				tc.change,
			})
			require.ErrorIs(t, err, tc.wantErr)

			stored, err := svc.GetStep(ctx, "step-ok")
			require.NoError(t, err)
			assert.Equal(t, map[string]bool{"reviewer": false}, models.ParticipantSeatRoles(stored.Events),
				"a rejected batch applies none of its changes")
		})
	}
}
