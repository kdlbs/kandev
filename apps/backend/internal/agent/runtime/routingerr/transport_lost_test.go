package routingerr

import (
	"testing"
	"time"
)

// realPeerDisconnected is the exact ACP transport-death envelope observed in
// production: the upstream provider connection drops mid-turn on prompt-send,
// with no subprocess exit and no session/load failure.
const realPeerDisconnected = `{"code":-32603,"message":"Internal error","data":{"error":"peer disconnected before response"}}`

func TestMatchRuntimeEnvironmentRules_TransportLost(t *testing.T) {
	got, ok := matchRuntimeEnvironmentRules(realPeerDisconnected)
	if !ok {
		t.Fatalf("expected match, got none")
	}
	if got.Code != CodeAgentTransportLost {
		t.Fatalf("Code = %q, want %q", got.Code, CodeAgentTransportLost)
	}
	if got.ClassifierRule != transportLostRuleID {
		t.Fatalf("ClassifierRule = %q, want %q", got.ClassifierRule, transportLostRuleID)
	}
	if got.Confidence != ConfHigh {
		t.Fatalf("Confidence = %q, want %q", got.Confidence, ConfHigh)
	}
}

func TestMatchRuntimeEnvironmentRules_TransportLost_ConnectionClosed(t *testing.T) {
	got, ok := matchRuntimeEnvironmentRules("agent error: connection closed unexpectedly")
	if !ok || got.Code != CodeAgentTransportLost {
		t.Fatalf("expected agent_transport_lost match, got ok=%v err=%v", ok, got)
	}
}

func TestClassify_TransportLost_Invariants(t *testing.T) {
	resetInjection()
	e := Classify(Input{
		Phase:      PhasePromptSend,
		ProviderID: "claude-acp",
		Stderr:     realPeerDisconnected,
	})
	if e == nil {
		t.Fatal("expected non-nil Error")
	}
	if e.Code != CodeAgentTransportLost {
		t.Fatalf("Code = %q, want %q", e.Code, CodeAgentTransportLost)
	}
	if !e.AutoRetryable {
		t.Errorf("AutoRetryable = false, want true (recoverable transport drop)")
	}
	if e.FallbackAllowed {
		t.Errorf("FallbackAllowed = true, want false (the resume token belongs to the current provider)")
	}
	if e.UserAction {
		t.Errorf("UserAction = true, want false (no user action needed)")
	}
	if e.Phase != PhasePromptSend {
		t.Errorf("Phase = %q, want %q", e.Phase, PhasePromptSend)
	}
}

func TestDecide_TransportLost_ShortRetry(t *testing.T) {
	resetInjection()
	e := Classify(Input{Phase: PhasePromptSend, ProviderID: "claude-acp", Stderr: realPeerDisconnected})
	if got := Decide(ContextKanban, e, time.Time{}); got != DecisionShortRetry {
		t.Fatalf("Decide(ContextKanban, ...) = %q, want %q", got, DecisionShortRetry)
	}
	if got := Decide(ContextOffice, e, time.Time{}); got != DecisionShortRetry {
		t.Fatalf("Decide(ContextOffice, ...) = %q, want %q", got, DecisionShortRetry)
	}
}

func TestIsTransientProviderError_TransportLost(t *testing.T) {
	if !IsTransientProviderError(realPeerDisconnected) {
		t.Errorf("IsTransientProviderError(peer disconnected) = false, want true")
	}
}

func TestIsAvailabilityCode_TransportLostExcluded(t *testing.T) {
	if IsAvailabilityCode(CodeAgentTransportLost) {
		t.Errorf("IsAvailabilityCode(CodeAgentTransportLost) = true, want false (not a model-availability problem)")
	}
}

func TestClassify_TransportLost_ExcludesCancellationSignatures(t *testing.T) {
	cases := []string{
		"context canceled",
		"context deadline exceeded",
		"cancel escalated: prompt abandoned after cancel",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			resetInjection()
			e := Classify(Input{Phase: PhasePromptSend, ProviderID: "claude-acp", Stderr: msg})
			if e.Code == CodeAgentTransportLost {
				t.Errorf("Classify(%q).Code = %q, want anything but %q", msg, e.Code, CodeAgentTransportLost)
			}
			if Decide(ContextKanban, e, time.Time{}) == DecisionShortRetry {
				t.Errorf("Decide(ContextKanban, Classify(%q)) = DecisionShortRetry, want not short-retry", msg)
			}
		})
	}
}

func TestClassify_TransportLost_OverloadedTakesPrecedence(t *testing.T) {
	resetInjection()
	msg := `API Error: 529 Overloaded. peer disconnected before response`
	e := Classify(Input{Phase: PhasePromptSend, ProviderID: "claude-acp", Stderr: msg})
	if e.Code != CodeProviderOverloaded {
		t.Fatalf("Code = %q, want %q (529-overloaded rule must win when both signatures co-occur)", e.Code, CodeProviderOverloaded)
	}
}
