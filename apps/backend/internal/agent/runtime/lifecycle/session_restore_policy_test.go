package lifecycle

import (
	"context"
	"errors"
	"expvar"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestRestorePolicyPreservesNativeIdentity(t *testing.T) {
	t.Parallel()

	identity := RestoreIdentity{
		SessionID:         "session-1",
		IncarnationID:     "incarnation-1",
		HarnessGeneration: 3,
		NativeSessionID:   "native-1",
	}

	for _, reason := range []RestoreReason{
		RestoreReasonTransport,
		RestoreReasonAuthentication,
		RestoreReasonConfiguration,
		RestoreReasonPermission,
		RestoreReasonUnknown,
	} {
		t.Run(string(reason), func(t *testing.T) {
			decision := DecideRestore(RestoreRequest{
				Identity: identity,
				Failure:  RestoreFailure{Reason: reason},
				Action:   RestoreActionNativeResume,
			})
			if decision.Outcome != RestoreOutcomeBlocked {
				t.Fatalf("outcome = %q, want %q", decision.Outcome, RestoreOutcomeBlocked)
			}
			if !decision.PreserveNativeIdentity {
				t.Fatal("blocked restore must preserve the native identity")
			}
			if decision.AllowsContextContinuation {
				t.Fatal("non-state failures must not authorize context continuation")
			}
		})
	}
}

func TestRestorePolicyRequiresExplicitContinuation(t *testing.T) {
	t.Parallel()

	base := RestoreRequest{
		Identity: RestoreIdentity{
			SessionID:       "session-1",
			NativeSessionID: "native-1",
		},
		Failure: RestoreFailure{Reason: RestoreReasonNativeStateMissing},
	}

	blocked := DecideRestore(base)
	if blocked.Outcome != RestoreOutcomeBlocked || !blocked.PreserveNativeIdentity {
		t.Fatalf("implicit recovery decision = %+v, want blocked native-preserving decision", blocked)
	}
	if !blocked.AllowsContextContinuation {
		t.Fatal("missing native state should expose an explicit continuation action")
	}

	continued := DecideRestore(RestoreRequest{
		Identity:              base.Identity,
		Failure:               base.Failure,
		Action:                RestoreActionContinueFromHistory,
		ExplicitAuthorization: true,
	})
	if continued.Outcome != RestoreOutcomeContextContinued {
		t.Fatalf("authorized continuation outcome = %q, want %q", continued.Outcome, RestoreOutcomeContextContinued)
	}
	if !continued.PreserveNativeIdentity {
		t.Fatal("native identity must remain recoverable until replacement commits")
	}

	branch := DecideRestore(RestoreRequest{
		Identity:              base.Identity,
		Failure:               base.Failure,
		Action:                RestoreActionResumeNewBranch,
		ExplicitAuthorization: true,
	})
	if branch.Outcome != RestoreOutcomeBlocked || branch.AllowsContextContinuation {
		t.Fatalf("branch replacement decision = %+v, want native-only blocked decision", branch)
	}
}

func TestRestorePolicyUsesTypedAgentctlReason(t *testing.T) {
	t.Parallel()

	err := &agentctl.SessionRestoreOperationError{
		Code:    "SESSION_RESTORE_REQUIRED",
		Message: "native session state is unavailable",
		Details: map[string]interface{}{"reason": "native_state_missing"},
	}
	if got := classifyRestoreFailure(err); got != RestoreReasonNativeStateMissing {
		t.Fatalf("typed restore reason = %q, want %q", got, RestoreReasonNativeStateMissing)
	}

	unknown := &agentctl.SessionRestoreOperationError{
		Code:    "SESSION_RESTORE_BLOCKED",
		Message: "restore blocked",
		Details: map[string]interface{}{"reason": "provider_specific_failure"},
	}
	if got := classifyRestoreFailure(unknown); got != RestoreReasonUnknown {
		t.Fatalf("unknown typed restore reason = %q, want %q", got, RestoreReasonUnknown)
	}
}

func TestInitializeSessionNativeLoadFailureDoesNotCreateReplacement(t *testing.T) {
	attempts := expvar.Get("agent_restore_attempts_total").(*expvar.Map)
	metricKey := "outcome=blocked;reason=native_state_missing;agent_type=other"
	beforeMetric := expvarValue(attempts, metricKey)

	mock := newMockAgentServer(t)
	defer mock.Close()
	mock.handler = func(msg ws.Message) *ws.Message {
		switch msg.Action {
		case "agent.initialize":
			resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{
				"success": true,
				"agent_info": map[string]string{
					"name":    "test-agent",
					"version": "1.0.0",
				},
			})
			return resp
		case "agent.session.load":
			return mustNewSessionError(msg)
		default:
			return mock.defaultHandler(msg)
		}
	}

	client := createTestClient(t, mock.server.URL)
	defer client.Close()
	if err := client.StreamUpdates(context.Background(), func(agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("connect stream: %v", err)
	}
	waitForWSConnected(t, mock)

	sm := NewSessionManager(newSessionTestLogger(), make(chan struct{}))
	config := &testAgent{
		id:      "test-agent",
		enabled: true,
		runtimeConfig: &agents.RuntimeConfig{
			Cmd:      agents.NewCommand("test-agent"),
			Protocol: agent.ProtocolACP,
			SessionConfig: agents.SessionConfig{
				NativeSessionResume: true,
			},
			ResourceLimits: agents.ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
		},
	}

	_, err := sm.InitializeSession(context.Background(), client, config, "native-1", "/workspace", nil)
	if err == nil {
		t.Fatal("expected native load failure")
	}
	var restoreErr *RestoreRequiredError
	if !errors.As(err, &restoreErr) {
		t.Fatalf("error = %v, want RestoreRequiredError", err)
	}
	if restoreErr.Decision.Reason != RestoreReasonNativeStateMissing {
		t.Fatalf("restore reason = %q, want %q", restoreErr.Decision.Reason, RestoreReasonNativeStateMissing)
	}
	if afterMetric := expvarValue(attempts, metricKey); afterMetric != beforeMetric+1 {
		t.Fatalf("restore attempt metric = %d, want %d", afterMetric, beforeMetric+1)
	}

	actions := mock.getActionLog()
	for _, action := range actions {
		if action == "agent.session.new" {
			t.Fatal("native load failure must not silently create a replacement session")
		}
	}
}

func expvarValue(metrics *expvar.Map, key string) int64 {
	if value := metrics.Get(key); value != nil {
		return value.(*expvar.Int).Value()
	}
	return 0
}

func mustNewSessionError(msg ws.Message) *ws.Message {
	resp, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Resource not found", nil)
	if err != nil {
		panic(err)
	}
	return resp
}
