package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestStoppedRemoteTerminalSubmissionIDUsesExactSavedBindingAfterProjection(t *testing.T) {
	execution := &AgentExecution{
		ID: "execution-terminal-binding", TaskID: "task-terminal-binding", SessionID: "session-terminal-binding",
		DeliveryStreamID: "stream-terminal-binding", DeliveryIncarnationID: "incarnation-terminal-binding",
		DeliveryHarnessGeneration: 3,
		metadata: map[string]interface{}{models.SessionMetaKeyDeliveryPromptTurnBinding: models.AgentDeliveryPromptTurnBinding{
			PromptGeneration: 9, TaskID: "task-terminal-binding", SessionID: "session-terminal-binding",
			ExecutionID: "execution-terminal-binding", IncarnationID: "incarnation-terminal-binding",
			HarnessGeneration: 3, StreamID: "stream-terminal-binding", SubmissionID: "submission-terminal-binding",
			TurnID: "turn-terminal-binding",
		}},
	}
	snapshot := &RemoteRecoverySnapshot{SessionID: execution.SessionID, TaskID: execution.TaskID,
		AgentExecutionID: execution.ID, Runtime: string(agentruntime.RuntimeRemoteDocker)}

	submissionID, err := stoppedRemoteTerminalSubmissionID(snapshot, execution)
	require.NoError(t, err)
	require.Equal(t, "submission-terminal-binding", submissionID,
		"the saved prompt binding identifies the terminal submission after SQL projection retires it from the active set")

	foreign := &AgentExecution{
		ID: "successor-execution", TaskID: execution.TaskID, SessionID: execution.SessionID,
		DeliveryStreamID: execution.DeliveryStreamID, DeliveryIncarnationID: execution.DeliveryIncarnationID,
		DeliveryHarnessGeneration: execution.DeliveryHarnessGeneration, metadata: execution.metadata,
	}
	submissionID, err = stoppedRemoteTerminalSubmissionID(snapshot, foreign)
	require.NoError(t, err)
	require.Empty(t, submissionID, "a binding from another execution cannot identify the stopped submission")
}

func TestStoppedRemoteLegacyTurnSettlesWithoutDurableSubmission(t *testing.T) {
	turn := &models.Turn{ID: "legacy-open-turn"}
	execution := &AgentExecution{DeliveryMode: DurableDeliveryLegacy}
	turnID, outcome, err := stoppedRemoteInterruptedOpenTurn(turn, nil, execution, &stoppedRemoteDeliveryReplay{})
	require.NoError(t, err)
	require.Equal(t, turn.ID, turnID)
	require.Empty(t, outcome, "legacy delivery has no retained terminal result")
}

func TestApplyRemoteLegacySessionEvidencePreservesSavedNativeSessionID(t *testing.T) {
	execution := &AgentExecution{ACPSessionID: "native-saved"}
	instance := &ExecutorInstance{
		DeliveryLegacyEvidence: true,
		agentSessionAssociation: &agentctl.AgentSessionAssociation{
			IncarnationID: "legacy-incarnation", HarnessGeneration: 5,
		},
	}
	applyRemoteLegacySessionEvidence(execution, instance)
	require.Equal(t, "native-saved", execution.ACPSessionID)
	require.Equal(t, "legacy-incarnation", execution.DeliveryIncarnationID)
	require.Equal(t, uint64(5), execution.DeliveryHarnessGeneration)
}

func TestStoppedRemoteWorkflowOutcomeAcceptsExactExistingCompletion(t *testing.T) {
	expected := &models.AgentDeliveryEffect{
		EffectKey: "workflow.on_turn_complete:turn-terminal-outcome", EffectType: "workflow.on_turn_complete",
		State: models.DeliveryEffectCompleted, StreamID: "stream-terminal-outcome", Sequence: 19,
		SessionID: "session-terminal-outcome", IncarnationID: "incarnation-terminal-outcome",
		HarnessGeneration: 4, SubmissionID: "submission-terminal-outcome", TurnID: "turn-terminal-outcome",
		Outcome: models.DeliveryWorkflowOutcomeStoppedRecovery,
	}
	for _, outcome := range []string{
		models.DeliveryWorkflowOutcomeApplied,
		models.DeliveryWorkflowOutcomeNoAutomaticTransition,
		models.DeliveryWorkflowOutcomeNotRequired,
		models.DeliveryWorkflowOutcomeStoppedRecovery,
	} {
		t.Run(outcome, func(t *testing.T) {
			existing := *expected
			existing.Outcome = outcome
			require.True(t, stoppedRemoteWorkflowOutcomeMatches(&existing, expected))
		})
	}
	foreign := *expected
	foreign.Sequence++
	require.False(t, stoppedRemoteWorkflowOutcomeMatches(&foreign, expected),
		"an outcome for a different retained terminal sequence cannot settle this turn")
}
