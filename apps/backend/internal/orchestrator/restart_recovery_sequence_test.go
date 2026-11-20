package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// Reproduces the evidenced sequence: a restart with a turn in flight, a manual
// recover while the guard is held, then another message for the session. The
// session ends in the state it was in, with no message lost.
func TestRestartSequenceDetachedTurnGuardedRecoverAndQueuedMessages(t *testing.T) {
	{
		retryable := true
		t.Run("retryable guard", func(t *testing.T) {
			service, repo, manager, session, store := newSilentRestoreEligibilityFixture(t, "provider-session", "sequence")
			ctx := context.Background()
			fixture := &silentRestoreCheckpointFixture{ctx: ctx, repo: repo, session: session}
			seedDetachedTurn(t, service, fixture)
			service.agentManager = &ownerMismatchRecoverer{mockAgentManager: manager, result: lifecycle.DeliveryReconciliationResult{
				Outcome: lifecycle.DeliveryReconciliationBlocked, Reason: "delivery_evidence_unavailable",
				Err: evidencedOwnerMismatchError(),
			}}

			// 1. Restart with a turn in flight: the 409 OWNER_MISMATCH settles the turn.
			service.runSilentRestoreRecoveryPass(ctx, store)
			require.Zero(t, openTurnCount(t, repo, session.ID))

			// 2. The session is waiting with two queued messages.
			require.NoError(t, repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateWaitingForInput, ""))
			service.messageQueue = newAuthoritativeMemoryQueue(repo, testLogger())
			for _, content := range []string{"queued one", "queued two"} {
				_, err := service.messageQueue.QueueMessage(ctx, session.ID, session.TaskID, content, "", messagequeue.QueuedByUser, false, nil)
				require.NoError(t, err)
			}
			queueBefore := service.messageQueue.GetStatus(ctx, session.ID)
			require.NotZero(t, queueBefore.Count)

			// 3. Manual recover while the recovery guard refuses the launch.
			guard := &SessionRecoveryGuardError{SessionID: session.ID, Retryable: retryable, Cause: lifecycle.ErrSessionRecoveryGuarded}
			mock := manager
			mock.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				return nil, guard
			}
			_, err := service.RecoverSession(ctx, session.TaskID, session.ID, "resume")
			require.Error(t, err)
			require.True(t, errors.Is(err, lifecycle.ErrSessionRecoveryGuarded) || isRetryableSessionRecoveryGuardError(err),
				"the refusal is reported to the caller: %v", err)
			after, err := repo.GetTaskSession(ctx, session.ID)
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateWaitingForInput, after.State, "a refused recover leaves the session state alone")
			require.Equal(t, queueBefore, service.messageQueue.GetStatus(ctx, session.ID), "queued messages are untouched")

			// 4. A peer message arriving now is queued behind them, not dropped.
			_, err = service.messageQueue.QueueMessage(ctx, session.ID, session.TaskID, "peer message", "", messagequeue.QueuedByAgent, false, nil)
			require.NoError(t, err)
			var queued []string
			for _, entry := range service.messageQueue.GetStatus(ctx, session.ID).Entries {
				queued = append(queued, entry.Content)
			}
			require.Contains(t, strings.Join(queued, "\n"), "queued one")
			require.Contains(t, strings.Join(queued, "\n"), "queued two")
			require.Contains(t, strings.Join(queued, "\n"), "peer message")
		})
	}
}

// The unstoppable-agent refusal needs another restart, so it stays a visible
// FAILED session; both refusals are recognized so a peer message is kept.
func TestRecoveryGuardRefusalPredicates(t *testing.T) {
	retryable := fmt.Errorf("resume: %w", &SessionRecoveryGuardError{Retryable: true})
	unstoppable := &SessionRecoveryGuardError{Retryable: false}
	require.True(t, executor.IsRecoveryGuardRefusal(retryable))
	require.True(t, executor.IsRecoveryGuardRefusal(unstoppable))
	require.True(t, executor.IsRetryableRecoveryGuardRefusal(retryable))
	require.False(t, executor.IsRetryableRecoveryGuardRefusal(unstoppable))
	require.False(t, executor.IsRecoveryGuardRefusal(errors.New("other")))
}
