//go:build linux

package processidentity

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func startGroupLeader(t *testing.T) (*exec.Cmd, Identity) {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	identity, err := Capture(cmd.Process.Pid)
	require.NoError(t, err)
	require.Equal(t, identity.PID, identity.GroupID, "the agent child must lead its own process group")
	return cmd, identity
}

func stopGroupLeader(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	require.NoError(t, syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL))
	_ = cmd.Wait()
}

func TestOwnedGroupTerminated(t *testing.T) {
	cmd, identity := startGroupLeader(t)
	terminated, err := OwnedGroupTerminated(identity)
	require.NoError(t, err)
	require.False(t, terminated, "a live group leader is not terminated")

	stopGroupLeader(t, cmd)
	terminated, err = OwnedGroupTerminated(identity)
	require.NoError(t, err)
	require.True(t, terminated, "an exited group with no remaining members is terminated")

	_, err = OwnedGroupTerminated(Identity{PID: identity.PID, GroupID: identity.PID})
	require.ErrorIs(t, err, ErrUnverifiableIdentity, "a missing birth token proves nothing")
	_, err = OwnedGroupTerminated(Identity{PID: identity.PID, GroupID: identity.PID + 1, BirthToken: identity.BirthToken})
	require.ErrorIs(t, err, ErrUnverifiableIdentity, "a non-leader identity proves nothing")
	_, err = OwnedGroupTerminated(Identity{PID: 1, GroupID: 1, BirthToken: identity.BirthToken})
	require.ErrorIs(t, err, ErrUnverifiableIdentity, "negative PID one is not a process-group probe")
}

func TestOriginalOwnerTerminatedByAgentGroup(t *testing.T) {
	runtime, err := Capture(os.Getpid())
	require.NoError(t, err)
	cmd, agent := startGroupLeader(t)
	owner := runtime.WithAgentGroup(agent)
	require.Equal(t, agent.PID, owner.AgentGroupID)

	terminated, err := OriginalOwnerTerminated(owner)
	require.NoError(t, err)
	require.False(t, terminated, "a live runtime and a live agent group are not terminated")

	stopGroupLeader(t, cmd)
	terminated, err = OriginalOwnerTerminated(owner)
	require.NoError(t, err)
	require.True(t, terminated, "the exited agent group proves termination while the shared runtime lives")

	legacy, err := OriginalOwnerTerminated(runtime)
	require.NoError(t, err)
	require.False(t, legacy, "a binding without agent group evidence still needs the runtime to exit")

	unverifiable := runtime.WithAgentGroup(Identity{PID: agent.PID, GroupID: agent.PID})
	require.Zero(t, unverifiable.AgentGroupID, "an agent identity without a birth token is not recorded")
}

func TestOwnedGroupTerminationRequiresNoSurvivingChild(t *testing.T) {
	leader, identity := startGroupLeader(t)
	child := exec.Command("sleep", "30")
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: identity.GroupID}
	require.NoError(t, child.Start())
	t.Cleanup(func() {
		if child.ProcessState == nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	})
	member, err := Capture(child.Process.Pid)
	require.NoError(t, err)
	require.Equal(t, identity.GroupID, member.GroupID)

	require.NoError(t, leader.Process.Kill())
	_ = leader.Wait()
	terminated, err := OwnedGroupTerminated(identity)
	require.NoError(t, err)
	require.False(t, terminated, "an exited leader does not prove that its child stopped writing")

	require.NoError(t, child.Process.Kill())
	_ = child.Wait()
	terminated, err = OwnedGroupTerminated(identity)
	require.NoError(t, err)
	require.True(t, terminated)
}

func TestOwnedGroupTerminationRejectsChangedBirth(t *testing.T) {
	_, identity := startGroupLeader(t)
	boot, start, err := parseBirthToken(identity.BirthToken)
	require.NoError(t, err)
	identity.BirthToken = fmt.Sprintf("linux:%s:%d", boot, start+1)
	terminated, err := OwnedGroupTerminated(identity)
	require.NoError(t, err)
	require.False(t, terminated, "a different birth never proves that a live group is empty")

	identity.BirthToken = fmt.Sprintf("linux:another-boot:%d", start)
	terminated, err = OwnedGroupTerminated(identity)
	require.NoError(t, err)
	require.True(t, terminated, "a process from a previous boot cannot remain alive")
}
