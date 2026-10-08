package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// gatewayServerFailureSample is a text sample that classifies as a
// high-confidence, fallback-allowed provider diagnostic on its own — used to
// prove that handleAgentStreamEvent's original message_chunk dispatch trusts the
// carried ProviderDiagnosticCandidate marker rather than re-deriving that
// classification itself from the chunk text
// (AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.20).
const gatewayServerFailureSample = "API Error: 500 Internal server error."

func TestHandleAgentStreamEvent_CodexUsageLimitDiagnosticCorrelatesWithPromptError(t *testing.T) {
	svc, _ := newTransientTestService(t)
	armTransientPromptEvidence(svc)
	const notice = "You've hit your usage limit. try again at Sep 27th, 2026 3:09 AM"

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "execution-1",
		AgentType:   "codex-acp",
		Data: &lifecycle.AgentStreamEventData{
			Type:                        "message_chunk",
			Text:                        notice,
			PromptGeneration:            7,
			ProviderDiagnosticCandidate: true,
		},
	})

	got := svc.withPromptAttemptEvidence(watcher.AgentEventData{
		SessionID:        "s1",
		AgentExecutionID: "execution-1",
		AgentID:          "codex-acp",
		PromptGeneration: 7,
		ErrorMessage:     "Internal error",
		ProviderError: &streams.ProviderError{
			Source:     streams.ProviderErrorSourceCodexACP,
			ProviderID: "codex-acp",
			Message:    notice,
			OccurredAt: time.Now().UTC(),
		},
	})
	if got.OutputObserved {
		t.Fatal("matching Codex usage-limit diagnostic and typed prompt error were treated as generated output")
	}
	if !svc.promptAttemptPreResultSafe(got) {
		t.Fatal("original Codex diagnostic did not satisfy the existing pre-result recovery fence")
	}
}

// TestHandleAgentStreamEvent_MessageChunkHonorsUnmarkedProviderDiagnosticText
// proves the orchestrator no longer re-derives the provider-diagnostic
// classification from message_chunk text: a chunk whose text would
// classify as a high-confidence diagnostic on its own, but which arrives
// without the carried marker (as a non-assistant chunk would from the ACP
// conversion boundary), must be treated as ordinary output.
func TestHandleAgentStreamEvent_MessageChunkHonorsUnmarkedProviderDiagnosticText(t *testing.T) {
	svc, _ := newTransientTestService(t)
	armTransientPromptEvidence(svc)

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "execution-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:                        "message_chunk",
			Text:                        gatewayServerFailureSample,
			PromptGeneration:            7,
			ProviderDiagnosticCandidate: false,
		},
	})

	got := svc.withPromptAttemptEvidence(watcher.AgentEventData{
		SessionID:        "s1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 7,
		ErrorMessage:     gatewayServerFailureSample,
	})
	if !got.OutputObserved {
		t.Fatal("unmarked chunk was treated as a transport diagnostic by re-deriving classification from text")
	}
}

// TestHandleAgentStreamEvent_MessageChunkTracksMarkedProviderDiagnosticForCorrelation
// proves a marked chunk still feeds the AC.23 diagnostic-code/text correlation
// fence: once handleAgentStreamEvent reads the carried marker, a terminal
// failure whose normalized message contains the recorded diagnostic text stays
// safe to automatically retry.
func TestHandleAgentStreamEvent_MessageChunkTracksMarkedProviderDiagnosticForCorrelation(t *testing.T) {
	svc, _ := newTransientTestService(t)
	armTransientPromptEvidence(svc)

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "execution-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:                        "message_chunk",
			Text:                        gatewayServerFailureSample,
			PromptGeneration:            7,
			ProviderDiagnosticCandidate: true,
		},
	})

	got := svc.withPromptAttemptEvidence(watcher.AgentEventData{
		SessionID:        "s1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 7,
		ErrorMessage:     "Internal error: " + gatewayServerFailureSample + " This is a server-side issue, usually temporary.",
	})
	if got.OutputObserved {
		t.Fatal("marked provider-diagnostic chunk contained in the terminal message was treated as generated output")
	}
	if !svc.promptAttemptPreResultSafe(got) {
		t.Fatal("marked provider-diagnostic chunk contained in the terminal message was not pre-result safe")
	}
}

// TestHandleAgentStreamEvent_MessageChunkMarkedDiagnosticDoesNotAdvanceTurnProgress
// pins AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.21's "shall not advance turn
// progress" clause at the original message_chunk dispatch site: a marked
// diagnostic must stay out of generated-output progress while it remains
// available to the guarded recovery correlation check.
func TestHandleAgentStreamEvent_MessageChunkMarkedDiagnosticDoesNotAdvanceTurnProgress(t *testing.T) {
	svc, mc := newTransientTestService(t)
	armTransientPromptEvidence(svc)
	eb := &recordingEventBus{}
	svc.eventBus = eb

	svc.registerBackgroundTask("s1", "subagent-1")
	emitForegroundIdle(svc, "t1", "s1")
	if got := svc.foregroundActivityValue("s1"); got != v1.ForegroundActivityBackground {
		t.Fatalf("setup: session must be background-idle before the marked chunk arrives, got %q", got)
	}
	eb.events = nil

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "execution-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:                        "message_chunk",
			Text:                        gatewayServerFailureSample,
			PromptGeneration:            7,
			ProviderDiagnosticCandidate: true,
		},
	})

	if got := activityValues(eb); len(got) != 0 {
		t.Fatalf("marked provider-diagnostic chunk must not advance turn progress / publish an activity change, got %v", got)
	}
	if got := svc.foregroundActivityValue("s1"); got != v1.ForegroundActivityBackground {
		t.Fatalf("marked provider-diagnostic chunk flipped the session out of background-idle, got %q", got)
	}
	if mc.agentStreamWrites != 0 {
		t.Fatalf("original diagnostic evidence must not reach the transcript writer, got %d writes", mc.agentStreamWrites)
	}
	got := svc.withPromptAttemptEvidence(watcher.AgentEventData{
		SessionID:        "s1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 7,
		ErrorMessage:     gatewayServerFailureSample,
	})
	if got.OutputObserved || !svc.promptAttemptPreResultSafe(got) {
		t.Fatalf("matching original diagnostic evidence = %+v, want pre-result-safe without generated output", got)
	}
}

func TestHandleAgentStreamEvent_TranscriptProjectionDoesNotChangeRecoveryEvidence(t *testing.T) {
	for _, projection := range []struct {
		name      string
		typeName  string
		candidate bool
	}{
		{name: "message", typeName: "message_streaming"},
		{name: "message_marked", typeName: "message_streaming", candidate: true},
		{name: "thinking", typeName: "thinking_streaming"},
		{name: "thinking_marked", typeName: "thinking_streaming", candidate: true},
	} {
		t.Run(projection.name, func(t *testing.T) {
			svc, mc := newTransientTestService(t)
			eb := &recordingEventBus{}
			svc.eventBus = eb
			armTransientPromptEvidence(svc)

			// Seed the genuine per-chunk recovery evidence first. A later
			// transcript projection must neither clear nor duplicate it.
			svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
				TaskID:      "t1",
				SessionID:   "s1",
				ExecutionID: "execution-1",
				Data: &lifecycle.AgentStreamEventData{
					Type:                        "message_chunk",
					Text:                        gatewayServerFailureSample,
					PromptGeneration:            7,
					ProviderDiagnosticCandidate: true,
				},
			})
			evidence, ok := svc.promptAttemptForSession("s1")
			if !ok {
				t.Fatal("prompt evidence disappeared after original diagnostic")
			}
			evidence.mu.Lock()
			originalCode, originalText := evidence.providerDiagnosticCode, evidence.providerDiagnosticText
			evidence.mu.Unlock()
			if originalCode == "" || originalText == "" {
				t.Fatalf("original diagnostic evidence = %q/%q, want classified code and text", originalCode, originalText)
			}

			terminalFailure := watcher.AgentEventData{
				SessionID:        "s1",
				AgentExecutionID: "execution-1",
				PromptGeneration: 7,
				ErrorMessage:     gatewayServerFailureSample,
			}
			if got := svc.withPromptAttemptEvidence(terminalFailure); !svc.promptAttemptPreResultSafe(got) {
				t.Fatalf("seeded diagnostic did not satisfy matching terminal-error fence: %+v", got)
			}

			svc.registerBackgroundTask("s1", "subagent-1")
			emitForegroundIdle(svc, "t1", "s1")
			eb.events = nil

			svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
				TaskID:      "t1",
				SessionID:   "s1",
				ExecutionID: "execution-1",
				Data: &lifecycle.AgentStreamEventData{
					Type:                        projection.typeName,
					MessageID:                   "msg-1",
					Text:                        "merged visible transcript projection",
					PromptGeneration:            7,
					ProviderDiagnosticCandidate: projection.candidate,
				},
			})

			evidence.mu.Lock()
			if evidence.effect || evidence.providerDiagnosticCode != originalCode || evidence.providerDiagnosticText != originalText {
				t.Fatalf("%s projection changed recorded diagnostic: effect=%v diagnostic=%q/%q, want %q/%q", projection.name, evidence.effect, evidence.providerDiagnosticCode, evidence.providerDiagnosticText, originalCode, originalText)
			}
			evidence.mu.Unlock()
			if got := svc.foregroundActivityValue("s1"); got != v1.ForegroundActivityBackground {
				t.Fatalf("%s projection changed foreground activity to %q", projection.name, got)
			}
			if got := activityValues(eb); len(got) != 0 {
				t.Fatalf("%s projection published foreground progress: %v", projection.name, got)
			}
			if got := svc.withPromptAttemptEvidence(terminalFailure); !svc.promptAttemptPreResultSafe(got) {
				t.Fatalf("%s projection broke matching-error pre-result safety: %+v", projection.name, got)
			}
			if projection.typeName == "thinking_streaming" {
				if mc.thinkingWrites != 1 {
					t.Fatalf("%s projection writes = %d, want visible content to remain persisted", projection.name, mc.thinkingWrites)
				}
			} else if mc.agentStreamWrites != 1 || len(mc.agentStreamTexts) != 1 || mc.agentStreamTexts[0] != "merged visible transcript projection" {
				t.Fatalf("%s projection writes = %d, texts=%v, want visible content to remain persisted", projection.name, mc.agentStreamWrites, mc.agentStreamTexts)
			}
		})
	}
}

func TestHandleAgentStreamEvent_OriginalAssistantAndReasoningOutputDoesNotWriteTranscript(t *testing.T) {
	for _, tc := range []struct {
		name     string
		typeName string
		text     string
	}{
		{name: "assistant", typeName: "message_chunk", text: "ordinary answer"},
		{name: "reasoning", typeName: "reasoning", text: "thinking through the answer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, mc := newTransientTestService(t)
			eb := &recordingEventBus{}
			svc.eventBus = eb
			armTransientPromptEvidence(svc)
			svc.registerBackgroundTask("s1", "subagent-1")
			emitForegroundIdle(svc, "t1", "s1")
			eb.events = nil
			svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
				TaskID:      "t1",
				SessionID:   "s1",
				ExecutionID: "execution-1",
				Data: &lifecycle.AgentStreamEventData{
					Type:             tc.typeName,
					Text:             tc.text,
					PromptGeneration: 7,
				},
			})

			got := svc.withPromptAttemptEvidence(watcher.AgentEventData{
				SessionID:        "s1",
				AgentExecutionID: "execution-1",
				PromptGeneration: 7,
			})
			if !got.OutputObserved {
				t.Fatal("ordinary original output was not recorded as progress")
			}
			if mc.agentStreamWrites != 0 {
				t.Fatalf("original %s event wrote transcript %d times", tc.typeName, mc.agentStreamWrites)
			}
			if got := svc.foregroundActivityValue("s1"); got != v1.ForegroundActivityGenerating {
				t.Fatalf("ordinary original %s output left foreground activity at %q", tc.typeName, got)
			}
			if got := activityValues(eb); len(got) != 1 || got[0] != string(v1.ForegroundActivityGenerating) {
				t.Fatalf("ordinary original %s output published foreground activity %v, want one generating transition", tc.typeName, got)
			}
		})
	}
}
