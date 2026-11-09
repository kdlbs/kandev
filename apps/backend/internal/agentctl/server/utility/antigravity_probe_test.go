package utility

import (
	"context"
	"errors"
	"testing"

	acp "github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

func TestAntigravityProbeAndInferencePolicy(t *testing.T) {
	if info := inferenceClientInfo(acpcompat.AntigravityAgentID, "fallback"); info.Name != "zed" || info.Version != "0.0.0" {
		t.Fatalf("antigravity client info = %+v", info)
	}
	if info := inferenceClientInfo("claude-acp", "fallback"); info.Name != "fallback" {
		t.Fatalf("other client info = %+v", info)
	}
	policy := acpcompat.AntigravityAuthPolicy(false)
	err := applyAntigravityAuth(context.Background(), nil, acpcompat.AntigravityAgentID, policy,
		acp.InitializeResponse{}, nil)
	if !errors.Is(err, acpcompat.ErrAuthRequired) {
		t.Fatalf("missing advertised method error = %v, want auth required", err)
	}
	if err := applyAntigravityAuth(context.Background(), nil, "claude-acp", policy,
		acp.InitializeResponse{}, nil); err != nil {
		t.Fatalf("other agent got Antigravity policy: %v", err)
	}
}
