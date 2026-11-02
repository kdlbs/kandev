package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.13
func TestPromptAcceptanceRestoreRequiresAuthenticatedActiveTurn(t *testing.T) {
	for _, scenario := range []string{"accepted", "dispatching", "prepared", "terminal", "missing", "foreign_stream", "foreign_generation", "successor"} {
		t.Run(scenario, func(t *testing.T) {
			repository := durableAdoptionRepository()
			repository.submissions["submission-1"] = &models.AgentDeliverySubmission{
				ID: "submission-1", SessionID: "session-1", IncarnationID: "incarnation-1",
				HarnessGeneration: 2, State: models.DeliverySubmissionDispatching,
			}
			status := durableAdoptionStatus()
			peer := journal.SubmissionSummary{
				ID: "submission-1", SessionID: "session-1", IncarnationID: "incarnation-1",
				HarnessGeneration: 2, StreamID: "stream-1", State: journal.SubmissionAccepted,
			}
			binding := deliveryPromptTurnBinding{
				TaskID: "task-1", SessionID: "session-1", ExecutionID: "execution-1",
				IncarnationID: "incarnation-1", HarnessGeneration: 2, StreamID: "stream-1",
				SubmissionID: "submission-1", TurnID: "turn-1", PromptGeneration: 7,
			}
			execution := &AgentExecution{
				ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
				metadata: map[string]interface{}{deliveryPromptTurnBindingMetadataKey: binding},
			}
			switch scenario {
			case "dispatching":
				peer.State = journal.SubmissionDispatching
			case "prepared":
				peer.State = journal.SubmissionPrepared
			case "terminal":
				peer.State, peer.TerminalEventRetained, peer.TerminalSequence = journal.SubmissionCompleted, true, 3
			case "missing":
				execution.metadata = nil
			case "foreign_stream":
				peer.StreamID = "foreign"
			case "foreign_generation":
				peer.HarnessGeneration++
			case "successor":
				execution.promptGeneration = 8
			}
			status.Submissions = []journal.SubmissionSummary{peer}
			err := newAdoptionManager(repository).restoreRecoveredDelivery(context.Background(), execution,
				&ExecutorInstance{DeliveryStatus: status})
			want := scenario == "accepted" || scenario == "dispatching"
			if want && err != nil {
				t.Fatal(err)
			}
			sm := NewSessionManager(newSessionTestLogger(), make(chan struct{}))
			snapshot, owned := sm.capturePromptStall(execution, 7)
			if got := owned && snapshot.accepted; got != want {
				t.Fatalf("restored acceptance = %t, want %t (restore error: %v)", got, want, err)
			}
			if evidence := execution.promptAttemptEvidenceSnapshot(); evidence.OutputObserved || evidence.EffectObserved {
				t.Fatal("restoring acceptance invented output or effects")
			}
			if scenario == "successor" && execution.promptGenerationSnapshot() != 8 {
				t.Fatal("restoring acceptance replaced a successor generation")
			}
		})
	}
}

func TestPromptAcceptanceRestoredGenerationRejectsOlderCompletion(t *testing.T) {
	manager, _ := createTestManagerWithTracking()
	execution := createTestExecution("execution", "task", "session")
	execution.promptGeneration = 7
	execution.dispatchedPromptGeneration = 7
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	manager.publishRecoveredExecutionRunning(t.Context(), execution)
	manager.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete", SessionID: "session", PromptGeneration: 6,
	})
	if execution.promptGenerationSnapshot() != 7 || execution.promptCompletionGeneration != 0 {
		t.Fatal("a stale completion changed the authenticated restored generation")
	}
}
