//go:build linux

package process

import (
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentProcessIdentityTracksRunningGroupLeader(t *testing.T) {
	m := &Manager{}
	require.Nil(t, m.AgentProcessIdentity(), "no identity before an agent starts")

	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	m.recordAgentProcessIdentity(cmd.Process.Pid)
	identity := m.AgentProcessIdentity()
	require.NotNil(t, identity)
	require.Equal(t, cmd.Process.Pid, identity.PID)
	require.Equal(t, identity.PID, identity.GroupID)
	require.NotEmpty(t, identity.BirthToken)

	m.clearAgentProcessIdentity(cmd.Process.Pid + 1)
	require.NotNil(t, m.AgentProcessIdentity(), "another PID's exit does not clear the running child")
	m.clearAgentProcessIdentity(cmd.Process.Pid)
	require.Nil(t, m.AgentProcessIdentity(), "the exited child is forgotten")
}
