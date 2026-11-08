package orchestrator

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestPublishTaskMovedRoundTripsPromotionEntryID(t *testing.T) {
	for _, entryID := range []int64{1<<53 + 1, 0} {
		t.Run(map[bool]string{true: "positive", false: "legacy zero"}[entryID > 0], func(t *testing.T) {
			ctx := context.Background()
			eventBus := bus.NewMemoryEventBus(testLogger())
			t.Cleanup(eventBus.Close)

			received := make(chan watcher.TaskMovedEventData, 1)
			eventWatcher := watcher.NewWatcher(eventBus, watcher.EventHandlers{
				OnTaskMoved: func(_ context.Context, data watcher.TaskMovedEventData) {
					received <- data
				},
			}, "promotion-entry-test", testLogger())
			require.NoError(t, eventWatcher.Start(ctx))
			t.Cleanup(func() { require.NoError(t, eventWatcher.Stop()) })

			service := &Service{eventBus: eventBus, logger: testLogger()}
			service.publishTaskMoved(ctx, &models.Task{
				ID: "task-promotion-entry", WorkflowID: "workflow-promotion-entry",
				WorkflowStepTransitionID: entryID,
				Metadata: map[string]interface{}{
					models.MetaKeyQueuePromotionPending: true,
				},
			}, "workflow-source", "step-source", "step-target", "session-promotion-entry")

			select {
			case data := <-received:
				require.True(t, data.QueuePromotion)
				require.Equal(t, entryID, data.StepTransitionID)
			case <-time.After(time.Second):
				t.Fatal("task.moved was not decoded by the watcher")
			}
		})
	}
}

func TestClaimMovedPromotionUsesLegacyZeroIDFallback(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-promotion-legacy", "session-promotion-legacy", "step-target")
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "task-promotion-legacy", models.MetaKeyQueuePromotionPending,
		map[string]interface{}{"from_step_id": "step-source"}))

	service := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	task, err := repo.GetTask(ctx, "task-promotion-legacy")
	require.NoError(t, err)
	require.True(t, service.claimMovedPromotion(ctx, task, 0))

	claimedTask, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.False(t, service.claimMovedPromotion(ctx, claimedTask, 0))
}

func TestClaimMovedPromotionLegacyZeroIDPreservesPinnedEntry(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-promotion-pinned", "session-promotion-pinned", "step-target")
	entryID, err := repo.GetLatestTaskStepTransitionID(ctx, "task-promotion-pinned")
	require.NoError(t, err)
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "task-promotion-pinned", models.MetaKeyQueuePromotionPending,
		map[string]interface{}{
			"from_step_id": "step-source",
			models.QueuePromotionEntryTransitionIDKey: strconv.FormatInt(entryID, 10),
		}))

	service := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	task, err := repo.GetTask(ctx, "task-promotion-pinned")
	require.NoError(t, err)
	require.False(t, service.claimMovedPromotion(ctx, task, 0))

	unchanged, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	_, markerPresent := unchanged.Metadata[models.MetaKeyQueuePromotionPending]
	require.True(t, markerPresent, "a legacy delivery cannot consume an identity-pinned promotion")
}

func TestHandleTaskMovedCrossStepPromotionReplacesPreviousRoute(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	fixture.stepGetter.steps["step-a"] = &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", Position: 0,
		AgentProfileID:          "profile-a",
		ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
	}
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1,
		AgentProfileID:            "profile-b",
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
	}
	require.NoError(t, fixture.repo.SetTaskMetadataKey(ctx, "t1", models.MetaKeyWorkflowSessionRoute,
		models.WorkflowSessionRoute{
			OperationID:       "route-step-a",
			DestinationStepID: "step-a",
			EntryIdentity:     fixture.svc.workflowEntryIdentity(ctx, "t1"),
			TargetKind:        workflowSessionRouteTargetProfile,
			AgentProfileID:    "profile-a",
			DestinationID:     fixture.current.ID,
			Phase:             workflowSessionRouteCommitted,
		}))

	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	task.WorkflowStepID = "step-b"
	task.WIPAdmitted = true
	task.QueuedForStepID = ""
	task.Metadata[models.MetaKeyQueuePromotionPending] = map[string]interface{}{"from_step_id": "step-a"}
	require.NoError(t, fixture.repo.UpdateTask(ctx, task))
	entryID, err := fixture.repo.GetLatestTaskStepTransitionID(ctx, task.ID)
	require.NoError(t, err)
	require.Positive(t, entryID)

	fixture.svc.handleTaskMoved(ctx, watcher.TaskMovedEventData{
		TaskID: "t1", FromStepID: "step-a", ToStepID: "step-b", SessionID: fixture.current.ID,
		WorkflowID: "wf1", TaskDescription: "Test", WIPAdmitted: true,
		QueuePromotion: true, StepTransitionID: entryID,
	})

	promoted, err := fixture.repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	_, markerPresent := promoted.Metadata[models.MetaKeyQueuePromotionPending]
	require.False(t, markerPresent, "the current promotion event should claim its pending marker")
	require.Eventually(t, func() bool {
		updated, getErr := fixture.repo.GetTask(ctx, task.ID)
		if getErr != nil {
			return false
		}
		route, ok := models.LoadWorkflowSessionRoute(updated.Metadata)
		return ok && route.DestinationStepID == "step-b" &&
			route.EntryIdentity == workflowEntryIdentityForID(entryID) &&
			route.DestinationID != fixture.current.ID
	}, 2*time.Second, 10*time.Millisecond, "the destination route must use the promoted entry identity")
}
