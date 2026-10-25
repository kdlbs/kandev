//go:build linux

package processidentity

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSandboxApprovalRequiresLiveOwnedProcessAncestry(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "sh", "-c", "sleep 30 & echo $!; wait")
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
	command.WaitDelay = time.Second
	stdout, err := command.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, command.Start())
	defer func() {
		if command.ProcessState == nil {
			_ = command.Cancel()
			_ = command.Wait()
		}
	}()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	require.NoError(t, err)
	supervisor, err := Capture(command.Process.Pid)
	require.NoError(t, err)
	child, err := Capture(pid)
	require.NoError(t, err)
	owned, err := IsDescendant(ctx, supervisor, child)
	require.NoError(t, err)
	require.True(t, owned)
	unrelated, err := Capture(os.Getpid())
	require.NoError(t, err)
	owned, _ = IsDescendant(ctx, supervisor, unrelated)
	require.False(t, owned)
	reused := child
	reused.BirthToken = "linux:foreign-boot:1"
	owned, err = IsDescendant(ctx, supervisor, reused)
	require.Error(t, err)
	require.False(t, owned)
	require.NoError(t, command.Cancel())
	require.Error(t, command.Wait())
	owned, err = IsDescendant(ctx, supervisor, child)
	require.Error(t, err)
	require.False(t, owned)
}
