package acp

import (
	"strings"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestProviderErrorFromErrorMergesAllowlistedMetadata pins
// AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.22: rpc_code, error_kind,
// provider_id and model_id are merged onto the generic ACP prompt-error
// projection, and raw Data never crosses the boundary.
func TestProviderErrorFromErrorMergesAllowlistedMetadata(t *testing.T) {
	err := &sdk.RequestError{
		Code:    -32603,
		Message: "Internal error: API Error: 500 Internal server error.",
		Data:    map[string]any{"errorKind": "server_error", "private_token": "must-not-cross-boundary"},
	}

	got := ProviderErrorFromError(err, "claude-acp", "claude-sonnet-5")
	if got == nil {
		t.Fatal("ProviderErrorFromError() = nil, want a projection")
	}
	if got.RPCCode != -32603 {
		t.Fatalf("rpc_code = %d, want -32603", got.RPCCode)
	}
	if got.ErrorKind != "server_error" {
		t.Fatalf("error_kind = %q, want server_error", got.ErrorKind)
	}
	if got.ProviderID != "claude-acp" {
		t.Fatalf("provider_id = %q, want claude-acp", got.ProviderID)
	}
	if got.ModelID != "claude-sonnet-5" {
		t.Fatalf("model_id = %q, want claude-sonnet-5", got.ModelID)
	}
}

// TestProviderErrorFromErrorOmitsModelIDWhenNoModelSettled pins the "omitted
// when no model is settled" half of AC.22.
func TestProviderErrorFromErrorOmitsModelIDWhenNoModelSettled(t *testing.T) {
	err := &sdk.RequestError{Code: -32603, Message: "provider failed"}

	got := ProviderErrorFromError(err, "claude-acp", "")
	if got == nil {
		t.Fatal("ProviderErrorFromError() = nil, want a projection")
	}
	if got.ModelID != "" {
		t.Fatalf("model_id = %q, want empty (no model settled)", got.ModelID)
	}
	if got.ProviderID != "claude-acp" {
		t.Fatalf("provider_id = %q, want claude-acp", got.ProviderID)
	}
}

// TestProviderErrorFromErrorDropsMalformedOrOversizedMetadata pins R3-F9 and
// R3-F10: error_kind, provider_id and model_id share one allowlist rule (at
// most 64 bytes of [A-Za-z0-9_.-]), and a Data shape other than
// map[string]any yields no error_kind at all. A dropped field never
// invalidates the rest of the projection.
func TestProviderErrorFromErrorDropsMalformedOrOversizedMetadata(t *testing.T) {
	oversized := strings.Repeat("x", 65)

	t.Run("oversized error_kind dropped", func(t *testing.T) {
		err := &sdk.RequestError{Code: -32603, Message: "provider failed", Data: map[string]any{"errorKind": oversized}}
		got := ProviderErrorFromError(err, "claude-acp", "")
		if got == nil {
			t.Fatal("ProviderErrorFromError() = nil, want a projection")
		}
		if got.ErrorKind != "" {
			t.Fatalf("error_kind = %q, want empty", got.ErrorKind)
		}
	})

	t.Run("malformed provider_id dropped", func(t *testing.T) {
		err := &sdk.RequestError{Code: -32603, Message: "provider failed"}
		got := ProviderErrorFromError(err, "claude acp!", "")
		if got == nil {
			t.Fatal("ProviderErrorFromError() = nil, want a projection")
		}
		if got.ProviderID != "" {
			t.Fatalf("provider_id = %q, want empty", got.ProviderID)
		}
	})

	t.Run("oversized model_id dropped", func(t *testing.T) {
		err := &sdk.RequestError{Code: -32603, Message: "provider failed"}
		got := ProviderErrorFromError(err, "", oversized)
		if got == nil {
			t.Fatal("ProviderErrorFromError() = nil, want a projection")
		}
		if got.ModelID != "" {
			t.Fatalf("model_id = %q, want empty", got.ModelID)
		}
	})

	t.Run("Data is a string yields no error_kind but rpc_code still present", func(t *testing.T) {
		err := &sdk.RequestError{Code: -32603, Message: "provider failed", Data: "unavailable"}
		got := ProviderErrorFromError(err, "", "")
		if got == nil {
			t.Fatal("ProviderErrorFromError() = nil, want a projection")
		}
		if got.ErrorKind != "" {
			t.Fatalf("error_kind = %q, want empty", got.ErrorKind)
		}
		if got.RPCCode != -32603 {
			t.Fatalf("rpc_code = %d, want -32603", got.RPCCode)
		}
		if strings.Contains(got.Message, "unavailable") {
			t.Fatalf("message leaked raw Data: %q", got.Message)
		}
	})
}

// TestProviderErrorFromErrorNeverLeaksRawDataWhenMessageSanitizesEmpty pins
// AC.22's unconditional "raw JSON-RPC error data shall never cross the
// boundary": a *acp.RequestError whose Message sanitizes to empty (here, an
// all-URL message) must still yield a non-nil projection, so a caller never
// falls back to the SDK's own Error(), which JSON-marshals the raw Data
// verbatim.
func TestProviderErrorFromErrorNeverLeaksRawDataWhenMessageSanitizesEmpty(t *testing.T) {
	err := &sdk.RequestError{
		Code:    -32603,
		Message: "https://gateway.internal/billing",
		Data:    map[string]any{"account_id": "acct_topsecret", "session_token": "sk-do-not-leak"},
	}

	if !strings.Contains(err.Error(), "sk-do-not-leak") {
		t.Fatalf("test setup invalid: raw Error() does not contain the secret it is meant to prove is contained: %q", err.Error())
	}

	got := ProviderErrorFromError(err, "claude-acp", "")
	if got == nil {
		t.Fatal("ProviderErrorFromError() = nil, want a non-nil projection so callers never fall back to err.Error()")
	}
	if strings.Contains(got.Message, "sk-do-not-leak") || strings.Contains(got.Message, "acct_topsecret") {
		t.Fatalf("message leaked raw Data: %q", got.Message)
	}
	if got.Message == "" {
		t.Fatal("message is empty, want a fixed generic message")
	}
	if got.RPCCode != -32603 {
		t.Fatalf("rpc_code = %d, want -32603 (metadata merge should still run)", got.RPCCode)
	}
}

// TestProviderErrorFromErrorNeverOverwritesRicherExtractor pins the merge
// rule: allowlisted metadata fills only fields the winning projection left
// empty, so OpenCode's own provider_id/model_id (read from structured stderr
// fields) are never overwritten by the generic adapter-state metadata.
func TestProviderErrorFromErrorNeverOverwritesRicherExtractor(t *testing.T) {
	richer := &providerPromptError{ProviderError: streams.ProviderError{
		Source:     streams.ProviderErrorSourceOpenCodeStderr,
		ProviderID: "opencode-go",
		ModelID:    "kimi-k3",
		Message:    "5-hour usage limit reached",
		OccurredAt: time.Now(),
	}}

	got := ProviderErrorFromError(richer, "opencode-acp", "gpt-5")
	if got == nil {
		t.Fatal("ProviderErrorFromError() = nil, want a projection")
	}
	if got.ProviderID != "opencode-go" {
		t.Fatalf("provider_id = %q, want the richer extractor's opencode-go", got.ProviderID)
	}
	if got.ModelID != "kimi-k3" {
		t.Fatalf("model_id = %q, want the richer extractor's kimi-k3", got.ModelID)
	}
	if got.RPCCode != 0 || got.ErrorKind != "" {
		t.Fatalf("stderr-correlated error is not an acp.RequestError: rpc_code=%d error_kind=%q, want both absent", got.RPCCode, got.ErrorKind)
	}
}

// TestProviderErrorFromErrorZeroRPCCodeMeansAbsent pins "JSON-RPC forbids 0,
// so 0 means absent" from the metadata table.
func TestProviderErrorFromErrorZeroRPCCodeMeansAbsent(t *testing.T) {
	err := &sdk.RequestError{Code: 0, Message: "provider failed"}
	got := ProviderErrorFromError(err, "", "")
	if got == nil {
		t.Fatal("ProviderErrorFromError() = nil, want a projection")
	}
	if got.RPCCode != 0 {
		t.Fatalf("rpc_code = %d, want 0 (absent)", got.RPCCode)
	}
}

// TestAdapterProviderErrorContextReportsAgentIDAndSettledModel pins R3-F6
// (use Adapter.agentID, there is no negotiated id) and the "settled model"
// definition: model_id is empty until a session-configuration or model event
// has published a non-empty current model id.
func TestAdapterProviderErrorContextReportsAgentIDAndSettledModel(t *testing.T) {
	a := newTestAdapter()
	a.agentID = "claude-acp"

	providerID, modelID := a.ProviderErrorContext()
	if providerID != "claude-acp" {
		t.Fatalf("provider_id = %q, want claude-acp", providerID)
	}
	if modelID != "" {
		t.Fatalf("model_id = %q, want empty before any model is settled", modelID)
	}

	a.availableConfigOptions = []streams.ConfigOption{
		{ID: "model", CurrentValue: "claude-sonnet-5"},
	}
	if _, modelID := a.ProviderErrorContext(); modelID != "claude-sonnet-5" {
		t.Fatalf("model_id = %q, want claude-sonnet-5 once settled", modelID)
	}
}
