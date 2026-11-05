package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/sandbox"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestSandboxPolicyAdmissionPrecedesLaunchEffects(t *testing.T) {
	blocked := errors.New("sandbox policy unavailable")
	manager := &Manager{}
	manager.SetSandboxAdmission(func(context.Context, *LaunchRequest) error { return blocked })
	profile := mcpprofile.New(mcpprofile.SurfaceManagedConversation, nil, nil)
	_, err := manager.Launch(t.Context(), &LaunchRequest{McpProfile: &profile})
	require.ErrorIs(t, err, blocked)
}

func TestSandboxPolicyAdmissionPreservesExecutorAuthorization(t *testing.T) {
	blocked := errors.New("executor access denied")
	manager := &Manager{executorAdmission: func(ctx context.Context, _, _, _ string) (context.Context, error) { return ctx, blocked }}
	called := false
	manager.SetSandboxAdmission(func(context.Context, *LaunchRequest) error { called = true; return nil })
	_, err := manager.Launch(t.Context(), &LaunchRequest{})
	require.ErrorIs(t, err, blocked)
	require.False(t, called)
}

func TestSandboxRemoteTerminalTransportRefusesLocalEnvelope(t *testing.T) {
	runner := &remotePassthroughRunner{}
	_, err := runner.Start(t.Context(), process.InteractiveStartRequest{Sandbox: &sandbox.Launch{}})
	require.ErrorContains(t, err, "does not support sandbox launches")
}
