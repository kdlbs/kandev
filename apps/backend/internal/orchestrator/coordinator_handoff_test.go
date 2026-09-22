package orchestrator

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestHandoffCoordinatorPrimaryPreservesFIFOAndFencesPredecessor(t *testing.T) {
	ctx := context.Background()
	svc, repo, queue := newCoordinatorHandoffFixture(t)
	first, err := queue.QueueMessage(ctx, "session-old", "task", "first", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	second, err := queue.QueueMessage(ctx, "session-old", "task", "second", "gpt-5.6-terra", messagequeue.QueuedByAgent, false, nil)
	require.NoError(t, err)

	request := coordinatorHandoffFixtureRequest("handoff-1")
	result, err := svc.HandoffCoordinatorPrimary(ctx, request)
	require.NoError(t, err)
	require.True(t, result.Changed)
	require.Equal(t, "gpt-5.6-terra", result.EffectiveModel)
	require.Equal(t, []string{first.ID, second.ID}, result.QueueEntryIDs)
	require.NotEmpty(t, result.QueueDigest)
	require.Equal(t, "session-new", result.AutomationTarget)

	primary, err := repo.GetPrimarySessionByTaskID(ctx, "task")
	require.NoError(t, err)
	require.Equal(t, "session-new", primary.ID)
	plan, err := repo.GetTaskPlan(ctx, "task")
	require.NoError(t, err)
	require.Equal(t, "durable coordinator plan", plan.Content)
	predecessor, err := repo.GetTaskSession(ctx, "session-old")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionRouteStateCoordinatorHandoffFenced, predecessor.RouteState)
	require.Equal(t, "handoff-1", predecessor.RouteReason)
	require.Empty(t, queue.GetStatus(ctx, "session-old").Entries)
	destination := queue.GetStatus(ctx, "session-new").Entries
	require.Equal(t, []string{"first", "second"}, []string{destination[0].Content, destination[1].Content})

	replayed, err := svc.HandoffCoordinatorPrimary(ctx, request)
	require.NoError(t, err)
	require.False(t, replayed.Changed)
	require.Equal(t, result.QueueEntryIDs, replayed.QueueEntryIDs)

	_, err = svc.HandoffCoordinatorPrimary(ctx, coordinatorHandoffFixtureRequest("stale-operation"))
	require.ErrorIs(t, err, ErrCoordinatorHandoffConflict)
}

func TestHandoffCoordinatorPrimaryFailureRetainsPredecessor(t *testing.T) {
	ctx := context.Background()
	svc, repo, queue := newCoordinatorHandoffFixture(t)
	_, err := queue.QueueMessage(ctx, "session-new", "task", "unexpected successor work", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)

	_, err = svc.HandoffCoordinatorPrimary(ctx, coordinatorHandoffFixtureRequest("handoff-fails"))
	require.ErrorIs(t, err, ErrCoordinatorHandoffConflict)
	primary, primaryErr := repo.GetPrimarySessionByTaskID(ctx, "task")
	require.NoError(t, primaryErr)
	require.Equal(t, "session-old", primary.ID)
	predecessor, predecessorErr := repo.GetTaskSession(ctx, "session-old")
	require.NoError(t, predecessorErr)
	require.Empty(t, predecessor.RouteState)
}

func TestCoordinatorHandoffRepositoryRollbackRestoresPredecessor(t *testing.T) {
	ctx := context.Background()
	_, repo, _ := newCoordinatorHandoffFixture(t)
	request := coordinatorHandoffFixtureRequest("handoff-rollback")

	changed, err := repo.PromoteCoordinatorSuccessor(ctx, request.TaskID,
		request.PredecessorSessionID, request.SuccessorSessionID,
		request.PredecessorQueueIncarnationID, request.SuccessorQueueIncarnationID,
		request.OperationID)
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, repo.RollbackCoordinatorSuccessor(ctx, request.TaskID,
		request.PredecessorSessionID, request.SuccessorSessionID,
		request.PredecessorQueueIncarnationID, request.SuccessorQueueIncarnationID,
		request.OperationID))

	primary, err := repo.GetPrimarySessionByTaskID(ctx, request.TaskID)
	require.NoError(t, err)
	require.Equal(t, request.PredecessorSessionID, primary.ID)
	require.Empty(t, primary.RouteState)
	successor, err := repo.GetTaskSession(ctx, request.SuccessorSessionID)
	require.NoError(t, err)
	require.False(t, successor.IsPrimary)

	// The rollback is operation-owned and cannot be replayed or stolen.
	err = repo.RollbackCoordinatorSuccessor(ctx, request.TaskID,
		request.PredecessorSessionID, request.SuccessorSessionID,
		request.PredecessorQueueIncarnationID, request.SuccessorQueueIncarnationID,
		"another-operation")
	require.ErrorIs(t, err, models.ErrCoordinatorHandoffConflict)
}

func newCoordinatorHandoffFixture(t *testing.T, withReceipt ...bool) (*Service, *tasksqlite.Repository, *messagequeue.Service) {
	t.Helper()
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "coordinator-handoff.db"))
	require.NoError(t, err)
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	repo, err := tasksqlite.NewWithDB(db, db, nil)
	require.NoError(t, err)
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	require.NoError(t, err)
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	now := time.Now().UTC().Round(0)
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "workspace", Name: "Workspace"}))
	require.NoError(t, repo.CreateTask(context.Background(), &models.Task{ID: "task", WorkspaceID: "workspace", Title: "Coordinator"}))
	require.NoError(t, repo.CreateTaskPlan(context.Background(), &models.TaskPlan{
		ID: "plan", TaskID: "task", Title: "Coordinator plan", Content: "durable coordinator plan",
	}))
	for _, session := range []*models.TaskSession{
		{ID: "session-old", TaskID: "task", QueueIncarnationID: "inc-old", State: models.TaskSessionStateRunning, AgentProfileID: "profile-cheap"},
		{ID: "session-new", TaskID: "task", QueueIncarnationID: "inc-new", State: models.TaskSessionStateWaitingForInput, AgentProfileID: "profile-cheap"},
	} {
		require.NoError(t, repo.CreateTaskSession(context.Background(), session))
	}
	require.NoError(t, repo.SetSessionPrimary(context.Background(), "session-old"))
	_, err = repo.AssignExactProfileAssignment(context.Background(), &models.ExactProfileAssignment{
		TaskID: "task", WorkspaceID: "workspace", AgentProfileID: "profile-cheap",
		ProfileRevision: now, Generation: 1,
	})
	require.NoError(t, err)
	if len(withReceipt) > 0 && !withReceipt[0] {
		return &Service{repo: repo, messageQueue: queue}, repo, queue
	}
	_, err = repo.RecordExactProfileLaunchReceipt(context.Background(), &models.ExactProfileLaunchReceipt{
		TaskID: "task", SessionID: "session-new", AgentProfileID: "profile-cheap",
		ProfileRevision: now, Generation: 1, Model: "gpt-5.6-terra",
		Outcome: models.ExactProfileLaunchOutcomeApplied, InferenceStarted: true,
	})
	require.NoError(t, err)
	return &Service{repo: repo, messageQueue: queue}, repo, queue
}

func TestHandoffCoordinatorPrimaryRejectsBootReadyWithoutInferenceReceipt(t *testing.T) {
	ctx := context.Background()
	svc, repo, queue := newCoordinatorHandoffFixture(t, false)
	queued, err := queue.QueueMessage(ctx, "session-old", "task", "retain", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	_, err = svc.HandoffCoordinatorPrimary(ctx, coordinatorHandoffFixtureRequest("boot-ready-no-inference"))
	require.ErrorIs(t, err, ErrCoordinatorSuccessorModel)
	primary, err := repo.GetPrimarySessionByTaskID(ctx, "task")
	require.NoError(t, err)
	require.Equal(t, "session-old", primary.ID)
	require.Empty(t, queue.GetStatus(ctx, "session-new").Entries)
	require.Equal(t, []string{queued.ID}, []string{queue.GetStatus(ctx, "session-old").Entries[0].ID})
}

func TestHandoffCoordinatorPrimaryRejectsFailedClosedReceipt(t *testing.T) {
	ctx := context.Background()
	svc, repo, queue := newCoordinatorHandoffFixture(t, false)
	_, err := repo.RecordExactProfileLaunchReceipt(ctx, &models.ExactProfileLaunchReceipt{
		TaskID: "task", SessionID: "session-new", AgentProfileID: "profile-cheap",
		Generation: 1, ProfileRevision: time.Unix(1_726_500_000, 0).UTC(), Model: "gpt-5.6-terra",
		Outcome: models.ExactProfileLaunchOutcomeFailedClosed, FailureReason: "startup failed",
	})
	require.NoError(t, err)
	queued, err := queue.QueueMessage(ctx, "session-old", "task", "retain", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)

	_, err = svc.HandoffCoordinatorPrimary(ctx, coordinatorHandoffFixtureRequest("failed-closed"))
	require.ErrorIs(t, err, ErrCoordinatorSuccessorModel)
	primary, err := repo.GetPrimarySessionByTaskID(ctx, "task")
	require.NoError(t, err)
	require.Equal(t, "session-old", primary.ID)
	require.Empty(t, queue.GetStatus(ctx, "session-new").Entries)
	require.Equal(t, []string{queued.ID}, []string{queue.GetStatus(ctx, "session-old").Entries[0].ID})
}

func coordinatorHandoffFixtureRequest(operationID string) CoordinatorHandoffRequest {
	return CoordinatorHandoffRequest{
		TaskID: "task", PredecessorSessionID: "session-old", SuccessorSessionID: "session-new",
		PredecessorQueueIncarnationID: "inc-old", SuccessorQueueIncarnationID: "inc-new",
		ExpectedAgentProfileID: "profile-cheap", ExpectedModel: "gpt-5.6-terra", OperationID: operationID,
	}
}

func TestCoordinatorHandoffRejectsUnverifiedModel(t *testing.T) {
	svc, repo, _ := newCoordinatorHandoffFixture(t)
	receipt, err := repo.GetExactProfileLaunchReceipt(context.Background(), "task", "session-new")
	require.NoError(t, err)
	require.NotNil(t, receipt)
	request := coordinatorHandoffFixtureRequest("wrong-model")
	request.ExpectedModel = "gpt-6-astra"
	_, err = svc.HandoffCoordinatorPrimary(context.Background(), request)
	require.True(t, errors.Is(err, ErrCoordinatorSuccessorModel))
}
