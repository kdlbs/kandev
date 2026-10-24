package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type passthroughResumeReadinessManager struct {
	*mockAgentManager
	ready func(string, string, string) bool
}

type passthroughReadinessTurnService struct {
	*inactiveTurnService
	turn *models.Turn
	err  error
}

func (s *passthroughReadinessTurnService) GetActiveTurn(context.Context, string) (*models.Turn, error) {
	return s.turn, s.err
}

func TestPassthroughResumeReadinessRequiresDurableIdleEvidence(t *testing.T) {
	for _, condition := range []string{"missing service", "active durable turn", "lookup failure"} {
		t.Run(condition, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)
			seedExecutorRunning(t, repo, "session1", "task1", "exec-1")
			manager := &passthroughResumeReadinessManager{mockAgentManager: &mockAgentManager{repoForExecutionLookup: repo}}
			manager.ready = func(string, string, string) bool {
				t.Fatal("runtime evidence cannot bypass missing durable idle evidence")
				return true
			}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			attempt, _, err := svc.beginResumeAttempt(context.Background(), "task1", "session1")
			require.NoError(t, err)
			attempt.setExecutionID("exec-1")
			lookupErr := errors.New("turn lookup unavailable")
			switch condition {
			case "missing service":
				svc.turnService = nil
			case "active durable turn":
				svc.turnService = &passthroughReadinessTurnService{turn: &models.Turn{ID: "active"}}
			case "lookup failure":
				svc.turnService = &passthroughReadinessTurnService{err: lookupErr}
			}
			ready, err := svc.reconcilePassthroughResumeReady(cancellableResumeContext(attempt), "session1")
			require.False(t, ready)
			if condition == "lookup failure" {
				require.ErrorIs(t, err, lookupErr)
			} else {
				require.NoError(t, err)
			}
			session, err := repo.GetTaskSession(context.Background(), "session1")
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateStarting, session.State)
		})
	}
}

func TestWaitForSessionReadyAdoptsIdleTerminalWithoutCompletion(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-1")
	manager := &passthroughResumeReadinessManager{mockAgentManager: &mockAgentManager{repoForExecutionLookup: repo}}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.turnService = &inactiveTurnService{}
	attempt, _, err := svc.beginResumeAttempt(context.Background(), "task1", "session1")
	require.NoError(t, err)
	attempt.setExecutionID("exec-1")
	manager.ready = func(string, string, string) bool { return true }
	ctx, cancel := context.WithTimeout(cancellableResumeContext(attempt), 5*time.Second)
	defer cancel()
	require.NoError(t, svc.waitForSessionReady(ctx, "session1"))
	require.True(t, svc.activeResumeAttemptOwnsStartup("session1", "exec-1", attempt.identity()), "readiness is not prompt acceptance")
}

func (m *passthroughResumeReadinessManager) IsLocalPassthroughReadyForResume(sessionID, executionID, attemptID string) bool {
	return m.ready(sessionID, executionID, attemptID)
}

func TestReconcilePassthroughResumeReadyPreservesPromptOwnership(t *testing.T) {
	for _, condition := range []string{"idle", "busy", "running", "accepted", "cancelled", "replaced", "active turn", "reserved turn", "no attempt"} {
		t.Run(condition, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)
			seedExecutorRunning(t, repo, "session1", "task1", "exec-1")
			manager := &passthroughResumeReadinessManager{mockAgentManager: &mockAgentManager{isPassthrough: true, isAgentRunning: true, repoForExecutionLookup: repo}}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			svc.turnService = &inactiveTurnService{}
			attempt, owner, err := svc.beginResumeAttempt(context.Background(), "task1", "session1")
			require.NoError(t, err)
			require.True(t, owner)
			attempt.setExecutionID("exec-1")
			ctx := cancellableResumeContext(attempt)
			manager.ready = func(sessionID, executionID, attemptID string) bool {
				assertPassthroughAdmissionHeld(t, svc, sessionID)
				require.Equal(t, "exec-1", executionID)
				require.Equal(t, attempt.identity(), attemptID)
				return condition != "busy"
			}
			switch condition {
			case "running":
				require.NoError(t, repo.UpdateTaskSessionState(ctx, "session1", models.TaskSessionStateRunning, ""))
			case "accepted":
				svc.acceptResumeAttemptAtPromptAcceptance("exec-1", attempt)
			case "cancelled":
				svc.invalidateResumeAttempt("session1")
			case "replaced":
				attempt.setExecutionID("exec-2")
			case "active turn":
				svc.activeTurns.Store("session1", "turn-current")
			case "reserved turn":
				svc.reservedPromptTurns.Store("session1", newReservedPromptTurn("turn-reserved"))
			case "no attempt":
				ctx = context.Background()
			}
			reconciled, err := svc.reconcilePassthroughResumeReady(ctx, "session1")
			if condition == "cancelled" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, condition == "idle", reconciled)
			session, err := repo.GetTaskSession(context.Background(), "session1")
			require.NoError(t, err)
			expected := models.TaskSessionStateStarting
			if condition == "idle" {
				expected = models.TaskSessionStateWaitingForInput
			}
			if condition == "running" {
				expected = models.TaskSessionStateRunning
			}
			require.Equal(t, expected, session.State)
		})
	}
}
