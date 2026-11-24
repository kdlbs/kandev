package process

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestStopAgentProcessLeavesWorkspaceProcessesRunningAndAdmitsNewOnes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses the Unix sleep command")
	}
	workDir := t.TempDir()
	mgr := NewManager(&config.InstanceConfig{WorkDir: workDir, AgentEnv: os.Environ()}, newTestLogger(t))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mgr.StopForTeardown(ctx); err != nil {
			t.Errorf("stop manager: %v", err)
		}
	})

	first, err := mgr.StartProcess(context.Background(), StartProcessRequest{
		SessionID:  "session-1",
		Kind:       streams.ProcessKindDev,
		Command:    "sleep 30",
		WorkingDir: workDir,
	})
	require.NoError(t, err)

	require.NoError(t, mgr.StopAgentProcess(context.Background()))
	stillRunning, ok := mgr.GetProcess(first.ID, false)
	require.True(t, ok)
	require.Equal(t, streams.ProcessStatusRunning, stillRunning.Status)

	second, err := mgr.StartProcess(context.Background(), StartProcessRequest{
		SessionID:  "session-1",
		Kind:       streams.ProcessKindDev,
		Command:    "sleep 30",
		WorkingDir: workDir,
	})
	require.NoError(t, err)
	require.NotEmpty(t, second.ID)
}

func TestCopilotContextTierReadsTheStartedChildArguments(t *testing.T) {
	mgr := &Manager{
		cfg: &config.InstanceConfig{AgentType: acpcompat.CopilotAgentID},
		cmd: &exec.Cmd{
			Args:    []string{"copilot", "--context", acpcompat.CopilotContextTierLong},
			Process: &os.Process{Pid: 42},
		},
	}
	mgr.status.Store(StatusRunning)

	tier, running := mgr.CopilotContextTier()
	require.True(t, running)
	require.Equal(t, acpcompat.CopilotContextTierLong, tier)
}
