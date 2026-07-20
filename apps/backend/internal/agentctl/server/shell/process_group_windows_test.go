//go:build windows

package shell

import (
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestConfigureShellProcessStartsSuspendedForJobAssignment(t *testing.T) {
	cmd := exec.Command("cmd.exe")

	configureShellProcess(cmd)

	require.NotNil(t, cmd.SysProcAttr)
	require.NotZero(t, cmd.SysProcAttr.CreationFlags&syscall.CREATE_NEW_PROCESS_GROUP)
	require.NotZero(t, cmd.SysProcAttr.CreationFlags&windows.CREATE_SUSPENDED)
}
