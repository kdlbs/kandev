package orchestrator

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInterruptionContinuationPhaseAccepted(t *testing.T) {
	svc, _, _ := dispatchContinuationFixture(t)
	mc := svc.messageCreator.(*mockMessageCreator)
	latest := mc.sessionMessages[len(mc.sessionMessages)-1]
	require.Equal(t, "continuing", latest.metadata["recovery_phase"])
	require.Equal(t, 1, latest.metadata["attempts_started"])
	require.Equal(t, "cursor-acp", latest.metadata["provider_name"], "phase updates preserve the original provider label")
}

type continuationTurnObserver struct {
	TurnService
	completions int
}

func (s *continuationTurnObserver) CompleteTurn(ctx context.Context, turnID string) error {
	s.completions++
	return s.TurnService.CompleteTurn(ctx, turnID)
}

func TestInterruptionContinuationSettlementAmbiguousDispatch(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	observer := &continuationTurnObserver{TurnService: svc.turnService}
	svc.turnService = observer
	mgr.promptErr = errors.New("connection reset by peer")
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	require.Zero(t, observer.completions, "uncertain acceptance must not report successful turn completion")
	require.Len(t, mgr.capturedPrompts, 1)
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned, "ambiguous dispatch is manual, never redispatched")
	require.Equal(t, int32(1), mgr.cancelAgentCalls.Load(), "uncertain acceptance must be cancelled before manual recovery")
	active, err := svc.turnService.GetActiveTurn(context.Background(), "s1")
	require.NoError(t, err)
	require.Nil(t, active, "recovery feedback must not create a new agent turn")
}

func TestInterruptionContinuationSettlementCancelFailureIsManual(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	mgr.promptErr = errors.New("connection reset by peer")
	mgr.cancelAgentErr = errors.New("provider cancel acknowledgement unavailable")
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	require.Len(t, mgr.capturedPrompts, 1)
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned, "failed cancellation must retire the automatic owner")
	mc := svc.messageCreator.(*mockMessageCreator)
	last := mc.sessionMessages[len(mc.sessionMessages)-1]
	require.Equal(t, "manual", last.metadata["recovery_disposition"])
	session, err := svc.repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	require.Equal(t, "FAILED", string(session.State), "unconfirmed cancellation must not expose an idle promptable runtime")
}

type continuationSettlementFailure struct {
	*continuationTurnObserver
}

func (s *continuationSettlementFailure) UpdateTurn(context.Context, *models.Turn) error {
	return errors.New("interrupted turn storage unavailable")
}

func TestInterruptionContinuationSettlementStorageFailureNeverCompletesTurn(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	turns := &repoTurnService{repo: svc.repo.(*sqliterepo.Repository)}
	turn, err := turns.StartTurn(context.Background(), "s1")
	require.NoError(t, err)
	svc.activeTurns.Store("s1", turn.ID)
	observer := &continuationTurnObserver{TurnService: turns}
	svc.turnService = &continuationSettlementFailure{continuationTurnObserver: observer}
	svc.handleAgentFailedLocked(context.Background(), data)
	require.Zero(t, observer.completions, "failed interruption persistence must never emit successful completion")
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned, "storage failure must not dispatch continuation")
	session, err := svc.repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	require.Equal(t, "FAILED", string(session.State))
}
