package orchestrator

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

// The repeated-failure policy requires complete diagnostic identity. An exit
// code or a common message prefix does not establish that identity.
func TestPostStartUnknownFailuresKeepManualRecovery(t *testing.T) {
	prefix := "provider response header timeout: " + strings.Repeat("upstream gateway did not answer ", 10)
	for name, messages := range map[string][2]string{
		"same exit code, distinct stderr": {
			"Agent process exited with code 1: first stderr line",
			"Agent process exited with code 1: a different stderr line",
		},
		"same bounded prefix, distinct tail": {prefix + " request one", prefix + " request two"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
			t.Cleanup(fixture.svc.stopDynamicSuccessorWorkers)
			for index, executionID := range []string{"execution-one", "execution-two"} {
				if index > 0 {
					fixture.resume(t, executionID)
					fixture.persistRunningExecution(t, executionID)
				}
				generation := uint64(index + 1)
				beginEmptyTurnPrompt(fixture, executionID, generation)
				fixture.svc.handleAgentStreamEvent(fixture.ctx, &lifecycle.AgentStreamEventPayload{
					TaskID: fixture.taskID, SessionID: fixture.sessionID, ExecutionID: executionID,
					OwnerKind: lifecycle.ExecutionOwnerTask,
					Data: &lifecycle.AgentStreamEventData{
						Type: agentEventError, Error: messages[index], PromptGeneration: generation,
					},
				})
				status, candidate, routeGeneration, streak := loadEmptyTurnPolicy(t, fixture)
				if status != "action_required" || candidate != "candidate-a" || routeGeneration != 1 || streak != nil {
					t.Fatalf("post-start failure %d = %s/%s/%d streak %+v; want manual recovery without a streak",
						index+1, status, candidate, routeGeneration, streak)
				}
			}
			if len(fixture.agentManager.startAgentProcessCalls) != 0 {
				t.Fatal("unknown post-start failures launched a successor")
			}
		})
	}
}
