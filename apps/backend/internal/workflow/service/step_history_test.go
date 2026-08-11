package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/workflow/models"
)

func TestCreateStepTransition_PersistsMetadataAndActor(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	actorID := "user-1"
	metadata := map[string]interface{}{
		"signal_source":  "agent",
		"signal_summary": "finished the step",
	}

	err := svc.CreateStepTransition(ctx, "sess-1", "step-a", "step-b", models.StepTransitionTriggerAutoComplete, &actorID, metadata)
	require.NoError(t, err)

	history, err := svc.ListHistoryBySession(ctx, "sess-1")
	require.NoError(t, err)
	require.Len(t, history, 1)

	got := history[0]
	require.NotNil(t, got.FromStepID)
	require.Equal(t, "step-a", *got.FromStepID)
	require.Equal(t, "step-b", got.ToStepID)
	require.Equal(t, models.StepTransitionTriggerAutoComplete, got.Trigger)
	require.NotNil(t, got.ActorID)
	require.Equal(t, "user-1", *got.ActorID)
	require.Equal(t, "agent", got.Metadata["signal_source"])
	require.Equal(t, "finished the step", got.Metadata["signal_summary"])
}

func TestCreateStepTransition_NilMetadataAndFromStep(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	err := svc.CreateStepTransition(ctx, "sess-2", "", "step-first", models.StepTransitionTriggerManual, nil, nil)
	require.NoError(t, err)

	history, err := svc.ListHistoryBySession(ctx, "sess-2")
	require.NoError(t, err)
	require.Len(t, history, 1)

	got := history[0]
	require.Nil(t, got.FromStepID)
	require.Nil(t, got.ActorID)
	require.Nil(t, got.Metadata)
}
