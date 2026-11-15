package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

func reasoningStreamPayload(
	fixture unclassifiedWorkflowFenceFixture,
	eventType, executionID string,
	generation uint64,
	text string,
) *lifecycle.AgentStreamEventPayload {
	return &lifecycle.AgentStreamEventPayload{
		TaskID: fixture.taskID, SessionID: fixture.sessionID, ExecutionID: executionID,
		OwnerKind: lifecycle.ExecutionOwnerTask,
		Data:      &lifecycle.AgentStreamEventData{Type: eventType, PromptGeneration: generation, Text: text},
	}
}

func promptEvidenceSnapshot(t *testing.T, fixture unclassifiedWorkflowFenceFixture) (output, nonReasoning, effect, known bool) {
	t.Helper()
	evidence, ok := fixture.svc.promptAttemptForSession(fixture.sessionID)
	if !ok {
		t.Fatal("prompt attempt evidence missing")
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	return evidence.output, evidence.nonReasoningOutput, evidence.effect, evidence.evidenceKnown
}

// A turn that streams only reasoning and then ends normally is an empty turn:
// each such completion is counted, and the second one reaches the threshold
// and launches the successor candidate.
func TestReasoningOnlyEmptyTurnCompletionIsCounted(t *testing.T) {
	fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
	defer fixture.svc.stopDynamicSuccessorWorkers()

	beginEmptyTurnPrompt(fixture, "execution-one", 1)
	fixture.svc.handleAgentStreamEvent(fixture.ctx,
		reasoningStreamPayload(fixture, streams.EventTypeReasoning, "execution-one", 1, "thinking about it"))
	fixture.svc.handleAgentStreamEvent(fixture.ctx, emptyTurnCompletePayload(fixture, "execution-one", 1, endTurnData()))

	status, candidate, generation, streak := loadEmptyTurnPolicy(t, fixture)
	if status != "action_required" || candidate != "candidate-a" || generation != 1 {
		t.Fatalf("route after first reasoning-only turn = %s/%s/%d, want action_required/candidate-a/1", status, candidate, generation)
	}
	if streak == nil || streak.Count != 1 || string(streak.Origin) != "empty_turn_completion" {
		t.Fatalf("streak after first reasoning-only turn = %+v, want empty_turn_completion count 1", streak)
	}

	fixture.resume(t, "execution-two")
	fixture.persistRunningExecution(t, "execution-two")
	beginEmptyTurnPrompt(fixture, "execution-two", 2)
	fixture.svc.handleAgentStreamEvent(fixture.ctx,
		reasoningStreamPayload(fixture, streams.EventTypeReasoning, "execution-two", 2, "still thinking"))
	fixture.svc.handleAgentStreamEvent(fixture.ctx, emptyTurnCompletePayload(fixture, "execution-two", 2, endTurnData()))

	_, candidate, generation, _ = loadEmptyTurnPolicy(t, fixture)
	if candidate != "candidate-b" || generation != 2 {
		t.Fatalf("route after second reasoning-only turn = %s/%d, want candidate-b/2", candidate, generation)
	}
}

// Reasoning does not make a turn empty when the same turn also produced an
// answer chunk, plan output on the complete frame, or a tool effect.
func TestReasoningWithOtherOutputIsNotCounted(t *testing.T) {
	for name, testCase := range map[string]struct {
		observe func(fixture unclassifiedWorkflowFenceFixture)
		plan    bool
	}{
		"answer chunk": {observe: func(f unclassifiedWorkflowFenceFixture) {
			f.svc.handleAgentStreamEvent(f.ctx, reasoningStreamPayload(f, streams.EventTypeMessageChunk, "execution-one", 1, "the answer"))
		}},
		"tool call": {observe: func(f unclassifiedWorkflowFenceFixture) {
			f.svc.handleAgentStreamEvent(f.ctx, reasoningStreamPayload(f, agentEventToolCall, "execution-one", 1, ""))
		}},
		"tool update": {observe: func(f unclassifiedWorkflowFenceFixture) {
			f.svc.handleAgentStreamEvent(f.ctx, reasoningStreamPayload(f, agentEventToolUpdate, "execution-one", 1, ""))
		}},
		"complete frame plan": {plan: true},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
			defer fixture.svc.stopDynamicSuccessorWorkers()
			fixture.svc.messageCreator = &mockMessageCreator{}

			beginEmptyTurnPrompt(fixture, "execution-one", 1)
			fixture.svc.handleAgentStreamEvent(fixture.ctx,
				reasoningStreamPayload(fixture, streams.EventTypeReasoning, "execution-one", 1, "thinking"))
			if testCase.observe != nil {
				testCase.observe(fixture)
			}
			complete := emptyTurnCompletePayload(fixture, "execution-one", 1, endTurnData())
			if testCase.plan {
				complete.Data.Data.(map[string]interface{})["plan_content"] = "the plan"
			}
			fixture.svc.handleAgentStreamEvent(fixture.ctx, complete)

			status, candidate, generation, streak := loadEmptyTurnPolicy(t, fixture)
			if status != "active" || candidate != "candidate-a" || generation != 1 || streak != nil {
				t.Fatalf("route after reasoning plus %s = %s/%s/%d streak %+v, want unchanged active candidate-a/1 without a streak",
					name, status, candidate, generation, streak)
			}
			if len(fixture.agentManager.startAgentProcessCalls) != 0 {
				t.Fatalf("successor launches after reasoning plus %s = %d, want none", name, len(fixture.agentManager.startAgentProcessCalls))
			}
		})
	}
}

// withEmptyTurnEvidence drops reasoning from the output fence but keeps
// lifecycle-supplied output, tool effects, and every non-reasoning output.
func TestWithEmptyTurnEvidenceOutputFence(t *testing.T) {
	for name, testCase := range map[string]struct {
		observe         func(fixture unclassifiedWorkflowFenceFixture)
		lifecycleOutput bool
		wantOutput      bool
		wantEffect      bool
	}{
		"reasoning only": {observe: func(f unclassifiedWorkflowFenceFixture) {
			f.svc.observePromptReasoning(f.sessionID, "execution-one", 1)
		}},
		"reasoning with answer": {observe: func(f unclassifiedWorkflowFenceFixture) {
			f.svc.observePromptReasoning(f.sessionID, "execution-one", 1)
			f.svc.observePromptAttempt(f.sessionID, "execution-one", 1, true, false)
		}, wantOutput: true},
		"reasoning with tool effect": {observe: func(f unclassifiedWorkflowFenceFixture) {
			f.svc.observePromptReasoning(f.sessionID, "execution-one", 1)
			f.svc.observePromptAttempt(f.sessionID, "execution-one", 1, false, true)
		}, wantEffect: true},
		"lifecycle output preserved over reasoning only": {observe: func(f unclassifiedWorkflowFenceFixture) {
			f.svc.observePromptReasoning(f.sessionID, "execution-one", 1)
		}, lifecycleOutput: true, wantOutput: true},
		"lifecycle output preserved without reasoning": {lifecycleOutput: true, wantOutput: true},
		"no observations": {},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
			defer fixture.svc.stopDynamicSuccessorWorkers()
			beginEmptyTurnPrompt(fixture, "execution-one", 1)
			if testCase.observe != nil {
				testCase.observe(fixture)
			}
			data := watcher.AgentEventData{
				SessionID: fixture.sessionID, AgentExecutionID: "execution-one", PromptGeneration: 1,
				EvidenceKnown: testCase.lifecycleOutput, OutputObserved: testCase.lifecycleOutput,
			}

			got := fixture.svc.withEmptyTurnEvidence(data)
			if !got.EvidenceKnown {
				t.Fatal("EvidenceKnown = false, want the matching attempt's evidence to be known")
			}
			if got.OutputObserved != testCase.wantOutput || got.EffectObserved != testCase.wantEffect {
				t.Fatalf("output/effect = %v/%v, want %v/%v",
					got.OutputObserved, got.EffectObserved, testCase.wantOutput, testCase.wantEffect)
			}
		})
	}
}

// Reasoning keeps the conservative fence for every other failure origin: the
// ordinary evidence path reports it as output, and a failed prompt that
// streamed reasoning is not counted as an unclassified failure.
func TestReasoningBlocksRoutingForNonEmptyTurnFailure(t *testing.T) {
	for name, testCase := range map[string]struct {
		reasoning  bool
		wantStreak bool
	}{
		"without reasoning": {wantStreak: true},
		"with reasoning":    {reasoning: true},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
			defer fixture.svc.stopDynamicSuccessorWorkers()
			data := fixture.event("execution-one", 1)
			if testCase.reasoning {
				fixture.svc.observePromptReasoning(fixture.sessionID, "execution-one", 1)
			}

			evidence := fixture.svc.withPromptAttemptEvidence(data)
			if evidence.OutputObserved != testCase.reasoning || dynamicPreResultSafe(evidence) == testCase.reasoning {
				t.Fatalf("OutputObserved=%v preResultSafe=%v with reasoning=%v",
					evidence.OutputObserved, dynamicPreResultSafe(evidence), testCase.reasoning)
			}

			fixture.svc.routeDynamicAgentFailure(fixture.ctx, data, fixture.failure())
			_, candidate, generation, streak := loadEmptyTurnPolicy(t, fixture)
			if candidate != "candidate-a" || generation != 1 {
				t.Fatalf("route = %s/%d, want candidate-a/1", candidate, generation)
			}
			if (streak != nil) != testCase.wantStreak {
				t.Fatalf("streak = %+v, want counted=%v", streak, testCase.wantStreak)
			}
		})
	}
}

// A reasoning or evidence event from another execution or an older prompt
// generation is stale: it must not change the current attempt's evidence, and
// the stale completion reads back as unknown evidence.
func TestStaleIdentityLeavesPromptEvidenceUnchanged(t *testing.T) {
	for name, testCase := range map[string]struct {
		executionID string
		generation  uint64
	}{
		"older generation":   {executionID: "execution-one", generation: 1},
		"other execution":    {executionID: "execution-stale", generation: 2},
		"older execution id": {executionID: "execution-stale", generation: 1},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
			defer fixture.svc.stopDynamicSuccessorWorkers()
			beginEmptyTurnPrompt(fixture, "execution-one", 2)

			fixture.svc.observePromptReasoning(fixture.sessionID, testCase.executionID, testCase.generation)
			if output, nonReasoning, effect, known := promptEvidenceSnapshot(t, fixture); output || nonReasoning || effect || !known {
				t.Fatalf("evidence after stale reasoning = output %v nonReasoning %v effect %v known %v, want untouched known record",
					output, nonReasoning, effect, known)
			}

			got := fixture.svc.withEmptyTurnEvidence(watcher.AgentEventData{
				SessionID: fixture.sessionID, AgentExecutionID: testCase.executionID, PromptGeneration: testCase.generation,
			})
			if got.EvidenceKnown || got.OutputObserved || got.EffectObserved {
				t.Fatalf("stale completion evidence = %+v, want unknown without output or effect", got)
			}
			if output, nonReasoning, effect, known := promptEvidenceSnapshot(t, fixture); output || nonReasoning || effect || !known {
				t.Fatalf("evidence after stale lookup = output %v nonReasoning %v effect %v known %v, want untouched known record",
					output, nonReasoning, effect, known)
			}
		})
	}
}

// When the prompt attempt record is gone, evidence is reported unknown rather
// than empty, so a cleared record can never authorize counting an empty turn.
// withEmptyTurnEvidence looks the record up a second time after
// withPromptAttemptEvidence; a clear landing between the two lookups takes the
// same EvidenceKnown=false exit, which this fail-closed outcome pins.
func TestWithEmptyTurnEvidenceFailsClosedWithoutRecord(t *testing.T) {
	fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
	defer fixture.svc.stopDynamicSuccessorWorkers()
	beginEmptyTurnPrompt(fixture, "execution-one", 1)
	fixture.svc.observePromptReasoning(fixture.sessionID, "execution-one", 1)
	fixture.svc.clearPromptAttemptEvidence(fixture.sessionID, "execution-one", 1)

	got := fixture.svc.withEmptyTurnEvidence(watcher.AgentEventData{
		SessionID: fixture.sessionID, AgentExecutionID: "execution-one", PromptGeneration: 1,
		EvidenceKnown: true,
	})
	if got.EvidenceKnown || got.OutputObserved || got.EffectObserved || dynamicPreResultSafe(got) {
		t.Fatalf("evidence without a record = %+v, want unknown and not pre-result safe", got)
	}
}
