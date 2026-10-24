package orchestrator

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// TestPromptAttemptPreResultSafeRepeatedTerminalEventOnlyFirstWins pins R3-F11:
// duplicate or repeated delivery of the same terminal failure event must not
// authorize recovery twice. There is no new synchronization for this — the
// existing identity fence plus clearPromptAttemptEvidence's first-write-wins
// deletion is the whole mechanism, so a redelivered event after the first
// consumer has cleared the record finds no evidence and is rejected.
func TestPromptAttemptPreResultSafeRepeatedTerminalEventOnlyFirstWins(t *testing.T) {
	var service Service
	service.beginPromptAttempt("session-1", "execution-1", 5, false)

	data := watcher.AgentEventData{
		SessionID:        "session-1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 5,
		ErrorMessage:     "some terminal provider failure",
	}

	first := service.withPromptAttemptEvidence(data)
	if !first.EvidenceKnown || first.OutputObserved || first.EffectObserved {
		t.Fatalf("first terminal event evidence = %+v, want known with no output/effect", first)
	}
	if !service.promptAttemptPreResultSafe(first) {
		t.Fatal("first delivery of the terminal event was not pre-result safe")
	}
	service.clearPromptAttemptEvidence("session-1", "execution-1", 5)

	second := service.withPromptAttemptEvidence(data)
	if second.EvidenceKnown {
		t.Fatal("repeated terminal event still reported known evidence after the first delivery cleared it")
	}
	if service.promptAttemptPreResultSafe(second) {
		t.Fatal("repeated terminal event was treated as pre-result safe a second time")
	}
}

// TestObserveProviderDiagnosticAfterTerminalFailureClearedIsNoOp pins R3-F11's
// other ordering case: a diagnostic notification that arrives after the
// terminal failure has already been observed and cleared must not resurrect
// or fabricate evidence for that execution/generation. observeProviderDiagnostic
// only writes through promptAttemptForSession's Load, never LoadOrStore, so a
// cleared record stays absent.
func TestObserveProviderDiagnosticAfterTerminalFailureClearedIsNoOp(t *testing.T) {
	var service Service
	const diagnostic = "API Error: Repeated 529 Overloaded errors. The API is at capacity."

	service.beginPromptAttempt("session-1", "execution-1", 5, false)
	service.clearPromptAttemptEvidence("session-1", "execution-1", 5)

	// Late-arriving diagnostic notification for the already-cleared attempt.
	service.observeProviderDiagnostic("session-1", "execution-1", 5, "", diagnostic)

	got := service.withPromptAttemptEvidence(watcher.AgentEventData{
		SessionID:        "session-1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 5,
		ErrorMessage:     diagnostic,
	})
	if got.EvidenceKnown {
		t.Fatal("late provider diagnostic notification resurrected evidence cleared by an earlier terminal failure")
	}
}

// TestPromptAttemptEvidenceUsesLifecycleDiagnosticWhenFailureArrivesFirst
// models the two NATS subscriptions that can deliver agent.failed before the
// matching agent.stream event. The terminal lifecycle snapshot must carry the
// marked diagnostic so a matching provider failure stays safe, while a
// mismatched terminal message still fails closed.
func TestPromptAttemptEvidenceUsesLifecycleDiagnosticWhenFailureArrivesFirst(t *testing.T) {
	const diagnostic = "API Error: Repeated 529 Overloaded errors. The API is at capacity."

	for _, tc := range []struct {
		name     string
		terminal string
		wantSafe bool
	}{
		{
			name:     "matching terminal failure",
			terminal: "Internal error: " + diagnostic + " This is a server-side issue, usually temporary - try again in a moment.",
			wantSafe: true,
		},
		{
			name:     "mismatched terminal failure",
			terminal: "Internal error: API Error: 500 Internal server error. This is a server-side issue, usually temporary - try again in a moment.",
			wantSafe: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var service Service
			service.beginPromptAttempt("session-1", "execution-1", 5, false)

			got := service.withPromptAttemptEvidence(watcher.AgentEventData{
				SessionID:                   "session-1",
				AgentExecutionID:            "execution-1",
				PromptGeneration:            5,
				EvidenceKnown:               true,
				ProviderDiagnosticCandidate: true,
				ProviderDiagnosticText:      diagnostic,
				ErrorMessage:                tc.terminal,
			})
			if service.promptAttemptPreResultSafe(got) != tc.wantSafe {
				t.Fatalf("promptAttemptPreResultSafe = %v, want %v; evidence = %+v", service.promptAttemptPreResultSafe(got), tc.wantSafe, got)
			}
		})
	}
}

// TestPromptAttemptEvidenceStructuredTerminalIsOrderIndependent pins that a
// text-classified diagnostic chunk still matches a terminal failure whose
// structured signals refine the code (Codex usage-limit prose classifies as
// quota_limited, HTTP 429 as rate_limited). Recovery must not depend on
// whether the stream chunk or the terminal failure is delivered first.
func TestPromptAttemptEvidenceStructuredTerminalIsOrderIndependent(t *testing.T) {
	const diagnostic = "You've hit your usage limit. try again at Sep 27th, 2026 3:09 AM UTC"
	terminal := func() watcher.AgentEventData {
		return watcher.AgentEventData{
			SessionID:        "session-1",
			AgentExecutionID: "execution-1",
			PromptGeneration: 5,
			AgentID:          "codex-acp",
			ErrorMessage:     diagnostic,
			ProviderError: &streams.ProviderError{
				Source:     streams.ProviderErrorSourceCodexACP,
				Message:    diagnostic,
				HTTPStatus: 429,
				OccurredAt: time.Date(2026, 9, 27, 2, 0, 0, 0, time.UTC),
			},
		}
	}

	t.Run("chunk first", func(t *testing.T) {
		var service Service
		service.beginPromptAttempt("session-1", "execution-1", 5, false)
		service.observeProviderDiagnostic("session-1", "execution-1", 5, "codex-acp", diagnostic)

		got := service.withPromptAttemptEvidence(terminal())
		if !service.promptAttemptPreResultSafe(got) {
			t.Fatalf("chunk-first structured terminal failure was not pre-result safe; evidence = %+v", got)
		}
	})

	t.Run("terminal first", func(t *testing.T) {
		var service Service
		service.beginPromptAttempt("session-1", "execution-1", 5, false)

		data := terminal()
		data.EvidenceKnown = true
		data.ProviderDiagnosticCandidate = true
		data.ProviderDiagnosticText = diagnostic
		got := service.withPromptAttemptEvidence(data)
		if !service.promptAttemptPreResultSafe(got) {
			t.Fatalf("terminal-first structured terminal failure was not pre-result safe; evidence = %+v", got)
		}
	})

	t.Run("chunk first with ordinary output", func(t *testing.T) {
		var service Service
		service.beginPromptAttempt("session-1", "execution-1", 5, false)
		service.observeProviderDiagnostic("session-1", "execution-1", 5, "codex-acp", diagnostic)
		service.observePromptAttempt("session-1", "execution-1", 5, true, false)

		got := service.withPromptAttemptEvidence(terminal())
		if service.promptAttemptPreResultSafe(got) {
			t.Fatal("ordinary output after the diagnostic chunk was erased by the structured terminal failure")
		}
	})
}
