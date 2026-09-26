package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// Contract coverage: a recognized provider failure still cannot override the
// dynamic route's pre-result and effect-safety gate.
func TestDynamicPreResultRequiresExplicitKnownEvidence(t *testing.T) {
	usageLimitFailure := watcher.AgentEventData{
		AgentID:             "codex-acp",
		ErrorMessage:        `{"code":-32603,"message":"Internal error","data":{"codexErrorInfo":"usageLimitExceeded","message":"You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Sep 1st, 2026 3:14 PM."}}`,
		DynamicRouteAttempt: true,
		EvidenceKnown:       true,
	}
	if !dynamicPreResultSafe(usageLimitFailure) {
		t.Fatal("pre-result usage-limit failure was not safe to route")
	}

	unsafeCases := []struct {
		name string
		data watcher.AgentEventData
	}{
		{name: "unknown evidence", data: watcher.AgentEventData{DynamicRouteAttempt: true}},
		{name: "assistant output", data: func() watcher.AgentEventData {
			data := usageLimitFailure
			data.OutputObserved = true
			return data
		}()},
		{name: "tool effect", data: func() watcher.AgentEventData {
			data := usageLimitFailure
			data.EffectObserved = true
			return data
		}()},
	}
	for _, test := range unsafeCases {
		t.Run(test.name, func(t *testing.T) {
			if dynamicPreResultSafe(test.data) {
				t.Fatalf("case %q was incorrectly treated as pre-result safe", test.name)
			}
		})
	}
}

func TestDynamicAttemptEvidenceTreatsMatchingACPProviderDiagnosticAsPreResult(t *testing.T) {
	var service Service
	const message = "API Error: Repeated 529 Overloaded errors. The API is at capacity."
	service.beginPromptAttempt("session-1", "execution-1", 1, true)
	service.observeProviderDiagnostic("session-1", "execution-1", 1, message)

	got := service.withDynamicAttemptEvidence(watcher.AgentEventData{
		SessionID:        "session-1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 1,
		ErrorMessage:     message,
	})
	if got.OutputObserved {
		t.Fatal("matching ACP provider diagnostic was treated as generated output")
	}
	if !dynamicPreResultSafe(got) {
		t.Fatal("matching ACP provider diagnostic was not safe to route")
	}

	// A different failure must not erase the output fence.
	service.beginPromptAttempt("session-2", "execution-2", 1, true)
	service.observeProviderDiagnostic("session-2", "execution-2", 1, message)
	other := service.withDynamicAttemptEvidence(watcher.AgentEventData{
		SessionID:        "session-2",
		AgentExecutionID: "execution-2",
		PromptGeneration: 1,
		ErrorMessage:     "API Error: 500 Internal server error",
	})
	if !other.OutputObserved {
		t.Fatal("mismatched provider failure erased the output fence")
	}
	if gotCode := matchingProviderFailureCode(got); gotCode != routingerr.CodeProviderOverloaded {
		t.Fatalf("matching provider failure code = %q, want %q", gotCode, routingerr.CodeProviderOverloaded)
	}
}

func TestDynamicAttemptEvidenceRequiresLocalIdentityFence(t *testing.T) {
	var service Service
	got := service.withDynamicAttemptEvidence(watcher.AgentEventData{
		SessionID:        "session-1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 7,
		EvidenceKnown:    true,
	})
	if got.EvidenceKnown {
		t.Fatal("lifecycle evidence without a local attempt record was accepted")
	}
	if dynamicPreResultSafe(got) {
		t.Fatal("lifecycle evidence without a local attempt record was treated as pre-result safe")
	}
}

func TestDynamicAttemptEvidenceRejectsAmbiguousExecutionEvents(t *testing.T) {
	var service Service
	service.beginDynamicAttempt("session-1")
	service.bindDynamicAttemptExecution("session-1", "execution-1")

	service.observeDynamicAttempt("session-1", "", true, false)
	got := service.withDynamicAttemptEvidence(watcher.AgentEventData{
		SessionID:           "session-1",
		AgentExecutionID:    "execution-1",
		DynamicRouteAttempt: true,
	})
	if got.EvidenceKnown {
		t.Fatal("missing execution identity did not invalidate evidence")
	}
	if dynamicPreResultSafe(got) {
		t.Fatal("ambiguous execution event was treated as pre-result safe")
	}
}

func TestDynamicAttemptEvidenceRejectsStaleExecution(t *testing.T) {
	var service Service
	service.beginDynamicAttempt("session-1")
	service.bindDynamicAttemptExecution("session-1", "execution-2")

	got := service.withDynamicAttemptEvidence(watcher.AgentEventData{
		SessionID:        "session-1",
		AgentExecutionID: "execution-1",
	})
	if got.EvidenceKnown {
		t.Fatal("stale execution was accepted by evidence fence")
	}
	if dynamicPreResultSafe(got) {
		t.Fatal("stale execution was treated as pre-result safe")
	}
}
