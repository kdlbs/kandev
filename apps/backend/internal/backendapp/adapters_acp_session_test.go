package backendapp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	orchestratorexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fixedLifecycleProfileResolver struct {
	profile *lifecycle.AgentProfileInfo
}

func (r fixedLifecycleProfileResolver) ResolveProfile(context.Context, string) (*lifecycle.AgentProfileInfo, error) {
	return r.profile, nil
}

func TestLifecycleAdapter_ResolveAgentProfileForwardsNativeSessionResume(t *testing.T) {
	const profileID = "profile-uuid-6f5192e6-634b-4d35-96cf-34f57d6b55cf"
	mgr := lifecycle.NewManager(
		nil, nil, nil, nil,
		fixedLifecycleProfileResolver{profile: &lifecycle.AgentProfileInfo{
			ProfileID: profileID,
			AgentID:   "agent-uuid-cb5a9b96-75bb-4c75-8812-56663e699e6f",
			AgentName: "auggie", NativeSessionResume: true,
		}},
		nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), newTestLogger(),
	)
	adapter := newLifecycleAdapter(mgr, nil, newTestLogger())

	resolved, err := adapter.ResolveAgentProfile(context.Background(), profileID)
	require.NoError(t, err)
	require.Equal(t, "auggie", resolved.AgentName)
	require.True(t, resolved.NativeSessionResume,
		"the production lifecycle adapter must preserve the capability used by recovery admission")
}

type durableDeliveryCapabilityProvider interface {
	DurableDeliveryCapabilityForExecution(
		context.Context,
		string,
	) (runtimeapi.DurableDeliveryCapability, bool)
}

// acpSessionIDProvider mirrors the unexported interface
// orchestrator.Service.currentACPSessionID asserts s.agentManager against.
// Declaring it independently here — instead of importing an orchestrator
// helper — proves the production *lifecycleAdapter satisfies the shape
// orchestrator actually needs, not a copy orchestrator happens to export.
type acpSessionIDProvider interface {
	GetACPSessionIDForSession(sessionID string) (string, bool)
}

type initialPromptAdmissionRegistrar interface {
	RegisterInitialPromptAdmissionCallbacks(string, func() error, func(), func()) error
}

type initialPromptDispatchRegistrar interface {
	RegisterInitialPromptDispatchCallbacks(string, func(), func()) error
}

type submissionAwarePromptProvider interface {
	PromptAgentWithDispatchCallbackAndSubmissionID(
		context.Context,
		string,
		string,
		[]v1.MessageAttachment,
		bool,
		func(),
		string,
	) (*orchestratorexecutor.PromptResult, error)
}

type admissionSubmissionAwarePromptProvider interface {
	PromptAgentWithAdmissionCallbackAndSubmissionID(
		context.Context,
		string,
		string,
		[]v1.MessageAttachment,
		bool,
		func() error,
		func(),
		string,
	) (*orchestratorexecutor.PromptResult, error)
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

func TestLifecycleAdapter_RegistersInitialPromptAdmissionOnRestart(t *testing.T) {
	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), newTestLogger())
	if err := mgr.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
		ID: "exec-model-switch-restart", TaskID: "task-model-switch", SessionID: "session-model-switch",
		WorkspacePath: t.TempDir(),
	}); err != nil {
		t.Fatalf("seed replacement execution: %v", err)
	}
	var client orchestratorexecutor.AgentManagerClient = newLifecycleAdapter(mgr, nil, newTestLogger())
	registrar, ok := client.(initialPromptAdmissionRegistrar)
	if !ok {
		t.Fatal("production lifecycleAdapter cannot register the replacement startup prompt's final admission callback")
	}
	if err := registrar.RegisterInitialPromptAdmissionCallbacks(
		"exec-model-switch-restart", func() error { return nil }, func() {}, func() {},
	); err != nil {
		t.Fatalf("register replacement startup admission: %v", err)
	}
	if err := registrar.RegisterInitialPromptAdmissionCallbacks(
		"missing-model-switch-restart", func() error { return nil }, nil, nil,
	); err == nil {
		t.Fatal("adapter did not forward lifecycle execution lookup errors")
	}
	dispatchRegistrar, ok := client.(initialPromptDispatchRegistrar)
	if !ok {
		t.Fatal("production lifecycleAdapter cannot register the replacement startup prompt's dispatch callbacks")
	}
	if err := dispatchRegistrar.RegisterInitialPromptDispatchCallbacks(
		"exec-model-switch-restart", func() {}, func() {},
	); err != nil {
		t.Fatalf("register replacement startup dispatch callbacks: %v", err)
	}
}

func TestLifecycleAdapter_SatisfiesDurableDeliveryCapabilitySeam(t *testing.T) {
	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), newTestLogger())
	var client orchestratorexecutor.AgentManagerClient = newLifecycleAdapter(mgr, nil, newTestLogger())

	provider, ok := client.(durableDeliveryCapabilityProvider)
	if !ok {
		t.Fatal("production lifecycleAdapter, held as executor.AgentManagerClient, does not satisfy " +
			"DurableDeliveryCapabilityForExecution(context.Context, string); orchestrator would treat " +
			"a durable peer as legacy and omit its canonical SQL submission")
	}
	if _, advertised := provider.DurableDeliveryCapabilityForExecution(context.Background(), "missing-execution"); advertised {
		t.Fatal("missing execution unexpectedly advertised durable delivery")
	}
}

func TestLifecycleAdapter_SatisfiesSubmissionAwarePromptSeam(t *testing.T) {
	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), newTestLogger())
	var client orchestratorexecutor.AgentManagerClient = newLifecycleAdapter(mgr, nil, newTestLogger())

	if _, ok := client.(submissionAwarePromptProvider); !ok {
		t.Fatal("production lifecycleAdapter does not forward the submission-aware prompt capability; " +
			"the executor would fall back to a prompt call that loses the canonical delivery identity")
	}
	if _, ok := client.(admissionSubmissionAwarePromptProvider); !ok {
		t.Fatal("production lifecycleAdapter does not forward the combined admission and submission capability")
	}
}

func TestBuildLifecycleLaunchRequestCarriesInitialDeliverySubmissionID(t *testing.T) {
	request := &orchestratorexecutor.LaunchAgentRequest{
		TaskID:                      "task-1",
		SessionID:                   "session-1",
		InitialDeliverySubmissionID: "message-1",
	}

	launch := buildLifecycleLaunchRequest(request, "/workspace", "profile-1")
	if launch.InitialDeliverySubmissionID != "message-1" {
		t.Fatalf("initial delivery submission ID = %q, want message-1", launch.InitialDeliverySubmissionID)
	}
}
