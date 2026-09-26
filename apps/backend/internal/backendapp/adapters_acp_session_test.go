package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	orchestratorexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
)

// acpSessionIDProvider mirrors the unexported interface
// orchestrator.Service.currentACPSessionID asserts s.agentManager against.
// Declaring it independently here — instead of importing an orchestrator
// helper — proves the production *lifecycleAdapter satisfies the shape
// orchestrator actually needs, not a copy orchestrator happens to export.
type acpSessionIDProvider interface {
	GetACPSessionIDForSession(sessionID string) (string, bool)
}

// TestLifecycleAdapter_SatisfiesACPSessionIDSeam is the regression test for a
// QA-caught production gap: orchestrator.Service.currentACPSessionID selects
// its generation-safety seam with an optional type assertion
// (agentManager.(interface{ GetACPSessionIDForSession(string) (string, bool) })).
// mockAgentManager implements that method directly, so orchestrator package
// tests exercised a stronger seam than production ever wired: the concrete
// production type assigned to orchestrator.Service's agentManager field —
// *backendapp.lifecycleAdapter, built by newLifecycleAdapter exactly as
// setupOrchestrator does — did not forward the method, so the assertion
// failed silently at runtime and storeResumeToken's stale-event guard plus
// the reset-failure reconcile (event_handlers_workflow.go) were dead code in
// production despite green unit tests. This asserts the real adapter type,
// held through the same executor.AgentManagerClient interface production
// wires it as, satisfies the seam.
func TestLifecycleAdapter_SatisfiesACPSessionIDSeam(t *testing.T) {
	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), newTestLogger())
	var client orchestratorexecutor.AgentManagerClient = newLifecycleAdapter(mgr, nil, newTestLogger())

	provider, ok := client.(acpSessionIDProvider)
	if !ok {
		t.Fatal("production lifecycleAdapter, held as executor.AgentManagerClient, does not satisfy " +
			"GetACPSessionIDForSession(string) (string, bool) — the same assertion orchestrator.Service." +
			"currentACPSessionID performs would fail silently and disable the generation-safety guard")
	}

	if acpSessionID, found := provider.GetACPSessionIDForSession("no-such-session"); found {
		t.Fatalf("expected (\"\", false) for an unknown session, got (%q, %v)", acpSessionID, found)
	}
}

// TestLifecycleAdapter_GetACPSessionIDForSession_ForwardsLiveIdentity proves
// the forwarder itself observes the live execution's current ACP session id
// through a real *lifecycle.Manager — not a mock's independently-configured
// return value. Seeds an execution directly into the manager's execution
// store (the same seam lifecycle package tests use), then reads it back
// through the adapter exactly as orchestrator.Service.currentACPSessionID
// would.
func TestLifecycleAdapter_GetACPSessionIDForSession_ForwardsLiveIdentity(t *testing.T) {
	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), newTestLogger())
	adapter := newLifecycleAdapter(mgr, nil, newTestLogger())

	const sessionID = "sess-live-acp"
	const liveACPSessionID = "acp-session-currently-owned-by-the-execution"
	if err := mgr.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
		ID:            "exec-live-acp",
		TaskID:        "task-1",
		SessionID:     sessionID,
		ACPSessionID:  liveACPSessionID,
		WorkspacePath: t.TempDir(),
	}); err != nil {
		t.Fatalf("seed execution: %v", err)
	}

	acpSessionID, ok := adapter.GetACPSessionIDForSession(sessionID)
	if !ok {
		t.Fatal("expected the adapter to report the live execution's ACP session id")
	}
	if acpSessionID != liveACPSessionID {
		t.Fatalf("adapter did not forward the manager's live identity: got %q, want %q",
			acpSessionID, liveACPSessionID)
	}

	if _, ok := adapter.GetACPSessionIDForSession("no-such-session"); ok {
		t.Fatal("expected (_, false) for a session with no registered execution")
	}
}
