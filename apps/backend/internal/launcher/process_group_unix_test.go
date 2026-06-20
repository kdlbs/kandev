//go:build linux || darwin

package launcher

import (
	"os/exec"
	"testing"
)

func TestConfigureManagedProcessCreatesProcessGroup(t *testing.T) {
	cmd := exec.Command("kandev")
	configureManagedProcess(cmd)

	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatalf("configureManagedProcess should set Setpgid")
	}
}
