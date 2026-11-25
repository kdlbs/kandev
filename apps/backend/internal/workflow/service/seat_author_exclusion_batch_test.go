package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/workflow/models"
)

func TestSetSeatAuthorExclusion_ExecutorDenialOnALaterWorkflowWritesNothing(t *testing.T) {
	svc, _, provider := setupTestServiceWithProvider(t)
	provider.addWorkflow("wf-1", "ws-1", "Office Default")
	provider.addWorkflow("wf-2", "ws-1", "Private executor")
	createStep(t, svc, seatStepFixture("wf-1", "step-1"))
	createStep(t, svc, seatStepFixture("wf-2", "step-2"))
	denied := errors.New("executor is private")
	svc.SetExecutorMutationChecker(func(_ context.Context, workflowID string) error {
		if workflowID == "wf-2" {
			return denied
		}
		return nil
	})
	ctx := context.Background()

	_, err := svc.SetSeatAuthorExclusion(ctx, "ws-1", []SeatAuthorExclusionChange{
		{StepID: "step-1", Role: "reviewer", Enabled: true},
		{StepID: "step-2", Role: "reviewer", Enabled: true},
	})
	require.ErrorIs(t, err, denied)

	stored, err := svc.GetStep(ctx, "step-1")
	require.NoError(t, err)
	assert.False(t, models.ParticipantSeatRoles(stored.Events)["reviewer"],
		"a batch rejected on a later step applies none of its changes")
}

func TestSetSeatAuthorExclusion_ConcurrentRoleChangesOnOneStepBothPersist(t *testing.T) {
	svc, _, provider := setupTestServiceWithProvider(t)
	provider.addWorkflow("wf-1", "ws-1", "Office Default")
	step := seatStepFixture("wf-1", "step-review")
	step.Events.OnEnter = append(step.Events.OnEnter, models.OnEnterAction{
		Type: models.OnEnterEnsureParticipantSeat, Config: map[string]interface{}{"role": "approver"},
	})
	createStep(t, svc, step)
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, role := range []string{"reviewer", "approver"} {
		wg.Add(1)
		go func(i int, role string) {
			defer wg.Done()
			_, errs[i] = svc.SetSeatAuthorExclusion(ctx, "ws-1", []SeatAuthorExclusionChange{
				{StepID: "step-review", Role: role, Enabled: true},
			})
		}(i, role)
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	stored, err := svc.GetStep(ctx, "step-review")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"reviewer": true, "approver": true}, models.ParticipantSeatRoles(stored.Events))
}

func TestSetSeatAuthorExclusion_ReturnsTheSavedVersion(t *testing.T) {
	svc, _, provider := setupTestServiceWithProvider(t)
	provider.addWorkflow("wf-1", "ws-1", "Office Default")
	createStep(t, svc, seatStepFixture("wf-1", "step-review"))
	ctx := context.Background()

	updated, err := svc.SetSeatAuthorExclusion(ctx, "ws-1", []SeatAuthorExclusionChange{
		{StepID: "step-review", Role: "reviewer", Enabled: true},
	})
	require.NoError(t, err)
	stored, err := svc.GetStep(ctx, "step-review")
	require.NoError(t, err)
	require.Len(t, updated, 1)
	assert.True(t, updated[0].UpdatedAt.Equal(stored.UpdatedAt))
	assert.Equal(t, stored.Events, updated[0].Events)
}
