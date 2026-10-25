package sqlite

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

func allocationRouteFixture(t *testing.T, repo *Repository) models.WorkflowSessionRoute {
	t.Helper()
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: "allocation-task", Title: "Allocation", WorkflowStepID: "step-a"}))
	entry, err := repo.GetLatestTaskStepTransitionID(t.Context(), "allocation-task")
	require.NoError(t, err)
	return models.WorkflowSessionRoute{OperationID: "accepted-allocation", EntryIdentity: fmt.Sprintf("entry:%020d", entry), DestinationStepID: "step-a", TargetKind: "profile", AgentProfileID: "profile-a", SourceSessionID: "original", Phase: "prepared"}
}

func TestWorkflowAllocationPreservesRecordedDestinationStates(t *testing.T) {
	for _, state := range []models.TaskSessionState{models.TaskSessionStateCreated, models.TaskSessionStateFailed, models.TaskSessionStateCancelled, models.TaskSessionStateCompleted} {
		t.Run(string(state), func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			route := allocationRouteFixture(t, repo)
			ctx := models.WithExecutorActor(t.Context(), "allocation-actor")
			first := &models.TaskSession{ID: "first", TaskID: "allocation-task", AgentProfileID: "profile-a", State: state}
			require.NoError(t, repo.CreateTaskSessionWithWorkflowSessionRoute(ctx, first, &route))
			before, err := repo.GetTask(ctx, first.TaskID)
			require.NoError(t, err)
			err = repo.CreateTaskSessionWithWorkflowSessionRoute(ctx, &models.TaskSession{ID: "duplicate", TaskID: first.TaskID}, &route)
			require.ErrorIs(t, err, repoerrors.ErrWorkflowAllocationRecorded)
			rows, err := repo.ListTaskSessions(ctx, first.TaskID)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Equal(t, state, rows[0].State)
			require.Equal(t, "allocation-actor", rows[0].ExecutorActorUserID)
			after, err := repo.GetTask(ctx, first.TaskID)
			require.NoError(t, err)
			require.Equal(t, before.Metadata, after.Metadata)
		})
	}
}

func TestWorkflowAllocationRejectsStaleEntryAfterRevisit(t *testing.T) {
	repo := newRepoForSessionTests(t)
	route := allocationRouteFixture(t, repo)
	ctx := t.Context()
	task, err := repo.GetTask(ctx, "allocation-task")
	require.NoError(t, err)
	task.WorkflowStepID = "step-b"
	require.NoError(t, repo.UpdateTask(ctx, task))
	task, err = repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	task.WorkflowStepID = "step-a"
	require.NoError(t, repo.UpdateTask(ctx, task))
	err = repo.CreateTaskSessionWithWorkflowSessionRoute(ctx, &models.TaskSession{ID: "stale", TaskID: task.ID}, &route)
	require.ErrorIs(t, err, repoerrors.ErrWorkflowAllocationStaleEntry)
	rows, err := repo.ListTaskSessions(ctx, task.ID)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestWorkflowAllocationRejectsStaleDestinationStep(t *testing.T) {
	repo := newRepoForSessionTests(t)
	route := allocationRouteFixture(t, repo)
	task, err := repo.GetTask(t.Context(), "allocation-task")
	require.NoError(t, err)
	task.WorkflowStepID = "step-b"
	require.NoError(t, repo.UpdateTask(t.Context(), task))
	err = repo.CreateTaskSessionWithWorkflowSessionRoute(t.Context(), &models.TaskSession{ID: "stale", TaskID: task.ID}, &route)
	require.ErrorIs(t, err, repoerrors.ErrWorkflowAllocationStaleEntry)
	rows, err := repo.ListTaskSessions(t.Context(), task.ID)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestWorkflowAllocationConcurrentTransactions(t *testing.T) {
	verifyWorkflowAllocationConcurrentTransactions(t, newRepoForSessionTests(t))
}

func verifyWorkflowAllocationConcurrentTransactions(t *testing.T, repo *Repository) {
	t.Helper()
	route := allocationRouteFixture(t, repo)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- repo.CreateTaskSessionWithWorkflowSessionRoute(context.Background(), &models.TaskSession{ID: fmt.Sprintf("candidate-%d", i), TaskID: "allocation-task", State: models.TaskSessionStateCreated}, &route)
		}()
	}
	close(start)
	wg.Wait()
	first, second := <-errs, <-errs
	require.NotEqual(t, first == nil, second == nil, "exactly one allocation must commit")
	if first != nil {
		require.ErrorIs(t, first, repoerrors.ErrWorkflowAllocationRecorded)
	} else {
		require.ErrorIs(t, second, repoerrors.ErrWorkflowAllocationRecorded)
	}
	rows, err := repo.ListTaskSessions(t.Context(), "allocation-task")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	task, err := repo.GetTask(t.Context(), "allocation-task")
	require.NoError(t, err)
	recorded, ok := models.LoadWorkflowSessionRoute(task.Metadata)
	require.True(t, ok)
	require.Equal(t, rows[0].ID, recorded.DestinationID)
}
