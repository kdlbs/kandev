package orchestrator

import (
	"testing"

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
