package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
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

func TestPromptTaskCancelsAsyncWaitOnlyAfterProviderAcceptance(t *testing.T) {
	for _, test := range []struct {
		name       string
		promptErr  error
		wantCancel bool
	}{
		{name: "accepted prompt", wantCancel: true},
		{name: "rejected prompt", promptErr: errors.New("provider rejected prompt")},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			svc, manager, repo := capacityTestService(t, 1)
			seedTaskAndSession(t, repo, "task-async-wait-prompt", "session-async-wait-prompt", models.TaskSessionStateWaitingForInput)
			seedExecutorRunning(t, repo, "session-async-wait-prompt", "task-async-wait-prompt", "exec-async-wait-prompt")
			markTargetPromptReady(t, svc, "task-async-wait-prompt", "session-async-wait-prompt", "exec-async-wait-prompt")
			svc.turnService = &repoBackedTurnService{repo: repo}
			manager.promptErr = test.promptErr
			gate := &fakeAsyncQuestionGate{parked: true}
			svc.SetAsyncQuestionGate(gate)

			_, err := svc.promptTask(
				ctx, "task-async-wait-prompt", "session-async-wait-prompt",
				"continue", "", false, nil, false, launchOriginManual, promptTaskOptions{},
			)

			if test.promptErr != nil {
				require.ErrorIs(t, err, test.promptErr)
			} else {
				require.NoError(t, err)
			}
			require.Len(t, manager.capturedPrompts, 1, "the production prompt path must reach the provider")
			if test.wantCancel {
				require.Equal(t, []string{"session-async-wait-prompt:newer_prompt"}, gate.cancels)
				return
			}
			require.Empty(t, gate.cancels, "a rejected provider prompt must leave its parked wait intact")
		})
	}
}
