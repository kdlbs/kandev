package utility

import (
	"context"
	"errors"
	"testing"

	acp "github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

func TestAntigravityProbeAndInferencePolicy(t *testing.T) {
	if info := inferenceClientInfo(acpcompat.AntigravityAgentID, "fallback", acpcompat.AntigravityAuthPolicy(false)); info.Name != "zed" || info.Version != "0.0.0" {
		t.Fatalf("antigravity client info = %+v", info)
	}
	if info := inferenceClientInfo("claude-acp", "fallback", nil); info.Name != "fallback" {
		t.Fatalf("other client info = %+v", info)
	}
	cliPolicy := acpcompat.AntigravityAuthPolicyForRuntime(acpcompat.AntigravityRuntimeCLI, "1.5.15", false)
	if info := inferenceClientInfo(acpcompat.AntigravityAgentID, "fallback", cliPolicy); info.Name != "fallback" {
		t.Fatalf("CLI client info = %+v, want normal Kandev identity", info)
	}
	policy := acpcompat.AntigravityAuthPolicy(false)
	err := applyAntigravityAuth(context.Background(), nil, acpcompat.AntigravityAgentID, policy,
		acp.InitializeResponse{}, nil)
	if !errors.Is(err, acpcompat.ErrAuthRequired) {
		t.Fatalf("missing advertised method error = %v, want auth required", err)
	}
	if err := applyAntigravityAuth(context.Background(), nil, acpcompat.AntigravityAgentID, cliPolicy,
		acp.InitializeResponse{}, nil); err != nil {
		t.Fatalf("CLI auth should use cached provider credentials without ACP authenticate: %v", err)
	}
	if err := applyAntigravityAuth(context.Background(), nil, "claude-acp", policy,
		acp.InitializeResponse{}, nil); err != nil {
		t.Fatalf("other agent got Antigravity policy: %v", err)
	}
}
