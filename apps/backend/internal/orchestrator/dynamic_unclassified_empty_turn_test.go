package orchestrator

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

func emptyTurnCompletePayload(
	fixture unclassifiedWorkflowFenceFixture,
	executionID string,
	promptGeneration uint64,
	data map[string]interface{},
) *lifecycle.AgentStreamEventPayload {
	return &lifecycle.AgentStreamEventPayload{
		TaskID: fixture.taskID, SessionID: fixture.sessionID, ExecutionID: executionID,
		OwnerKind: lifecycle.ExecutionOwnerTask,
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete, PromptGeneration: promptGeneration, Data: data,
		},
	}
}

func endTurnData() map[string]interface{} {
	return map[string]interface{}{"stop_reason": "end_turn"}
}

func beginEmptyTurnPrompt(fixture unclassifiedWorkflowFenceFixture, executionID string, generation uint64) {
	fixture.agentManager.currentPromptExecutionID = executionID
	fixture.agentManager.currentPromptGeneration.Store(generation)
	fixture.svc.beginPromptAttempt(fixture.sessionID, executionID, generation, true)
}

func loadEmptyTurnPolicy(t *testing.T, fixture unclassifiedWorkflowFenceFixture) (string, string, int64, *dynamic.UnclassifiedStreak) {
	t.Helper()
	state, err := fixture.repo.LoadRouteState(fixture.ctx, fixture.sessionID)
	if err != nil || state == nil {
		t.Fatalf("LoadRouteState: %+v, %v", state, err)
	}
	var policy dynamic.PolicyState
	if state.PolicyStateJSON != "" {
		if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policy); err != nil {
			t.Fatalf("decode policy state: %v", err)
		}
	}
	return state.Status, state.ExecutionProfileID, state.Generation, policy.Unclassified
}

func TestEmptyTurnCompletionCountsUnclassifiedFailure(t *testing.T) {
	fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
	defer fixture.svc.stopDynamicSuccessorWorkers()

	beginEmptyTurnPrompt(fixture, "execution-one", 1)
	fixture.svc.handleAgentStreamEvent(fixture.ctx, emptyTurnCompletePayload(fixture, "execution-one", 1, endTurnData()))

	status, candidate, generation, streak := loadEmptyTurnPolicy(t, fixture)
	if status != "action_required" || candidate != "candidate-a" || generation != 1 {
		t.Fatalf("route after first empty turn = %s/%s/%d, want action_required/candidate-a/1", status, candidate, generation)
	}
	if streak == nil || streak.Count != 1 || string(streak.Origin) != "empty_turn_completion" {
		t.Fatalf("streak after first empty turn = %+v, want empty_turn_completion count 1", streak)
	}

	// The same turn's process exit must not clear the count it just produced.
	fixture.svc.clearPromptAttemptEvidence(fixture.sessionID, "execution-one", 1)
	completed := fixture.event("execution-one", 1)
	completed.ProviderError = nil
	fixture.svc.clearDynamicUnclassifiedStreakForCompletion(fixture.ctx, completed)
	if _, _, _, streak = loadEmptyTurnPolicy(t, fixture); streak == nil || streak.Count != 1 {
		t.Fatalf("streak after the counted turn's completion = %+v, want count 1", streak)
	}

	fixture.resume(t, "execution-two")
	fixture.persistRunningExecution(t, "execution-two")
	beginEmptyTurnPrompt(fixture, "execution-two", 2)
	fixture.svc.handleAgentStreamEvent(fixture.ctx, emptyTurnCompletePayload(fixture, "execution-two", 2, endTurnData()))

	_, candidate, generation, _ = loadEmptyTurnPolicy(t, fixture)
	if candidate != "candidate-b" || generation != 2 {
		t.Fatalf("route after threshold empty turn = %s/%d, want candidate-b/2", candidate, generation)
	}
}

// A normal end_turn whose assistant text arrives only on the complete frame is
// not an empty turn: the earlier stream events carried no output, so the
// detector must read the completion frame itself before counting a failure.
func TestCompleteFrameTextIsNotAnEmptyTurn(t *testing.T) {
	testCompleteFrameOutputIsNotEmpty(t, false)
}

func TestCompleteFramePlanIsNotAnEmptyTurn(t *testing.T) {
	testCompleteFrameOutputIsNotEmpty(t, true)
}

func testCompleteFrameOutputIsNotEmpty(t *testing.T, plan bool) {
	t.Helper()
	fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
	defer fixture.svc.stopDynamicSuccessorWorkers()
	messages := &mockMessageCreator{}
	fixture.svc.messageCreator = messages

	// A genuine empty completion below the threshold counts one failure.
	beginEmptyTurnPrompt(fixture, "execution-one", 1)
	fixture.svc.handleAgentStreamEvent(fixture.ctx, emptyTurnCompletePayload(fixture, "execution-one", 1, endTurnData()))
	if _, _, _, streak := loadEmptyTurnPolicy(t, fixture); streak == nil || streak.Count != 1 {
		t.Fatalf("setup empty turn streak = %+v, want count 1", streak)
	}

	// The next turn produced text on the complete frame only. Counted, it would
	// reach the threshold, launch a successor and drop the text.
	fixture.resume(t, "execution-two")
	fixture.persistRunningExecution(t, "execution-two")
	beginEmptyTurnPrompt(fixture, "execution-two", 2)
	payload := emptyTurnCompletePayload(fixture, "execution-two", 2, endTurnData())
	payload.Data.TurnID = "turn-two"
	if plan {
		payload.Data.Data.(map[string]interface{})["plan_content"] = "the delayed answer"
	} else {
		payload.Data.Text = "the delayed answer"
	}
	fixture.svc.handleAgentStreamEvent(fixture.ctx, payload)

	_, candidate, generation, streak := loadEmptyTurnPolicy(t, fixture)
	if candidate != "candidate-a" || generation != 1 {
		t.Fatalf("route after complete-frame text = %s/%d, want candidate-a/1", candidate, generation)
	}
	if streak != nil {
		t.Fatalf("streak after complete-frame text = %+v, want success to clear the streak", streak)
	}
	if len(fixture.agentManager.startAgentProcessCalls) != 0 {
		t.Fatalf("successor launches after complete-frame text = %d, want none", len(fixture.agentManager.startAgentProcessCalls))
	}
	if plan {
		if len(messages.sessionMessages) != 1 || messages.sessionMessages[0].content != "the delayed answer" {
			t.Fatalf("persisted plan messages = %+v, want the complete-frame plan saved once", messages.sessionMessages)
		}
		return
	}
	if len(messages.agentMessages) != 1 || messages.agentMessages[0].content != "the delayed answer" {
		t.Fatalf("persisted agent messages = %+v, want the complete-frame text saved once", messages.agentMessages)
	}
}

func TestEmptyTurnCompletionPreconditions(t *testing.T) {
	for name, testCase := range map[string]struct {
		data    map[string]interface{}
		prepare func(unclassifiedWorkflowFenceFixture)
		stale   bool
	}{
		"cancelled stop reason": {data: map[string]interface{}{"stop_reason": "cancelled"}},
		"error completion":      {data: map[string]interface{}{"stop_reason": "end_turn", "is_error": true}},
		"missing stop reason":   {data: map[string]interface{}{}},
		"assistant output": {data: endTurnData(), prepare: func(f unclassifiedWorkflowFenceFixture) {
			f.svc.observePromptAttempt(f.sessionID, "execution-one", 1, true, false)
		}},
		"tool activity": {data: endTurnData(), prepare: func(f unclassifiedWorkflowFenceFixture) {
			f.svc.observePromptAttempt(f.sessionID, "execution-one", 1, false, true)
		}},
		"stale prompt generation": {data: endTurnData(), stale: true},
		"step veto": {data: endTurnData(), prepare: func(f unclassifiedWorkflowFenceFixture) {
			steps := f.svc.workflowStepGetter.(*mockStepGetter).steps
			vetoed := *steps["step-1"]
			vetoed.DisableUnclassifiedFallback = true
			steps["step-1"] = &vetoed
		}},
		"unknown step": {data: endTurnData(), prepare: func(f unclassifiedWorkflowFenceFixture) {
			delete(f.svc.workflowStepGetter.(*mockStepGetter).steps, "step-1")
		}},
		"candidate policy absent": {data: endTurnData(), prepare: func(f unclassifiedWorkflowFenceFixture) {
			f.svc.SetProfileExecutionResolver(newWorkflowDynamicProfileResolverWithCandidates(t, f.profile.ID,
				[]workflowDynamicCandidate{{executionProfileID: "candidate-a", enabled: true}, {executionProfileID: "candidate-b", enabled: true}},
				dynamic.WithPersistence(f.barrier), dynamic.WithStateLoader(f.barrier)))
		}},
		"dynamic routing unavailable": {data: endTurnData(), prepare: func(f unclassifiedWorkflowFenceFixture) {
			f.resolver.SetEnabled(false)
		}},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
			defer fixture.svc.stopDynamicSuccessorWorkers()
			beginEmptyTurnPrompt(fixture, "execution-one", 1)
			if testCase.prepare != nil {
				testCase.prepare(fixture)
			}
			generation := uint64(1)
			if testCase.stale {
				generation = 2
			}
			fixture.svc.handleAgentStreamEvent(fixture.ctx, emptyTurnCompletePayload(fixture, "execution-one", generation, testCase.data))
			fixture.resolver.SetEnabled(true)

			status, candidate, routeGeneration, streak := loadEmptyTurnPolicy(t, fixture)
			if status != "active" || candidate != "candidate-a" || routeGeneration != 1 || streak != nil {
				t.Fatalf("route after %s = %s/%s/%d streak %+v, want unchanged active candidate-a/1 without a streak",
					name, status, candidate, routeGeneration, streak)
			}
			if _, counted := fixture.svc.countedEmptyTurns.Load(fixture.sessionID); counted {
				t.Fatalf("%s left a counted empty turn record", name)
			}
		})
	}
}
