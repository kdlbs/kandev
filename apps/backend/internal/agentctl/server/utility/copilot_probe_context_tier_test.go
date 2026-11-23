package utility

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"go.uber.org/zap"
)

type fakeCopilotProbeAgent struct {
	*probeCaptureAgent
	mu          sync.Mutex
	configCalls []string
}

func (a *fakeCopilotProbeAgent) wireOptions(modelID, effort string) []acp.SessionConfigOption {
	modeCat := acp.SessionConfigOptionCategoryMode
	modelCat := acp.SessionConfigOptionCategoryModel
	thoughtCat := acp.SessionConfigOptionCategoryThoughtLevel
	modeOpts := acp.SessionConfigSelectOptionsUngrouped{{Value: "agent", Name: "Agent"}}
	modelOpts := acp.SessionConfigSelectOptionsUngrouped{
		{Value: "gpt-5.4", Name: "GPT-5.4"},
		{Value: "gpt-6.2-sol", Name: "GPT-6.2 Sol"},
	}
	thoughtOpts := acp.SessionConfigSelectOptionsUngrouped{
		{Value: "medium", Name: "Medium"},
		{Value: "high", Name: "High"},
	}
	return []acp.SessionConfigOption{
		{Select: &acp.SessionConfigOptionSelect{
			Id: "mode", Name: "Session Mode", Type: "select", Category: &modeCat, CurrentValue: "agent",
			Options: acp.SessionConfigSelectOptions{Ungrouped: &modeOpts},
		}},
		{Select: &acp.SessionConfigOptionSelect{
			Id: "model", Name: "Model", Type: "select", Category: &modelCat, CurrentValue: acp.SessionConfigValueId(modelID),
			Options: acp.SessionConfigSelectOptions{Ungrouped: &modelOpts},
		}},
		{Select: &acp.SessionConfigOptionSelect{
			Id: "reasoning_effort", Name: "Reasoning Effort", Type: "select", Category: &thoughtCat, CurrentValue: acp.SessionConfigValueId(effort),
			Options: acp.SessionConfigSelectOptions{Ungrouped: &thoughtOpts},
		}},
	}
}

func (a *fakeCopilotProbeAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{
		SessionId:     acp.SessionId("copilot-probe-sess"),
		ConfigOptions: a.wireOptions("gpt-5.4", "medium"),
	}, nil
}

func (a *fakeCopilotProbeAgent) SetSessionConfigOption(_ context.Context, req acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	configID := string(req.ValueId.ConfigId)
	value := string(req.ValueId.Value)
	a.mu.Lock()
	a.configCalls = append(a.configCalls, configID+"="+value)
	a.mu.Unlock()
	if configID == acpcompat.CopilotContextTierOptionID {
		return acp.SetSessionConfigOptionResponse{}, acp.NewInvalidParams("unknown configId context_tier")
	}
	modelID := "gpt-5.4"
	effort := "medium"
	if configID == "model" {
		modelID = value
	}
	if configID == "reasoning_effort" {
		effort = value
	}
	return acp.SetSessionConfigOptionResponse{
		ConfigOptions: a.wireOptions(modelID, effort),
	}, nil
}

// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-001.1, AC-AGENTS-COPILOT-CONTEXT-TIER-001.2, AC-AGENTS-COPILOT-CONTEXT-TIER-001.4
func TestProbeACPSession_CopilotSynthesizesAndPreservesContextTier(t *testing.T) {
	runProbe := func(t *testing.T, model string, requested map[string]string, launchArgs []string) (*ProbeResponse, []string) {
		t.Helper()
		c2aR, c2aW := io.Pipe()
		a2cR, a2cW := io.Pipe()
		t.Cleanup(func() {
			_ = c2aR.Close()
			_ = c2aW.Close()
			_ = a2cR.Close()
			_ = a2cW.Close()
		})
		fake := &fakeCopilotProbeAgent{probeCaptureAgent: &probeCaptureAgent{}}
		acp.NewAgentSideConnection(fake, a2cW, c2aR)
		e := NewACPInferenceExecutor(zap.NewNop())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := e.probeACPSessionWithContext(
			ctx, c2aW, a2cR, t.TempDir(),
			acpcompat.CopilotAgentID, model, "", requested, nil,
			authSessionArgs{launchArgs: launchArgs},
		)
		if err != nil {
			t.Fatalf("probeACPSessionWithContext: %v", err)
		}
		fake.mu.Lock()
		calls := append([]string(nil), fake.configCalls...)
		fake.mu.Unlock()
		return resp, calls
	}

	t.Run("default probe synthesizes context_tier with default value", func(t *testing.T) {
		resp, _ := runProbe(t, "", nil, []string{"npx", "@github/copilot", "--acp"})
		assertProbeContextTier(t, resp.ConfigOptions, "default")
	})

	t.Run("model resolution skips unsupported context_tier RPC and preserves requested long_context", func(t *testing.T) {
		resp, calls := runProbe(
			t,
			"gpt-6.2-sol",
			map[string]string{"context_tier": "long_context", "reasoning_effort": "high"},
			[]string{"npx", "@github/copilot", "--acp"},
		)
		for _, call := range calls {
			if call == "context_tier=long_context" {
				t.Fatalf("probe must not send unsupported context_tier RPC: %v", calls)
			}
		}
		assertProbeContextTier(t, resp.ConfigOptions, "long_context")
	})

	t.Run("profile probe reflects CLI --context launch arg", func(t *testing.T) {
		resp, _ := runProbe(t, "", nil, []string{"npx", "@github/copilot", "--acp", "--context", "long_context"})
		assertProbeContextTier(t, resp.ConfigOptions, "long_context")
	})
}

func assertProbeContextTier(t *testing.T, opts []ProbeConfigOption, want string) {
	t.Helper()
	for _, opt := range opts {
		if opt.ID == acpcompat.CopilotContextTierOptionID {
			if opt.CurrentValue != want {
				t.Fatalf("context_tier CurrentValue = %q, want %q", opt.CurrentValue, want)
			}
			return
		}
	}
	t.Fatalf("missing context_tier in probe options: %+v", opts)
}
