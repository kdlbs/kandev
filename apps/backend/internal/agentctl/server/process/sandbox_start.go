package process

import (
	"github.com/kandev/kandev/internal/common/sandbox"
	"os/exec"
)

func startSandboxCommand(command *exec.Cmd, launch *sandbox.Launch, record *sandboxStartRecord) error {
	release, err := sandbox.ConfigureContainment(command, launch)
	if err != nil {
		return err
	}
	defer release()
	if record != nil {
		record.dispatched = true
	}
	err = command.Start()
	if record != nil && err != nil && command.Process == nil {
		record.dispatched = false
	}
	return err
}
