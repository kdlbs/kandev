package acp

import (
	"testing"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestRetainableACPApplicationErrorRequiresClassifiedProviderCapacity(t *testing.T) {
	for _, test := range []struct {
		name      string
		provider  string
		message   string
		data      any
		wantCode  string
		wantValid bool
	}{
		{name: "codex model capacity", provider: codexAgentID, message: "Selected model is at capacity. Please try a different model.", wantCode: "model_capacity", wantValid: true},
		{name: "claude overload", provider: claudeAgentID, message: "529 Overloaded. Please try again later.", wantCode: "provider_overloaded", wantValid: true},
		{name: "claude rate limit", provider: claudeAgentID, message: "Rate limit exceeded. Please retry shortly.", wantCode: "rate_limited", wantValid: true},
		{name: "marked mock capacity", provider: mockAgentID, message: "Selected model is at capacity. Please try a different model.", data: map[string]any{"kandevMock": map[string]any{"retainedProviderCapacity": true}}, wantCode: "model_capacity", wantValid: true},
		{name: "unmarked mock capacity", provider: mockAgentID, message: "Selected model is at capacity. Please try a different model."},
		{name: "untrusted provider marker", provider: "opencode-acp", message: "Selected model is at capacity. Please try a different model.", data: map[string]any{"kandevMock": map[string]any{"retainedProviderCapacity": true}}},
		{name: "generic internal error", provider: codexAgentID, message: "Internal error", wantValid: false},
		{name: "unsupported adapter", provider: "opencode-acp", message: "Rate limit exceeded", wantValid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := newTestAdapterForAgent(test.provider)
			providerError, ok := a.retainableACPApplicationError(&sdk.RequestError{Code: -32603, Message: test.message, Data: test.data})
			if ok != test.wantValid {
				t.Fatalf("retainable = %v, want %v", ok, test.wantValid)
			}
			if !ok {
				if providerError != nil {
					t.Fatalf("provider error = %+v on rejected diagnostic", providerError)
				}
				return
			}
			if !providerError.Valid() || providerError.Source != streams.ProviderErrorSourceACPPrompt {
				t.Fatalf("provider error = %+v, want a valid ACP prompt diagnostic", providerError)
			}
			classified := providerError.ErrorKind
			if classified != "" && classified != test.wantCode {
				t.Fatalf("provider error kind = %q, want %q", classified, test.wantCode)
			}
			if providerError.Message == "" {
				t.Fatal("provider error has no sanitized message")
			}
		})
	}
}
