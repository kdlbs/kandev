//go:build darwin

package processidentity

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
func TestDarwinCapturedIdentityRecognizesLiveOwner(t *testing.T) {
	identity := darwinIsolatedProcess(t)
	for _, suffix := range []string{"0", "abcdef"} {
		t.Run("legacy_session_pointer_"+suffix, func(t *testing.T) {
			stored := identity
			stored.BirthToken = stored.BirthToken[:strings.LastIndex(stored.BirthToken, ":")+1] + suffix
			state, err := Inspect(stored)
			require.NoError(t, err)
			require.Equal(t, StateAlive, state)
			terminated, err := OwnedSessionTerminated(stored)
			require.NoError(t, err)
			require.False(t, terminated)
		})
	}
	for _, mutate := range []func(*Identity){
		func(i *Identity) { i.BirthToken = "darwin:1:0:0" },
		func(i *Identity) { i.GroupID++ },
		func(i *Identity) { i.SessionID++ },
	} {
		other := identity
		mutate(&other)
		state, err := Inspect(other)
		require.NoError(t, err)
		require.Equal(t, StateReused, state)
		terminated, err := OwnedSessionTerminated(other)
		require.ErrorIs(t, err, ErrUnverifiableIdentity)
		require.False(t, terminated)
		require.Error(t, TerminateOwnedSession(context.Background(), other, 0))
	}
	require.Error(t, TerminateOwnedSession(context.Background(), identity, 0))
}

func TestDarwinExitedOwnerHasVerifiableTermination(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	identity, err := Capture(cmd.Process.Pid)
	require.NoError(t, err)
	require.NoError(t, cmd.Process.Kill())
	_ = cmd.Wait()
	state, err := Inspect(identity)
	require.NoError(t, err)
	require.Equal(t, StateExited, state)
	terminated, err := OwnedSessionTerminated(identity)
	require.NoError(t, err)
	require.True(t, terminated)
	identity.BirthToken = "invalid"
	_, err = OwnedSessionTerminated(identity)
	require.ErrorIs(t, err, ErrUnverifiableIdentity)
}

func TestDarwinSurvivingDescendantBlocksRecoveryAndAmbiguousCleanup(t *testing.T) {
	unrelated := darwinIsolatedProcess(t)
	read, write, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = read.Close(); _ = write.Close() })
	cmd := exec.Command("/bin/sh", "-c", "sleep 60 & echo $! >&3; wait")
	cmd.ExtraFiles = []*os.File{write}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); _ = cmd.Wait() })
	require.NoError(t, write.Close())
	var child int
	_, err = fmt.Fscanln(read, &child)
	require.NoError(t, err)
	identity, err := Capture(cmd.Process.Pid)
	require.NoError(t, err)
	require.NoError(t, cmd.Process.Kill())
	_ = cmd.Wait()
	require.NoError(t, syscall.Kill(child, 0))
	terminated, err := OwnedSessionTerminated(identity)
	require.NoError(t, err)
	require.False(t, terminated, "the exited leader still has a live child")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.ErrorIs(t, TerminateOwnedSession(ctx, identity, 100*time.Millisecond), ErrUnverifiableIdentity)
	require.NoError(t, syscall.Kill(child, 0), "session membership alone cannot authorize a signal")
	terminated, err = OwnedSessionTerminated(identity)
	require.NoError(t, err)
	require.False(t, terminated)
	state, err := Inspect(unrelated)
	require.NoError(t, err)
	require.Equal(t, StateAlive, state, "containment must preserve unrelated sessions")
}

func darwinIsolatedProcess(t *testing.T) Identity {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	identity, err := Capture(cmd.Process.Pid)
	require.NoError(t, err)
	require.Equal(t, cmd.Process.Pid, identity.SessionID)
	return identity
}

func TestDarwinSessionEnumerationRejectsChangedProcessSnapshot(t *testing.T) {
	identity := darwinIsolatedProcess(t)
	proc, err := unix.SysctlKinfoProc("kern.proc.pid", identity.PID)
	require.NoError(t, err)
	group, err := darwinSessionMemberGroup(*proc, identity.SessionID)
	require.NoError(t, err)
	require.Equal(t, identity.GroupID, group)
	for _, field := range []string{"birth", "group"} {
		t.Run(field, func(t *testing.T) {
			stale := *proc
			if field == "birth" {
				stale.Proc.P_starttime.Sec--
			} else {
				stale.Eproc.Pgid++
			}
			group, err := darwinSessionMemberGroup(stale, identity.SessionID)
			require.ErrorIs(t, err, ErrUnverifiableIdentity)
			require.Zero(t, group, "do not mix stale group data with current session membership")
		})
	}
}
