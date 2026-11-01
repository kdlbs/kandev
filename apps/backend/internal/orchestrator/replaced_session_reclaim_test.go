package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestPrimaryReplacementStopsSettledOldExecution(t *testing.T) {
	ctx := context.Background()
	svc, _, row, stopped := newReplacedSessionFixture(t)
	require.NoError(t, svc.PromotePrimarySession(ctx, "replacement-session"))
	requireNoReplacedStop(t, stopped, "promotion must not stop the old runtime inline")
	svc.reclaimReplacedSessionsOnce(ctx)
	awaitReplacedExecutionStop(t, stopped, row.AgentExecutionID)
	primary, err := svc.repo.GetTaskSession(ctx, "replacement-session")
	require.NoError(t, err)
	require.True(t, primary.IsPrimary)
	require.Equal(t, models.TaskSessionStateWaitingForInput, primary.State)

	// The stop writes no session state, and the resulting agent.stopped event
	// keeps the old session resumable instead of cancelling it.
	svc.handleAgentStopped(ctx, watcher.AgentEventData{
		TaskID: row.TaskID, SessionID: row.SessionID, AgentExecutionID: row.AgentExecutionID,
	})
	old, err := svc.repo.GetTaskSession(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, old.State)
	requireNoReplacedClaim(t, svc, row.SessionID)
}

func newReplacedSessionFixture(t *testing.T) (*Service, *idleParkingTestAgent, *models.ExecutorRunning, chan string) {
	t.Helper()
	ctx := context.Background()
	svc, agent, row := newIdleParkingFixture(t, true, 60, "codex-acp")
	svc.config.ReplacedSessionReclaim = true
	require.NoError(t, svc.repo.SetSessionPrimary(ctx, row.SessionID))
	require.NoError(t, svc.repo.(*sqliterepo.Repository).CreateTaskSession(ctx, &models.TaskSession{
		ID: "replacement-session", TaskID: row.TaskID, State: models.TaskSessionStateWaitingForInput,
		StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}))
	agent.repoForExecutionLookup = svc.repo
	svc.executor = executor.NewExecutor(agent, svc.repo.(*sqliterepo.Repository), testLogger(), executor.ExecutorConfig{})
	stopped := make(chan string, 2)
	agent.stopAgentWithReasonFunc = func(_ context.Context, executionID, _ string, _ bool) error {
		stopped <- executionID
		return nil
	}
	beforePrimary, beforeErr := svc.primarySessionID(ctx, row.TaskID)
	require.NoError(t, beforeErr)
	require.Equal(t, row.SessionID, beforePrimary)
	return svc, agent, row, stopped
}

func TestPrimaryReplacementLeavesOldExecutionWhenReclaimDisabled(t *testing.T) {
	ctx := context.Background()
	svc, _, row, stopped := newReplacedSessionFixture(t)
	svc.config.ReplacedSessionReclaim = false
	require.NoError(t, svc.PromotePrimarySession(ctx, "replacement-session"))
	svc.reclaimReplacedSessionsOnce(ctx)
	requireNoReplacedStop(t, stopped, "reclaim disabled but the old execution was stopped")
	requireNoReplacedClaim(t, svc, row.SessionID)
	primary, err := svc.repo.GetTaskSession(ctx, "replacement-session")
	require.NoError(t, err)
	require.True(t, primary.IsPrimary)
}

// Automatic promotion, deletes, and workflow switches share SetPrimarySession.
// They must not touch the old session's runtime even with the flag on.
func TestAutomaticPrimaryPromotionDoesNotReclaim(t *testing.T) {
	ctx := context.Background()
	svc, _, row, stopped := newReplacedSessionFixture(t)
	require.NoError(t, svc.SetPrimarySession(ctx, "replacement-session"))
	svc.reclaimReplacedSessionsOnce(ctx)
	requireNoReplacedStop(t, stopped, "automatic promotion stopped the old execution")
	requireNoReplacedClaim(t, svc, row.SessionID)
	primary, err := svc.repo.GetTaskSession(ctx, "replacement-session")
	require.NoError(t, err)
	require.True(t, primary.IsPrimary)
}

func TestQueueTransferDoesNotReclaimDonor(t *testing.T) {
	ctx := context.Background()
	svc, _, row, stopped := newReplacedSessionFixture(t)
	_, err := svc.messageQueue.QueueMessage(ctx, row.SessionID, row.TaskID, "successor prompt", "", "user", false, nil)
	require.NoError(t, err)
	require.NoError(t, svc.transferQueuedSessionState(ctx, row.TaskID, row.SessionID, "replacement-session"))
	svc.reclaimReplacedSessionsOnce(ctx)
	requireNoReplacedStop(t, stopped, "a queue transfer stopped the donor execution")
	requireNoReplacedClaim(t, svc, row.SessionID)
	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "replacement-session").Count)
}

func awaitReplacedExecutionStop(t *testing.T, stopped <-chan string, expected string) {
	t.Helper()
	select {
	case executionID := <-stopped:
		require.Equal(t, expected, executionID)
	case <-time.After(3 * time.Second):
		t.Fatal("demoted settled execution was not stopped")
	}
}

func requireNoReplacedStop(t *testing.T, stopped <-chan string, message string) {
	t.Helper()
	select {
	case executionID := <-stopped:
		t.Fatalf("%s: %s", message, executionID)
	default:
	}
}

func requireNoReplacedClaim(t *testing.T, svc *Service, sessionID string) {
	t.Helper()
	_, registered := svc.replacedSessionReclaims.Load(sessionID)
	require.False(t, registered, "replaced-session registration must not remain")
}

func TestPrimaryReplacementDefersUntilActiveTurnSettles(t *testing.T) {
	ctx := context.Background()
	svc, _, row, stopped := newReplacedSessionFixture(t)
	svc.turnService = &alwaysActiveTurnService{}
	require.NoError(t, svc.PromotePrimarySession(ctx, "replacement-session"))
	svc.reclaimReplacedSessionsOnce(ctx)
	requireNoReplacedStop(t, stopped, "replacement stopped an active turn")
	old, err := svc.repo.GetTaskSession(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, old.State)
	svc.turnService = &inactiveTurnService{}
	svc.reclaimReplacedSessionsOnce(ctx)
	awaitReplacedExecutionStop(t, stopped, row.AgentExecutionID)
}

func TestPrimaryReplacementDoesNotStopRotatedExecution(t *testing.T) {
	ctx := context.Background()
	svc, _, row, stopped := newReplacedSessionFixture(t)
	require.NoError(t, svc.PromotePrimarySession(ctx, "replacement-session"))
	row.AgentExecutionID = "rotated-execution"
	require.NoError(t, svc.repo.UpsertExecutorRunning(ctx, row))
	svc.reclaimReplacedSessionsOnce(ctx)
	requireNoReplacedStop(t, stopped, "stale replacement stopped a rotated execution")
	requireNoReplacedClaim(t, svc, row.SessionID)
}

func TestPrimaryReplacementDoesNotStopRepromotedSession(t *testing.T) {
	ctx := context.Background()
	svc, _, row, stopped := newReplacedSessionFixture(t)
	require.NoError(t, svc.PromotePrimarySession(ctx, "replacement-session"))
	require.NoError(t, svc.SetPrimarySession(ctx, row.SessionID))
	svc.reclaimReplacedSessionsOnce(ctx)
	requireNoReplacedStop(t, stopped, "stale replacement stopped a re-promoted primary")
	requireNoReplacedClaim(t, svc, row.SessionID)
	current, err := svc.repo.GetTaskSession(ctx, row.SessionID)
	require.NoError(t, err)
	require.True(t, current.IsPrimary)
}

func TestPrimaryReplacementAllowsCallerOwnedPromotionLock(t *testing.T) {
	ctx := context.Background()
	svc, _, row, stopped := newReplacedSessionFixture(t)
	release := svc.acquireSessionLifecycleLock("replacement-session")
	done := make(chan error, 1)
	go func() { done <- svc.PromotePrimarySession(ctx, "replacement-session") }()
	returned := false
	select {
	case err := <-done:
		require.NoError(t, err)
		returned = true
	case <-time.After(2 * time.Second):
	}
	release()
	if !returned {
		require.NoError(t, <-done)
	}
	require.True(t, returned, "primary promotion must not re-lock a caller-owned lifecycle mutex")
	svc.reclaimReplacedSessionsOnce(ctx)
	awaitReplacedExecutionStop(t, stopped, row.AgentExecutionID)
}

// A protected owner can never be stopped by the claim, so the registration is
// dropped instead of being rescanned on every tick.
func TestPrimaryReplacementPreservesProtectedOwner(t *testing.T) {
	ctx := context.Background()
	svc, _, row, stopped := newReplacedSessionFixture(t)
	task, err := svc.repo.GetTask(ctx, row.TaskID)
	require.NoError(t, err)
	task.Origin = models.TaskOriginCoordinator
	require.NoError(t, svc.repo.(*sqliterepo.Repository).UpdateTask(ctx, task))
	require.NoError(t, svc.PromotePrimarySession(ctx, "replacement-session"))
	_, registered := svc.replacedSessionReclaims.Load(row.SessionID)
	require.True(t, registered)
	svc.reclaimReplacedSessionsOnce(ctx)
	requireNoReplacedStop(t, stopped, "replacement stopped a protected coordinator owner")
	requireNoReplacedClaim(t, svc, row.SessionID)
	old, err := svc.repo.GetTaskSession(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, old.State)
}

// A workflow profile switch promotes the destination before it parks the
// source. With the flag on, that automatic promotion must leave the source's
// runtime to the switch so parking can claim it.
func TestProfileSwitchParkWithReclaimOnParksSource(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	fixture.svc.config.ReplacedSessionReclaim = true
	fixture.svc.turnService = &inactiveTurnService{}

	destination, err := fixture.svc.createNewSessionForStepWithEndPolicy(ctx, "t1", fixture.current, "profile-b", fixture.endPolicy)
	require.NoError(t, err)
	fixture.svc.reclaimReplacedSessionsOnce(ctx)

	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, source.State)
	require.False(t, source.IsPrimary)
	_, parked := models.LoadWorkflowParking(source.Metadata)
	require.True(t, parked, "source session must be parked by the switch")
	promoted, err := fixture.repo.GetTaskSession(ctx, destination.ID)
	require.NoError(t, err)
	require.True(t, promoted.IsPrimary)
	fixture.agentMgr.mu.Lock()
	defer fixture.agentMgr.mu.Unlock()
	require.Len(t, fixture.agentMgr.stopAgentArgs, 1, "only the park stop may reach the source execution")
}
