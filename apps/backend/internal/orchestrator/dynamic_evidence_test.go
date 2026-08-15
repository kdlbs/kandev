package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

func TestDynamicPreResultRequiresExplicitKnownEvidence(t *testing.T) {
	if dynamicPreResultSafe(watcher.AgentEventData{DynamicRouteAttempt: true}) {
		t.Fatal("unknown dynamic attempt was treated as pre-result safe")
	}
	if !dynamicPreResultSafe(watcher.AgentEventData{
		DynamicRouteAttempt: true,
		EvidenceKnown:       true,
	}) {
		t.Fatal("known no-output/no-effect attempt was not pre-result safe")
	}
	if dynamicPreResultSafe(watcher.AgentEventData{
		DynamicRouteAttempt: true,
		EvidenceKnown:       true,
		OutputObserved:      true,
	}) {
		t.Fatal("output-producing attempt was treated as pre-result safe")
	}
	if dynamicPreResultSafe(watcher.AgentEventData{
		DynamicRouteAttempt: true,
		EvidenceKnown:       true,
		EffectObserved:      true,
	}) {
		t.Fatal("effect-producing attempt was treated as pre-result safe")
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
