package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestReapPromptUnreadyExecution_OwnCleanupAllowsLaterRecovery(t *testing.T) {
	for _, failFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(failFirst), func(t *testing.T) {
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task", "session", models.TaskSessionStateWaitingForInput)
			var calls int
			stopErr := errors.New("teardown unavailable")
			manager := &mockAgentManager{
				getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return "execution", nil },
				stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
					calls++
					if failFirst && calls == 1 {
						return stopErr
					}
					return nil
				},
			}
			svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)
			err := svc.reapPromptUnreadyExecution(t.Context(), "session", errors.New("not ready"))
			if failFirst {
				require.ErrorIs(t, err, stopErr)
				require.NoError(t, svc.reapPromptUnreadyExecution(t.Context(), "session", errors.New("retry")))
			} else {
				require.NoError(t, err)
			}
			require.True(t, svc.cleanupAgentExecutionWithReason(t.Context(), "execution", "task", "session", "recovery"), "successful exact cleanup must permit workflow recovery")
			expected := 1
			if failFirst {
				expected++
			}
			require.Equal(t, expected, calls, "completed cleanup must not repeat teardown")
		})
	}
}

// Probe the real admission locks at the state-read boundary. If admission can
// enter, run the production prompt claim and let it adopt the captured turn.
func TestStopSession_ExecutionLessSettlementSerializesPromptAdmission(t *testing.T) {
	for _, boundary := range []string{"lifecycle", "prompt"} {
		t.Run(boundary, func(t *testing.T) {
			svc, repo, _, sessionID, originalTurn := newExecutionLessStopService(t, models.TaskSessionStateWaitingForInput)
			var admitted bool
			var once atomic.Bool
			svc.repo = &coordinatorStopRepoHooks{repoStore: repo, getSessionFunc: func(ctx context.Context, id string) (*models.TaskSession, error) {
				snapshot, err := repo.GetTaskSession(ctx, id)
				if err != nil || !once.CompareAndSwap(false, true) {
					return snapshot, err
				}
				var release func()
				if boundary == "lifecycle" {
					var acquired bool
					release, acquired = svc.tryAcquireSessionLifecycleLock(id)
					if !acquired {
						return snapshot, nil
					}
				} else {
					lock, cleanup := svc.acquireCancelInFlightGuard(id)
					if !lock.TryLock() {
						cleanup()
						return snapshot, nil
					}
					release = func() { lock.Unlock(); cleanup() }
				}
				release()
				_, _, turnID, _, _, done, claimErr := svc.claimSessionRunningForPrompt(ctx, snapshot.TaskID, id, "", false, nil, nil, "", false)
				require.NoError(t, claimErr)
				if done != nil {
					done()
				}
				require.Equal(t, originalTurn, turnID, "the prompt reuses the captured turn")
				admitted = true
				return snapshot, nil
			}}
			require.NoError(t, svc.StopSession(t.Context(), sessionID, "stop", false))
			if admitted {
				active, err := repo.GetActiveTurnBySessionID(t.Context(), sessionID)
				require.NoError(t, err, "a concurrently admitted prompt must keep its turn")
				require.Equal(t, originalTurn, active.ID)
			} else {
				// Once settlement releases admission, a prompt creates a distinct turn.
				_, _, turnID, _, _, done, err := svc.claimSessionRunningForPrompt(t.Context(), "task-execution-less-stop", sessionID, "", false, nil, nil, "", false)
				require.NoError(t, err)
				if done != nil {
					done()
				}
				require.NotEmpty(t, turnID)
				require.NotEqual(t, originalTurn, turnID)
			}
		})
	}
}

func TestStopSession_ExecutionAppearsAfterInitialAbsence(t *testing.T) {
	svc, repo, _, sessionID, turnID := newExecutionLessStopService(t, models.TaskSessionStateWaitingForInput)
	var reads int
	svc.agentManager.(*mockAgentManager).getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
		reads++
		if reads == 1 {
			return "", lifecycle.ErrNoExecutionForSession
		}
		return "successor-execution", nil
	}
	require.ErrorIs(t, svc.StopSession(t.Context(), sessionID, "stop", false), lifecycle.ErrNoExecutionForSession)
	active, err := repo.GetActiveTurnBySessionID(t.Context(), sessionID)
	require.NoError(t, err)
	require.Equal(t, turnID, active.ID)
}

type settlementTurnFailure struct {
	TurnService
	lookupErr   error
	completeErr error
}

func (s settlementTurnFailure) GetActiveTurn(ctx context.Context, id string) (*models.Turn, error) {
	if s.lookupErr != nil {
		return nil, s.lookupErr
	}
	return s.TurnService.GetActiveTurn(ctx, id)
}

func (s settlementTurnFailure) CompleteTurn(ctx context.Context, id string) error {
	if s.completeErr != nil {
		return s.completeErr
	}
	return s.TurnService.CompleteTurn(ctx, id)
}

func TestStopSession_ExecutionLessSettlementStateAndFailureMatrix(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateWaitingForInput, models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed, models.TaskSessionStateCancelled, models.TaskSessionStateIdle,
	} {
		t.Run(string(state), func(t *testing.T) {
			svc, repo, _, sessionID, turnID := newExecutionLessStopService(t, state)
			err := svc.StopSession(t.Context(), sessionID, "stop", true)
			if state == models.TaskSessionStateIdle {
				require.ErrorIs(t, err, lifecycle.ErrNoExecutionForSession)
				active, lookupErr := repo.GetActiveTurnBySessionID(t.Context(), sessionID)
				require.NoError(t, lookupErr)
				require.Equal(t, turnID, active.ID)
			} else {
				require.NoError(t, err)
				active, lookupErr := svc.turnService.GetActiveTurn(t.Context(), sessionID)
				require.NoError(t, lookupErr)
				require.Nil(t, active)
			}
			session, lookupErr := repo.GetTaskSession(t.Context(), sessionID)
			require.NoError(t, lookupErr)
			require.Equal(t, state, session.State)
		})
	}
	for _, boundary := range []string{"lookup", "complete", "session"} {
		t.Run(boundary, func(t *testing.T) {
			svc, repo, _, sessionID, turnID := newExecutionLessStopService(t, models.TaskSessionStateWaitingForInput)
			failure := errors.New("settlement store unavailable")
			switch boundary {
			case "lookup":
				svc.turnService = settlementTurnFailure{TurnService: svc.turnService, lookupErr: failure}
			case "complete":
				svc.turnService = settlementTurnFailure{TurnService: svc.turnService, completeErr: failure}
			case "session":
				svc.repo = &coordinatorStopRepoHooks{repoStore: repo, getSessionFunc: func(context.Context, string) (*models.TaskSession, error) { return nil, failure }}
			}
			require.ErrorIs(t, svc.StopSession(t.Context(), sessionID, "stop", false), failure)
			active, err := repo.GetActiveTurnBySessionID(t.Context(), sessionID)
			require.NoError(t, err)
			require.Equal(t, turnID, active.ID)
		})
	}
}

func TestStopSession_ExecutionLessSettlementRefusesLifecycleOwner(t *testing.T) {
	svc, repo, _, sessionID, turnID := newExecutionLessStopService(t, models.TaskSessionStateWaitingForInput)
	release := svc.acquireSessionLifecycleLock(sessionID)
	defer release()
	require.ErrorIs(t, svc.StopSession(t.Context(), sessionID, "stop", false), lifecycle.ErrNoExecutionForSession)
	active, err := repo.GetActiveTurnBySessionID(t.Context(), sessionID)
	require.NoError(t, err)
	require.Equal(t, turnID, active.ID)
}

func TestStopSession_ExecutionLessSettlementPreservesSuccessorTurn(t *testing.T) {
	svc, repo, _, sessionID, original := newExecutionLessStopService(t, models.TaskSessionStateWaitingForInput)
	var successor *models.Turn
	var once atomic.Bool
	svc.repo = &coordinatorStopRepoHooks{repoStore: repo, getSessionFunc: func(ctx context.Context, id string) (*models.TaskSession, error) {
		session, err := repo.GetTaskSession(ctx, id)
		if err == nil && once.CompareAndSwap(false, true) {
			require.NoError(t, repo.CompleteTurn(ctx, original))
			successor, err = svc.turnService.StartTurn(ctx, id)
			require.NoError(t, err)
		}
		return session, err
	}}
	require.Error(t, svc.StopSession(t.Context(), sessionID, "stop", false))
	active, err := repo.GetActiveTurnBySessionID(t.Context(), sessionID)
	require.NoError(t, err)
	require.Equal(t, successor.ID, active.ID)
}

func TestStopSession_ExecutionLessSettlementRefusesActiveOperation(t *testing.T) {
	for _, kind := range []string{"cancellation", "resume"} {
		t.Run(kind, func(t *testing.T) {
			svc, repo, _, sessionID, turnID := newExecutionLessStopService(t, models.TaskSessionStateWaitingForInput)
			if kind == "cancellation" {
				operation, owner := svc.claimCancellation(sessionID, cancellationKindExplicit)
				require.True(t, owner)
				defer svc.finishCancellation(sessionID, operation, nil)
			} else {
				attempt, owner := svc.resumeAttemptStore().begin(t.Context(), "task-execution-less-stop", sessionID)
				require.True(t, owner)
				defer attempt.finish(svc.resumeAttemptStore())
			}
			require.ErrorIs(t, svc.StopSession(t.Context(), sessionID, "stop", false), lifecycle.ErrNoExecutionForSession)
			active, err := repo.GetActiveTurnBySessionID(t.Context(), sessionID)
			require.NoError(t, err)
			require.Equal(t, turnID, active.ID)
		})
	}
}

func TestStopSession_ExecutionLessRuntimeRecheckPropagatesError(t *testing.T) {
	svc, repo, _, sessionID, turnID := newExecutionLessStopService(t, models.TaskSessionStateWaitingForInput)
	var calls int
	failure := errors.New("runtime registry unavailable")
	svc.agentManager.(*mockAgentManager).getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
		calls++
		if calls == 1 {
			return "", lifecycle.ErrNoExecutionForSession
		}
		return "", failure
	}
	require.ErrorIs(t, svc.StopSession(t.Context(), sessionID, "stop", false), failure)
	active, err := repo.GetActiveTurnBySessionID(t.Context(), sessionID)
	require.NoError(t, err)
	require.Equal(t, turnID, active.ID)
}
