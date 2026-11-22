//go:build windows

package shell

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// ConPTY creates the shell process itself, so exec.Cmd attributes would be
// ignored. The kill-on-close job is attached to the running process instead.
func TestConfigureShellProcessLeavesCreationToConPTY(t *testing.T) {
	cmd := exec.Command("cmd.exe")

	configureShellProcess(cmd)

	require.Nil(t, cmd.SysProcAttr)
}
