package changes

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestTurnChangeWriterIntervalsExcludeCompletedPending(t *testing.T) {
	startedAt := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(time.Minute)
	store, turns, coordinator := newWriterIntervalFixture(t, completedAt)
	first := writerIntervalAdmission("turn-1", startedAt)
	require.NoError(t, coordinator.Admit(context.Background(), first, &coordinatorCheckpointClient{capturedAt: &startedAt}))

	secondAt := completedAt.Add(4 * time.Minute)
	second := writerIntervalAdmission("turn-2", secondAt)
	require.NoError(t, coordinator.Admit(context.Background(), second, &coordinatorCheckpointClient{capturedAt: &secondAt}))

	firstSet := store.changeSets[turnChangeSetID(first.TurnID)]
	secondSet := store.changeSets[turnChangeSetID(second.TurnID)]
	require.Empty(t, firstSet.OverlapIntervals, "a completed pending capture is not a live writer")
	require.Empty(t, secondSet.OverlapIntervals)
	require.Nil(t, firstSet.TerminalAt, "admission must not settle another turn")
	_ = turns
}

func TestTurnChangeWriterIntervalsPreserveRealOverlap(t *testing.T) {
	startedAt := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(5 * time.Minute)
	store, _, coordinator := newWriterIntervalFixture(t, completedAt)
	first := writerIntervalAdmission("turn-1", startedAt)
	require.NoError(t, coordinator.Admit(context.Background(), first, &coordinatorCheckpointClient{capturedAt: &startedAt}))

	secondAt := startedAt.Add(time.Minute)
	second := writerIntervalAdmission("turn-2", secondAt)
	require.NoError(t, coordinator.Admit(context.Background(), second, &coordinatorCheckpointClient{capturedAt: &secondAt}))

	firstSet := store.changeSets[turnChangeSetID(first.TurnID)]
	secondSet := store.changeSets[turnChangeSetID(second.TurnID)]
	require.Len(t, firstSet.OverlapIntervals, 1)
	require.Len(t, secondSet.OverlapIntervals, 1)
	require.NotNil(t, firstSet.OverlapIntervals[0].EndedAt)
	require.True(t, firstSet.OverlapIntervals[0].EndedAt.Equal(completedAt))
	require.Equal(t, firstSet.ID, secondSet.OverlapIntervals[0].ChangeSetID)
}

func TestTurnChangeWriterIntervalsTouchingBoundaries(t *testing.T) {
	startedAt := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(time.Minute)
	store, _, coordinator := newWriterIntervalFixture(t, completedAt)
	first := writerIntervalAdmission("turn-1", startedAt)
	require.NoError(t, coordinator.Admit(context.Background(), first, &coordinatorCheckpointClient{capturedAt: &startedAt}))

	second := writerIntervalAdmission("turn-2", completedAt)
	require.NoError(t, coordinator.Admit(context.Background(), second, &coordinatorCheckpointClient{capturedAt: &completedAt}))

	firstSet := store.changeSets[turnChangeSetID(first.TurnID)]
	secondSet := store.changeSets[turnChangeSetID(second.TurnID)]
	require.Empty(t, firstSet.OverlapIntervals)
	require.Empty(t, secondSet.OverlapIntervals)
}

func TestTurnChangeOrphanReconcileMixedState(t *testing.T) {
	completedAt := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	store, _, coordinator := newWriterIntervalFixture(t, completedAt)
	completed := &models.TurnChangeSet{
		ID: "set-completed", TaskID: "task-1", TaskSessionID: "session-1", TurnID: "turn-1",
		Revision: 1, CaptureEnabled: true, StartAccepted: true,
		Availability: models.TurnChangeAvailabilityPending,
	}
	active := &models.TurnChangeSet{
		ID: "set-active", TaskID: "task-1", TaskSessionID: "session-1", TurnID: "turn-2",
		Revision: 1, CaptureEnabled: true, StartAccepted: true,
		Availability: models.TurnChangeAvailabilityPending,
	}
	store.changeSets[completed.ID] = completed
	store.changeSets[active.ID] = active

	coordinator.SetTurnChangeOperationGuard(fixedTurnChangeOperationGuard{state: TurnChangeOperationActive})
	require.NoError(t, coordinator.ReconcileUnfinished(context.Background()))
	require.Nil(t, completed.TerminalAt, "an active operation protects the completed row")
	require.Nil(t, active.TerminalAt)

	coordinator.SetTurnChangeOperationGuard(fixedTurnChangeOperationGuard{state: TurnChangeOperationUnknown})
	require.NoError(t, coordinator.ReconcileUnfinished(context.Background()))
	require.Nil(t, completed.TerminalAt, "unknown runtime ownership fails closed")

	coordinator.SetTurnChangeOperationGuard(fixedTurnChangeOperationGuard{state: TurnChangeOperationInactive})
	require.NoError(t, coordinator.ReconcileUnfinished(context.Background()))
	require.NotNil(t, completed.TerminalAt)
	require.True(t, completed.TerminalAt.Equal(completedAt), "canonical completion supplies the terminal time")
	require.Equal(t, "restart_unavailable", completed.TerminalOutcome)
	require.Nil(t, active.TerminalAt)
}

func TestTurnChangeOrphanReconcilePagedCandidates(t *testing.T) {
	completedAt := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	store, turns, coordinator := newWriterIntervalFixture(t, completedAt)
	store.changeSets = map[string]*models.TurnChangeSet{}
	for index := 0; index < 150; index++ {
		id := fmt.Sprintf("set-%03d", index)
		store.changeSets[id] = &models.TurnChangeSet{
			ID: id, TaskID: "task-1", TaskSessionID: "session-1", TurnID: "turn-1",
			Revision: 1, CaptureEnabled: true, StartAccepted: true,
			Availability: models.TurnChangeAvailabilityPending,
		}
	}
	turns["turn-1"] = &models.Turn{ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1", CompletedAt: &completedAt}

	require.NoError(t, coordinator.ReconcileUnfinished(context.Background()))
	for id, changeSet := range store.changeSets {
		require.NotNil(t, changeSet.TerminalAt, "candidate %s was skipped by pagination", id)
	}
}

type fixedTurnChangeOperationGuard struct{ state TurnChangeOperationState }

func (g fixedTurnChangeOperationGuard) TurnChangeCaptureOperation(context.Context, string, int64) TurnChangeOperationState {
	return g.state
}

func newWriterIntervalFixture(
	t *testing.T,
	completedAt time.Time,
) (*coordinatorStore, coordinatorTurnReaderMap, *Coordinator) {
	t.Helper()
	turns := coordinatorTurnReaderMap{
		"turn-1": {ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1", CompletedAt: &completedAt},
		"turn-2": {ID: "turn-2", TaskID: "task-1", TaskSessionID: "session-1"},
	}
	store := newCoordinatorStore()
	store.completedAtByTurn = map[string]*time.Time{"turn-1": &completedAt}
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	coordinator := NewCoordinator(store, turns, policy, NewContentService(store, nil), func() time.Time {
		return completedAt
	})
	return store, turns, coordinator
}

func writerIntervalAdmission(turnID string, _ time.Time) Admission {
	admission := coordinatorAdmission()
	admission.TurnID = turnID
	admission.ExecutionID = "execution-" + turnID
	admission.StartupAttemptID = "startup-" + turnID
	admission.PromptGeneration = uint64(len(turnID) + 1)
	return admission
}

func TestWriterOverlapReconciliationRetriesRepeatedCASConflicts(t *testing.T) {
	startedAt := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	store, _, coordinator := newWriterIntervalFixture(t, startedAt.Add(time.Hour))
	first := writerIntervalAdmission("turn-1", startedAt)
	require.NoError(t, coordinator.Admit(context.Background(), first, &coordinatorCheckpointClient{capturedAt: &startedAt}))
	second := writerIntervalAdmission("turn-2", startedAt)
	second.ExecutionID = "execution-two"
	second.StartupAttemptID = "startup-two"
	store.overlapCASConflicts = 2
	require.NoError(t, coordinator.Admit(context.Background(), second, &coordinatorCheckpointClient{capturedAt: &startedAt}))

	firstSet := store.changeSets[turnChangeSetID("turn-1")]
	secondSet := store.changeSets[turnChangeSetID("turn-2")]
	require.Len(t, firstSet.OverlapIntervals, 1)
	require.Len(t, secondSet.OverlapIntervals, 1)
}

func TestWriterOverlapReconciliationFailsAfterRepeatedCASConflicts(t *testing.T) {
	startedAt := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	store, _, coordinator := newWriterIntervalFixture(t, startedAt.Add(time.Hour))
	first := writerIntervalAdmission("turn-1", startedAt)
	require.NoError(t, coordinator.Admit(context.Background(), first, &coordinatorCheckpointClient{capturedAt: &startedAt}))
	second := writerIntervalAdmission("turn-2", startedAt)
	store.overlapCASConflicts = 3
	err := coordinator.Admit(context.Background(), second, &coordinatorCheckpointClient{capturedAt: &startedAt})
	require.ErrorContains(t, err, "lost repeated revision conflicts")

	firstSet := store.changeSets[turnChangeSetID("turn-1")]
	secondSet := store.changeSets[turnChangeSetID("turn-2")]
	require.Empty(t, firstSet.OverlapIntervals, "a rejected correction must not be published")
	require.Empty(t, secondSet.OverlapIntervals)
}
