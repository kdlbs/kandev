package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestCancelAgent_GuardDeadlineSettlesOperation(t *testing.T) {
	t.Run("initial acquisition", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedTaskAndSession(t, repo, "task-deadline-initial", "session-deadline-initial", models.TaskSessionStateRunning)
		seedExecutorRunning(t, repo, "session-deadline-initial", "task-deadline-initial", "execution-deadline-initial")
		recorded := &recordingEventBus{}
		agent := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
		svc.eventBus = recorded
		operation := beginDeadlineTestCancellation(t, svc, "session-deadline-initial", cancellationKindExplicit)
		holder := svc.lockCancelInFlightGuard("session-deadline-initial")

		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- svc.runExplicitCancellationOwned(ctx, "session-deadline-initial", operation) }()
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			holder.release()
			t.Fatal("context deadline did not expire")
		}
		select {
		case err := <-done:
			require.ErrorIs(t, err, context.DeadlineExceeded)
		case <-time.After(time.Second):
			holder.release()
			err := <-done
			svc.finishCancellationWithActions(context.Background(), "session-deadline-initial", operation, err)
			t.Fatal("initial guard acquisition did not observe the operation deadline")
		}
		holder.release()
		svc.finishCancellationWithActions(ctx, "session-deadline-initial", operation, context.DeadlineExceeded)

		require.False(t, svc.CancellationPending("session-deadline-initial"))
		requireNoCancelGuardReferences(t, svc, "session-deadline-initial")
		require.Equal(t, int32(0), agent.cancelAgentCalls.Load(), "lifecycle cancellation must not start without the guard")
		_, revision := svc.CancellationPendingSnapshot("session-deadline-initial")
		require.Equal(t, uint64(2), revision)
		events := cancellationPendingEvents(recorded)
		require.Len(t, events, 2)
		require.Equal(t, true, events[0].event.Data.(map[string]interface{})["cancellation_pending"])
		require.Equal(t, uint64(1), events[0].event.Data.(map[string]interface{})["cancellation_revision"])
		require.Equal(t, false, events[1].event.Data.(map[string]interface{})["cancellation_pending"])
		require.Equal(t, uint64(2), events[1].event.Data.(map[string]interface{})["cancellation_revision"])
		require.NoError(t, svc.CancelAgent(context.Background(), "session-deadline-initial"), "a fresh retry should acquire the released guard")
		require.Equal(t, int32(1), agent.cancelAgentCalls.Load())
		requireNoCancelGuardReferences(t, svc, "session-deadline-initial")
	})

	t.Run("reacquisition after runtime cancellation", func(t *testing.T) {
		const taskID, sessionID = "task-deadline-reacquire", "session-deadline-reacquire"
		repo := setupTestRepo(t)
		seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
		seedExecutorRunning(t, repo, sessionID, taskID, "execution-deadline-reacquire")
		agent := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
		entered := make(chan struct{}, 1)
		releaseRuntime := make(chan struct{})
		agent.cancelAgentFunc = func(context.Context, string) error {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-releaseRuntime
			return nil
		}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
		svc.turnService = &repoTurnService{repo: repo}
		_, err := svc.turnService.StartTurn(context.Background(), sessionID)
		require.NoError(t, err)
		operation := beginDeadlineTestCancellation(t, svc, sessionID, cancellationKindExplicit)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- svc.runExplicitCancellationOwned(ctx, sessionID, operation) }()
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("runtime cancellation did not start")
		}
		holder := svc.lockCancelInFlightGuard(sessionID)
		close(releaseRuntime)
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			holder.release()
			t.Fatal("context deadline did not expire")
		}
		select {
		case err := <-done:
			require.ErrorIs(t, err, context.DeadlineExceeded)
		case <-time.After(time.Second):
			holder.release()
			err := <-done
			svc.finishCancellationWithActions(context.Background(), sessionID, operation, err)
			t.Fatal("post-runtime guard reacquisition did not observe the operation deadline")
		}
		require.False(t, operation.providerCancelOutcomeReady, "the expired owner must not publish a late guarded outcome")
		fresh, err := repo.GetTaskSession(context.Background(), sessionID)
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateRunning, fresh.State)
		activeTurn, err := svc.turnService.GetActiveTurn(context.Background(), sessionID)
		require.NoError(t, err)
		require.NotNil(t, activeTurn, "turn closure must wait for guard ownership")
		holder.release()
		svc.finishCancellationWithActions(ctx, sessionID, operation, context.DeadlineExceeded)
		require.False(t, svc.CancellationPending(sessionID))
		requireNoCancelGuardReferences(t, svc, sessionID)
		require.NoError(t, svc.CancelAgent(context.Background(), sessionID), "a fresh cancellation can reconcile after the failed attempt")
		fresh, err = repo.GetTaskSession(context.Background(), sessionID)
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateWaitingForInput, fresh.State)
		requireNoCancelGuardReferences(t, svc, sessionID)
	})

	t.Run("mixed-source actions", func(t *testing.T) {
		cases := []struct {
			name       string
			ownerKind  cancellationKind
			join       func(*Service, string, func(context.Context, *cancelOperation) (bool, error)) (*cancelOperation, bool, *cancellationAction)
			wantAction bool
		}{
			{
				name:      "explicit owner with silent join",
				ownerKind: cancellationKindExplicit,
				join: func(svc *Service, sessionID string, action func(context.Context, *cancelOperation) (bool, error)) (*cancelOperation, bool, *cancellationAction) {
					return svc.claimCancellationWithAction(sessionID, cancellationKindSilent, action)
				},
				wantAction: true,
			},
			{
				name:      "silent owner with explicit join",
				ownerKind: cancellationKindSilent,
				join: func(svc *Service, sessionID string, action func(context.Context, *cancelOperation) (bool, error)) (*cancelOperation, bool, *cancellationAction) {
					return svc.claimExplicitCancellation(sessionID, action)
				},
				wantAction: true,
			},
			{
				name:      "peer owner with explicit join",
				ownerKind: cancellationKindPeer,
				join: func(svc *Service, sessionID string, action func(context.Context, *cancelOperation) (bool, error)) (*cancelOperation, bool, *cancellationAction) {
					return svc.claimExplicitCancellation(sessionID, action)
				},
				wantAction: true,
			},
			{
				name:      "send now rejects explicit join",
				ownerKind: cancellationKindQueueSendNow,
				join: func(svc *Service, sessionID string, action func(context.Context, *cancelOperation) (bool, error)) (*cancelOperation, bool, *cancellationAction) {
					return svc.claimExplicitCancellation(sessionID, action)
				},
				wantAction: false,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				sessionID := "session-deadline-mixed-" + string(tc.ownerKind)
				svc := &Service{logger: testLogger()}
				operation, owner := svc.claimCancellation(sessionID, tc.ownerKind)
				require.True(t, owner)
				var actionMutations int
				joined, joinedOwner, action := tc.join(svc, sessionID, func(context.Context, *cancelOperation) (bool, error) {
					actionMutations++
					return true, nil
				})
				require.Same(t, operation, joined)
				require.False(t, joinedOwner)
				require.Equal(t, tc.wantAction, action != nil)
				holder := svc.lockCancelInFlightGuard(sessionID)

				ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
				done := make(chan struct{})
				go func() {
					svc.finishCancellationWithActions(ctx, sessionID, operation, nil)
					close(done)
				}()
				select {
				case <-done:
				case <-time.After(time.Second):
					holder.release()
					<-done
					cancel()
					t.Fatal("joined source action did not settle after the operation deadline")
				}
				if action != nil {
					_, actionErr := action.wait(context.Background())
					require.ErrorIs(t, actionErr, context.DeadlineExceeded)
				} else {
					require.NoError(t, operation.wait(context.Background()))
				}
				require.NoError(t, operation.wait(context.Background()), "the owner still settles its original result")

				successor, successorOwner := svc.claimCancellation(sessionID, cancellationKindExplicit)
				require.True(t, successorOwner)
				holder.release()
				cancel()
				require.Equal(t, 0, actionMutations, "an expired source action must not run after guard release")
				require.Same(t, successor, svc.currentCancellation(sessionID), "old cleanup must preserve successor ownership")
				svc.finishCancellation(sessionID, successor, nil)
				requireNoCancelGuardReferences(t, svc, sessionID)
			})
		}
	})
}

func TestCancelAgent_GuardDeadlineCannotMutateSuccessor(t *testing.T) {
	const sessionID = "session-deadline-successor"
	svc := &Service{logger: testLogger()}
	operation, owner := svc.claimCancellation(sessionID, cancellationKindSilent)
	require.True(t, owner)
	var lateMutation bool
	_, joinedOwner, action := svc.claimExplicitCancellation(sessionID, func(context.Context, *cancelOperation) (bool, error) {
		lateMutation = true
		return true, nil
	})
	require.False(t, joinedOwner)
	holder := svc.lockCancelInFlightGuard(sessionID)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		svc.finishCancellationWithActions(ctx, sessionID, operation, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		holder.release()
		<-done
		t.Fatal("expired cancellation action did not finish")
	}
	_, actionErr := action.wait(context.Background())
	require.ErrorIs(t, actionErr, context.DeadlineExceeded)
	successor, successorOwner := svc.claimCancellation(sessionID, cancellationKindExplicit)
	require.True(t, successorOwner)
	holder.release()
	require.False(t, lateMutation, "a timed-out action must not execute after the mutex becomes available")
	require.Same(t, successor, svc.currentCancellation(sessionID))
	svc.finishCancellation(sessionID, successor, nil)
	requireNoCancelGuardReferences(t, svc, sessionID)
}

func beginDeadlineTestCancellation(t *testing.T, svc *Service, sessionID string, kind cancellationKind) *cancelOperation {
	t.Helper()
	operation, owner := svc.claimCancellation(sessionID, kind)
	require.True(t, owner)
	operation.projectionRelease = svc.beginCancellationProjection(sessionID)
	return operation
}

func requireNoCancelGuardReferences(t *testing.T, svc *Service, sessionID string) {
	t.Helper()
	svc.cancelInFlightMu.Lock()
	defer svc.cancelInFlightMu.Unlock()
	if guard := svc.cancelInFlight[sessionID]; guard != nil {
		t.Fatalf("cancellation guard for %s retains %d references", sessionID, guard.refs)
	}
}
