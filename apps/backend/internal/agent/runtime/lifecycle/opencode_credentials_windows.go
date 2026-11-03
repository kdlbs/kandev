package lifecycle

import (
	"os/exec"
	"sync"
	"syscall"

	"github.com/kandev/kandev/internal/agentctl/server/winproc"
	"golang.org/x/sys/windows"
)

func configureCredentialExportProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED,
	}
}

func captureCredentialExportProcess(cmd *exec.Cmd) (func(), error) {
	job, err := winproc.InstallKillOnCloseJobForSuspendedCommand(cmd)
	if err != nil {
		return nil, err
	}
	return sync.OnceFunc(func() { _ = job.Close() }), nil
}
