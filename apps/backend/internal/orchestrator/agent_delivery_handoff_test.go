package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// newHandoffService builds the smallest service that admits and finalizes a
// real durable submission. The advertised peer keeps the protocol on V1.
func newHandoffService(t *testing.T) (*Service, *sqliterepo.Repository) {
	t.Helper()
	repo := setupTestRepo(t)
	agentManager := &durableDeliveryTestAgentManager{
		mockAgentManager: &mockAgentManager{},
		advertised:       true,
		capability: agentruntime.DurableDeliveryCapability{
			Version: journal.CurrentVersion,
			Durable: true,
		},
	}
	return createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager), repo
}

// admitHandoffSubmission admits one durable prompt for a handoff regression and
// returns the runtime the dispatch finalizer would settle.
func admitHandoffSubmission(
	t *testing.T,
	service *Service,
	repo *sqliterepo.Repository,
	sessionID, submissionID, prompt string,
) (*agentDeliverySubmissionRuntime, *models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	runtime, err := service.prepareAgentDeliverySubmission(
		ctx, session, "execution-"+sessionID, prompt, nil,
		promptTaskOptions{
			deliveryProtocol:     messagequeue.DeliveryProtocolPending,
			deliverySubmissionID: submissionID,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, runtime)
	return runtime, session
}

func finishHandoffDispatch(
	t *testing.T,
	service *Service,
	taskID, sessionID, submissionID, prompt, stopReason string,
	runtime *agentDeliverySubmissionRuntime,
) *PromptResult {
	t.Helper()
	outcome := &promptDispatchOutcome{}
	outcome.recordAccepted(nil)
	result, err := service.finishPromptExecutorDispatch(
		context.Background(), taskID, sessionID, prompt, false, false, nil,
		promptClaimRollback{turnID: "turn-" + submissionID}, promptTaskOptions{}, nil, nil,
		&executor.PromptResult{StopReason: stopReason, AgentMessage: "turn finished"}, nil,
		outcome, nil, runtime,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	return result
}

func requireDispatchingSubmission(
	t *testing.T,
	repo *sqliterepo.Repository,
	submissionID string,
) *models.AgentDeliverySubmission {
	t.Helper()
	stored, err := repo.GetAgentDeliverySubmission(context.Background(), submissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionDispatching, stored.State)
	require.Equal(t, "dispatching", stored.Outcome)
	require.Empty(t, stored.TerminalStreamID)
	require.Zero(t, stored.TerminalSequence)
	require.Empty(t, stored.TerminalTurnID)
	require.Nil(t, stored.FullySettledAt)
	return stored
}

func TestForegroundHandoffPreservesDurableSubmission(t *testing.T) {
	service, repo := newHandoffService(t)
	seedSession(t, repo, "task-handoff", "session-handoff", "", "incarnation-handoff")
	runtime, _ := admitHandoffSubmission(
		t, service, repo, "session-handoff", "handoff-original", "old instruction",
	)
	result := finishHandoffDispatch(
		t, service, "task-handoff", "session-handoff", "handoff-original",
		"old instruction", streams.EventTypeForegroundIdle, runtime,
	)
	require.Equal(t, streams.EventTypeForegroundIdle, result.StopReason)
	requireDispatchingSubmission(t, repo, "prompt:handoff-original")
	require.False(t, runtime.completed, "handoff must not mark the runtime completed")
}

func handoffTerminalEvent(
	sessionID, incarnationID string,
	generation, sequence int64,
	submissionID, streamID, eventType string,
) models.AgentDeliveryEvent {
	payload, _ := json.Marshal(map[string]string{"type": eventType, "turn_id": "turn-" + submissionID})
	return models.AgentDeliveryEvent{
		SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: generation,
		StreamID: streamID, Sequence: sequence, SubmissionID: submissionID,
		EventType: eventType, Payload: payload, Terminal: true, ReceivedAt: time.Now().UTC(),
	}
}

func projectAndSettleHandoffTerminal(
	t *testing.T,
	repo *sqliterepo.Repository,
	event models.AgentDeliveryEvent,
) {
	t.Helper()
	ctx := context.Background()
	projectOrchestratorDeliveryEvent(t, repo, ctx, event)
	settled, err := repo.SettleAgentDeliveryTerminal(
		ctx, event.StreamID, event.Sequence, models.DeliverySubmissionCompleted, time.Now().UTC(),
	)
	require.NoError(t, err)
	require.True(t, settled)
}

func TestForegroundHandoffLateTerminalSettlesOriginalSubmission(t *testing.T) {
	for _, terminalTiming := range []string{"before return", "after return"} {
		t.Run(terminalTiming, func(t *testing.T) {
			ctx := context.Background()
			service, repo := newHandoffService(t)
			seedSession(t, repo, "task-handoff-late", "session-handoff-late", "", "incarnation-handoff-late")
			runtime, _ := admitHandoffSubmission(
				t, service, repo, "session-handoff-late", "handoff-late", "old instruction",
			)
			event := persistedTerminalEvent(
				"session-handoff-late", "incarnation-handoff-late", 1,
				"prompt:handoff-late", "session-handoff-late:g1", "complete",
			)
			if terminalTiming == "before return" {
				projectOrchestratorDeliveryEvent(t, repo, ctx, event)
				settled, err := repo.SettleAgentDeliveryTerminal(
					ctx, event.StreamID, event.Sequence, models.DeliverySubmissionCompleted, time.Now().UTC(),
				)
				require.NoError(t, err)
				require.True(t, settled)
			}
			finishHandoffDispatch(
				t, service, "task-handoff-late", "session-handoff-late", "handoff-late",
				"old instruction", streams.EventTypeForegroundIdle, runtime,
			)
			if terminalTiming == "after return" {
				projectOrchestratorDeliveryEvent(t, repo, ctx, event)
				settled, err := repo.SettleAgentDeliveryTerminal(
					ctx, event.StreamID, event.Sequence, models.DeliverySubmissionCompleted, time.Now().UTC(),
				)
				require.NoError(t, err)
				require.True(t, settled)
			}
			original, err := repo.GetAgentDeliverySubmission(ctx, "prompt:handoff-late")
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionCompleted, original.State)
			require.Equal(t, "completed", original.Outcome)

			session, err := repo.GetTaskSession(ctx, "session-handoff-late")
			require.NoError(t, err)
			successor, err := service.prepareAgentDeliverySubmission(
				ctx, session, "execution-session-handoff-late", "next instruction", nil,
				promptTaskOptions{
					deliveryProtocol:     messagequeue.DeliveryProtocolPending,
					deliverySubmissionID: "handoff-successor",
				},
			)
			require.NoError(t, err)
			require.NotNil(t, successor)
			successorRow, err := repo.GetAgentDeliverySubmission(ctx, "prompt:handoff-successor")
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionDispatching, successorRow.State)
			retained, err := repo.GetAgentDeliverySubmission(ctx, "prompt:handoff-late")
			require.NoError(t, err)
			require.Equal(t, original, retained, "late original terminal must not settle the successor")
		})
	}
	t.Run("predecessor terminal after successor", func(t *testing.T) {
		ctx := context.Background()
		service, repo := newHandoffService(t)
		seedSession(t, repo, "task-handoff-successor", "session-handoff-successor", "", "incarnation-handoff-successor")
		runtime, _ := admitHandoffSubmission(
			t, service, repo, "session-handoff-successor", "handoff-predecessor", "old instruction",
		)
		finishHandoffDispatch(
			t, service, "task-handoff-successor", "session-handoff-successor", "handoff-predecessor",
			"old instruction", streams.EventTypeForegroundIdle, runtime,
		)
		session, err := repo.GetTaskSession(ctx, "session-handoff-successor")
		require.NoError(t, err)
		successor, err := service.prepareAgentDeliverySubmission(
			ctx, session, "execution-session-handoff-successor", "next instruction", nil,
			promptTaskOptions{
				deliveryProtocol:     messagequeue.DeliveryProtocolPending,
				deliverySubmissionID: "handoff-next",
			},
		)
		require.NoError(t, err)
		require.NotNil(t, successor, "a successor may start while the predecessor stays unresolved")
		beforeSuccessor, err := repo.GetAgentDeliverySubmission(ctx, "prompt:handoff-next")
		require.NoError(t, err)
		require.Equal(t, models.DeliverySubmissionDispatching, beforeSuccessor.State)

		projectAndSettleHandoffTerminal(t, repo, handoffTerminalEvent(
			"session-handoff-successor", "incarnation-handoff-successor", 1, 1,
			"prompt:handoff-predecessor", "session-handoff-successor:g1", "complete",
		))
		predecessor, err := repo.GetAgentDeliverySubmission(ctx, "prompt:handoff-predecessor")
		require.NoError(t, err)
		require.Equal(t, models.DeliverySubmissionCompleted, predecessor.State)
		retainedSuccessor, err := repo.GetAgentDeliverySubmission(ctx, "prompt:handoff-next")
		require.NoError(t, err)
		require.Equal(t, models.DeliverySubmissionDispatching, retainedSuccessor.State)
		require.Empty(t, retainedSuccessor.TerminalStreamID)
		require.Zero(t, retainedSuccessor.TerminalSequence)
		require.Equal(t, beforeSuccessor.HarnessGeneration, retainedSuccessor.HarnessGeneration)
		require.Equal(t, beforeSuccessor.DispatchAttemptID, retainedSuccessor.DispatchAttemptID)

		projectAndSettleHandoffTerminal(t, repo, handoffTerminalEvent(
			"session-handoff-successor", "incarnation-handoff-successor", 1, 2,
			"prompt:handoff-next", "session-handoff-successor:g1", "complete",
		))
		settledSuccessor, err := repo.GetAgentDeliverySubmission(ctx, "prompt:handoff-next")
		require.NoError(t, err)
		require.Equal(t, models.DeliverySubmissionCompleted, settledSuccessor.State)
		require.Equal(t, "completed", settledSuccessor.Outcome)
	})
}

func TestForegroundHandoffInterruptedBeforeTerminal(t *testing.T) {
	service, repo := newHandoffService(t)
	seedSession(t, repo, "task-handoff-interrupt", "session-handoff-interrupt", "", "incarnation-handoff-interrupt")
	runtime, _ := admitHandoffSubmission(
		t, service, repo, "session-handoff-interrupt", "handoff-interrupt", "old instruction",
	)
	finishHandoffDispatch(
		t, service, "task-handoff-interrupt", "session-handoff-interrupt", "handoff-interrupt",
		"old instruction", streams.EventTypeForegroundIdle, runtime,
	)
	before := requireDispatchingSubmission(t, repo, "prompt:handoff-interrupt")
	require.NoError(t, runtime.markInterrupted(context.WithoutCancel(context.Background()), "prompt_dispatch_failed"))
	stored, err := repo.GetAgentDeliverySubmission(context.Background(), "prompt:handoff-interrupt")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
	require.Equal(t, before.Payload, stored.Payload)
	require.Equal(t, before.PayloadHash, stored.PayloadHash)
	require.Zero(t, stored.TerminalSequence)
}
