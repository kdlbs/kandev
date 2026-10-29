package hostutility

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestSandboxPolicyBlocksUtilityBeforeProcessAndCache(t *testing.T) {
	blocked := errors.New("sandbox executor context is required")
	manager := NewManager(registry.NewRegistry(logger.Default()), "", 0, nil, logger.Default())
	manager.SetSandboxAdmission(func(context.Context, *ProfileProbeContext) error { return blocked })
	_, err := manager.ProbeProfileCapabilities(t.Context(), "missing-agent", ProfileCapabilityRequest{Context: ProfileProbeContext{Scope: "operator"}})
	require.ErrorIs(t, err, blocked)
	_, err = manager.ResolveModelConfig(t.Context(), "missing-agent", ModelConfigResolutionRequest{Model: "model", ProfileContext: &ProfileProbeContext{Scope: "operator"}})
	require.ErrorIs(t, err, blocked)
	_, err = manager.ExecuteProfilePrompt(t.Context(), "profile", "prompt")
	require.ErrorIs(t, err, blocked)
	_, err = manager.ResolveModelConfig(t.Context(), "missing-agent", ModelConfigResolutionRequest{Model: "model"})
	require.ErrorIs(t, err, blocked)
	_, err = manager.ExecutePromptWithMCP(t.Context(), "missing-agent", "model", "", "prompt", nil)
	require.ErrorIs(t, err, blocked)
	caps := manager.probeWithOverrides(t.Context(), &instance{agentType: "missing-agent"}, nil, false, agents.Command{}, "", nil, nil)
	require.Equal(t, StatusUnsupported, caps.Status)
}
