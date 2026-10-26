//go:build darwin

package processidentity

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGroupMembersSeesLeaderAndChildUntilKilled(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "sleep 30 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	require.NoError(t, cmd.Start())
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	require.Eventually(t, func() bool {
		members, err := GroupMembers(cmd.Process.Pid)
		return err == nil && len(members) >= 2
	}, 5*time.Second, 20*time.Millisecond)
	require.NoError(t, syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL))
	<-exited
	require.Eventually(t, func() bool {
		members, err := GroupMembers(cmd.Process.Pid)
		return err == nil && len(members) == 0
	}, 5*time.Second, 20*time.Millisecond)
}

func TestGroupMembersEmptyForUnusedGroup(t *testing.T) {
	members, err := GroupMembers(2_000_000_000)
	require.NoError(t, err)
	require.Empty(t, members)
}

func TestGroupMembersRejectsInvalidGroup(t *testing.T) {
	_, err := GroupMembers(0)
	require.ErrorIs(t, err, ErrUnverifiableIdentity)
}
