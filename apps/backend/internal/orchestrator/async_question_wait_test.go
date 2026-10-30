package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeAsyncQuestionGate struct {
	parked  bool
	cancels []string
}

func (g *fakeAsyncQuestionGate) HasParkedWait(context.Context, string) bool { return g.parked }

func (g *fakeAsyncQuestionGate) CancelActiveWait(_ context.Context, sessionID, reason string) error {
	g.cancels = append(g.cancels, sessionID+":"+reason)
	return nil
}

func TestAsyncQuestionsNoImplicitWait(t *testing.T) {
	repo := setupTestRepo(t)
	svc, _ := newTestServiceWithObservedLogger(t, repo)
	gate := &fakeAsyncQuestionGate{}
	svc.SetAsyncQuestionGate(gate)
	ctx := context.Background()

	require.False(t, svc.sessionHasPendingClarification(ctx, "session-no-question"),
		"no request and no gate must not block completion")
	require.False(t, svc.sessionHasLiveClarification(ctx, "session-no-question"),
		"no request and no gate must not hold the queue")
}

func TestAsyncQuestionsParkedGateBlocksCompletionAndDrain(t *testing.T) {
	repo := setupTestRepo(t)
	svc, _ := newTestServiceWithObservedLogger(t, repo)
	gate := &fakeAsyncQuestionGate{parked: true}
	svc.SetAsyncQuestionGate(gate)
	ctx := context.Background()

	require.True(t, svc.sessionHasPendingClarification(ctx, "session-parked"),
		"a parked explicit gate withholds on_turn_complete")
	require.True(t, svc.sessionHasLiveClarification(ctx, "session-parked"),
		"a parked explicit gate holds unrelated queue entries")
}

func TestAsyncQuestionsCancelledSessionStopsBlocking(t *testing.T) {
	repo := setupTestRepo(t)
	svc, _ := newTestServiceWithObservedLogger(t, repo)
	gate := &fakeAsyncQuestionGate{}
	svc.SetAsyncQuestionGate(gate)
	ctx := context.Background()

	require.False(t, svc.sessionHasPendingClarification(ctx, "session-answered"))
	require.NoError(t, gate.CancelActiveWait(ctx, "session-answered", "newer_prompt"))
	require.Equal(t, []string{"session-answered:newer_prompt"}, gate.cancels)
}
