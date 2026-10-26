package lifecycle

import (
	"context"
	"testing"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSandboxedNativeTerminalIsRefusedOnADarwinHost(t *testing.T) {
	previous := sandboxHostOS
	sandboxHostOS = "darwin"
	t.Cleanup(func() { sandboxHostOS = previous })
	manager, runner, execution := newPassthroughRunnerManager(t)
	execution.SandboxSelection = &policy.SelectionContext{OrgID: "org", ExecutorID: "executor"}
	captured := false
	manager.SetExecutionSnapshotRecorder(func(context.Context, *AgentExecution, models.ExecutionSnapshot, *contract.Launch) error {
		captured = true
		return nil
	})
	request := process.InteractiveStartRequest{SessionID: execution.SessionID, WorkingDir: t.TempDir(), Command: []string{"/bin/sh", "-c", "exit 0"}, ImmediateStart: true}
	info, err := manager.startSandboxInteractive(t.Context(), execution, runner, request)
	require.ErrorIs(t, err, policy.ErrTransportUnsupported)
	require.Nil(t, info)
	require.False(t, captured, "no snapshot or attempt is written for the refusal")
}

func TestUnsandboxedNativeTerminalIsUnaffectedOnADarwinHost(t *testing.T) {
	previous := sandboxHostOS
	sandboxHostOS = "darwin"
	t.Cleanup(func() { sandboxHostOS = previous })
	manager, runner, execution := newPassthroughRunnerManager(t)
	require.Nil(t, execution.SandboxSelection)
	request := process.InteractiveStartRequest{SessionID: execution.SessionID, WorkingDir: t.TempDir(), Command: []string{"/bin/sh", "-c", "read ignored"}, ImmediateStart: true}
	info, err := manager.startSandboxInteractive(t.Context(), execution, runner, request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = runner.Stop(t.Context(), info.ID) })
}
