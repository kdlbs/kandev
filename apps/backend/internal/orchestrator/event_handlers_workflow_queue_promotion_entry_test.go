package orchestrator

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// newSameStepPromotionFixture builds a task whose route was committed by step-a
// and queues it on the full step-b. It returns the ledger id of that queued
// entry.
func newSameStepPromotionFixture(t *testing.T) (*profileSwitchFixture, int64) {
	t.Helper()
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
	task.WIPAdmitted = false
	task.QueuedForStepID = "step-b"
	require.NoError(t, fixture.repo.UpdateTask(ctx, task), "queue task on the full destination step")
	queuedEntryID, err := fixture.repo.GetLatestTaskStepTransitionID(ctx, "t1")
	require.NoError(t, err)
	require.Positive(t, queuedEntryID, "entering the destination step must write its ledger row")
	return fixture, queuedEntryID
}

// promoteSameStep admits the queued task on step-b through the repository's
// same-step promotion write, which records no ledger row of its own.
func promoteSameStep(t *testing.T, fixture *profileSwitchFixture) {
	t.Helper()
	ctx := context.Background()
	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	task.WIPAdmitted = true
	task.QueuedForStepID = ""
	task.QueuedAt = nil
	task.Metadata[models.MetaKeyQueuePromotionPending] = map[string]interface{}{
		"from_step_id": "step-b",
	}
	claimed, err := fixture.repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, task, "step-b", "step-b", 0)
	require.NoError(t, err)
	require.True(t, claimed, "same-step promotion must claim the queued row")
	require.Zero(t, task.WorkflowStepTransitionID, "a same-step promotion writes no ledger row")
}

func handleQueuePromotionAndWait(t *testing.T, fixture *profileSwitchFixture) {
	t.Helper()
	entryComplete := make(chan struct{})
	fixture.svc.onTaskQueuePromotionEntryComplete = func() { close(entryComplete) }
	fixture.svc.handleTaskQueuePromoted(context.Background(), watcher.TaskEventData{TaskID: "t1"})
	select {
	case <-entryComplete:
	case <-time.After(2 * time.Second):
		t.Fatal("queue-promotion entry did not complete")
	}
}

// AC-TASKS-WORKFLOW-STAGE-MOVES-002.1: the promotion delivery carries no
// transition id, so the admitted entry must still replace the route committed
// by the step the task left.
func TestHandleTaskQueuePromotedSameStepEntryReplacesPreviousStepRoute(t *testing.T) {
	ctx := context.Background()
	fixture, queuedEntryID := newSameStepPromotionFixture(t)
	promoteSameStep(t, fixture)

	handleQueuePromotionAndWait(t, fixture)

	promoted, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	route, ok := models.LoadWorkflowSessionRoute(promoted.Metadata)
	require.True(t, ok)
	require.Equal(t, "step-b", route.DestinationStepID,
		"the promoted entry must replace the route committed by the previous step")
	require.Equal(t, workflowEntryIdentityForID(queuedEntryID), route.EntryIdentity)
	require.NotEqual(t, fixture.current.ID, route.DestinationID)

	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	require.NoError(t, err)
	if source.State != models.TaskSessionStateWaitingForInput || source.IsPrimary {
		t.Fatalf("source after promotion = state %s primary %t, want parked nonprimary", source.State, source.IsPrimary)
	}
}

// AC-TASKS-WORKFLOW-STAGE-MOVES-002.2: the promotion write pins the entry it
// admits. A delivery handled after the task left and re-entered the step acts
// for that pinned entry, so the entry guard drops it instead of running the
// newer entry's on_enter.
func TestHandleTaskQueuePromotedSameStepIgnoresReentryAfterPromotion(t *testing.T) {
	ctx := context.Background()
	fixture, queuedEntryID := newSameStepPromotionFixture(t)
	promoteSameStep(t, fixture)

	promotedTask, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	marker, ok := promotedTask.Metadata[models.MetaKeyQueuePromotionPending].(map[string]interface{})
	require.True(t, ok, "promotion marker must remain until the entry is handled")
	require.Equal(t, queuedEntryID, queuePromotionStampedEntryID(marker),
		"the promotion must pin the queued entry on its marker")

	for _, stepID := range []string{"step-a", "step-b"} {
		moved, getErr := fixture.repo.GetTask(ctx, "t1")
		require.NoError(t, getErr)
		moved.WorkflowStepID = stepID
		require.NoError(t, fixture.repo.UpdateTask(ctx, moved))
	}
	reentryID, err := fixture.repo.GetLatestTaskStepTransitionID(ctx, "t1")
	require.NoError(t, err)
	require.Greater(t, reentryID, queuedEntryID, "re-entry must write a newer ledger row")

	handleQueuePromotionAndWait(t, fixture)

	after, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	route, ok := models.LoadWorkflowSessionRoute(after.Metadata)
	require.True(t, ok)
	require.Equal(t, "step-a", route.DestinationStepID,
		"a promotion delivery for the earlier entry must not dispatch the re-entry's on_enter")
}

func TestQueuePromotionStampedEntryIDPreservesFullLedgerRange(t *testing.T) {
	for _, entryID := range []int64{1<<53 + 1, 1<<63 - 1} {
		encoded, err := json.Marshal(map[string]interface{}{
			models.QueuePromotionEntryTransitionIDKey: strconv.FormatInt(entryID, 10),
		})
		require.NoError(t, err)
		var marker map[string]interface{}
		require.NoError(t, json.Unmarshal(encoded, &marker))
		require.Equal(t, entryID, queuePromotionStampedEntryID(marker))
	}
	for _, legacyID := range []interface{}{int(17), int64(17), float64(17)} {
		require.EqualValues(t, 17, queuePromotionStampedEntryID(map[string]interface{}{
			models.QueuePromotionEntryTransitionIDKey: legacyID,
		}))
	}
	for _, invalidID := range []string{"not-an-entry", "9223372036854775808"} {
		require.Zero(t, queuePromotionStampedEntryID(map[string]interface{}{
			models.QueuePromotionEntryTransitionIDKey: invalidID,
		}))
	}
}
