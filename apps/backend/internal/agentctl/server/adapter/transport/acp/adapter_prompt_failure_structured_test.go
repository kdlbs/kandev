package acp

import (
	"testing"

	sdk "github.com/coder/acp-go-sdk"
)

// A provider failure whose only routing signal is structured metadata (the
// ACP SDK flattens the message to "Internal error") must still be recognised
// as provider capacity, so the turn is retained instead of being torn down as
// a generic internal error.
//
// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.32
func TestRetainableACPApplicationErrorUsesStructuredProviderSignals(t *testing.T) {
	for _, test := range []struct {
		name      string
		provider  string
		data      any
		wantValid bool
	}{
		{name: "claude rate limit kind", provider: claudeAgentID, data: map[string]any{"errorKind": "rate_limit_error"}, wantValid: true},
		{name: "claude overloaded kind", provider: claudeAgentID, data: map[string]any{"errorKind": "overloaded_error"}, wantValid: true},
		{name: "explicit http 429", provider: codexAgentID, data: map[string]any{"statusCode": float64(429), "errorCode": "rate_limit"}, wantValid: true},
		{name: "explicit http 529", provider: claudeAgentID, data: map[string]any{"statusCode": float64(529)}, wantValid: true},
		{name: "kind outside its provider namespace", provider: codexAgentID, data: map[string]any{"errorKind": "rate_limit_error"}},
		{name: "generic wrapper kind", provider: claudeAgentID, data: map[string]any{"errorKind": "AI_APICallError"}},
		{name: "fractional status ignored", provider: claudeAgentID, data: map[string]any{"statusCode": 429.5}},
		{name: "opaque provider code only", provider: claudeAgentID, data: map[string]any{"errorCode": "rate_limit"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := newTestAdapterForAgent(test.provider)
			providerError, ok := a.retainableACPApplicationError(&sdk.RequestError{Code: -32603, Message: "Internal error", Data: test.data})
			if ok != test.wantValid {
				t.Fatalf("retainable = %v, want %v (provider error %+v)", ok, test.wantValid, providerError)
			}
			if ok && (providerError == nil || !providerError.Valid()) {
				t.Fatalf("provider error = %+v, want a valid ACP prompt diagnostic", providerError)
			}
		})
	}
}
