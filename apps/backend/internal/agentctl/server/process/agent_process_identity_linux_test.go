//go:build linux

package process

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/config"
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

type agentIdentityConnectFailure struct{ *stubAdapter }

func (*agentIdentityConnectFailure) Connect(io.Writer, io.Reader) error {
	return errors.New("adapter connection failed")
}

func TestAgentProcessIdentityAbsentAfterFailedStartup(t *testing.T) {
	m := &Manager{
		cfg:     &config.InstanceConfig{SessionID: "session"},
		logger:  newTestLogger(t),
		cmd:     exec.Command("sleep", "30"),
		adapter: &agentIdentityConnectFailure{newStubAdapter()},
	}
	setAgentProcGroup(m.cmd)
	t.Cleanup(func() {
		_ = m.closeStderrPipe()
		if m.stdin != nil {
			_ = m.stdin.Close()
		}
		if m.stdout != nil {
			_ = m.stdout.Close()
		}
		if m.cmd.Process != nil && m.cmd.ProcessState == nil {
			_ = m.cmd.Process.Kill()
			_ = m.cmd.Wait()
		}
		_ = m.adapter.Close()
	})

	_, err := m.startManagedProcess()
	require.ErrorContains(t, err, "adapter connection failed")
	require.NotNil(t, m.cmd.ProcessState)
	require.Nil(t, m.AgentProcessIdentity())
	descriptor, err := m.DeliveryRecoveryDescriptor(context.Background(), "")
	require.NoError(t, err)
	require.Nil(t, descriptor.AgentProcess, "failed startup never publishes a dead attempt as the current agent")
}
