//go:build linux

package lifecycle

import (
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/stretchr/testify/require"
)

func TestHistoricalProcessTerminationAcceptsExitedAgentGroupForLiveExecution(t *testing.T) {
	sharedRuntime, err := processidentity.Capture(os.Getpid())
	require.NoError(t, err)

	agent := exec.Command("sleep", "30")
	agent.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, agent.Start())
	t.Cleanup(func() {
		_ = agent.Process.Kill()
		_ = agent.Wait()
	})

	agentIdentity, err := processidentity.Capture(agent.Process.Pid)
	require.NoError(t, err)
	owner := sharedRuntime.WithAgentGroup(agentIdentity)
	require.NoError(t, agent.Process.Kill())
	_ = agent.Wait()

	terminated := historicalProcessTermination(DeliveryRecordEvidenceRequest{
		ExecutionID: "source-execution", OriginalRuntime: &owner,
	}, "source-execution")
	require.NotNil(t, terminated)
	require.True(t, *terminated, "the exited agent group proves termination while the shared runtime and execution row remain")
}
