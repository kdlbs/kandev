//go:build linux

package processcontainment

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/stretchr/testify/require"
)

func TestProcessContainmentOwnsDetachedDescendant(t *testing.T) {
	if os.Getenv("KANDEV_TEST_SANDBOX_CGROUP") != "1" {
		t.Skip("requires delegated cgroup v2 and clone3")
	}
	group, err := Create()
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, group.Kill(ctx))
		require.NoError(t, group.Remove())
	})
	command := exec.Command(os.Args[0], "-test.run=^TestProcessContainmentHelper$")
	command.Env = append(os.Environ(), "KANDEV_CONTAINMENT_HELPER=parent")
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	input, err := command.StdinPipe()
	require.NoError(t, err)
	output, err := command.StdoutPipe()
	require.NoError(t, err)
	lease, err := group.Configure(command)
	require.NoError(t, err)
	require.NoError(t, command.Start())
	require.NoError(t, lease.Close())
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	line, err := bufio.NewReader(output).ReadString('\n')
	require.NoError(t, err)
	pid, err := strconv.Atoi(line[:len(line)-1])
	require.NoError(t, err)
	child, err := processidentity.Capture(pid)
	require.NoError(t, err)
	require.NotEqual(t, command.Process.Pid, child.SessionID)
	contained, err := group.Contains(pid)
	require.NoError(t, err)
	require.True(t, contained)
	contained, err = group.Contains(os.Getpid())
	require.NoError(t, err)
	require.False(t, contained, "the host process must stay outside the attempt")
	t.Cleanup(func() {
		if state, _ := processidentity.Inspect(child); state == processidentity.StateAlive {
			owned, _ := os.FindProcess(pid)
			_ = owned.Kill()
		}
	})
	require.NoError(t, input.Close())
	require.NoError(t, command.Wait())
	state, err := processidentity.Inspect(child)
	require.NoError(t, err)
	require.Equal(t, processidentity.StateAlive, state)
	empty, err := group.Empty()
	require.NoError(t, err)
	require.False(t, empty, "supervisor exit must not hide detached children")
	require.Error(t, group.Remove(), "live descendants prevent retirement")
	wrong := *group
	wrong.Inode++
	require.Error(t, wrong.Kill(t.Context()), "changed kernel identity must not signal anything")
	state, err = processidentity.Inspect(child)
	require.NoError(t, err)
	require.Equal(t, processidentity.StateAlive, state)
	require.NoError(t, group.Kill(t.Context()))
	empty, err = group.Empty()
	require.NoError(t, err)
	require.True(t, empty)
	state, err = processidentity.Inspect(child)
	require.NoError(t, err)
	require.Equal(t, processidentity.StateExited, state)
	require.NoError(t, group.Remove())
	require.NoError(t, group.Remove(), "retirement is idempotent")
}

func TestProcessContainmentHelper(t *testing.T) {
	switch os.Getenv("KANDEV_CONTAINMENT_HELPER") {
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestProcessContainmentHelper$")
		child.Env = []string{"KANDEV_CONTAINMENT_HELPER=child"}
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if child.Start() != nil {
			os.Exit(2)
		}
		fmt.Println(child.Process.Pid)
		_, _ = os.Stdin.Read(make([]byte, 1))
		os.Exit(0)
	case "child":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
}
