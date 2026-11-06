package acp

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// Remote prompt errors preserve structured capacity evidence even when the
// visible message is generic. Runtime retention follows response provenance;
// classification remains independent and provider-scoped.
//
// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.32
func TestRetainedACPApplicationErrorPreservesStructuredProviderSignals(t *testing.T) {
	for _, test := range []struct {
		name         string
		provider     string
		data         any
		wantCapacity bool
	}{
		{name: "claude rate limit kind", provider: claudeAgentID, data: map[string]any{"errorKind": "rate_limit_error"}, wantCapacity: true},
		{name: "claude overloaded kind", provider: claudeAgentID, data: map[string]any{"errorKind": "overloaded_error"}, wantCapacity: true},
		{name: "explicit http 429", provider: codexAgentID, data: map[string]any{"statusCode": float64(429), "errorCode": "rate_limit"}, wantCapacity: true},
		{name: "explicit http 529", provider: claudeAgentID, data: map[string]any{"statusCode": float64(529)}, wantCapacity: true},
		{name: "kind outside its provider namespace", provider: codexAgentID, data: map[string]any{"errorKind": "rate_limit_error"}},
		{name: "generic wrapper kind", provider: claudeAgentID, data: map[string]any{"errorKind": "AI_APICallError"}},
		{name: "fractional status ignored", provider: claudeAgentID, data: map[string]any{"statusCode": 429.5}},
		{name: "opaque provider code only", provider: claudeAgentID, data: map[string]any{"errorCode": "rate_limit"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := newTestAdapterForAgent(test.provider)
			err := remoteACPRequestError(t, -32603, "Internal error", test.data)
			providerError, ok := a.retainedACPApplicationError(err)
			if !ok || providerError == nil || !providerError.Valid() {
				t.Fatalf("provider error = %+v, retained = %v; want a valid remote ACP diagnostic", providerError, ok)
			}
			classified := routingerr.Classify(providerError.RoutingInput(routingerr.PhasePromptSend, a.agentID, providerError.Message))
			capacity := classified.Confidence == routingerr.ConfHigh && !classified.UserAction &&
				(classified.Code == routingerr.CodeProviderOverloaded || classified.Code == routingerr.CodeModelCapacity || classified.Code == routingerr.CodeRateLimited)
			if capacity != test.wantCapacity {
				t.Fatalf("capacity = %v, want %v (classification %+v, provider error %+v)", capacity, test.wantCapacity, classified, providerError)
			}
		})
	}
}
